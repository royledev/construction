package httpdelivery

import (
	"bytes"
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

// Opt-in real MySQL regression coverage for T5's ownership and optimistic
// locking paths. It cleans up only rows created with this test's unique suffix.
func TestWorkTrackingHTTPPersistenceRegression(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set; skipping MySQL HTTP integration test")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("t5%010x", time.Now().UnixNano()&0xffffffffff)
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
	ownerEmployee := create("employees", map[string]any{"employee_code": suffix + "e", "full_name": "T5本人"}, "employee_code", suffix+"e")
	otherEmployee := create("employees", map[string]any{"employee_code": suffix + "o", "full_name": "T5他人"}, "employee_code", suffix+"o")
	ownerAccount := create("accounts", map[string]any{"employee_id": ownerEmployee, "login_id": suffix + "a", "password_hash": "x", "role": "user"}, "login_id", suffix+"a")
	otherAccount := create("accounts", map[string]any{"employee_id": otherEmployee, "login_id": suffix + "b", "password_hash": "x", "role": "user"}, "login_id", suffix+"b")
	itemID := create("master_items", map[string]any{"item_type": "schedule", "item_code": suffix, "name": "T5工程"}, "item_code", suffix)
	t.Cleanup(func() {
		if err := db.Table("schedules").Where("employee_id IN ?", []uint64{ownerEmployee, otherEmployee}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup schedules: %v", err)
		}
		var reportIDs []uint64
		if err := db.Table("daily_reports").Where("employee_id IN ?", []uint64{ownerEmployee, otherEmployee}).Pluck("id", &reportIDs).Error; err != nil {
			t.Errorf("load daily reports for cleanup: %v", err)
		}
		if len(reportIDs) > 0 {
			if err := db.Table("daily_reports").Where("parent_report_id IN ?", reportIDs).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup daily report children: %v", err)
			}
		}
		if err := db.Table("daily_reports").Where("employee_id IN ?", []uint64{ownerEmployee, otherEmployee}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup daily reports: %v", err)
		}
		if err := db.Table("master_items").Where("id = ?", itemID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup master item: %v", err)
		}
		if err := db.Table("accounts").Where("id IN ?", []uint64{ownerAccount, otherAccount}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup accounts: %v", err)
		}
		if err := db.Table("employees").Where("id IN ?", []uint64{ownerEmployee, otherEmployee}).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup employees: %v", err)
		}
		for _, row := range []struct {
			table  string
			column string
			ids    []uint64
		}{
			{"schedules", "employee_id", []uint64{ownerEmployee, otherEmployee}},
			{"daily_reports", "employee_id", []uint64{ownerEmployee, otherEmployee}},
			{"master_items", "id", []uint64{itemID}},
			{"accounts", "id", []uint64{ownerAccount, otherAccount}},
			{"employees", "id", []uint64{ownerEmployee, otherEmployee}},
		} {
			var remaining int64
			if err := db.Table(row.table).Where(row.column+" IN ?", row.ids).Count(&remaining).Error; err != nil {
				t.Errorf("verify cleanup %s: %v", row.table, err)
			} else if remaining != 0 {
				t.Errorf("cleanup left %d %s rows", remaining, row.table)
			}
		}
	})
	gin.SetMode(gin.TestMode)
	sessions := NewSessionManager("work-tracking-test", false)
	router := gin.New()
	api := router.Group("/api")
	handler := NewMasterHandler(db)
	handler.Register(api, sessions, NewSessionValidator(db))
	request := func(method, path string, account uint64, payload any) *httptest.ResponseRecorder {
		var raw []byte
		if payload != nil {
			raw, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		workerID := ownerEmployee
		if account == otherAccount {
			workerID = otherEmployee
		}
		token, _, issueErr := sessions.IssueUser(domain.AuthenticatedUser{ID: account, WorkerType: "employee", WorkerID: workerID, SessionVersion: 1})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		req.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if got := request(http.MethodGet, "/api/daily-reports/my?month=2026-07", ownerAccount, nil); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"items"`)) {
		t.Fatalf("my month = %d %s", got.Code, got.Body.String())
	}
	// Two simultaneous initializations must still yield exactly one seeded parent
	// per calendar day.
	month, _ := time.Parse("2006-01", "2026-08")
	errs := make(chan error, 2)
	go func() { errs <- handler.ensureMyMonth(workerSubject{Type: "employee", ID: ownerEmployee}, month) }()
	go func() { errs <- handler.ensureMyMonth(workerSubject{Type: "employee", ID: ownerEmployee}, month) }()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent month initialization: %v", err)
		}
	}
	var seededCount int64
	if err := db.Table("daily_reports").Where("employee_id = ? AND report_date >= '2026-08-01' AND report_date < '2026-09-01' AND is_month_seeded_parent = 1", ownerEmployee).Count(&seededCount).Error; err != nil || seededCount != 31 {
		t.Fatalf("seeded parents = %d, err=%v", seededCount, err)
	}
	var report struct {
		ID       uint64 `gorm:"column:id"`
		Revision uint64 `gorm:"column:revision"`
	}
	if err := db.Table("daily_reports").Select("id, revision").Where("employee_id = ? AND report_date = ? AND parent_report_id IS NULL", ownerEmployee, "2026-07-01").Take(&report).Error; err != nil {
		t.Fatal(err)
	}
	patch := map[string]any{"started_at": "09:00", "ended_at": "17:01", "work_description": "配管", "revision": report.Revision}
	if got := request(http.MethodPatch, fmt.Sprintf("/api/daily-reports/%d", report.ID), ownerAccount, patch); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"break_minutes":60`)) {
		t.Fatalf("owner patch = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPatch, fmt.Sprintf("/api/daily-reports/%d", report.ID), otherAccount, patch); got.Code != http.StatusNotFound {
		t.Fatalf("other daily patch = %d", got.Code)
	}
	if got := request(http.MethodPatch, fmt.Sprintf("/api/daily-reports/%d", report.ID), ownerAccount, patch); got.Code != http.StatusConflict {
		t.Fatalf("stale daily patch = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/children", report.ID), ownerAccount, nil); got.Code != http.StatusCreated {
		t.Fatalf("child = %d %s", got.Code, got.Body.String())
	}
	duplicateResponse := request(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/duplicate", report.ID), ownerAccount, nil)
	if duplicateResponse.Code != http.StatusCreated {
		t.Fatalf("duplicate parent = %d %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	var duplicateBody struct {
		Item struct {
			ID             uint64  `json:"id"`
			ParentReportID *uint64 `json:"parent_report_id"`
		} `json:"item"`
	}
	if err := json.Unmarshal(duplicateResponse.Body.Bytes(), &duplicateBody); err != nil {
		t.Fatalf("decode duplicate parent: %v", err)
	}
	if duplicateBody.Item.ID == 0 || duplicateBody.Item.ParentReportID != nil {
		t.Fatalf("duplicate must create a new parent: %#v", duplicateBody.Item)
	}
	schedule := map[string]any{"employee_id": ownerEmployee, "schedule_date": "2026-07-01", "slot": 1, "master_item_id": itemID, "revision": 0}
	if got := request(http.MethodPatch, "/api/schedules/cell", ownerAccount, schedule); got.Code != http.StatusCreated || !bytes.Contains(got.Body.Bytes(), []byte(`"master_item_name":"T5工程"`)) {
		t.Fatalf("schedule create = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPatch, "/api/schedules/cell", ownerAccount, map[string]any{"employee_id": ownerEmployee, "schedule_date": "2026-07-01", "slot": 2, "master_item_id": 0, "revision": 0}); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("zero master id = %d", got.Code)
	}
	// Concurrent creation of a vacant cell must be success/conflict, never a
	// duplicate-key 500 response.
	concurrent := map[string]any{"employee_id": ownerEmployee, "schedule_date": "2026-07-01", "slot": 2, "master_item_id": itemID, "revision": 0}
	codes := make(chan int, 2)
	go func() { codes <- request(http.MethodPatch, "/api/schedules/cell", ownerAccount, concurrent).Code }()
	go func() { codes <- request(http.MethodPatch, "/api/schedules/cell", ownerAccount, concurrent).Code }()
	first, second := <-codes, <-codes
	if !((first == http.StatusCreated && second == http.StatusConflict) || (second == http.StatusCreated && first == http.StatusConflict)) {
		t.Fatalf("concurrent schedule create = %d,%d", first, second)
	}
	schedule["employee_id"] = otherEmployee
	if got := request(http.MethodPatch, "/api/schedules/cell", ownerAccount, schedule); got.Code != http.StatusForbidden {
		t.Fatalf("other schedule edit = %d", got.Code)
	}
}

// TestWorkTrackingConcurrentMutationsRegression exercises MySQL row locks via
// the HTTP handlers. The assertions intentionally cover the final persisted
// state, not merely a preferred ordering of concurrent requests.
func TestWorkTrackingConcurrentMutationsRegression(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set; skipping MySQL concurrency integration test")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("tc%010x", time.Now().UnixNano()&0xffffffffff)
	if err := db.Table("employees").Create(map[string]any{"employee_code": suffix + "e", "status": "active", "full_name": "並行テスト従業員"}).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	var employeeID uint64
	if err := db.Table("employees").Select("id").Where("employee_code = ?", suffix+"e").Take(&employeeID).Error; err != nil {
		t.Fatalf("load employee: %v", err)
	}
	if err := db.Table("accounts").Create(map[string]any{"employee_id": employeeID, "login_id": suffix + "a", "password_hash": "x", "role": "admin"}).Error; err != nil {
		t.Fatalf("create admin account: %v", err)
	}
	var accountID uint64
	if err := db.Table("accounts").Select("id").Where("login_id = ?", suffix+"a").Take(&accountID).Error; err != nil {
		t.Fatalf("load admin account: %v", err)
	}
	var createdReportIDs []uint64
	var createdSlots []uint64
	t.Cleanup(func() {
		var reportIDs []uint64
		if err := db.Table("daily_reports").Where("employee_id = ?", employeeID).Pluck("id", &reportIDs).Error; err != nil {
			t.Errorf("load daily reports for cleanup: %v", err)
		}
		if len(reportIDs) > 0 {
			if err := db.Table("daily_reports").Where("parent_report_id IN ?", reportIDs).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup daily report children: %v", err)
			}
		}
		if err := db.Table("daily_reports").Where("employee_id = ?", employeeID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup daily reports: %v", err)
		}
		if len(createdSlots) > 0 {
			if err := db.Table("schedule_slots").Where("slot IN ?", createdSlots).Delete(map[string]any{}).Error; err != nil {
				t.Errorf("cleanup schedule slots: %v", err)
			}
		}
		if err := db.Table("accounts").Where("id = ?", accountID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup account: %v", err)
		}
		if err := db.Table("employees").Where("id = ?", employeeID).Delete(map[string]any{}).Error; err != nil {
			t.Errorf("cleanup employee: %v", err)
		}
		for _, row := range []struct {
			table  string
			column string
			value  any
		}{
			{"daily_reports", "employee_id", employeeID},
			{"accounts", "id", accountID},
			{"employees", "id", employeeID},
		} {
			var remaining int64
			if err := db.Table(row.table).Where(row.column+" = ?", row.value).Count(&remaining).Error; err != nil {
				t.Errorf("verify cleanup %s: %v", row.table, err)
			} else if remaining != 0 {
				t.Errorf("cleanup left %d %s rows", remaining, row.table)
			}
		}
		if len(createdSlots) > 0 {
			var remaining int64
			if err := db.Table("schedule_slots").Where("slot IN ?", createdSlots).Count(&remaining).Error; err != nil {
				t.Errorf("verify cleanup schedule slots: %v", err)
			} else if remaining != 0 {
				t.Errorf("cleanup left %d schedule slots", remaining)
			}
		}
		_ = createdReportIDs // Records are found by employee_id to include raced children.
	})

	gin.SetMode(gin.TestMode)
	sessions := NewSessionManager("work-tracking-concurrency-test", false)
	router := gin.New()
	api := router.Group("/api")
	handler := NewMasterHandler(db)
	handler.Register(api, sessions, NewSessionValidator(db))
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		token, _, issueErr := sessions.IssueUser(domain.AuthenticatedUser{ID: accountID, WorkerType: "employee", WorkerID: employeeID, SessionVersion: 1})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: sessions.CookieName(), Value: token})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	createParent := func(date, label string) uint64 {
		t.Helper()
		marker := suffix + "-" + label
		if err := db.Table("daily_reports").Create(map[string]any{"employee_id": employeeID, "report_date": date, "work_description": marker, "revision": 1}).Error; err != nil {
			t.Fatalf("create daily report %s: %v", label, err)
		}
		var id uint64
		if err := db.Table("daily_reports").Select("id").Where("employee_id = ? AND work_description = ?", employeeID, marker).Take(&id).Error; err != nil {
			t.Fatalf("load daily report %s: %v", label, err)
		}
		createdReportIDs = append(createdReportIDs, id)
		return id
	}
	concurrent := func(left, right func() int) (int, int) {
		start := make(chan struct{})
		type result struct {
			left bool
			code int
		}
		codes := make(chan result, 2)
		go func() { <-start; codes <- result{left: true, code: left()} }()
		go func() { <-start; codes <- result{code: right()} }()
		close(start)
		first, second := <-codes, <-codes
		if first.left {
			return first.code, second.code
		}
		return second.code, first.code
	}

	deleteDate := "2031-01-03"
	firstDeleteID := createParent(deleteDate, "delete-a")
	secondDeleteID := createParent(deleteDate, "delete-b")
	firstStatus, secondStatus := concurrent(
		func() int {
			return request(http.MethodDelete, fmt.Sprintf("/api/daily-reports/%d", firstDeleteID)).Code
		},
		func() int {
			return request(http.MethodDelete, fmt.Sprintf("/api/daily-reports/%d", secondDeleteID)).Code
		},
	)
	successes := 0
	for _, status := range []int{firstStatus, secondStatus} {
		if status == http.StatusNoContent {
			successes++
		} else if status != http.StatusConflict {
			t.Fatalf("concurrent daily deletes returned %d and %d, want 204/409", firstStatus, secondStatus)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent daily deletes succeeded %d times, want exactly one", successes)
	}
	var active int64
	if err := db.Table("daily_reports").Where("id IN ? AND deleted_at IS NULL", []uint64{firstDeleteID, secondDeleteID}).Count(&active).Error; err != nil || active < 1 {
		t.Fatalf("concurrent daily deletes left active=%d err=%v, want at least one", active, err)
	}

	runParentChildRace := func(date, endpoint, label string) {
		t.Helper()
		parentID := createParent(date, label+"-parent")
		createParent(date, label+"-sibling") // permits the parent delete to proceed.
		deleteStatus, childStatus := concurrent(
			func() int { return request(http.MethodDelete, fmt.Sprintf("/api/daily-reports/%d", parentID)).Code },
			func() int {
				return request(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/%s", parentID, endpoint)).Code
			},
		)
		if deleteStatus != http.StatusNoContent && deleteStatus != http.StatusConflict {
			t.Fatalf("%s delete status = %d, want 204/409", label, deleteStatus)
		}
		if childStatus != http.StatusCreated && childStatus != http.StatusNotFound {
			t.Fatalf("%s child status = %d, want 201/404", label, childStatus)
		}
		var orphaned int64
		if err := db.Table("daily_reports child").Joins("JOIN daily_reports parent ON parent.id = child.parent_report_id").Where("child.deleted_at IS NULL AND parent.deleted_at IS NOT NULL AND child.employee_id = ?", employeeID).Count(&orphaned).Error; err != nil || orphaned != 0 {
			t.Fatalf("%s left active child of deleted parent: count=%d err=%v", label, orphaned, err)
		}
	}
	runParentChildRace("2031-01-04", "children", "add-child")
	runParentChildRace("2031-01-05", "duplicate", "duplicate")

	// Different parent rows on the same day take their target locks in opposite
	// orders before both request the day set. The handler must retry the MySQL
	// deadlock victim, so both additions complete without a 500.
	firstParentID := createParent("2031-01-06", "different-parent-a")
	secondParentID := createParent("2031-01-06", "different-parent-b")
	firstAdd, secondAdd := concurrent(
		func() int {
			return request(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/children", firstParentID)).Code
		},
		func() int {
			return request(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/children", secondParentID)).Code
		},
	)
	if firstAdd != http.StatusCreated || secondAdd != http.StatusCreated {
		t.Fatalf("same-day different-parent additions = %d,%d, want both 201", firstAdd, secondAdd)
	}
	for _, parentID := range []uint64{firstParentID, secondParentID} {
		var children int64
		if err := db.Table("daily_reports").Where("parent_report_id = ? AND deleted_at IS NULL", parentID).Count(&children).Error; err != nil || children != 1 {
			t.Fatalf("same-day parent %d child count=%d err=%v, want one", parentID, children, err)
		}
	}

	var beforeMax uint64
	if err := db.Table("schedule_slots").Select("COALESCE(MAX(slot), 0)").Scan(&beforeMax).Error; err != nil {
		t.Fatalf("read schedule slot maximum: %v", err)
	}
	type slotResponse struct {
		Code int
		Slot uint64
	}
	start := make(chan struct{})
	responses := make(chan slotResponse, 2)
	for range 2 {
		go func() {
			<-start
			response := request(http.MethodPost, "/api/schedule-slots")
			var body struct {
				Item struct {
					Slot uint64 `json:"slot"`
				} `json:"item"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				responses <- slotResponse{Code: response.Code}
				return
			}
			responses <- slotResponse{Code: response.Code, Slot: body.Item.Slot}
		}()
	}
	close(start)
	firstSlot, secondSlot := <-responses, <-responses
	if firstSlot.Code != http.StatusCreated || secondSlot.Code != http.StatusCreated || firstSlot.Slot == 0 || secondSlot.Slot == 0 || firstSlot.Slot == secondSlot.Slot {
		t.Fatalf("concurrent schedule slot creation = %#v %#v, want two distinct 201 slots", firstSlot, secondSlot)
	}
	createdSlots = []uint64{firstSlot.Slot, secondSlot.Slot}
	if firstSlot.Slot <= beforeMax || secondSlot.Slot <= beforeMax {
		t.Fatalf("concurrent schedule slots %d,%d did not advance from %d", firstSlot.Slot, secondSlot.Slot, beforeMax)
	}
	var createdSlotCount int64
	if err := db.Table("schedule_slots").Where("slot IN ?", createdSlots).Count(&createdSlotCount).Error; err != nil || createdSlotCount != 2 {
		t.Fatalf("concurrent schedule slots persisted=%d err=%v, want two", createdSlotCount, err)
	}
}
