package httpdelivery

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// registerWorkTracking contains the intentionally small, revision-protected
// commands used by the daily report and schedule grids.  They live beside the
// ledger handler so the same authentication and admin middleware is reused.
func (h *MasterHandler) registerWorkTracking(secured *gin.RouterGroup) {
	secured.GET("/daily-reports", h.listDailyReports)
	secured.GET("/daily-reports/my", h.myDailyReports)
	secured.PATCH("/daily-reports/:id", h.patchDailyReport)
	secured.DELETE("/daily-reports/:id", h.deleteMyDailyReport)
	secured.POST("/daily-reports/:id/children", h.addDailyReportChild)
	secured.POST("/daily-reports/:id/duplicate", h.duplicateDailyReport)
	secured.GET("/schedules", h.listSchedules)
	secured.GET("/schedule-slots", h.listScheduleSlots)
	secured.POST("/schedule-slots", h.requireAdmin, h.createScheduleSlot)
	secured.PATCH("/schedules/cell", h.patchScheduleCell)
}

func workMonth(c *gin.Context) (time.Time, bool) {
	month := c.DefaultQuery("month", time.Now().In(time.FixedZone("Asia/Tokyo", 9*60*60)).Format("2006-01"))
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "月はYYYY-MM形式で指定してください。"})
		return time.Time{}, false
	}
	return parsed, true
}
func monthBounds(month time.Time) (string, string) {
	return month.Format("2006-01-02"), month.AddDate(0, 1, 0).Format("2006-01-02")
}

type workerSubject struct {
	Type string
	ID   uint64
}

func (s workerSubject) reportColumn() string {
	if s.Type == "subcontractor" {
		return "subcontractor_worker_id"
	}
	return "employee_id"
}
func (s workerSubject) scheduleColumns() map[string]any {
	if s.Type == "subcontractor" {
		return map[string]any{"employee_id": nil, "subcontractor_worker_id": s.ID}
	}
	return map[string]any{"employee_id": s.ID, "subcontractor_worker_id": nil}
}

func (h *MasterHandler) accountWorkerSubject(c *gin.Context) (workerSubject, bool) {
	var row struct {
		EmployeeID            *uint64 `gorm:"column:employee_id"`
		SubcontractorWorkerID *uint64 `gorm:"column:subcontractor_worker_id"`
	}
	if err := h.db.Table("accounts").Select("employee_id, subcontractor_worker_id").Where("id = ? AND deleted_at IS NULL", c.GetUint64("accountID")).Take(&row).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "セッションが無効です。"})
		return workerSubject{}, false
	}
	if row.EmployeeID != nil {
		return workerSubject{Type: "employee", ID: *row.EmployeeID}, true
	}
	if row.SubcontractorWorkerID != nil {
		return workerSubject{Type: "subcontractor", ID: *row.SubcontractorWorkerID}, true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "セッションが無効です。"})
	return workerSubject{}, false
}

func (h *MasterHandler) ensureMyMonth(subject workerSubject, month time.Time) error {
	start, end := monthBounds(month)
	var existing []time.Time
	if err := h.db.Table("daily_reports").Where(subject.reportColumn()+" = ? AND report_date >= ? AND report_date < ? AND parent_report_id IS NULL AND deleted_at IS NULL", subject.ID, start, end).Pluck("report_date", &existing).Error; err != nil {
		return err
	}
	present := map[string]bool{}
	for _, date := range existing {
		present[date.Format("2006-01-02")] = true
	}
	rows := make([]map[string]any, 0, 31)
	for day := month; day.Before(month.AddDate(0, 1, 0)); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		if !present[date] {
			row := map[string]any{"report_date": date, "break_minutes": 0, "is_month_seeded_parent": true}
			row[subject.reportColumn()] = subject.ID
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return h.db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.Insert{Modifier: "IGNORE"}).Table("daily_reports").Create(&rows).Error
	})
}

func (h *MasterHandler) myDailyReports(c *gin.Context) {
	subject, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	month, ok := workMonth(c)
	if !ok {
		return
	}
	if err := h.ensureMyMonth(subject, month); err != nil {
		serverError(c, err)
		return
	}
	start, end := monthBounds(month)
	var rows []map[string]any
	err := h.db.Table("daily_reports dr").Select("dr.*, p.site_name, cl.name AS client_name").Joins("LEFT JOIN projects p ON p.id = dr.project_id").Joins("LEFT JOIN clients cl ON cl.id = dr.client_id").Where("dr."+subject.reportColumn()+" = ? AND dr.report_date >= ? AND dr.report_date < ? AND dr.deleted_at IS NULL", subject.ID, start, end).Order("dr.report_date, CASE WHEN dr.parent_report_id IS NULL THEN 0 ELSE 1 END, dr.id").Find(&rows).Error
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *MasterHandler) listDailyReports(c *gin.Context) {
	month, ok := workMonth(c)
	if !ok {
		return
	}
	start, end := monthBounds(month)
	var rows []map[string]any
	err := h.db.Table("daily_reports dr").
		Select("dr.*, p.site_name, cl.name AS client_name, e.full_name AS employee_name, e.occupation, sw.full_name AS subcontractor_worker_name, sc.name AS subcontractor_name, COALESCE(sc.name, op.company_name) AS company_name, CASE WHEN dr.employee_id IS NULL THEN 'subcontractor' ELSE 'employee' END AS worker_type").
		Joins("LEFT JOIN projects p ON p.id = dr.project_id").Joins("LEFT JOIN clients cl ON cl.id = dr.client_id").Joins("LEFT JOIN employees e ON e.id = dr.employee_id").Joins("LEFT JOIN subcontractor_workers sw ON sw.id = dr.subcontractor_worker_id").Joins("LEFT JOIN subcontractors sc ON sc.id = sw.subcontractor_id").Joins("JOIN organization_profiles op ON op.id = 1").
		Where("dr.report_date >= ? AND dr.report_date < ? AND dr.deleted_at IS NULL", start, end).
		Order("CASE WHEN dr.employee_id IS NULL THEN 1 ELSE 0 END, COALESCE(e.full_name, sc.name), COALESCE(sw.full_name, ''), dr.report_date, dr.id").Find(&rows).Error
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *MasterHandler) deleteMyDailyReport(c *gin.Context) {
	subject, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	id, err := idParam(c)
	if err != nil {
		return
	}
	var row struct {
		ReportDate     time.Time `gorm:"column:report_date"`
		ParentReportID *uint64   `gorm:"column:parent_report_id"`
	}
	if err := h.db.Table("daily_reports").Select("report_date, parent_report_id").Where("id = ? AND "+subject.reportColumn()+" = ? AND deleted_at IS NULL", id, subject.ID).Take(&row).Error; err != nil {
		dailyReportMissing(c, err)
		return
	}
	state := ""
	err = transactionWithRetry(h.db, func(tx *gorm.DB) error {
		state = ""
		// Lock the target before the day set. Child creation follows the same
		// parent-then-day order, preventing a delete/add-child lock inversion.
		var target []uint64
		if err := tx.Raw("SELECT id FROM daily_reports WHERE id = ? AND "+subject.reportColumn()+" = ? AND deleted_at IS NULL FOR UPDATE", id, subject.ID).Scan(&target).Error; err != nil {
			return err
		}
		if len(target) == 0 {
			return gorm.ErrRecordNotFound
		}
		var ids []uint64
		if err := tx.Raw("SELECT id FROM daily_reports WHERE "+subject.reportColumn()+" = ? AND report_date = ? AND deleted_at IS NULL FOR UPDATE", subject.ID, row.ReportDate).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) < 2 {
			state = "minimum"
			return nil
		}
		var children int64
		if err := tx.Table("daily_reports").Where("parent_report_id = ? AND deleted_at IS NULL", id).Count(&children).Error; err != nil {
			return err
		}
		if children > 0 {
			state = "children"
			return nil
		}
		return tx.Table("daily_reports").Where("id = ? AND deleted_at IS NULL", id).Update("deleted_at", time.Now()).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dailyReportMissing(c, err)
			return
		}
		serverError(c, err)
		return
	}
	if state == "minimum" {
		c.JSON(http.StatusConflict, gin.H{"error": "同日の日報が1件のため削除できません。", "code": "daily_report_minimum_one"})
		return
	}
	if state == "children" {
		c.JSON(http.StatusConflict, gin.H{"error": "子行がある親日報は削除できません。", "code": "daily_report_has_children"})
		return
	}
	c.Status(http.StatusNoContent)
}

var dailyPatchColumns = map[string]struct{}{"project_id": {}, "client_id": {}, "started_at": {}, "ended_at": {}, "work_description": {}}

func reportBreakMinutes(started, ended string) (int, bool) {
	if started == "" || ended == "" {
		return 0, true
	}
	parseWorkTime := func(value string) (time.Time, error) {
		// The API accepts HH:mm, while MySQL TIME values read back through the
		// driver include seconds. Both represent the same work-time value.
		for _, layout := range []string{"15:04", "15:04:05"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed, nil
			}
		}
		return time.Time{}, errors.New("invalid work time")
	}
	s, err1 := parseWorkTime(started)
	e, err2 := parseWorkTime(ended)
	if err1 != nil || err2 != nil || !e.After(s) {
		return 0, false
	}
	minutes := int(e.Sub(s).Minutes())
	if minutes > 8*60 {
		return 60, true
	}
	if minutes > 6*60 {
		return 45, true
	}
	return 0, true
}

func (h *MasterHandler) patchDailyReport(c *gin.Context) {
	subject, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	id, err := idParam(c)
	if err != nil {
		return
	}
	data, ok := body(c)
	if !ok {
		return
	}
	revision, valid := positiveID(data["revision"])
	if !valid {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "revisionが必要です。"})
		return
	}
	values := map[string]any{}
	for key := range dailyPatchColumns {
		if value, present := data[key]; present {
			values[key] = value
		}
	}
	if len(values) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "更新する項目がありません。"})
		return
	}
	for field, table := range map[string]string{"project_id": "projects", "client_id": "clients"} {
		if raw, present := values[field]; present && raw != nil {
			id, null, strict := strictNullablePositiveUint(raw)
			if !strict {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "参照IDの指定が不正です。"})
				return
			}
			if null {
				values[field] = nil
				continue
			}
			values[field] = id
			var count int64
			if err := h.db.Table(table).Where("id = ? AND deleted_at IS NULL", id).Count(&count).Error; err != nil {
				serverError(c, err)
				return
			}
			if count != 1 {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "有効な参照先を指定してください。"})
				return
			}
		}
	}
	for _, key := range []string{"started_at", "ended_at"} {
		if value, present := values[key]; present && value != nil {
			text, isString := value.(string)
			if !isString || (strings.TrimSpace(text) != "" && func() bool { _, err := time.Parse("15:04", strings.TrimSpace(text)); return err != nil }()) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "日報の入力内容が不正です。"})
				return
			}
			values[key] = strings.TrimSpace(text)
		}
	}
	var current struct {
		StartedAt *string `gorm:"column:started_at"`
		EndedAt   *string `gorm:"column:ended_at"`
	}
	if err := h.db.Table("daily_reports").Select("started_at, ended_at").Where("id = ? AND "+subject.reportColumn()+" = ? AND deleted_at IS NULL", id, subject.ID).Take(&current).Error; err != nil {
		dailyReportMissing(c, err)
		return
	}
	start, end := "", ""
	if current.StartedAt != nil {
		start = *current.StartedAt
	}
	if current.EndedAt != nil {
		end = *current.EndedAt
	}
	if value, present := values["started_at"]; present {
		start = strings.TrimSpace(toString(value))
	}
	if value, present := values["ended_at"]; present {
		end = strings.TrimSpace(toString(value))
	}
	breakMinutes, valid := reportBreakMinutes(start, end)
	if !valid {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "開始・終了時刻を正しく入力してください。"})
		return
	}
	values["break_minutes"], values["revision"] = breakMinutes, revision+1
	result := h.db.Table("daily_reports").Where("id = ? AND "+subject.reportColumn()+" = ? AND revision = ? AND deleted_at IS NULL", id, subject.ID, revision).Updates(values)
	if result.Error != nil {
		serverError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		conflict(c)
		return
	}
	row, err := h.dailyReportItem(id)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": row})
}

func (h *MasterHandler) dailyReportItem(id uint64) (map[string]any, error) {
	row := map[string]any{}
	err := h.db.Table("daily_reports dr").Select("dr.*, p.site_name, cl.name AS client_name").Joins("LEFT JOIN projects p ON p.id = dr.project_id").Joins("LEFT JOIN clients cl ON cl.id = dr.client_id").Where("dr.id = ?", id).Take(&row).Error
	return row, err
}
func dailyReportMissing(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "本人の日報が見つかりません。"})
		return
	}
	serverError(c, err)
}
func conflict(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{"error": "他の更新と競合しました。再読込してからやり直してください。", "code": "revision_conflict"})
}

func (h *MasterHandler) addDailyReportChild(c *gin.Context) {
	subject, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	id, err := idParam(c)
	if err != nil {
		return
	}
	childID, err := h.createSerializedDailyChild(subject, id, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dailyReportMissing(c, err)
			return
		}
		serverError(c, err)
		return
	}
	row, err := h.dailyReportItem(childID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": row})
}
func (h *MasterHandler) duplicateDailyReport(c *gin.Context) {
	subject, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	id, err := idParam(c)
	if err != nil {
		return
	}
	copyID, err := h.createSerializedDailyChild(subject, id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dailyReportMissing(c, err)
			return
		}
		serverError(c, err)
		return
	}
	row, err := h.dailyReportItem(copyID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": row})
}

func (h *MasterHandler) createSerializedDailyChild(subject workerSubject, parentID uint64, duplicate bool) (uint64, error) {
	var created uint64
	err := transactionWithRetry(h.db, func(tx *gorm.DB) error {
		parent := map[string]any{}
		if err := tx.Raw("SELECT * FROM daily_reports WHERE id=? AND "+subject.reportColumn()+"=? AND parent_report_id IS NULL AND deleted_at IS NULL FOR UPDATE", parentID, subject.ID).Scan(&parent).Error; err != nil {
			return err
		}
		if len(parent) == 0 {
			return gorm.ErrRecordNotFound
		}
		date := parent["report_date"]
		var ids []uint64
		if err := tx.Raw("SELECT id FROM daily_reports WHERE "+subject.reportColumn()+"=? AND report_date=? AND deleted_at IS NULL FOR UPDATE", subject.ID, date).Scan(&ids).Error; err != nil {
			return err
		}
		values := map[string]any{"report_date": date, "break_minutes": 0}
		if duplicate {
			values["project_id"], values["client_id"] = parent["project_id"], parent["client_id"]
		} else {
			values["parent_report_id"] = parentID
		}
		values[subject.reportColumn()] = subject.ID
		if err := tx.Table("daily_reports").Create(values).Error; err != nil {
			return err
		}
		created = insertedID(tx, values)
		if created == 0 {
			return errors.New("daily report id unavailable")
		}
		return nil
	})
	return created, err
}

func (h *MasterHandler) createDailyReport(values map[string]any) (uint64, error) {
	var id uint64
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("daily_reports").Create(&values).Error; err != nil {
			return err
		}
		id = insertedID(tx, values)
		if id == 0 {
			return errors.New("daily report id was not returned")
		}
		return nil
	})
	return id, err
}

func (h *MasterHandler) listSchedules(c *gin.Context) {
	month, ok := workMonth(c)
	if !ok {
		return
	}
	start, end := monthBounds(month)
	var employees, schedules, items, slots []map[string]any
	var companyName string
	if err := h.db.Table("organization_profiles").Select("company_name").Where("id=1").Scan(&companyName).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("employees").Select("id, employee_code, full_name, occupation").Where("deleted_at IS NULL AND status = 'active'").Order("employee_code").Find(&employees).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("schedules s").Select("s.*, mi.name AS master_item_name").Joins("LEFT JOIN master_items mi ON mi.id = s.master_item_id").Where("s.schedule_date >= ? AND s.schedule_date < ? AND s.deleted_at IS NULL", start, end).Find(&schedules).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("master_items").Select("id, item_code, name").Where("item_type = 'schedule' AND deleted_at IS NULL").Order("item_code").Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	var subcontractorWorkers []map[string]any
	if err := h.db.Table("subcontractor_workers sw").Select("sw.id, sw.full_name, sw.work_description, sc.name AS subcontractor_name").Joins("JOIN subcontractors sc ON sc.id = sw.subcontractor_id AND sc.deleted_at IS NULL").Where("sw.deleted_at IS NULL").Order("sc.name, sw.full_name").Find(&subcontractorWorkers).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := h.db.Table("schedule_slots").Select("slot").Order("slot").Find(&slots).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"company_name": companyName, "employees": employees, "subcontractor_workers": subcontractorWorkers, "items": items, "slots": slots, "schedules": schedules})
}
func (h *MasterHandler) listScheduleSlots(c *gin.Context) {
	var slots []map[string]any
	if err := h.db.Table("schedule_slots").Select("slot").Order("slot").Find(&slots).Error; err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": slots})
}
func (h *MasterHandler) createScheduleSlot(c *gin.Context) {
	var slot uint64
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = h.db.Transaction(func(tx *gorm.DB) error {
			var max uint64
			if err := tx.Raw("SELECT COALESCE(MAX(slot), 0) FROM schedule_slots FOR UPDATE").Scan(&max).Error; err != nil {
				return err
			}
			if max >= 100 {
				return errScheduleSlotLimit
			}
			slot = max + 1
			return tx.Table("schedule_slots").Create(map[string]any{"slot": slot}).Error
		})
		if err == nil || !isRetryableTransactionError(err) {
			break
		}
	}
	if errors.Is(err, errScheduleSlotLimit) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "追加できるslot数の上限です。"})
		return
	}
	if err != nil {
		dbError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": gin.H{"slot": slot}})
}

var errScheduleSlotLimit = errors.New("slot limit")

func isRetryableTransactionError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1205 || mysqlErr.Number == 1213)
}

// Daily-report operations lock a target then all active rows for its day.
// Separate targets on the same day can still deadlock, so retry only MySQL's
// transaction-victim errors; business errors are returned unchanged.
func transactionWithRetry(db *gorm.DB, fn func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = db.Transaction(fn)
		if err == nil || !isRetryableTransactionError(err) {
			return err
		}
	}
	return err
}

func (h *MasterHandler) patchScheduleCell(c *gin.Context) {
	owner, ok := h.accountWorkerSubject(c)
	if !ok {
		return
	}
	data, ok := body(c)
	if !ok {
		return
	}
	workerType, workerID := "employee", uint64(0)
	if rawType, hasType := data["worker_type"]; hasType {
		workerType = toString(rawType)
		workerID, _ = strictPositiveUint(data["worker_id"])
	} else {
		workerID, _ = strictPositiveUint(data["employee_id"])
	}
	slot, slotOK := strictPositiveUint(data["slot"])
	date := toString(data["schedule_date"])
	if !oneOf(workerType, "employee", "subcontractor") || workerID == 0 || !slotOK || slot > 100 || date == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "工程セルの指定が不正です。"})
		return
	}
	var slotCount int64
	if err := h.db.Table("schedule_slots").Where("slot = ?", slot).Count(&slotCount).Error; err != nil {
		serverError(c, err)
		return
	}
	if slotCount != 1 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "未定義のslotは指定できません。"})
		return
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "日付が不正です。"})
		return
	}
	if workerType != owner.Type || workerID != owner.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "工程は本人の行のみ編集できます。"})
		return
	}
	masterRaw, masterPresent := data["master_item_id"]
	revisionRaw, revisionPresent := data["revision"]
	masterID, masterNull, masterOK := strictNullablePositiveUint(masterRaw)
	revision, revisionOK := strictRevision(revisionRaw)
	if !masterPresent || !revisionPresent || !masterOK || !revisionOK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "工程項目またはrevisionの指定が不正です。"})
		return
	}
	if !masterNull {
		var count int64
		if err := h.db.Table("master_items").Where("id = ? AND item_type = 'schedule' AND deleted_at IS NULL", masterID).Count(&count).Error; err != nil {
			serverError(c, err)
			return
		}
		if count != 1 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "有効な工程項目を指定してください。"})
			return
		}
	}
	var existing map[string]any
	ownerValues := workerSubject{Type: workerType, ID: workerID}.scheduleColumns()
	err := h.db.Table("schedules").Where("worker_type = ? AND worker_id = ? AND schedule_date = ? AND slot = ? AND deleted_at IS NULL", workerType, workerID, date, slot).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if revision != 0 {
			conflict(c)
			return
		}
		values := map[string]any{"schedule_date": date, "slot": slot, "master_item_id": nullableScheduleID(masterID, masterNull), "revision": 1}
		for k, v := range ownerValues {
			values[k] = v
		}
		id, createErr := h.createScheduleCell(values)
		if createErr != nil {
			if isDuplicateKey(createErr) {
				conflict(c)
				return
			}
			serverError(c, createErr)
			return
		}
		row, itemErr := h.scheduleItem(id)
		if itemErr != nil {
			serverError(c, itemErr)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"item": row})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if asUint64(existing["revision"]) != revision {
		conflict(c)
		return
	}
	result := h.db.Table("schedules").Where("id = ? AND revision = ? AND deleted_at IS NULL", existing["id"], revision).Updates(map[string]any{"master_item_id": nullableScheduleID(masterID, masterNull), "revision": revision + 1})
	if result.Error != nil {
		serverError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		conflict(c)
		return
	}
	row, itemErr := h.scheduleItem(asUint64(existing["id"]))
	if itemErr != nil {
		serverError(c, itemErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": row})
}
func (h *MasterHandler) createScheduleCell(values map[string]any) (uint64, error) {
	var id uint64
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("schedules").Create(&values).Error; err != nil {
			return err
		}
		id = insertedID(tx, values)
		if id == 0 {
			return errors.New("schedule id was not returned")
		}
		return nil
	})
	return id, err
}
func (h *MasterHandler) scheduleItem(id uint64) (map[string]any, error) {
	row := map[string]any{}
	err := h.db.Table("schedules s").Select("s.*, mi.name AS master_item_name").Joins("LEFT JOIN master_items mi ON mi.id = s.master_item_id").Where("s.id = ?", id).Take(&row).Error
	return row, err
}
func isDuplicateKey(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
func duplicateKeyName(err error) string {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Message
	}
	return ""
}
func strictPositiveUint(value any) (uint64, bool) {
	id, null, ok := strictNullablePositiveUint(value)
	return id, ok && !null
}
func strictNullablePositiveUint(value any) (uint64, bool, bool) {
	if value == nil {
		return 0, true, true
	}
	var number uint64
	switch typed := value.(type) {
	case float64:
		if typed < 1 || typed != math.Trunc(typed) || typed > math.MaxUint64 {
			return 0, false, false
		}
		number = uint64(typed)
	case uint64:
		number = typed
	case int:
		if typed < 1 {
			return 0, false, false
		}
		number = uint64(typed)
	case string:
		if typed == "" || typed[0] == '0' {
			return 0, false, false
		}
		for _, char := range typed {
			if char < '0' || char > '9' {
				return 0, false, false
			}
		}
		parsed, err := strconv.ParseUint(typed, 10, 64)
		if err != nil || parsed == 0 {
			return 0, false, false
		}
		number = parsed
	default:
		return 0, false, false
	}
	if number == 0 {
		return 0, false, false
	}
	return number, false, true
}
func strictRevision(value any) (uint64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed < 0 || typed != math.Trunc(typed) || typed > math.MaxUint64 {
			return 0, false
		}
		return uint64(typed), true
	case uint64:
		return typed, true
	case int:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	default:
		return 0, false
	}
}
func nullableScheduleID(id uint64, isNull bool) any {
	if isNull {
		return nil
	}
	return id
}
