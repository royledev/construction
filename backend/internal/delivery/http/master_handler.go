package httpdelivery

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// MasterHandler owns the T3 ledger resources. The database schema is the source of
// truth; explicit field lists keep credentials and sensitive values out of responses.
type MasterHandler struct {
	db                *gorm.DB
	pdfRenderer       DocumentPDFRenderer
	pdfSlots          chan struct{}
	pdfDocumentLoader func(context.Context, documentKind, string) (map[string]any, error)
	pdfAccountMu      sync.Mutex
	pdfAccounts       map[uint64]struct{}
	pdfWorkTimeout    time.Duration
}

// NewMasterHandler accepts an optional PDF renderer so tests can use a
// deterministic fake. Production injects the Gotenberg client from cmd/api.
func NewMasterHandler(db *gorm.DB, renderers ...DocumentPDFRenderer) *MasterHandler {
	renderer := DocumentPDFRenderer(unavailableDocumentPDFRenderer{})
	if len(renderers) > 0 && renderers[0] != nil {
		renderer = renderers[0]
	}
	maxConcurrency := 2
	if provider, ok := renderer.(interface{ pdfMaxConcurrency() int }); ok {
		maxConcurrency = provider.pdfMaxConcurrency()
	}
	return &MasterHandler{
		db: db, pdfRenderer: renderer, pdfSlots: make(chan struct{}, maxConcurrency),
		pdfAccounts: make(map[uint64]struct{}), pdfWorkTimeout: DocumentPDFWorkTimeout,
	}
}

func (h *MasterHandler) validate(c *gin.Context, resource string, data map[string]any) bool {
	need := func(key, label string) bool {
		value, exists := data[key]
		return exists && strings.TrimSpace(strings.TrimSpace(toString(value))) != ""
	}
	fail := func(message string) bool {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": message})
		return false
	}
	switch resource {
	case "master_items":
		if !need("name", "項目名") {
			return fail("項目名は必須です。")
		}
		if !oneOf(toString(data["item_type"]), "work", "estimate_invoice", "unit_price", "schedule") {
			return fail("項目種別が不正です。")
		}
	case "employees":
		if !need("full_name", "氏名") {
			return fail("氏名は必須です。")
		}
		if value, exists := data["status"]; exists && !oneOf(toString(value), "active", "retired") {
			return fail("従業員ステータスが不正です。")
		}
		if message := h.validateEmployeeRelations(data); message != "" {
			return fail(message)
		}
	case "clients":
		if !need("name", "得意先名") {
			return fail("得意先名は必須です。")
		}
		if value, exists := data["status"]; exists && !oneOf(toString(value), "active", "inactive") {
			return fail("得意先ステータスが不正です。")
		}
		if value, exists := data["closing_day"]; exists && (asUint64(value) < 1 || asUint64(value) > 31) {
			return fail("請求締め日は1〜31で入力してください。")
		}
	case "subcontractors":
		if !need("name", "外注先名") {
			return fail("外注先名は必須です。")
		}
		if value, exists := data["status"]; exists && !oneOf(toString(value), "active", "inactive") {
			return fail("外注先ステータスが不正です。")
		}
		if message := validateWorkers(data); message != "" {
			return fail(message)
		}
	case "projects":
		clientID, clientOK := strictPositiveUint(data["client_id"])
		if !need("site_name", "現場名") || !clientOK {
			return fail("現場名、得意先は必須です。")
		}
		data["client_id"] = clientID
		if value, exists := data["status"]; exists && !oneOf(toString(value), "unreceived", "ordered", "completed", "lost") {
			return fail("案件ステータスが不正です。")
		}
		var count int64
		activeClient := h.db.Table("clients").Where("id = ? AND deleted_at IS NULL AND status = 'active'", clientID).Count(&count).Error == nil && count == 1
		if !activeClient && !h.isCurrentProjectClient(c.Param("id"), clientID) {
			return fail("有効な得意先を指定してください。")
		}
		manager, managerNull, managerOK := strictNullablePositiveUint(data["manager_employee_id"])
		if raw, exists := data["manager_employee_id"]; exists && raw != nil && !managerOK {
			return fail("現場担当従業員の指定が不正です。")
		} else if exists && !managerNull {
			data["manager_employee_id"] = manager
			var employeeCount int64
			activeManager := h.db.Table("employees").Where("id = ? AND deleted_at IS NULL AND status = 'active'", manager).Count(&employeeCount).Error == nil && employeeCount == 1
			if !activeManager && !h.isCurrentProjectManager(c.Param("id"), manager) {
				return fail("有効な現場担当従業員を指定してください。")
			}
		} else if exists {
			data["manager_employee_id"] = nil
		}
		if message := h.validateProjectMembers(data); message != "" {
			return fail(message)
		}
	}
	return true
}
func toString(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
func requiredString(row map[string]any, key string) bool {
	value, ok := row[key]
	return ok && strings.TrimSpace(toString(value)) != ""
}

func optionalString(row map[string]any, key string) bool {
	value, present := row[key]
	return !present || value == nil || func() bool { _, ok := value.(string); return ok }()
}

func optionalDate(row map[string]any, key string) bool {
	value, present := row[key]
	if !present || value == nil || toString(value) == "" {
		return !present || value == nil || func() bool { _, ok := value.(string); return ok }()
	}
	_, err := time.Parse("2006-01-02", toString(value))
	return err == nil
}

func arrayValue(data map[string]any, key string) ([]any, bool, bool) {
	value, present := data[key]
	if !present {
		return nil, false, true
	}
	rows, ok := value.([]any)
	return rows, true, ok
}

// positiveID accepts JSON numbers and the string IDs emitted by the current UI,
// but rejects zero, fractions, and every value that cannot name a database row.
func positiveID(value any) (uint64, bool) {
	switch v := value.(type) {
	case float64:
		if v <= 0 || math.Trunc(v) != v || v > math.MaxUint64 {
			return 0, false
		}
		return uint64(v), true
	case uint64:
		return v, v > 0
	case int:
		return uint64(v), v > 0
	case string:
		if strings.TrimSpace(v) == "" || strings.TrimSpace(v) != v {
			return 0, false
		}
		id, err := strconv.ParseUint(v, 10, 64)
		return id, err == nil && id > 0
	default:
		return 0, false
	}
}

func optionalPositiveID(row map[string]any, key string) (uint64, bool, bool) {
	value, present := row[key]
	if !present || value == nil {
		return 0, false, true
	}
	if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
		return 0, false, true
	}
	id, ok := positiveID(value)
	return id, true, ok
}

func (h *MasterHandler) validateEmployeeRelations(data map[string]any) string {
	return validateEmployeeRelations(data, func(id uint64) bool {
		var count int64
		return h.db.Table("master_items").Where("id = ? AND item_type = ? AND deleted_at IS NULL", id, "work").Count(&count).Error == nil && count == 1
	})
}

func validateEmployeeRelations(data map[string]any, workItemExists func(uint64) bool) string {
	if ids, present, typed := arrayValue(data, "workItemIds"); present {
		if !typed {
			return "作業項目は配列で指定してください。"
		}
		seen := map[uint64]bool{}
		for _, raw := range ids {
			id, ok := positiveID(raw)
			if !ok || seen[id] {
				return "作業項目の指定が不正です。"
			}
			seen[id] = true
			if !workItemExists(id) {
				return "有効な作業項目を指定してください。"
			}
		}
	}
	if qualifications, present, typed := arrayValue(data, "qualifications"); present {
		if !typed {
			return "資格情報は配列で指定してください。"
		}
		for _, raw := range qualifications {
			row, ok := raw.(map[string]any)
			if !ok || !requiredString(row, "qualification_name") || !optionalDate(row, "acquired_on") || !optionalDate(row, "renewed_on") || !optionalString(row, "notes") {
				return "資格情報の入力内容が不正です。"
			}
			if _, present, valid := optionalPositiveID(row, "id"); present && !valid {
				return "資格情報のIDが不正です。"
			}
		}
	}
	return ""
}

func validateWorkers(data map[string]any) string {
	workers, present, typed := arrayValue(data, "workers")
	if !present {
		return ""
	}
	if !typed {
		return "外注先従業員は配列で指定してください。"
	}
	for _, raw := range workers {
		worker, ok := raw.(map[string]any)
		if !ok || !requiredString(worker, "full_name") || !optionalString(worker, "work_description") || !optionalString(worker, "phone") || !optionalString(worker, "notes") {
			return "外注先従業員の入力内容が不正です。"
		}
		if _, present, valid := optionalPositiveID(worker, "id"); present && !valid {
			return "外注先従業員のIDが不正です。"
		}
	}
	return ""
}

func (h *MasterHandler) validateProjectMembers(data map[string]any) string {
	return validateProjectMembers(data, func(id uint64) bool {
		var count int64
		return h.db.Table("employees").Where("id = ? AND deleted_at IS NULL", id).Count(&count).Error == nil && count == 1
	}, func(id uint64) bool {
		var count int64
		return h.db.Table("subcontractor_workers sw").Joins("JOIN subcontractors s ON s.id = sw.subcontractor_id AND s.deleted_at IS NULL").Where("sw.id = ? AND sw.deleted_at IS NULL", id).Count(&count).Error == nil && count == 1
	})
}

func validateProjectMembers(data map[string]any, employeeExists, workerExists func(uint64) bool) string {
	members, present, typed := arrayValue(data, "members")
	if !present {
		return ""
	}
	if !typed {
		return "案件担当員は配列で指定してください。"
	}
	for _, raw := range members {
		member, ok := raw.(map[string]any)
		if !ok || !optionalString(member, "work_description") || !optionalString(member, "phone") || !optionalString(member, "notes") || !optionalDate(member, "start_on") || !optionalDate(member, "end_on") {
			return "案件担当員の入力内容が不正です。"
		}
		employeeID, employeeProvided, employeeValid := optionalPositiveID(member, "employee_id")
		workerID, workerProvided, workerValid := optionalPositiveID(member, "subcontractor_worker_id")
		if !employeeValid || !workerValid || employeeProvided == workerProvided { // exactly one is required; neither and both are invalid.
			return "案件担当員には自社従業員または外注作業員を1名指定してください。"
		}
		if employeeProvided {
			if !employeeExists(employeeID) {
				return "有効な自社従業員を指定してください。"
			}
		} else if !workerExists(workerID) {
			return "有効な外注作業員を指定してください。"
		}
	}
	return ""
}

func (h *MasterHandler) Register(api *gin.RouterGroup, sessions *SessionManager, validator SessionValidator) {
	secured := api.Group("", requireSession(sessions, validator))
	secured.GET("/master-items", h.listMasterItems)
	secured.GET("/master-items/:id", h.getMasterItem)
	secured.POST("/master-items", h.requireAdmin, h.createMasterItem)
	secured.PATCH("/master-items/:id", h.requireAdmin, h.updateMasterItem)
	secured.DELETE("/master-items/:id", h.requireAdmin, h.delete("master_items"))

	secured.GET("/employees", h.listEmployees)
	secured.GET("/employees/:id", h.getEmployee)
	secured.POST("/employees", h.requireAdmin, h.createEmployee)
	secured.PATCH("/employees/:id", h.requireAdmin, h.updateEmployee)
	secured.DELETE("/employees/:id", h.requireAdmin, h.delete("employees"))
	secured.DELETE("/employees/:id/qualifications/:qualificationID", h.requireAdmin, h.deleteQualification)
	secured.GET("/accounts", h.requireAdmin, h.listAccounts)
	secured.GET("/account-worker-choices", h.requireAdmin, h.accountWorkerChoices)
	secured.GET("/accounts/:id", h.requireAdmin, h.getAccount)
	secured.POST("/accounts", h.requireAdmin, h.createAccount)
	secured.PATCH("/accounts/:id", h.requireAdmin, h.updateAccount)
	secured.DELETE("/accounts/:id", h.requireAdmin, h.delete("accounts"))

	secured.GET("/clients", h.list("clients", clientColumns))
	secured.GET("/selectors", h.selectors)
	secured.GET("/clients/:id", h.get("clients", clientColumns))
	secured.POST("/clients", h.requireAdmin, h.create("clients", clientColumns))
	secured.PATCH("/clients/:id", h.requireAdmin, h.update("clients", clientColumns))
	secured.DELETE("/clients/:id", h.requireAdmin, h.delete("clients"))

	secured.GET("/subcontractors", h.list("subcontractors", subcontractorColumns))
	secured.GET("/subcontractors/:id", h.getSubcontractor)
	secured.POST("/subcontractors", h.requireAdmin, h.createSubcontractor)
	secured.PATCH("/subcontractors/:id", h.requireAdmin, h.updateSubcontractor)
	secured.DELETE("/subcontractors/:id", h.requireAdmin, h.delete("subcontractors"))
	secured.DELETE("/subcontractors/:id/workers/:workerID", h.requireAdmin, h.deleteSubcontractorWorker)

	secured.GET("/projects", h.listProjects)
	secured.GET("/projects/:id", h.getProject)
	secured.POST("/projects", h.requireAdmin, h.createProject)
	secured.PATCH("/projects/:id", h.requireAdmin, h.updateProject)
	secured.DELETE("/projects/:id", h.requireAdmin, h.delete("projects"))

	// Commercial documents are deliberately kept separate from the generic
	// master CRUD. Their totals are derived server-side from their lines.
	secured.GET("/estimates", h.listDocuments("estimate"))
	secured.GET("/estimates/:id", h.getDocument("estimate"))
	secured.GET("/estimates/:id/pdf", h.documentPDF("estimate"))
	secured.POST("/estimates", h.requireAdmin, h.saveDocument("estimate", 0))
	secured.PATCH("/estimates/:id", h.requireAdmin, h.updateDocument("estimate"))
	secured.DELETE("/estimates/:id", h.requireAdmin, h.delete("estimates"))

	secured.GET("/invoices", h.listDocuments("invoice"))
	secured.GET("/invoices/:id", h.getDocument("invoice"))
	secured.GET("/invoices/:id/pdf", h.documentPDF("invoice"))
	secured.POST("/invoices", h.requireAdmin, h.saveDocument("invoice", 0))
	secured.PATCH("/invoices/:id", h.requireAdmin, h.updateDocument("invoice"))
	secured.DELETE("/invoices/:id", h.requireAdmin, h.delete("invoices"))

	h.registerWorkTracking(secured)
}

var masterColumns = []string{"item_type", "item_code", "name", "notes"}
var clientColumns = []string{"client_code", "status", "name", "honorific", "kana", "phone", "fax", "email", "postal_code", "address", "contact_name", "contact_phone", "registration_number", "closing_day", "payment_due_rule", "payment_method", "payment_site_days", "bank_name", "bank_branch", "bank_account_no", "notes"}
var subcontractorColumns = []string{"subcontractor_code", "status", "name", "honorific", "kana", "phone", "fax", "email", "postal_code", "address", "contact_name", "contact_phone", "registration_number", "payment_method", "payment_site_days", "bank_name", "bank_branch", "bank_account_no", "notes"}
var projectColumns = []string{"project_code", "site_name", "site_address", "client_id", "manager_employee_id", "ordered_on", "construction_start_on", "construction_end_on", "completed_on", "lost_on", "status", "notes"}
var employeeColumns = []string{"employee_code", "status", "full_name", "full_name_kana", "occupation", "birth_date", "hired_on", "retired_on", "employment_insurance_no", "pension_no", "insurance_card_no", "my_number_encrypted", "postal_code", "address", "phone"}

func (h *MasterHandler) accountItem(id any) (map[string]any, error) {
	row := map[string]any{}
	err := h.db.Table("accounts a").Select("a.id, a.employee_id, a.subcontractor_worker_id, a.login_id, a.role, a.created_at, a.updated_at, CASE WHEN a.employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END AS worker_type, COALESCE(e.full_name, sw.full_name) AS worker_name").Joins("LEFT JOIN employees e ON e.id=a.employee_id").Joins("LEFT JOIN subcontractor_workers sw ON sw.id=a.subcontractor_worker_id").Where("a.id=? AND a.deleted_at IS NULL", id).Take(&row).Error
	return row, err
}
func (h *MasterHandler) listAccounts(c *gin.Context) {
	var rows []map[string]any
	if err := h.db.Table("accounts a").Select("a.id, a.employee_id, a.subcontractor_worker_id, a.login_id, a.role, CASE WHEN a.employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END AS worker_type, COALESCE(e.full_name, sw.full_name) AS worker_name").Joins("LEFT JOIN employees e ON e.id=a.employee_id").Joins("LEFT JOIN subcontractor_workers sw ON sw.id=a.subcontractor_worker_id").Where("a.deleted_at IS NULL").Order("a.login_id").Find(&rows).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
func (h *MasterHandler) accountWorkerChoices(c *gin.Context) {
	var employees, workers []map[string]any
	// Include inactive, non-deleted rows so an existing account can be edited
	// without its current subject disappearing from the Select. The UI disables
	// them for new assignments, and accountValues enforces that on write.
	if err := h.db.Table("employees").Select("id, employee_code, full_name, status").Where("deleted_at IS NULL").Order("employee_code").Find(&employees).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("subcontractor_workers sw").Select("sw.id, sw.full_name, sc.subcontractor_code, sc.name AS subcontractor_name, sc.status AS subcontractor_status").Joins("JOIN subcontractors sc ON sc.id=sw.subcontractor_id AND sc.deleted_at IS NULL").Where("sw.deleted_at IS NULL").Order("sc.subcontractor_code, sw.full_name").Find(&workers).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"employees": employees, "subcontractor_workers": workers})
}

// selectors exposes names and public codes only; no account credentials or
// employee sensitive fields are ever included in the dropdown payload.
func (h *MasterHandler) selectors(c *gin.Context) {
	var clients, employees, subcontractorWorkers, projects []map[string]any
	if err := h.db.Table("clients").Select("id, client_code, name, status").Where("deleted_at IS NULL").Order("client_code").Find(&clients).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("employees").Select("id, employee_code, full_name, status").Where("deleted_at IS NULL").Order("employee_code").Find(&employees).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("subcontractor_workers sw").Select("sw.id, sw.full_name, sc.subcontractor_code, sc.name AS subcontractor_name, sc.status AS subcontractor_status").Joins("JOIN subcontractors sc ON sc.id = sw.subcontractor_id AND sc.deleted_at IS NULL").Where("sw.deleted_at IS NULL").Order("sc.subcontractor_code, sw.full_name, sw.id").Find(&subcontractorWorkers).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("projects").Select("id, project_code, site_name").Where("deleted_at IS NULL").Order("project_code DESC").Find(&projects).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"clients": clients, "employees": employees, "subcontractor_workers": subcontractorWorkers, "projects": projects})
}
func (h *MasterHandler) isCurrentAccountWorker(accountID, workerType string, workerID uint64) bool {
	if accountID == "" {
		return false
	}
	column := "employee_id"
	if workerType == "subcontractor" {
		column = "subcontractor_worker_id"
	}
	var count int64
	return h.db.Table("accounts").Where("id = ? AND deleted_at IS NULL AND "+column+" = ?", accountID, workerID).Count(&count).Error == nil && count == 1
}
func (h *MasterHandler) isCurrentProjectClient(projectID string, clientID uint64) bool {
	if projectID == "" {
		return false
	}
	var count int64
	return h.db.Table("projects p").Joins("JOIN clients c ON c.id = p.client_id AND c.deleted_at IS NULL").Where("p.id = ? AND p.deleted_at IS NULL AND p.client_id = ?", projectID, clientID).Count(&count).Error == nil && count == 1
}
func (h *MasterHandler) isCurrentProjectManager(projectID string, employeeID uint64) bool {
	if projectID == "" {
		return false
	}
	var count int64
	return h.db.Table("projects p").Joins("JOIN employees e ON e.id = p.manager_employee_id AND e.deleted_at IS NULL").Where("p.id = ? AND p.deleted_at IS NULL AND p.manager_employee_id = ?", projectID, employeeID).Count(&count).Error == nil && count == 1
}
func (h *MasterHandler) getAccount(c *gin.Context) {
	row, err := h.accountItem(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": row})
}
func (h *MasterHandler) accountValues(c *gin.Context, data map[string]any, creating bool) (map[string]any, bool) {
	login := strings.TrimSpace(toString(data["login_id"]))
	role := toString(data["role"])
	if login == "" || !oneOf(role, "admin", "user") {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "ログインIDと権限を指定してください。"})
		return nil, false
	}
	workerType := toString(data["worker_type"])
	workerID, workerIDOK := strictPositiveUint(data["worker_id"])
	if !oneOf(workerType, "employee", "subcontractor") || !workerIDOK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "作業者区分と作業者を指定してください。"})
		return nil, false
	}
	if workerType == "employee" {
		var n int64
		active := h.db.Table("employees").Where("id=? AND deleted_at IS NULL AND status = 'active'", workerID).Count(&n).Error == nil && n == 1
		if !active && !h.isCurrentAccountWorker(c.Param("id"), workerType, workerID) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "有効な自社従業員を指定してください。"})
			return nil, false
		}
	} else {
		var n int64
		active := h.db.Table("subcontractor_workers sw").Joins("JOIN subcontractors sc ON sc.id=sw.subcontractor_id AND sc.deleted_at IS NULL AND sc.status = 'active'").Where("sw.id=? AND sw.deleted_at IS NULL", workerID).Count(&n).Error == nil && n == 1
		if !active && !h.isCurrentAccountWorker(c.Param("id"), workerType, workerID) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "有効な外注作業員を指定してください。"})
			return nil, false
		}
	}
	values := map[string]any{"login_id": login, "role": role, "employee_id": nil, "subcontractor_worker_id": nil}
	if workerType == "employee" {
		values["employee_id"] = workerID
	} else {
		values["subcontractor_worker_id"] = workerID
	}
	if password, present := data["password"].(string); present && strings.TrimSpace(password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			serverError(c, err)
			return nil, false
		}
		values["password_hash"] = string(hash)
	} else if creating {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "新規アカウントにはパスワードが必要です。"})
		return nil, false
	}
	return values, true
}
func (h *MasterHandler) createAccount(c *gin.Context) {
	data, ok := body(c)
	if !ok {
		return
	}
	values, ok := h.accountValues(c, data, true)
	if !ok {
		return
	}
	if err := h.db.Table("accounts").Create(values).Error; err != nil {
		if isDuplicateKey(err) {
			if strings.Contains(duplicateKeyName(err), "uq_accounts_login_id") {
				c.JSON(http.StatusConflict, gin.H{"error": "ログインIDは既に使用されています。", "code": "login_id_conflict"})
				return
			}
			c.JSON(http.StatusConflict, gin.H{"error": "この作業者には既にアカウントが登録されています。", "code": "worker_already_has_account"})
			return
		}
		dbError(c, err)
		return
	}
	row, err := h.accountItem(insertedID(h.db, values))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": row})
}
func (h *MasterHandler) updateAccount(c *gin.Context) {
	data, ok := body(c)
	if !ok {
		return
	}
	values, ok := h.accountValues(c, data, false)
	if !ok {
		return
	}
	values["session_version"] = gorm.Expr("session_version + 1")
	result := activeRecord(h.db, "accounts", c.Param("id")).Updates(values)
	if result.Error != nil {
		if isDuplicateKey(result.Error) {
			if strings.Contains(duplicateKeyName(result.Error), "uq_accounts_login_id") {
				c.JSON(http.StatusConflict, gin.H{"error": "ログインIDは既に使用されています。", "code": "login_id_conflict"})
				return
			}
			c.JSON(http.StatusConflict, gin.H{"error": "この作業者には既にアカウントが登録されています。", "code": "worker_already_has_account"})
			return
		}
		dbError(c, result.Error)
		return
	}
	if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(h.db, "accounts", c.Param("id")) }) {
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	h.getAccount(c)
}

func (h *MasterHandler) requireAdmin(c *gin.Context) {
	var row struct{ Role string }
	if err := h.db.Table("accounts").Select("role").Where("id = ? AND deleted_at IS NULL", c.GetUint64("accountID")).Take(&row).Error; err != nil || row.Role != "admin" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "管理者権限が必要です。"})
		return
	}
	c.Next()
}

func (h *MasterHandler) list(table string, columns []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var rows []map[string]any
		if err := h.db.Table(table).Select(append([]string{"id"}, columns...)).Where("deleted_at IS NULL").Order("id DESC").Find(&rows).Error; err != nil {
			serverError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": rows})
	}
}
func (h *MasterHandler) get(table string, columns []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, ok := h.find(table, columns, c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": row})
	}
}
func (h *MasterHandler) create(table string, columns []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, ok := body(c)
		if !ok {
			return
		}
		if !h.validate(c, table, data) {
			return
		}
		values := pick(data, columns)
		if column, prefix, scope, coded := codeSpec(table, data); coded {
			_ = scope
			if err := insertWithGeneratedCode(h.db, table, column, prefix, values); err != nil {
				dbError(c, err)
				return
			}
			c.JSON(http.StatusCreated, gin.H{"item": values})
			return
		}
		if err := h.db.Table(table).Create(values).Error; err != nil {
			dbError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"item": values})
	}
}
func (h *MasterHandler) update(table string, columns []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, ok := body(c)
		if !ok {
			return
		}
		if !h.validate(c, table, data) {
			return
		}
		values := pick(data, columns)
		for _, column := range immutableCodeColumns(table) {
			delete(values, column)
		}
		if table == "master_items" {
			delete(values, "item_type")
		}
		result := activeRecord(h.db, table, c.Param("id")).Updates(values)
		if result.Error != nil {
			dbError(c, result.Error)
			return
		}
		if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(h.db, table, c.Param("id")) }) {
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
		h.get(table, columns)(c)
	}
}

func immutableCodeColumns(table string) []string {
	switch table {
	case "projects":
		return []string{"project_code"}
	case "estimates":
		return []string{"estimate_code"}
	case "invoices":
		return []string{"invoice_code"}
	case "employees":
		return []string{"employee_code"}
	case "clients":
		return []string{"client_code"}
	case "subcontractors":
		return []string{"subcontractor_code"}
	case "master_items":
		return []string{"item_code"}
	}
	return nil
}
func (h *MasterHandler) delete(table string) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := activeRecord(h.db, table, c.Param("id")).Update("deleted_at", time.Now())
		if result.Error != nil {
			serverError(c, result.Error)
			return
		}
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func (h *MasterHandler) listMasterItems(c *gin.Context) {
	query := h.db.Table("master_items").Select(append([]string{"id"}, masterColumns...)).Where("deleted_at IS NULL")
	if itemType := c.Query("type"); itemType != "" {
		query = query.Where("item_type = ?", itemType)
	}
	var rows []map[string]any
	if err := query.Order("item_type, item_code").Find(&rows).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
func (h *MasterHandler) getMasterItem(c *gin.Context)    { h.get("master_items", masterColumns)(c) }
func (h *MasterHandler) createMasterItem(c *gin.Context) { h.create("master_items", masterColumns)(c) }
func (h *MasterHandler) updateMasterItem(c *gin.Context) { h.update("master_items", masterColumns)(c) }

func (h *MasterHandler) listEmployees(c *gin.Context) {
	var rows []map[string]any
	err := h.db.Table("employees e").Select("e.id, e.employee_code, e.status, e.full_name, e.occupation, e.phone").Where("e.deleted_at IS NULL").Order("e.employee_code").Find(&rows).Error
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
func (h *MasterHandler) getEmployee(c *gin.Context) {
	row, ok := h.find("employees", employeeColumns, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	maskSensitiveEmployeeFields(row)
	var qualifications []map[string]any
	if err := h.db.Table("employee_qualifications").Where("employee_id = ? AND deleted_at IS NULL", c.Param("id")).Order("id").Find(&qualifications).Error; err != nil {
		serverError(c, err)
		return
	}
	var workItemIDs []uint64
	if err := h.db.Table("employee_work_items").Where("employee_id = ?", c.Param("id")).Pluck("master_item_id", &workItemIDs).Error; err != nil {
		serverError(c, err)
		return
	}
	row["qualifications"], row["workItemIds"] = qualifications, workItemIDs
	c.JSON(http.StatusOK, gin.H{"item": row})
}
func (h *MasterHandler) deleteQualification(c *gin.Context) {
	result := h.db.Table("employee_qualifications").Where("id = ? AND employee_id = ? AND deleted_at IS NULL", c.Param("qualificationID"), c.Param("id")).Update("deleted_at", time.Now())
	if result.Error != nil {
		serverError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "資格情報が見つかりません。"})
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *MasterHandler) createEmployee(c *gin.Context) { h.saveEmployee(c, 0) }
func (h *MasterHandler) updateEmployee(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		return
	}
	h.saveEmployee(c, id)
}
func (h *MasterHandler) saveEmployee(c *gin.Context, id uint64) {
	data, ok := body(c)
	if !ok {
		return
	}
	if !h.validate(c, "employees", data) {
		return
	}
	tx := h.db.Begin()
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	values := pick(data, employeeColumns)
	if id == 0 {
		if err := insertWithGeneratedCode(tx, "employees", "employee_code", "EMP", values); err != nil {
			tx.Rollback()
			dbError(c, err)
			return
		}
		id = insertedID(tx, values)
	} else {
		delete(values, "employee_code")
		result := tx.Table("employees").Where("id = ? AND deleted_at IS NULL", id).Updates(values)
		if result.Error != nil {
			tx.Rollback()
			dbError(c, result.Error)
			return
		}
		if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(tx, "employees", id) }) {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
	}
	if !replaceEmployeeDetails(tx, id, data) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "従業員の関連データを保存できません。"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: strconv.FormatUint(id, 10)})
	h.getEmployee(c)
}
func replaceEmployeeDetails(tx *gorm.DB, employeeID uint64, data map[string]any) bool {
	if values, present := data["workItemIds"].([]any); present {
		if err := tx.Table("employee_work_items").Where("employee_id = ?", employeeID).Delete(map[string]any{}).Error; err != nil {
			return false
		}
		for _, raw := range values {
			if asUint64(raw) > 0 && tx.Table("employee_work_items").Create(map[string]any{"employee_id": employeeID, "master_item_id": asUint64(raw)}).Error != nil {
				return false
			}
		}
	}
	if values, present := data["qualifications"].([]any); present {
		kept := map[uint64]bool{}
		for _, raw := range values {
			qualification, valid := raw.(map[string]any)
			if !valid {
				return false
			}
			fields := pick(qualification, []string{"qualification_name", "acquired_on", "renewed_on", "notes"})
			if fields["qualification_name"] == nil {
				continue
			}
			if qualificationID, present, valid := optionalPositiveID(qualification, "id"); present && valid {
				result := tx.Table("employee_qualifications").Where("id = ? AND employee_id = ? AND deleted_at IS NULL", qualificationID, employeeID).Updates(fields)
				if result.Error != nil || result.RowsAffected != 1 {
					return false
				}
				kept[qualificationID] = true
			} else {
				fields["employee_id"] = employeeID
				if tx.Table("employee_qualifications").Create(fields).Error != nil {
					return false
				}
				newID := insertedID(tx, fields)
				if newID == 0 {
					return false
				}
				kept[newID] = true
			}
		}
		var existing []uint64
		if tx.Table("employee_qualifications").Where("employee_id = ? AND deleted_at IS NULL", employeeID).Pluck("id", &existing).Error != nil {
			return false
		}
		for _, qualificationID := range existing {
			if !kept[qualificationID] && tx.Table("employee_qualifications").Where("id = ?", qualificationID).Update("deleted_at", time.Now()).Error != nil {
				return false
			}
		}
	}
	return true
}

func (h *MasterHandler) getSubcontractor(c *gin.Context) {
	row, ok := h.find("subcontractors", subcontractorColumns, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	var workers []map[string]any
	if err := h.db.Table("subcontractor_workers").Where("subcontractor_id = ? AND deleted_at IS NULL", c.Param("id")).Order("id").Find(&workers).Error; err != nil {
		serverError(c, err)
		return
	}
	row["workers"] = workers
	c.JSON(http.StatusOK, gin.H{"item": row})
}
func (h *MasterHandler) createSubcontractor(c *gin.Context) {
	data, ok := body(c)
	if !ok {
		return
	}
	if !h.validate(c, "subcontractors", data) {
		return
	}
	tx := h.db.Begin()
	values := pick(data, subcontractorColumns)
	if err := insertWithGeneratedCode(tx, "subcontractors", "subcontractor_code", "SUB", values); err != nil {
		tx.Rollback()
		dbError(c, err)
		return
	}
	id := insertedID(tx, values)
	if id == 0 {
		tx.Rollback()
		serverError(c, nil)
		return
	}
	if workers, present := data["workers"].([]any); present {
		for _, raw := range workers {
			worker := raw.(map[string]any) // validated before this transaction starts.
			fields := pick(worker, []string{"full_name", "work_description", "phone", "notes"})
			fields["subcontractor_id"] = id
			if err := tx.Table("subcontractor_workers").Create(fields).Error; err != nil {
				tx.Rollback()
				dbError(c, err)
				return
			}
		}
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: strconv.FormatUint(id, 10)})
	h.getSubcontractor(c)
}
func (h *MasterHandler) updateSubcontractor(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		return
	}
	data, ok := body(c)
	if !ok {
		return
	}
	if !h.validate(c, "subcontractors", data) {
		return
	}
	tx := h.db.Begin()
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	values := pick(data, subcontractorColumns)
	delete(values, "subcontractor_code")
	if result := tx.Table("subcontractors").Where("id = ? AND deleted_at IS NULL", id).Updates(values); result.Error != nil {
		tx.Rollback()
		dbError(c, result.Error)
		return
	} else if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(tx, "subcontractors", id) }) {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	if err := replaceWorkers(tx, id, data); err != nil {
		tx.Rollback()
		dbError(c, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	h.getSubcontractor(c)
}
func replaceWorkers(tx *gorm.DB, id uint64, data map[string]any) error {
	values, present := data["workers"].([]any)
	if !present {
		return nil
	}
	kept := map[uint64]bool{}
	for _, raw := range values {
		worker := raw.(map[string]any) // validated before the existing rows are soft-deleted.
		fields := pick(worker, []string{"full_name", "work_description", "phone", "notes"})
		if workerID, hasID, valid := optionalPositiveID(worker, "id"); hasID && valid {
			result := tx.Table("subcontractor_workers").Where("id = ? AND subcontractor_id = ? AND deleted_at IS NULL", workerID, id).Updates(fields)
			if result.Error != nil || result.RowsAffected != 1 {
				return errors.New("worker not found")
			}
			kept[workerID] = true
		} else {
			fields["subcontractor_id"] = id
			if result := tx.Table("subcontractor_workers").Create(fields); result.Error != nil {
				return result.Error
			}
			newID := insertedID(tx, fields)
			if newID == 0 {
				return errors.New("worker id unavailable")
			}
			kept[newID] = true
		}
	}
	var existing []uint64
	if err := tx.Table("subcontractor_workers").Where("subcontractor_id = ? AND deleted_at IS NULL", id).Pluck("id", &existing).Error; err != nil {
		return err
	}
	for _, workerID := range existing {
		if !kept[workerID] {
			if err := tx.Table("subcontractor_workers").Where("id = ?", workerID).Update("deleted_at", time.Now()).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
func (h *MasterHandler) deleteSubcontractorWorker(c *gin.Context) {
	result := h.db.Table("subcontractor_workers").Where("id = ? AND subcontractor_id = ? AND deleted_at IS NULL", c.Param("workerID"), c.Param("id")).Update("deleted_at", time.Now())
	if result.Error != nil {
		serverError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "外注先従業員が見つかりません。"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *MasterHandler) listProjects(c *gin.Context) {
	var rows []map[string]any
	err := h.db.Table("projects p").Select("p.id, p.project_code, p.site_name, p.status, p.ordered_on, p.construction_start_on, p.construction_end_on, p.completed_on, COALESCE(c.name, '削除済み得意先') AS client_name").Joins("LEFT JOIN clients c ON c.id = p.client_id").Where("p.deleted_at IS NULL").Order("p.project_code DESC").Find(&rows).Error
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
func (h *MasterHandler) getProject(c *gin.Context) {
	h.respondProject(c, http.StatusOK)
}
func (h *MasterHandler) respondProject(c *gin.Context, status int) {
	row, ok := h.find("projects", projectColumns, c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
		return
	}
	var members []map[string]any
	if err := h.db.Table("project_members pm").Select("pm.*, e.full_name AS employee_name, sw.full_name AS subcontractor_worker_name, s.name AS subcontractor_name").Joins("LEFT JOIN employees e ON e.id = pm.employee_id").Joins("LEFT JOIN subcontractor_workers sw ON sw.id = pm.subcontractor_worker_id").Joins("LEFT JOIN subcontractors s ON s.id = sw.subcontractor_id").Where("pm.project_id = ? AND pm.deleted_at IS NULL", c.Param("id")).Find(&members).Error; err != nil {
		serverError(c, err)
		return
	}
	row["members"] = members
	c.JSON(status, gin.H{"item": row})
}
func (h *MasterHandler) createProject(c *gin.Context) { h.saveProject(c, 0) }
func (h *MasterHandler) updateProject(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		return
	}
	h.saveProject(c, id)
}
func (h *MasterHandler) saveProject(c *gin.Context, id uint64) {
	data, ok := body(c)
	if !ok {
		return
	}
	if !h.validate(c, "projects", data) {
		return
	}
	tx := h.db.Begin()
	values := pick(data, projectColumns)
	creating := id == 0
	if creating {
		if err := insertWithGeneratedCode(tx, "projects", "project_code", "PRJ", values); err != nil {
			tx.Rollback()
			dbError(c, err)
			return
		}
		id = insertedID(tx, values)
	} else {
		delete(values, "project_code")
		result := tx.Table("projects").Where("id = ? AND deleted_at IS NULL", id).Updates(values)
		if result.Error != nil {
			tx.Rollback()
			dbError(c, result.Error)
			return
		}
		if updateTargetMissing(result.RowsAffected, func() bool { return h.activeRecordExists(tx, "projects", id) }) {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"error": "データが見つかりません。"})
			return
		}
	}
	if members, present := data["members"].([]any); present {
		if tx.Table("project_members").Where("project_id = ?", id).Update("deleted_at", time.Now()).Error != nil {
			tx.Rollback()
			serverError(c, tx.Error)
			return
		}
		for _, raw := range members {
			member := raw.(map[string]any) // validated before the existing rows are soft-deleted.
			fields := pick(member, []string{"employee_id", "subcontractor_worker_id", "work_description", "start_on", "end_on", "phone", "notes"})
			if !employeeMemberFields(fields) {
				fields["employee_id"] = nil
			} else {
				fields["subcontractor_worker_id"] = nil
			}
			fields["project_id"] = id
			if result := tx.Table("project_members").Create(fields); result.Error != nil {
				tx.Rollback()
				dbError(c, result.Error)
				return
			}
		}
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: strconv.FormatUint(id, 10)})
	status := http.StatusOK
	if creating {
		status = http.StatusCreated
	}
	h.respondProject(c, status)
}

func (h *MasterHandler) find(table string, columns []string, rawID string) (map[string]any, bool) {
	var row map[string]any
	err := h.db.Table(table).Select(append([]string{"id"}, columns...)).Where("id = ? AND deleted_at IS NULL", rawID).Take(&row).Error
	return row, err == nil
}
func (h *MasterHandler) activeRecordExists(db *gorm.DB, table string, id any) bool {
	var count int64
	return activeRecord(db, table, id).Count(&count).Error == nil && count == 1
}
func activeRecord(db *gorm.DB, table string, id any) *gorm.DB {
	return db.Table(table).Where(activeRecordCondition, id)
}

const activeRecordCondition = "id = ? AND deleted_at IS NULL"

func updateTargetMissing(rowsAffected int64, exists func() bool) bool {
	return rowsAffected == 0 && !exists()
}
func maskSensitiveEmployeeFields(row map[string]any) {
	// These identifiers are intentionally never returned in full, even to admins.
	for _, key := range []string{"employment_insurance_no", "pension_no", "insurance_card_no", "my_number_encrypted"} {
		if value, exists := row[key]; exists && value != nil {
			row[key] = "••••••"
		}
	}
}
func employeeMemberFields(fields map[string]any) bool {
	_, present, valid := optionalPositiveID(fields, "employee_id")
	return present && valid
}
func body(c *gin.Context) (map[string]any, bool) {
	var data map[string]any
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入力内容を確認してください。"})
		return nil, false
	}
	return data, true
}
func pick(data map[string]any, columns []string) map[string]any {
	out := map[string]any{}
	for _, column := range columns {
		if value, ok := data[column]; ok {
			out[column] = value
		}
	}
	return out
}
func asUint64(value any) uint64 {
	switch v := value.(type) {
	case float64:
		return uint64(v)
	case uint64:
		return v
	case int:
		return uint64(v)
	case string:
		parsed, _ := strconv.ParseUint(v, 10, 64)
		return parsed
	}
	return 0
}
func insertedID(tx *gorm.DB, values map[string]any) uint64 {
	if id := asUint64(values["id"]); id != 0 {
		return id
	}
	var id uint64
	_ = tx.Raw("SELECT LAST_INSERT_ID()").Scan(&id).Error
	return id
}
func idParam(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不正なIDです。"})
		return 0, strconv.ErrSyntax
	}
	return id, nil
}
func dbError(c *gin.Context, err error) {
	if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		c.JSON(http.StatusConflict, gin.H{"error": "コードまたはログインIDが重複しています。"})
		return
	}
	serverError(c, err)
}
func serverError(c *gin.Context, _ error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "保存中にエラーが発生しました。"})
}
