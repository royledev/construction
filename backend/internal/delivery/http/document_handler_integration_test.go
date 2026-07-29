package httpdelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/royle/construction/backend/internal/domain"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Opt-in real MySQL coverage for commercial documents. It deliberately creates
// only timestamped rows and removes those exact rows at cleanup.
func TestDocumentHTTPPersistenceRegression(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set; skipping MySQL HTTP integration test")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("t4%010x", time.Now().UnixNano()&0xffffffffff)
	create := func(table string, values map[string]any, key string, value any) uint64 {
		t.Helper()
		if err := db.Table(table).Create(values).Error; err != nil {
			t.Fatal(err)
		}
		var id uint64
		if err := db.Table(table).Select("id").Where(key+" = ?", value).Take(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	clientID := create("clients", map[string]any{"client_code": suffix + "c", "name": "T4得意先"}, "client_code", suffix+"c")
	employeeID := create("employees", map[string]any{"employee_code": suffix + "e", "full_name": "T4管理者"}, "employee_code", suffix+"e")
	adminID := create("accounts", map[string]any{"employee_id": employeeID, "login_id": suffix + "a", "password_hash": "x", "role": "admin"}, "login_id", suffix+"a")
	userEmployeeID := create("employees", map[string]any{"employee_code": suffix + "u", "full_name": "T4一般"}, "employee_code", suffix+"u")
	userID := create("accounts", map[string]any{"employee_id": userEmployeeID, "login_id": suffix + "u", "password_hash": "x", "role": "user"}, "login_id", suffix+"u")
	projectID := create("projects", map[string]any{"project_code": "PRJ-" + suffix[2:], "site_name": "T4現場", "client_id": clientID}, "project_code", "PRJ-"+suffix[2:])
	var estimateID, invoiceID uint64
	t.Cleanup(func() {
		if invoiceID != 0 {
			if err := db.Table("invoice_lines").Where("invoice_id = ?", invoiceID).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup invoice lines: %v", err)
			}
			if err := db.Table("invoices").Where("id = ?", invoiceID).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup invoice: %v", err)
			}
		}
		if estimateID != 0 {
			if err := db.Table("estimate_lines").Where("estimate_id = ?", estimateID).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup estimate lines: %v", err)
			}
			if err := db.Table("estimates").Where("id = ?", estimateID).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup estimate: %v", err)
			}
		}
		if err := db.Table("projects").Where("id = ?", projectID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup project: %v", err)
		}
		if err := db.Table("accounts").Where("id IN ?", []uint64{adminID, userID}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup accounts: %v", err)
		}
		if err := db.Table("employees").Where("id IN ?", []uint64{employeeID, userEmployeeID}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup employees: %v", err)
		}
		if err := db.Table("clients").Where("id = ?", clientID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup client: %v", err)
		}
		for _, row := range []struct {
			table string
			id    uint64
		}{
			{"invoice_lines", invoiceID},
			{"invoices", invoiceID},
			{"estimate_lines", estimateID},
			{"estimates", estimateID},
			{"projects", projectID},
			{"accounts", adminID},
			{"accounts", userID},
			{"employees", employeeID},
			{"employees", userEmployeeID},
			{"clients", clientID},
		} {
			if row.id == 0 {
				continue
			}
			column := "id"
			if row.table == "estimate_lines" {
				column = "estimate_id"
			} else if row.table == "invoice_lines" {
				column = "invoice_id"
			}
			var remaining int64
			if err := db.Table(row.table).Where(column+" = ?", row.id).Count(&remaining).Error; err != nil {
				t.Errorf("verify cleanup %s: %v", row.table, err)
			} else if remaining != 0 {
				t.Errorf("cleanup left %d %s rows", remaining, row.table)
			}
		}
	})
	gin.SetMode(gin.TestMode)
	sessions := NewSessionManager("document-test", false)
	router := gin.New()
	api := router.Group("/api")
	NewMasterHandler(db, documentPDFTestRenderer{}).Register(api, sessions, NewSessionValidator(db))
	request := func(method, path string, account uint64, payload any) *httptest.ResponseRecorder {
		var body *bytes.Reader
		if payload == nil {
			body = bytes.NewReader(nil)
		} else {
			raw, e := json.Marshal(payload)
			if e != nil {
				t.Fatal(e)
			}
			body = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, body)
		if payload != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if account != 0 {
			workerID := employeeID
			if account == userID {
				workerID = userEmployeeID
			}
			token, _, e := sessions.IssueUser(domain.AuthenticatedUser{ID: account, WorkerType: "employee", WorkerID: workerID, SessionVersion: 1})
			if e != nil {
				t.Fatal(e)
			}
			r.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, fmt.Sprintf("/api/estimates?project_id=%d", projectID), adminID, nil); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("empty list = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/estimates", 0, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d", got.Code)
	}
	payload := map[string]any{"project_id": projectID, "issued_on": "2026-07-25", "status": "draft", "discount_amount": 0, "lines": []any{map[string]any{"item_name": "施工費 𠮷髙﨑", "quantity": 1, "unit": "式", "unit_price": 1000, "notes": "実API帳票"}}}
	if got := request(http.MethodPost, "/api/estimates", userID, payload); got.Code != http.StatusForbidden {
		t.Fatalf("user create = %d", got.Code)
	}
	if got := request(http.MethodPost, "/api/estimates", adminID, payload); got.Code != http.StatusCreated {
		t.Fatalf("admin create = %d %s", got.Code, got.Body.String())
	}
	if err := db.Table("estimates").Select("id").Where("project_id = ?", projectID).Take(&estimateID).Error; err != nil {
		t.Fatal(err)
	}
	assertPDF := func(path string) []byte {
		t.Helper()
		got := request(http.MethodGet, path, adminID, nil)
		if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "application/pdf" || got.Header().Get("Cache-Control") != "private, no-store" || !bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) {
			t.Fatalf("PDF %s = %d %q %q", path, got.Code, got.Header().Get("Content-Type"), got.Body.Bytes()[:min(len(got.Body.Bytes()), 16)])
		}
		return got.Body.Bytes()
	}
	assertPDF(fmt.Sprintf("/api/estimates/%d/pdf", estimateID))
	invoicePayload := map[string]any{"project_id": projectID, "issued_on": "2026-07-25", "billing_type": "individual", "discount_amount": 0, "lines": []any{map[string]any{"item_name": "請求施工費 𠮷髙﨑", "quantity": 1, "unit": "式", "unit_price": 2000, "notes": "実API帳票"}}}
	if got := request(http.MethodPost, "/api/invoices", adminID, invoicePayload); got.Code != http.StatusCreated {
		t.Fatalf("invoice create = %d %s", got.Code, got.Body.String())
	}
	if err := db.Table("invoices").Select("id").Where("project_id = ?", projectID).Take(&invoiceID).Error; err != nil {
		t.Fatal(err)
	}
	assertPDF(fmt.Sprintf("/api/invoices/%d/pdf", invoiceID))
	payload["lines"] = []any{map[string]any{"item_name": "数量上限", "quantity": "99999999999.999", "unit": "式", "unit_price": "0", "notes": ""}}
	if got := request(http.MethodPatch, fmt.Sprintf("/api/estimates/%d", estimateID), adminID, payload); got.Code != http.StatusOK {
		t.Fatalf("quantity maximum patch = %d %s", got.Code, got.Body.String())
	}
	payload["lines"] = []any{map[string]any{"item_name": "数量上限超過", "quantity": "100000000000.000", "unit": "式", "unit_price": "0", "notes": ""}}
	if got := request(http.MethodPatch, fmt.Sprintf("/api/estimates/%d", estimateID), adminID, payload); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("quantity overflow patch = %d %s", got.Code, got.Body.String())
	}
	var lineCount int64
	if err := db.Table("estimate_lines").Where("estimate_id = ?", estimateID).Count(&lineCount).Error; err != nil {
		t.Fatal(err)
	}
	if lineCount != 1 {
		t.Fatalf("failed patch replaced lines: %d", lineCount)
	}
	if err := db.Table("estimates").Where("id = ?", estimateID).Update("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, fmt.Sprintf("/api/estimates/%d", estimateID), adminID, nil); got.Code != http.StatusNotFound {
		t.Fatalf("deleted detail = %d", got.Code)
	}
	if got := request(http.MethodGet, fmt.Sprintf("/api/estimates/%d/pdf", estimateID), adminID, nil); got.Code != http.StatusNotFound {
		t.Fatalf("deleted PDF = %d", got.Code)
	}
	if err := db.Table("clients").Where("id = ?", clientID).Update("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/estimates", adminID, payload); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("deleted client = %d", got.Code)
	}
}

type documentPDFTestRenderer struct{}

func (documentPDFTestRenderer) Render(_ context.Context, html []byte) ([]byte, error) {
	if !bytes.Contains(html, []byte("<html lang=\"ja\">")) || !bytes.Contains(html, []byte("<tfoot>")) {
		return nil, fmt.Errorf("HTML contract missing")
	}
	return []byte("%PDF-1.7\nintegration-test"), nil
}
