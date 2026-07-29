package httpdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Amounts are Japanese yen. Persistence and PDF rendering always derive them
// from the lines, never from display-only client values.
type documentKind struct{ name, table, lines, number, typeColumn, title, totalLabel string }

func kindFor(name string) documentKind {
	if name == "invoice" {
		return documentKind{"invoice", "invoices", "invoice_lines", "invoice_code", "billing_type", "請求書", "御請求合計"}
	}
	return documentKind{"estimate", "estimates", "estimate_lines", "estimate_code", "status", "見積書", "御見積合計"}
}

type documentLineInput struct {
	ItemName  string
	Quantity  float64
	Unit      string
	UnitPrice float64
	Notes     string
}
type calculatedDocument struct {
	Lines                                  []documentLineInput
	Amounts                                []int64
	Subtotal, Discount, PreTax, Tax, Total int64
}

const (
	maxDocumentYen           int64 = 1_000_000_000_000
	maxDocumentDecimalCents        = 99_999_999_999_999 // MySQL DECIMAL(14,2)
	maxDocumentQuantityMilli       = 99_999_999_999_999 // MySQL DECIMAL(14,3)
	maxDocumentJSONBytes           = 2 << 20
	maxDocumentLines               = 500
	maxDocumentNotesBytes          = 65_535
)

func (h *MasterHandler) listDocuments(name string) gin.HandlerFunc {
	kind := kindFor(name)
	return func(c *gin.Context) {
		rows := make([]map[string]any, 0)
		query := h.db.Table(kind.table + " d").Select("d.id").Where("d.deleted_at IS NULL")
		if projectID := c.Query("project_id"); projectID != "" {
			query = query.Where("d.project_id = ?", projectID)
		}
		if err := query.Order("d.issued_on DESC, d.id DESC").Find(&rows).Error; err != nil {
			serverError(c, err)
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, item := range rows {
			document, err := h.loadDocument(c.Request.Context(), kind, fmt.Sprint(item["id"]))
			if err != nil {
				serverError(c, err)
				return
			}
			document["document_code"], document["document_type"] = document[kind.number], document[kind.typeColumn]
			items = append(items, document)
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}
func (h *MasterHandler) getDocument(name string) gin.HandlerFunc {
	kind := kindFor(name)
	return func(c *gin.Context) {
		row, err := h.loadDocument(c.Request.Context(), kind, c.Param("id"))
		if errors.Is(err, errDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
		if err != nil {
			serverError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": row})
	}
}
func (h *MasterHandler) updateDocument(name string) gin.HandlerFunc {
	kind := kindFor(name)
	return func(c *gin.Context) {
		id, err := idParam(c)
		if err != nil {
			return
		}
		h.saveDocument(kind.name, id)(c)
	}
}

func (h *MasterHandler) saveDocument(name string, id uint64) gin.HandlerFunc {
	kind := kindFor(name)
	return func(c *gin.Context) {
		data, ok := documentBody(c)
		if !ok {
			return
		}
		calculation, message := validateAndCalculateDocument(kind, data)
		if message != "" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": message})
			return
		}
		projectID, _ := documentID(data["project_id"])
		isNew := id == 0
		values := documentValues(kind, data, projectID, calculation.Discount)
		tx := h.db.Begin()
		if tx.Error != nil {
			serverError(c, tx.Error)
			return
		}
		if !activeProjectClientExists(tx, projectID) {
			tx.Rollback()
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "有効な案件と得意先を指定してください。"})
			return
		}
		if isNew {
			prefix := "EST"
			if kind.name == "invoice" {
				prefix = "INV"
			}
			if err := insertWithGeneratedCode(tx, kind.table, kind.number, prefix, values); err != nil {
				tx.Rollback()
				dbError(c, err)
				return
			}
			id = insertedID(tx, values)
		} else {
			delete(values, kind.number)
			result := tx.Table(kind.table).Where("id = ? AND deleted_at IS NULL", id).Updates(values)
			if result.Error != nil {
				tx.Rollback()
				dbError(c, result.Error)
				return
			}
			if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(tx, kind.table, id) }) {
				tx.Rollback()
				c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
				return
			}
			if err := tx.Table(kind.lines).Where(kind.name+"_id = ?", id).Delete(nil).Error; err != nil {
				tx.Rollback()
				serverError(c, err)
				return
			}
		}
		for i, line := range calculation.Lines {
			row := map[string]any{kind.name + "_id": id, "line_no": i + 1, "item_name": line.ItemName, "quantity": line.Quantity, "unit": nullableString(line.Unit), "unit_price": line.UnitPrice, "notes": nullableString(line.Notes)}
			if err := tx.Table(kind.lines).Create(row).Error; err != nil {
				tx.Rollback()
				dbError(c, err)
				return
			}
		}
		if err := tx.Commit().Error; err != nil {
			serverError(c, err)
			return
		}
		row, err := h.loadDocument(c.Request.Context(), kind, strconv.FormatUint(id, 10))
		if err != nil {
			serverError(c, err)
			return
		}
		if isNew {
			c.JSON(http.StatusCreated, gin.H{"item": row})
		} else {
			c.JSON(http.StatusOK, gin.H{"item": row})
		}
	}
}
func activeProjectClientExists(tx *gorm.DB, projectID uint64) bool {
	var count int64
	return tx.Table("projects p").Joins("JOIN clients c ON c.id = p.client_id AND c.deleted_at IS NULL").Where("p.id = ? AND p.deleted_at IS NULL", projectID).Count(&count).Error == nil && count == 1
}
func documentValues(kind documentKind, data map[string]any, projectID uint64, discount int64) map[string]any {
	values := map[string]any{"project_id": projectID, "issued_on": toString(data["issued_on"]), "contractor_name": nullableString(toString(data["contractor_name"])), "contractor_representative": nullableString(toString(data["contractor_representative"])), "contractor_address": nullableString(toString(data["contractor_address"])), "discount_amount": discount}
	values[kind.typeColumn] = toString(data[kind.typeColumn])
	return values
}
func validateAndCalculateDocument(kind documentKind, data map[string]any) (calculatedDocument, string) {
	if !documentStringWithin(data, "contractor_name", 160, 0) || !documentStringWithin(data, "contractor_representative", 120, 0) || !documentStringWithin(data, "contractor_address", 255, 0) {
		return calculatedDocument{}, "帳票発行者の文字数または入力形式が不正です。"
	}
	if strings.TrimSpace(toString(data["issued_on"])) == "" {
		return calculatedDocument{}, "発行日は必須です。"
	}
	if _, err := time.Parse("2006-01-02", toString(data["issued_on"])); err != nil {
		return calculatedDocument{}, "発行日が不正です。"
	}
	if _, ok := documentID(data["project_id"]); !ok {
		return calculatedDocument{}, "案件を指定してください。"
	}
	if kind.name == "estimate" && !oneOf(toString(data["status"]), "draft", "confirmed") {
		return calculatedDocument{}, "見積ステータスが不正です。"
	}
	if kind.name == "invoice" && !oneOf(toString(data["billing_type"]), "closing", "individual") {
		return calculatedDocument{}, "請求区分が不正です。"
	}
	raw, present, typed := arrayValue(data, "lines")
	if !present || !typed || len(raw) == 0 {
		return calculatedDocument{}, "明細を1行以上入力してください。"
	}
	if len(raw) > maxDocumentLines {
		return calculatedDocument{}, "明細は500行まで入力できます。"
	}
	calculation, message := calculateDocumentLines(raw)
	if message != "" {
		return calculatedDocument{}, message
	}
	discountCents, valid := scaledDecimal(data["discount_amount"], 2)
	if !valid || discountCents < 0 || discountCents > maxDocumentDecimalCents {
		return calculatedDocument{}, "値引きは0以上の数値で入力してください。"
	}
	calculation.Discount = (discountCents + 50) / 100
	if calculation.Discount > calculation.Subtotal {
		return calculatedDocument{}, "値引きは明細合計以下で入力してください。"
	}
	calculation.PreTax = calculation.Subtotal - calculation.Discount
	calculation.Tax = (calculation.PreTax + 5) / 10
	if calculation.PreTax > math.MaxInt64-calculation.Tax {
		return calculatedDocument{}, "合計金額が大きすぎます。"
	}
	calculation.Total = calculation.PreTax + calculation.Tax
	return calculation, ""
}
func documentBody(c *gin.Context) (map[string]any, bool) {
	var data map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxDocumentJSONBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "入力内容が大きすぎます。"})
			return nil, false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "入力内容を確認してください。"})
		return nil, false
	}
	if data == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入力内容を確認してください。"})
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "入力内容が大きすぎます。"})
			return nil, false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "入力内容を確認してください。"})
		return nil, false
	}
	return data, true
}
func documentID(value any) (uint64, bool) {
	if v, ok := value.(json.Number); ok {
		id, err := strconv.ParseUint(v.String(), 10, 64)
		return id, err == nil && id > 0
	}
	return positiveID(value)
}
func calculateDocumentLines(raw []any) (calculatedDocument, string) {
	calculation := calculatedDocument{}
	for _, rawLine := range raw {
		row, ok := rawLine.(map[string]any)
		if !ok || !requiredString(row, "item_name") || !optionalString(row, "unit") || !optionalString(row, "notes") || !documentLineStringsWithin(row) {
			return calculatedDocument{}, "明細の入力内容が不正です。"
		}
		quantityMilli, quantityOK := scaledDecimal(row["quantity"], 3)
		unitPriceCents, priceOK := scaledDecimal(row["unit_price"], 2)
		if !quantityOK || !priceOK || quantityMilli <= 0 || quantityMilli > maxDocumentQuantityMilli || unitPriceCents < 0 || unitPriceCents > maxDocumentDecimalCents {
			return calculatedDocument{}, "数量は0より大きく、単価は0以上の数値で入力してください。"
		}
		if unitPriceCents != 0 && quantityMilli > math.MaxInt64/unitPriceCents {
			return calculatedDocument{}, "明細金額が大きすぎます。"
		}
		product := quantityMilli * unitPriceCents
		if product > math.MaxInt64-50_000 {
			return calculatedDocument{}, "明細金額が大きすぎます。"
		}
		amount := (product + 50_000) / 100_000
		if amount > maxDocumentYen || calculation.Subtotal > maxDocumentYen-amount {
			return calculatedDocument{}, "合計金額が大きすぎます。"
		}
		line := documentLineInput{strings.TrimSpace(toString(row["item_name"])), float64(quantityMilli) / 1000, toString(row["unit"]), float64(unitPriceCents) / 100, toString(row["notes"])}
		calculation.Lines = append(calculation.Lines, line)
		calculation.Amounts = append(calculation.Amounts, amount)
		calculation.Subtotal += amount
	}
	return calculation, ""
}

func documentStringWithin(data map[string]any, key string, maxRunes, maxBytes int) bool {
	value, present := data[key]
	if !present || value == nil {
		return true
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	text = strings.TrimSpace(text)
	return (maxRunes == 0 || utf8.RuneCountInString(text) <= maxRunes) && (maxBytes == 0 || len(text) <= maxBytes)
}

func documentLineStringsWithin(row map[string]any) bool {
	return documentStringWithin(row, "item_name", 160, 0) && documentStringWithin(row, "unit", 24, 0) && documentStringWithin(row, "notes", 0, maxDocumentNotesBytes)
}
func scaledDecimal(value any, places int) (int64, bool) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case string:
		text = strings.TrimSpace(v)
	case float64:
		text = strconv.FormatFloat(v, 'f', -1, 64)
	case []byte:
		text = string(v)
	default:
		return 0, false
	}
	if text == "" || strings.HasPrefix(text, "-") {
		return 0, false
	}
	parts := strings.Split(text, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && len(parts[1]) > places {
		return 0, false
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, false
			}
		}
	}
	scale := int64(1)
	for range places {
		scale *= 10
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > math.MaxInt64/scale {
		return 0, false
	}
	fraction := int64(0)
	if len(parts) == 2 && parts[1] != "" {
		padded := parts[1] + strings.Repeat("0", places-len(parts[1]))
		fraction, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, false
		}
	}
	if whole > (math.MaxInt64-fraction)/scale {
		return 0, false
	}
	return whole*scale + fraction, true
}
func roundYen(value float64) int64 { return int64(math.Floor(value + .5)) }
func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
func number(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case uint:
		return float64(v)
	case []byte:
		n, _ := strconv.ParseFloat(string(v), 64)
		return n
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	default:
		return 0
	}
}

var errDocumentNotFound = errors.New("document not found")

func (h *MasterHandler) loadDocument(ctx context.Context, kind documentKind, id string) (map[string]any, error) {
	var row map[string]any
	err := h.db.WithContext(ctx).Table(kind.table+" d").Select("d.*, p.project_code, p.site_name, p.site_address, c.name AS client_name, c.honorific AS client_honorific, c.address AS client_address, c.contact_name AS client_contact_name, c.contact_phone AS client_contact_phone").Joins("JOIN projects p ON p.id = d.project_id").Joins("LEFT JOIN clients c ON c.id = p.client_id").Where("d.id = ? AND d.deleted_at IS NULL", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errDocumentNotFound
		}
		return nil, fmt.Errorf("load document: %w", err)
	}
	var lines []map[string]any
	if err := h.db.WithContext(ctx).Table(kind.lines).Where(kind.name+"_id = ?", id).Order("line_no").Find(&lines).Error; err != nil {
		return nil, fmt.Errorf("load document lines: %w", err)
	}
	raw := make([]any, 0, len(lines))
	for _, line := range lines {
		normalized := map[string]any{}
		for key, value := range line {
			if bytesValue, ok := value.([]byte); ok {
				normalized[key] = string(bytesValue)
			} else {
				normalized[key] = value
			}
		}
		raw = append(raw, normalized)
	}
	calculation, message := calculateDocumentLines(raw)
	if message != "" {
		return nil, fmt.Errorf("calculate stored document: %s", message)
	}
	discountCents, valid := scaledDecimal(row["discount_amount"], 2)
	if !valid || discountCents < 0 || discountCents > maxDocumentDecimalCents {
		return nil, errors.New("stored document has invalid discount")
	}
	calculation.Discount = (discountCents + 50) / 100
	if calculation.Discount > calculation.Subtotal {
		return nil, errors.New("stored document discount exceeds subtotal")
	}
	calculation.PreTax = calculation.Subtotal - calculation.Discount
	calculation.Tax = (calculation.PreTax + 5) / 10
	if calculation.PreTax > math.MaxInt64-calculation.Tax {
		return nil, errors.New("stored document total overflows")
	}
	calculation.Total = calculation.PreTax + calculation.Tax
	for i := range lines {
		lines[i]["amount"] = calculation.Amounts[i]
	}
	row["lines"] = lines
	row["subtotal"], row["discount_amount"], row["pre_tax"], row["tax_amount"], row["total"] = calculation.Subtotal, calculation.Discount, calculation.PreTax, calculation.Tax, calculation.Total
	return row, nil
}
func (h *MasterHandler) documentPDF(name string) gin.HandlerFunc {
	kind := kindFor(name)
	return func(c *gin.Context) {
		workContext, cancelWork := context.WithTimeout(c.Request.Context(), h.pdfWorkTimeout)
		defer cancelWork()
		releaseAccount, ok := h.acquirePDFAccount(c)
		if !ok {
			return
		}
		defer releaseAccount()
		select {
		case h.pdfSlots <- struct{}{}:
			defer func() { <-h.pdfSlots }()
		default:
			writeDocumentPDFError(c, documentPDFBusyError())
			return
		}
		loader := h.loadDocument
		if h.pdfDocumentLoader != nil {
			loader = h.pdfDocumentLoader
		}
		row, err := loader(workContext, kind, c.Param("id"))
		if errors.Is(err, errDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || workContext.Err() != nil {
				writeDocumentPDFError(c, err)
				return
			}
			serverError(c, err)
			return
		}
		html, err := renderDocumentHTML(kind, row)
		if err != nil {
			writeDocumentPDFError(c, err)
			return
		}
		if err := workContext.Err(); err != nil {
			writeDocumentPDFError(c, err)
			return
		}
		pdf, err := h.pdfRenderer.Render(workContext, html)
		if err != nil {
			writeDocumentPDFError(c, err)
			return
		}
		filename := url.PathEscape(fmt.Sprintf("%s-%s.pdf", kind.title, toString(row[kind.number])))
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", "attachment; filename=\"document.pdf\"; filename*=UTF-8''"+filename)
		c.Header("Cache-Control", "private, no-store")
		c.Data(http.StatusOK, "application/pdf", pdf)
	}
}

func (h *MasterHandler) acquirePDFAccount(c *gin.Context) (func(), bool) {
	rawAccountID, exists := c.Get("accountID")
	accountID, typed := rawAccountID.(uint64)
	if !exists || !typed || accountID == 0 {
		// All registered PDF routes run behind requireSession. Fail closed if a
		// future route is accidentally wired without that middleware; never
		// collapse missing identities into a shared anonymous bucket.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ログインが必要です。"})
		return nil, false
	}
	h.pdfAccountMu.Lock()
	if _, busy := h.pdfAccounts[accountID]; busy {
		h.pdfAccountMu.Unlock()
		c.Header("Retry-After", "1")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "同じアカウントでPDFを作成中です。"})
		return nil, false
	}
	h.pdfAccounts[accountID] = struct{}{}
	h.pdfAccountMu.Unlock()
	return func() {
		h.pdfAccountMu.Lock()
		delete(h.pdfAccounts, accountID)
		h.pdfAccountMu.Unlock()
	}, true
}

func writeDocumentPDFError(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "PDF作成がタイムアウトしました。"})
		return
	}
	var pdfErr *documentPDFError
	if errors.As(err, &pdfErr) {
		switch pdfErr.kind {
		case documentPDFTooLarge:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "帳票データが大きすぎます。"})
		case documentPDFUnavailable:
			if pdfErr.retryAfter > 0 {
				c.Header("Retry-After", strconv.FormatInt(int64(math.Ceil(pdfErr.retryAfter.Seconds())), 10))
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PDF作成サービスが一時的に利用できません。"})
		case documentPDFDeadline:
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "PDF作成がタイムアウトしました。"})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "PDF作成サービスから正常な応答を取得できません。"})
		}
		return
	}
	serverError(c, err)
}
