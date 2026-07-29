package httpdelivery

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxDocumentHTMLBytes = 4 << 20
	maxDocumentPDFBytes  = 32 << 20
	maxGotenbergError    = 64 << 10
	// GotenbergMaxTimeout bounds a single Chromium request. The broader PDF
	// work deadline also includes database loading and HTML rendering.
	GotenbergMaxTimeout    = 35 * time.Second
	DocumentPDFWorkTimeout = 45 * time.Second
)

// DocumentPDFRenderer is deliberately small: the handler supplies already
// escaped, self-contained HTML and receives a verified PDF or a typed error.
type DocumentPDFRenderer interface {
	Render(context.Context, []byte) ([]byte, error)
}

type documentPDFErrorKind uint8

const (
	documentPDFUnavailable documentPDFErrorKind = iota + 1
	documentPDFDeadline
	documentPDFBadGateway
	documentPDFTooLarge
)

type documentPDFError struct {
	kind       documentPDFErrorKind
	err        error
	retryAfter time.Duration
}

func (e *documentPDFError) Error() string { return e.err.Error() }
func (e *documentPDFError) Unwrap() error { return e.err }

type unavailableDocumentPDFRenderer struct{}

func (unavailableDocumentPDFRenderer) Render(context.Context, []byte) ([]byte, error) {
	return nil, &documentPDFError{kind: documentPDFUnavailable, err: errors.New("PDF renderer is not configured")}
}

// GotenbergClient implements the official Chromium HTML form contract. Files
// are intentionally flat: Gotenberg scopes local assets to a request directory.
type GotenbergClient struct {
	baseURL *url.URL
	client  *http.Client
	slots   chan struct{}
}

func NewGotenbergClient(rawURL string, timeout time.Duration) (*GotenbergClient, error) {
	return NewGotenbergClientWithConcurrency(rawURL, timeout, 2)
}

func NewGotenbergClientWithConcurrency(rawURL string, timeout time.Duration, maxConcurrency int) (*GotenbergClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid GOTENBERG_URL")
	}
	if timeout <= 0 || timeout > GotenbergMaxTimeout {
		return nil, fmt.Errorf("invalid GOTENBERG_TIMEOUT")
	}
	if maxConcurrency < 1 || maxConcurrency > 10 {
		return nil, fmt.Errorf("invalid GOTENBERG_MAX_CONCURRENCY")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Rendering must not escape through environment-configured proxies.
	return &GotenbergClient{
		baseURL: u,
		slots:   make(chan struct{}, maxConcurrency),
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (g *GotenbergClient) Render(ctx context.Context, html []byte) ([]byte, error) {
	if len(html) > maxDocumentHTMLBytes {
		return nil, &documentPDFError{kind: documentPDFTooLarge, err: errors.New("document HTML is too large")}
	}
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		return nil, documentPDFBusyError()
	}
	body, contentType := newGotenbergMultipartBody(html)
	defer body.Close()
	endpoint := *g.baseURL
	endpoint.Path += "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: err}
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := g.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, &documentPDFError{kind: documentPDFDeadline, err: err}
		}
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, &documentPDFError{kind: documentPDFUnavailable, err: err}
		}
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxGotenbergError))
		return nil, &documentPDFError{kind: documentPDFUnavailable, err: errors.New("Gotenberg unavailable")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxGotenbergError))
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: fmt.Errorf("Gotenberg returned %d", resp.StatusCode)}
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/pdf" {
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: errors.New("Gotenberg response is not a PDF")}
	}
	pdf, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentPDFBytes+1))
	if err != nil {
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: err}
	}
	if len(pdf) > maxDocumentPDFBytes {
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: errors.New("Gotenberg response is too large")}
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, &documentPDFError{kind: documentPDFBadGateway, err: errors.New("Gotenberg response has invalid PDF signature")}
	}
	return pdf, nil
}

func (g *GotenbergClient) pdfMaxConcurrency() int {
	return cap(g.slots)
}

func documentPDFBusyError() *documentPDFError {
	return &documentPDFError{kind: documentPDFUnavailable, err: errors.New("PDF renderer is busy"), retryAfter: time.Second}
}

func newGotenbergMultipartBody(html []byte) (io.ReadCloser, string) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		err := writeGotenbergMultipart(multipartWriter, html)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, multipartWriter.FormDataContentType()
}

func writeGotenbergMultipart(w *multipart.Writer, html []byte) error {
	for _, asset := range []struct {
		name  string
		value []byte
	}{{"index.html", html}, {"NotoSansJP-Regular.ttf", notoSansJPRegular}} {
		part, err := w.CreateFormFile("files", asset.name)
		if err != nil {
			return err
		}
		if _, err = io.Copy(part, bytes.NewReader(asset.value)); err != nil {
			return err
		}
	}
	for _, field := range []struct{ key, value string }{
		{"preferCssPageSize", "true"}, {"printBackground", "true"}, {"failOnResourceLoadingFailed", "true"},
		{"failOnConsoleExceptions", "true"}, {"waitForExpression", "document.readyState === 'complete'"},
	} {
		if err := w.WriteField(field.key, field.value); err != nil {
			return err
		}
	}
	return nil
}

//go:embed assets/fonts/NotoSansJP-Regular.ttf
var notoSansJPRegular []byte

//go:embed assets/templates/document.html.tmpl
var documentHTMLTemplate string

var parsedDocumentHTMLTemplate = template.Must(template.New("document.html.tmpl").Parse(documentHTMLTemplate))

type documentHTMLLine struct{ ItemName, Quantity, Unit, UnitPrice, Amount, Notes string }
type documentHTMLData struct {
	Lang, Title, Number, IssuedOn                                                     string
	ClientName, ClientHonorific, ClientAddress, ClientContactName, ClientContactPhone string
	ContractorName, ContractorRepresentative, ContractorAddress                       string
	Total, Subtotal, Discount, PreTax, Tax, TotalLabel                                string
	Lines                                                                             []documentHTMLLine
}

func renderDocumentHTML(kind documentKind, row map[string]any) ([]byte, error) {
	lines, ok := row["lines"].([]map[string]any)
	if !ok {
		return nil, errors.New("invalid document lines")
	}
	data := documentHTMLData{
		Lang: "ja", Title: kind.title, Number: pdfValue(row[kind.number]), IssuedOn: pdfDate(row["issued_on"]),
		ClientName: pdfValue(row["client_name"]), ClientHonorific: pdfValue(row["client_honorific"]), ClientAddress: pdfValue(row["client_address"]), ClientContactName: pdfValue(row["client_contact_name"]), ClientContactPhone: pdfValue(row["client_contact_phone"]),
		ContractorName: pdfValue(row["contractor_name"]), ContractorRepresentative: pdfValue(row["contractor_representative"]), ContractorAddress: pdfValue(row["contractor_address"]),
		Subtotal: yen(number(row["subtotal"])), Discount: yen(number(row["discount_amount"])), PreTax: yen(number(row["pre_tax"])), Tax: yen(number(row["tax_amount"])), Total: yen(number(row["total"])), TotalLabel: kind.totalLabel,
		Lines: make([]documentHTMLLine, 0, len(lines)),
	}
	for _, line := range lines {
		data.Lines = append(data.Lines, documentHTMLLine{pdfValue(line["item_name"]), fmt.Sprintf("%.3f", number(line["quantity"])), pdfValue(line["unit"]), yen(number(line["unit_price"])), yen(number(line["amount"])), pdfValue(line["notes"])})
	}
	var out bytes.Buffer
	if err := parsedDocumentHTMLTemplate.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("execute document template: %w", err)
	}
	if out.Len() > maxDocumentHTMLBytes {
		return nil, &documentPDFError{kind: documentPDFTooLarge, err: errors.New("document HTML is too large")}
	}
	return out.Bytes(), nil
}

func pdfValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func pdfDate(value any) string {
	switch date := value.(type) {
	case time.Time:
		return date.Format("2006-01-02")
	case *time.Time:
		if date != nil {
			return date.Format("2006-01-02")
		}
	}
	return toString(value)
}
func yen(value float64) string { return strconv.FormatInt(roundYen(value), 10) + "円" }
