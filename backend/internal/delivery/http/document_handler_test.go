package httpdelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDocumentCalculationIsServerAuthoritative(t *testing.T) {
	data := map[string]any{"project_id": float64(1), "issued_on": "2026-07-25", "status": "draft", "subtotal": float64(1), "tax_amount": float64(1), "total": float64(1), "discount_amount": float64(100), "lines": []any{map[string]any{"item_name": "施工費", "quantity": float64(2.5), "unit": "式", "unit_price": float64(1200), "notes": ""}}}
	calculated, message := validateAndCalculateDocument(kindFor("estimate"), data)
	if message != "" {
		t.Fatalf("valid document rejected: %s", message)
	}
	if calculated.Subtotal != 3000 || calculated.Discount != 100 || calculated.PreTax != 2900 || calculated.Tax != 290 || calculated.Total != 3190 {
		t.Fatalf("unexpected authoritative totals: %#v", calculated)
	}
}

func TestDocumentCalculationRejectsIntegerAndFloatBoundaryOverflows(t *testing.T) {
	base := map[string]any{"project_id": json.Number("1"), "issued_on": "2026-07-25", "status": "draft", "discount_amount": json.Number("0")}
	base["lines"] = []any{map[string]any{"item_name": "上限超過", "quantity": json.Number("999999999999.999"), "unit_price": json.Number("999999999999.99")}}
	if _, message := validateAndCalculateDocument(kindFor("estimate"), base); message == "" {
		t.Fatal("overflowing multiplication was accepted")
	}
	base["lines"] = []any{map[string]any{"item_name": "通常", "quantity": json.Number("1.005"), "unit_price": json.Number("100.00")}}
	base["discount_amount"] = json.Number("1000000000000.01")
	if _, message := validateAndCalculateDocument(kindFor("estimate"), base); message == "" {
		t.Fatal("discount boundary was accepted")
	}
}

func TestDocumentRejectsBadLinesBeforeAnyReplacement(t *testing.T) {
	base := map[string]any{"project_id": float64(1), "issued_on": "2026-07-25", "billing_type": "individual", "discount_amount": float64(0)}
	for _, lines := range []any{[]any{}, []any{"bad"}, []any{map[string]any{"item_name": "", "quantity": float64(1), "unit_price": float64(1)}}, []any{map[string]any{"item_name": "施工費", "quantity": float64(-1), "unit_price": float64(1)}}} {
		data := map[string]any{}
		for k, v := range base {
			data[k] = v
		}
		data["lines"] = lines
		if _, message := validateAndCalculateDocument(kindFor("invoice"), data); message == "" {
			t.Fatalf("invalid lines accepted: %#v", lines)
		}
	}
}

func TestDocumentBodyLimitsAndSingleJSONValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"one JSON object", `{"issued_on":"2026-07-26"}`, http.StatusOK, true},
		{"second JSON object", `{} {}`, http.StatusBadRequest, false},
		{"non-object", `null`, http.StatusBadRequest, false},
		{"too large", `{"notes":"` + strings.Repeat("a", maxDocumentJSONBytes) + `"}`, http.StatusRequestEntityTooLarge, false},
		{"too large after first value", `{}` + strings.Repeat(" ", maxDocumentJSONBytes+1), http.StatusRequestEntityTooLarge, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			_, ok := documentBody(c)
			if ok != tt.ok || recorder.Code != tt.status {
				t.Fatalf("ok=%v status=%d, want ok=%v status=%d", ok, recorder.Code, tt.ok, tt.status)
			}
		})
	}
}

func TestDocumentValidationCapsLinesAndDatabaseStrings(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"project_id": json.Number("1"), "issued_on": "2026-07-25", "status": "draft", "discount_amount": json.Number("0"),
			"lines": []any{map[string]any{"item_name": "施工費", "quantity": json.Number("1"), "unit": "式", "unit_price": json.Number("100"), "notes": ""}},
		}
	}
	tooMany := base()
	lines := make([]any, maxDocumentLines+1)
	for i := range lines {
		lines[i] = map[string]any{"item_name": "施工費", "quantity": json.Number("1"), "unit_price": json.Number("1")}
	}
	tooMany["lines"] = lines
	if _, message := validateAndCalculateDocument(kindFor("estimate"), tooMany); message != "明細は500行まで入力できます。" {
		t.Fatalf("line cap message = %q", message)
	}
	for _, tt := range []struct{ name, key, value string }{
		{"contractor", "contractor_name", strings.Repeat("あ", 161)},
		{"item", "item_name", strings.Repeat("あ", 161)},
		{"unit", "unit", strings.Repeat("あ", 25)},
		{"notes bytes", "notes", strings.Repeat("あ", maxDocumentNotesBytes/3+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := base()
			if tt.key == "contractor_name" {
				data[tt.key] = tt.value
			} else {
				data["lines"].([]any)[0].(map[string]any)[tt.key] = tt.value
			}
			if _, message := validateAndCalculateDocument(kindFor("estimate"), data); message == "" {
				t.Fatalf("overlong %s was accepted", tt.key)
			}
		})
	}
}

func TestDocumentDecimalValuesFitDatabaseColumns(t *testing.T) {
	base := func(unitPrice, discount string) map[string]any {
		return map[string]any{
			"project_id": json.Number("1"), "issued_on": "2026-07-25", "status": "draft", "discount_amount": json.Number(discount),
			"lines": []any{map[string]any{"item_name": "施工費", "quantity": json.Number("1"), "unit_price": json.Number(unitPrice)}},
		}
	}
	if _, message := validateAndCalculateDocument(kindFor("estimate"), base("999999999999.99", "999999999999.99")); message != "" {
		t.Fatalf("DECIMAL(14,2) maximum rejected: %s", message)
	}
	for _, tt := range []struct{ name, unitPrice, discount string }{
		{"unit price", "1000000000000.00", "0"},
		{"discount", "999999999999.99", "1000000000000.00"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, message := validateAndCalculateDocument(kindFor("estimate"), base(tt.unitPrice, tt.discount)); message == "" {
				t.Fatal("out-of-range DECIMAL(14,2) value was accepted")
			}
		})
	}
}

func TestDocumentQuantityFitsDatabaseColumnEvenWithZeroPrice(t *testing.T) {
	line := func(quantity string) []any {
		return []any{map[string]any{"item_name": "施工費", "quantity": json.Number(quantity), "unit_price": json.Number("0")}}
	}
	if _, message := calculateDocumentLines(line("99999999999.999")); message != "" {
		t.Fatalf("DECIMAL(14,3) maximum rejected: %s", message)
	}
	if _, message := calculateDocumentLines(line("100000000000.000")); message == "" {
		t.Fatal("out-of-range DECIMAL(14,3) quantity was accepted with zero price")
	}
}

func TestRenderDocumentHTMLStructureEscapingAndUnicode(t *testing.T) {
	for _, kind := range []documentKind{kindFor("estimate"), kindFor("invoice")} {
		t.Run(kind.name, func(t *testing.T) {
			row := documentHTMLTestRow(16)
			row["client_name"] = `<script>alert(1)</script>髙﨑𠮷𠮟&`
			row["contractor_address"] = "A</style><script>bad()</script>\nB"
			html, err := renderDocumentHTML(kind, row)
			if err != nil {
				t.Fatal(err)
			}
			text := string(html)
			for _, required := range []string{"<html lang=\"ja\">", "@page { size: A4", "NotoSansJP-Regular.ttf", "<thead>", "<tfoot>", kind.title, kind.totalLabel, "税込合計金額", "髙﨑𠮷𠮟", "1000000000000円"} {
				if !strings.Contains(text, required) {
					t.Fatalf("HTML missing %q", required)
				}
			}
			if strings.Contains(text, `<script>alert(1)</script>`) || strings.Contains(text, `</style><script>`) {
				t.Fatalf("HTML was not escaped: %s", text)
			}
			if got := strings.Count(text, "<tfoot>"); got != 1 || strings.Count(text, "<tfoot>") != strings.Count(text, "</tfoot>") {
				t.Fatalf("tfoot structure invalid")
			}
			for _, label := range []string{"合計", "値引き", "税抜合計", "消費税額（10%）", kind.totalLabel} {
				if !strings.Contains(text, ">"+label+"<") {
					t.Fatalf("tfoot label %q missing", label)
				}
			}
		})
	}
}

func TestRenderDocumentHTMLManyLinesAndEmpty(t *testing.T) {
	for _, count := range []int{0, 1, 16, 200} {
		html, err := renderDocumentHTML(kindFor("estimate"), documentHTMLTestRow(count))
		if err != nil {
			t.Fatalf("%d rows: %v", count, err)
		}
		if got := strings.Count(string(html), "<tr>"); got != count+6 {
			t.Fatalf("%d rows rendered as %d", count, got)
		}
	}
}

func TestGotenbergClientMultipartContractAndContext(t *testing.T) {
	seen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
		if r.URL.Path != "/forms/chromium/convert/html" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(int64(maxDocumentHTMLBytes + len(notoSansJPRegular) + 1024)); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{"preferCssPageSize": "true", "printBackground": "true", "failOnResourceLoadingFailed": "true", "failOnConsoleExceptions": "true", "waitForExpression": "document.readyState === 'complete'"} {
			if got := r.FormValue(key); got != want {
				t.Errorf("%s=%q want %q", key, got, want)
			}
		}
		files := r.MultipartForm.File["files"]
		if len(files) != 2 {
			t.Fatalf("files = %d", len(files))
		}
		gotNames := map[string]bool{}
		for _, file := range files {
			gotNames[file.Filename] = true
			f, _ := file.Open()
			defer f.Close()
			if file.Filename == "index.html" {
				b, _ := io.ReadAll(f)
				if !bytes.Contains(b, []byte("<html")) {
					t.Error("index HTML absent")
				}
			}
		}
		if !gotNames["index.html"] || !gotNames["NotoSansJP-Regular.ttf"] {
			t.Fatalf("unexpected assets %#v", gotNames)
		}
		w.Header().Set("Content-Type", "application/pdf; charset=binary")
		_, _ = w.Write([]byte("%PDF-1.7\nunit"))
	}))
	defer server.Close()
	client, err := NewGotenbergClient(server.URL, 35*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := client.Render(context.Background(), []byte("<html></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if !seen || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("Gotenberg contract not satisfied")
	}
}

func TestGotenbergClientErrorMapping(t *testing.T) {
	cases := []struct {
		name              string
		status            int
		contentType, body string
		want              documentPDFErrorKind
	}{
		{"bad request", 400, "text/plain", "no", documentPDFBadGateway}, {"conflict", 409, "text/plain", "no", documentPDFBadGateway}, {"server", 500, "text/plain", "no", documentPDFBadGateway}, {"unavailable", 503, "text/plain", "no", documentPDFUnavailable}, {"wrong mime", 200, "text/plain", "%PDF-1.7", documentPDFBadGateway}, {"invalid pdf", 200, "application/pdf", "no", documentPDFBadGateway},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer s.Close()
			c, _ := NewGotenbergClient(s.URL, time.Second)
			_, err := c.Render(context.Background(), []byte("ok"))
			var got *documentPDFError
			if !errors.As(err, &got) || got.kind != tt.want {
				t.Fatalf("error %#v, want %v", err, tt.want)
			}
		})
	}
	if c, err := NewGotenbergClient("http://example.test", time.Second); err != nil {
		t.Fatal(err)
	} else if _, err := c.Render(context.Background(), bytes.Repeat([]byte("a"), maxDocumentHTMLBytes+1)); !isDocumentPDFKind(err, documentPDFTooLarge) {
		t.Fatalf("oversized request = %v", err)
	}
	overse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-" + strings.Repeat("x", maxDocumentPDFBytes)))
	}))
	defer overse.Close()
	c, _ := NewGotenbergClient(overse.URL, 35*time.Second)
	if _, err := c.Render(context.Background(), []byte("ok")); !isDocumentPDFKind(err, documentPDFBadGateway) {
		t.Fatalf("oversized response = %v", err)
	}
	refused, _ := NewGotenbergClient("http://127.0.0.1:1", time.Second)
	if _, err := refused.Render(context.Background(), []byte("ok")); !isDocumentPDFKind(err, documentPDFUnavailable) {
		t.Fatalf("refused connection = %v", err)
	}
}

func TestGotenbergClientCancellationAndDeadline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// http.Client.Timeout may not immediately cancel a server-side request.
		// A bounded slow response keeps httptest.Server.Close deterministic.
		time.Sleep(100 * time.Millisecond)
	}))
	defer s.Close()
	c, _ := NewGotenbergClient(s.URL, 20*time.Millisecond)
	_, err := c.Render(context.Background(), []byte("ok"))
	if !isDocumentPDFKind(err, documentPDFDeadline) {
		t.Fatalf("deadline = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Render(ctx, []byte("ok"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}

func TestGotenbergClientRejectsRedirectsAndDisablesProxy(t *testing.T) {
	finalCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			finalCalled = true
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()
	client, err := NewGotenbergClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("Gotenberg client must not use environment proxy settings")
	}
	if _, err = client.Render(context.Background(), []byte("ok")); !isDocumentPDFKind(err, documentPDFBadGateway) {
		t.Fatalf("redirect = %v", err)
	}
	if finalCalled {
		t.Fatal("Gotenberg redirect was followed")
	}
}

func TestGotenbergClientSaturationIsImmediateAndRetryable(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer server.Close()
	client, err := NewGotenbergClientWithConcurrency(server.URL, time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() { _, err := client.Render(context.Background(), []byte("ok")); firstDone <- err }()
	<-started
	startedAt := time.Now()
	_, err = client.Render(context.Background(), []byte("ok"))
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("saturated request waited %s", elapsed)
	}
	var pdfErr *documentPDFError
	if !errors.As(err, &pdfErr) || pdfErr.kind != documentPDFUnavailable || pdfErr.retryAfter != time.Second {
		t.Fatalf("saturated request = %#v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first request = %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	writeDocumentPDFError(c, pdfErr)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("saturation response = %d Retry-After=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}

type documentPDFRendererFunc func(context.Context, []byte) ([]byte, error)

func (f documentPDFRendererFunc) Render(ctx context.Context, html []byte) ([]byte, error) {
	return f(ctx, html)
}

func TestDocumentPDFHandlerSaturationSkipsLoadAndRender(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadCalled, renderCalled := false, false
	renderer := documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		renderCalled = true
		return []byte("%PDF-1.7"), nil
	})
	handler := NewMasterHandler(nil, renderer)
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		loadCalled = true
		return nil, errors.New("must not load")
	}
	for range cap(handler.pdfSlots) {
		handler.pdfSlots <- struct{}{}
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/estimates/1/pdf", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Set("accountID", uint64(1))
	handler.documentPDF("estimate")(c)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("saturation response = %d Retry-After=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if loadCalled || renderCalled {
		t.Fatalf("load=%v render=%v; saturated handler must reject before work", loadCalled, renderCalled)
	}
	if got := documentPDFAccountCount(handler); got != 0 {
		t.Fatalf("account map retained %d entries after global rejection", got)
	}
}

func TestDocumentPDFAccountLimitPrecedesGlobalLimitAndReleases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var loadCalls atomic.Int32
	renderer := documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		started <- struct{}{}
		<-release
		return []byte("%PDF-1.7"), nil
	})
	handler := NewMasterHandler(nil, renderer)
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		loadCalls.Add(1)
		return documentHTMLTestRow(1), nil
	}
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- invokeDocumentPDF(handler, 7, "1", nil) }()
	waitDocumentPDFSignal(t, started)

	// Fill the remaining global slot. The duplicate account must still be
	// classified first as 429, proving account limiting precedes global limiting.
	handler.pdfSlots <- struct{}{}
	second := invokeDocumentPDF(handler, 7, "2", nil)
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") != "1" {
		t.Fatalf("same-account response = %d Retry-After=%q", second.Code, second.Header().Get("Retry-After"))
	}
	if got := loadCalls.Load(); got != 1 {
		t.Fatalf("same-account rejection called loader %d times", got)
	}
	<-handler.pdfSlots
	close(release)
	if first := <-firstDone; first.Code != http.StatusOK {
		t.Fatalf("first response = %d: %s", first.Code, first.Body.String())
	}
	if got := documentPDFAccountCount(handler); got != 0 {
		t.Fatalf("account map retained %d entries after completion", got)
	}

	// The completed account may immediately start another conversion.
	third := invokeDocumentPDF(handler, 7, "3", nil)
	if third.Code != http.StatusOK || loadCalls.Load() != 2 {
		t.Fatalf("released account response=%d loadCalls=%d", third.Code, loadCalls.Load())
	}
	if got := documentPDFAccountCount(handler); got != 0 {
		t.Fatalf("account map retained %d entries after reuse", got)
	}
}

func TestDocumentPDFAccountLimitAllowsDifferentAccounts(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	renderer := documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		started <- struct{}{}
		<-release
		return []byte("%PDF-1.7"), nil
	})
	handler := NewMasterHandler(nil, renderer)
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		return documentHTMLTestRow(1), nil
	}
	done := make(chan *httptest.ResponseRecorder, 2)
	go func() { done <- invokeDocumentPDF(handler, 11, "1", nil) }()
	waitDocumentPDFSignal(t, started)
	go func() { done <- invokeDocumentPDF(handler, 12, "2", nil) }()
	waitDocumentPDFSignal(t, started)
	if got := documentPDFAccountCount(handler); got != 2 {
		t.Fatalf("different accounts in flight = %d, want 2", got)
	}
	close(release)
	for range 2 {
		if response := <-done; response.Code != http.StatusOK {
			t.Fatalf("different-account response = %d: %s", response.Code, response.Body.String())
		}
	}
	if got := documentPDFAccountCount(handler); got != 0 {
		t.Fatalf("account map retained %d entries", got)
	}
}

func TestDocumentPDFAccountEntryReleasesOnCancellationAndPanic(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{}, 1)
		handler := NewMasterHandler(nil, documentPDFRendererFunc(func(ctx context.Context, _ []byte) ([]byte, error) {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}))
		handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
			return documentHTMLTestRow(1), nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- invokeDocumentPDF(handler, 21, "1", ctx) }()
		waitDocumentPDFSignal(t, started)
		cancel()
		<-done
		if got := documentPDFAccountCount(handler); got != 0 || len(handler.pdfSlots) != 0 {
			t.Fatalf("after cancellation account entries=%d global slots=%d", got, len(handler.pdfSlots))
		}
		handler.pdfRenderer = documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
			return []byte("%PDF-1.7"), nil
		})
		if response := invokeDocumentPDF(handler, 21, "2", nil); response.Code != http.StatusOK {
			t.Fatalf("account not reusable after cancellation: %d", response.Code)
		}
	})

	t.Run("panic", func(t *testing.T) {
		handler := NewMasterHandler(nil, documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
			panic("renderer panic")
		}))
		handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
			return documentHTMLTestRow(1), nil
		}
		recovered := false
		func() {
			defer func() { recovered = recover() != nil }()
			invokeDocumentPDF(handler, 22, "1", nil)
		}()
		if !recovered {
			t.Fatal("expected renderer panic")
		}
		if got := documentPDFAccountCount(handler); got != 0 || len(handler.pdfSlots) != 0 {
			t.Fatalf("after panic account entries=%d global slots=%d", got, len(handler.pdfSlots))
		}
		handler.pdfRenderer = documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
			return []byte("%PDF-1.7"), nil
		})
		if response := invokeDocumentPDF(handler, 22, "2", nil); response.Code != http.StatusOK {
			t.Fatalf("account not reusable after panic: %d", response.Code)
		}
	})
}

func TestDocumentPDFHandlerRejectsMissingAccountDefensively(t *testing.T) {
	var loadCalls atomic.Int32
	handler := NewMasterHandler(nil, documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		return []byte("%PDF-1.7"), nil
	}))
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		loadCalls.Add(1)
		return documentHTMLTestRow(1), nil
	}
	response := invokeDocumentPDF(handler, 0, "1", nil)
	if response.Code != http.StatusUnauthorized || loadCalls.Load() != 0 {
		t.Fatalf("missing account response=%d loadCalls=%d", response.Code, loadCalls.Load())
	}
	if got := documentPDFAccountCount(handler); got != 0 {
		t.Fatalf("missing account created %d map entries", got)
	}
}

func TestDocumentPDFWorkDeadlineReleasesSlotsAndSkipsRenderer(t *testing.T) {
	started := make(chan struct{}, 1)
	var renderCalls atomic.Int32
	handler := NewMasterHandler(nil, documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		renderCalls.Add(1)
		return []byte("%PDF-1.7"), nil
	}))
	handler.pdfWorkTimeout = 50 * time.Millisecond
	handler.pdfDocumentLoader = func(ctx context.Context, _ documentKind, _ string) (map[string]any, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	responseDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { responseDone <- invokeDocumentPDF(handler, 41, "1", nil) }()
	waitDocumentPDFSignal(t, started)
	response := <-responseDone
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("deadline response = %d: %s", response.Code, response.Body.String())
	}
	if renderCalls.Load() != 0 {
		t.Fatalf("renderer called %d times after loader deadline", renderCalls.Load())
	}
	if got := documentPDFAccountCount(handler); got != 0 || len(handler.pdfSlots) != 0 {
		t.Fatalf("deadline retained account=%d global=%d", got, len(handler.pdfSlots))
	}

	handler.pdfWorkTimeout = time.Second
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		return documentHTMLTestRow(1), nil
	}
	if retry := invokeDocumentPDF(handler, 41, "2", nil); retry.Code != http.StatusOK {
		t.Fatalf("same account not reusable after deadline: %d", retry.Code)
	}
}

func TestDocumentPDFWorkDeadlinePropagatesToRenderer(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	handler := NewMasterHandler(nil, documentPDFRendererFunc(func(ctx context.Context, _ []byte) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("renderer context has no deadline")
		}
		deadlineSeen <- deadline
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	handler.pdfWorkTimeout = 50 * time.Millisecond
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		return documentHTMLTestRow(1), nil
	}
	startedAt := time.Now()
	response := invokeDocumentPDF(handler, 42, "1", nil)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("renderer deadline response = %d: %s", response.Code, response.Body.String())
	}
	if deadline := <-deadlineSeen; deadline.Before(startedAt) || deadline.After(startedAt.Add(handler.pdfWorkTimeout+time.Second)) {
		t.Fatalf("renderer deadline %s is outside work budget", deadline)
	}
	if got := documentPDFAccountCount(handler); got != 0 || len(handler.pdfSlots) != 0 {
		t.Fatalf("renderer deadline retained account=%d global=%d", got, len(handler.pdfSlots))
	}
}

func TestDocumentPDFWriteTimeoutReleasesSlotsForNonReadingClient(t *testing.T) {
	rendered := make(chan struct{}, 1)
	largePDF := make([]byte, maxDocumentPDFBytes)
	copy(largePDF, "%PDF-1.7\n")
	handler := NewMasterHandler(nil, documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		rendered <- struct{}{}
		return largePDF, nil
	}))
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		return documentHTMLTestRow(1), nil
	}
	address := serveDocumentPDFForTest(t, handler, 100*time.Millisecond)
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := io.WriteString(connection, "GET /estimates/1/pdf HTTP/1.1\r\nHost: "+address+"\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	waitDocumentPDFSignal(t, rendered)
	// Deliberately do not read until the server's write deadline has elapsed.
	time.Sleep(250 * time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for documentPDFAccountCount(handler) != 0 || len(handler.pdfSlots) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("write timeout did not release account=%d global=%d", documentPDFAccountCount(handler), len(handler.pdfSlots))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(connection); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatalf("non-reading connection remained open after write timeout: %v", err)
		}
	}

	handler.pdfRenderer = documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		return []byte("%PDF-1.7"), nil
	})
	response, err := http.Get("http://" + address + "/estimates/1/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("account was not reusable after write timeout: %d", response.StatusCode)
	}
}

func TestDocumentPDFMaximumResponseCompletesWithinWriteBudget(t *testing.T) {
	largePDF := make([]byte, maxDocumentPDFBytes)
	copy(largePDF, "%PDF-1.7\n")
	handler := NewMasterHandler(nil, documentPDFRendererFunc(func(context.Context, []byte) ([]byte, error) {
		return largePDF, nil
	}))
	handler.pdfDocumentLoader = func(context.Context, documentKind, string) (map[string]any, error) {
		return documentHTMLTestRow(1), nil
	}
	address := serveDocumentPDFForTest(t, handler, 10*time.Second)
	response, err := http.Get("http://" + address + "/estimates/1/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body) != maxDocumentPDFBytes || !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Fatalf("maximum response status=%d bytes=%d", response.StatusCode, len(body))
	}
	if got := documentPDFAccountCount(handler); got != 0 || len(handler.pdfSlots) != 0 {
		t.Fatalf("maximum response retained account=%d global=%d", got, len(handler.pdfSlots))
	}
}

func serveDocumentPDFForTest(t *testing.T, handler *MasterHandler, writeTimeout time.Duration) string {
	t.Helper()
	router := gin.New()
	router.GET("/estimates/:id/pdf", func(c *gin.Context) {
		c.Set("accountID", uint64(31))
		c.Next()
	}, handler.documentPDF("estimate"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().String()
}

func invokeDocumentPDF(handler *MasterHandler, accountID uint64, documentID string, requestContext context.Context) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/estimates/"+documentID+"/pdf", nil)
	if requestContext != nil {
		request = request.WithContext(requestContext)
	}
	c.Request = request
	c.Params = gin.Params{{Key: "id", Value: documentID}}
	if accountID != 0 {
		c.Set("accountID", accountID)
	}
	handler.documentPDF("estimate")(c)
	return recorder
}

func documentPDFAccountCount(handler *MasterHandler) int {
	handler.pdfAccountMu.Lock()
	defer handler.pdfAccountMu.Unlock()
	return len(handler.pdfAccounts)
}

func waitDocumentPDFSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for PDF renderer")
	}
}

func TestGotenbergLongDocumentTotalsAppearOnce(t *testing.T) {
	gotenbergURL := os.Getenv("GOTENBERG_TEST_URL")
	if gotenbergURL == "" {
		t.Skip("set GOTENBERG_TEST_URL to run the real Chromium regression")
	}
	client, err := NewGotenbergClient(gotenbergURL, 35*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	html, err := renderDocumentHTML(kindFor("estimate"), documentHTMLTestRow(500))
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := client.Render(context.Background(), html)
	if err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("GOTENBERG_TEST_PDF_PATH")
	if path == "" {
		path = filepath.Join(t.TempDir(), "long-document.pdf")
	}
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext is required to inspect labels")
	}
	output, err := exec.Command(pdftotext, "-layout", path, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, output)
	}
	for _, label := range []string{"合計", "値引き", "税抜合計", "消費税額（10%）", "御見積合計"} {
		pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(label) + `\s+`)
		if got := len(pattern.FindAll(output, -1)); got != 1 {
			t.Fatalf("%q appears %d times in multi-page PDF\n%s", label, got, output)
		}
	}
}

func isDocumentPDFKind(err error, want documentPDFErrorKind) bool {
	var got *documentPDFError
	return errors.As(err, &got) && got.kind == want
}

func documentHTMLTestRow(count int) map[string]any {
	lines := make([]map[string]any, count)
	for i := range lines {
		lines[i] = map[string]any{"item_name": fmt.Sprintf("施工%d", i), "quantity": float64(1), "unit": "式", "unit_price": float64(100), "amount": int64(100), "notes": "長文\n備考"}
	}
	return map[string]any{"estimate_code": "EST-1", "invoice_code": "INV-1", "issued_on": "2026-07-26", "client_name": "得意先", "client_honorific": "御中", "client_address": "東京都", "client_contact_name": "担当", "client_contact_phone": "03", "contractor_name": "請負者", "contractor_representative": "代表", "contractor_address": "住所", "subtotal": int64(1_000_000_000_000), "discount_amount": int64(2), "pre_tax": int64(999_999_999_998), "tax_amount": int64(99_999_999_999), "total": int64(1_099_999_999_997), "lines": lines}
}

func TestMultipartFilesAreFlat(t *testing.T) { // guards accidental nested assets that Gotenberg does not recreate.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	f, _ := w.CreateFormFile("files", "index.html")
	_, _ = f.Write([]byte("x"))
	_ = w.Close()
	if !strings.Contains(w.FormDataContentType(), "multipart/form-data") {
		t.Fatal("multipart writer regression")
	}
}
