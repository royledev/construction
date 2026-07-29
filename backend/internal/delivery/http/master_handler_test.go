package httpdelivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/royle/construction/backend/internal/domain"
)

type sessionValidatorFunc func(context.Context, sessionClaims) (bool, error)

func (f sessionValidatorFunc) IsActive(ctx context.Context, claims sessionClaims) (bool, error) {
	return f(ctx, claims)
}

func TestPickOnlyAllowsDeclaredColumns(t *testing.T) {
	values := pick(map[string]any{"name": "配管工事", "role": "admin", "deleted_at": "bad"}, []string{"name"})
	if len(values) != 1 || values["name"] != "配管工事" {
		t.Fatalf("unexpected picked values: %#v", values)
	}
	if _, exists := values["role"]; exists {
		t.Fatal("unexpected non-whitelisted input")
	}
}

func TestAsUint64HandlesJSONNumberAndInvalidInput(t *testing.T) {
	if got := asUint64(float64(42)); got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
	if got := asUint64("not-a-number"); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestEmployeeRelatedArraysAreFullyValidatedBeforeReplacement(t *testing.T) {
	workItemExists := func(id uint64) bool { return id == 10 }
	cases := []struct {
		name string
		data map[string]any
	}{
		{"work items must be an array", map[string]any{"workItemIds": "10"}},
		{"work item id must be integral", map[string]any{"workItemIds": []any{10.5}}},
		{"work item must be a work-type reference", map[string]any{"workItemIds": []any{11}}},
		{"work item duplicates are rejected", map[string]any{"workItemIds": []any{10, 10}}},
		{"qualification must be an object", map[string]any{"qualifications": []any{"bad"}}},
		{"qualification name is required", map[string]any{"qualifications": []any{map[string]any{"qualification_name": " "}}}},
		{"qualification date must be valid", map[string]any{"qualifications": []any{map[string]any{"qualification_name": "玉掛け", "acquired_on": "2026/01/01"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateEmployeeRelations(tc.data, workItemExists); got == "" {
				t.Fatal("invalid related array was accepted; replacement must not start")
			}
		})
	}
	if got := validateEmployeeRelations(map[string]any{"workItemIds": []any{10}, "qualifications": []any{map[string]any{"qualification_name": "玉掛け", "acquired_on": "2026-01-01"}}}, workItemExists); got != "" {
		t.Fatalf("valid related arrays were rejected: %s", got)
	}
}

func TestWorkersAndMembersRejectMalformedOrInvalidReferences(t *testing.T) {
	if got := validateWorkers(map[string]any{"workers": []any{map[string]any{"full_name": " "}}}); got == "" {
		t.Fatal("worker without name was accepted")
	}
	if got := validateWorkers(map[string]any{"workers": []any{map[string]any{"full_name": "佐藤", "phone": 123}}}); got == "" {
		t.Fatal("worker with wrong field type was accepted")
	}
	if got := validateProjectMembers(map[string]any{"members": []any{map[string]any{"employee_id": 7, "subcontractor_worker_id": 8}}}, func(uint64) bool { return true }, func(uint64) bool { return true }); got == "" {
		t.Fatal("member with two references was accepted")
	}
	if got := validateProjectMembers(map[string]any{"members": []any{map[string]any{"employee_id": 7}}}, func(uint64) bool { return false }, func(uint64) bool { return true }); got == "" {
		t.Fatal("member with deleted or missing employee reference was accepted")
	}
	if got := validateProjectMembers(map[string]any{"members": []any{map[string]any{"subcontractor_worker_id": 8}}}, func(uint64) bool { return true }, func(uint64) bool { return false }); got == "" {
		t.Fatal("member with deleted or missing subcontractor worker reference was accepted")
	}
	if got := validateProjectMembers(map[string]any{"members": []any{map[string]any{"subcontractor_worker_id": 8, "start_on": "2026-07-25"}}}, func(uint64) bool { return false }, func(uint64) bool { return true }); got != "" {
		t.Fatalf("valid member was rejected: %s", got)
	}
}

func TestNoOpUpdateIsNotTreatedAsMissing(t *testing.T) {
	called := false
	if updateTargetMissing(0, func() bool { called = true; return true }) {
		t.Fatal("existing row with no changed fields was treated as missing")
	}
	if !called {
		t.Fatal("existence was not checked for a no-op update")
	}
	if updateTargetMissing(0, func() bool { return false }) == false {
		t.Fatal("missing row was not reported")
	}
	called = false
	if updateTargetMissing(1, func() bool { called = true; return false }) {
		t.Fatal("changed row was treated as missing")
	}
	if called {
		t.Fatal("existence was queried even though rows were updated")
	}
}

func TestSessionAuthorizationAndSensitiveMasking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sessions := NewSessionManager("test-secret", false)
	router := gin.New()
	validator := sessionValidatorFunc(func(_ context.Context, claims sessionClaims) (bool, error) {
		// IDs 2 and 3 represent cookies issued before account and employee
		// logical deletion respectively.
		return claims.AccountID == 1, nil
	})
	router.GET("/secured", requireSession(sessions, validator), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/secured", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated access = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	token, _, err := sessions.IssueUser(domain.AuthenticatedUser{ID: 1, WorkerType: "employee", WorkerID: 1, SessionVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secured", nil)
	req.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: token})
	router.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authenticated access = %d, want %d", authorized.Code, http.StatusNoContent)
	}
	for _, accountID := range []uint64{2, 3} {
		token, _, err := sessions.IssueUser(domain.AuthenticatedUser{ID: accountID, WorkerType: "employee", WorkerID: accountID, SessionVersion: 1})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secured", nil)
		req.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: token})
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("deleted account/employee cookie %d access = %d, want %d", accountID, response.Code, http.StatusUnauthorized)
		}
	}

	row := map[string]any{"full_name": "田中", "employment_insurance_no": "A-123", "pension_no": "P-123", "insurance_card_no": "I-123", "my_number_encrypted": []byte("secret")}
	maskSensitiveEmployeeFields(row)
	for _, key := range []string{"employment_insurance_no", "pension_no", "insurance_card_no", "my_number_encrypted"} {
		if row[key] != "••••••" {
			t.Fatalf("%s was not masked: %#v", key, row[key])
		}
	}
}

func TestLogicalDeleteAlwaysScopesToActiveRecord(t *testing.T) {
	if activeRecordCondition != "id = ? AND deleted_at IS NULL" {
		t.Fatalf("logical-delete scope changed unexpectedly: %q", activeRecordCondition)
	}
}
