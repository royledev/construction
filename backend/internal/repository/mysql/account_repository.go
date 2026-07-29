package mysql

import (
	"context"
	"errors"

	"github.com/royle/construction/backend/internal/domain"
	"github.com/royle/construction/backend/internal/repository"
	"gorm.io/gorm"
)

type AccountRepository struct{ db *gorm.DB }

func NewAccountRepository(db *gorm.DB) *AccountRepository { return &AccountRepository{db: db} }

type accountRow struct {
	ID                    uint64
	EmployeeID            *uint64
	SubcontractorWorkerID *uint64
	LoginID               string
	PasswordHash          string
	Role                  string
	SessionVersion        uint64
	DisplayName           string
}

func (r *AccountRepository) FindByLoginID(ctx context.Context, loginID string) (domain.Account, error) {
	return r.find(ctx, "a.login_id = ?", loginID)
}

func (r *AccountRepository) FindByID(ctx context.Context, id uint64) (domain.Account, error) {
	return r.find(ctx, "a.id = ?", id)
}

func (r *AccountRepository) find(ctx context.Context, condition string, value any) (domain.Account, error) {
	var row accountRow
	err := r.db.WithContext(ctx).Table("accounts AS a").
		Select("a.id, a.employee_id, a.subcontractor_worker_id, a.login_id, a.password_hash, a.role, a.session_version, COALESCE(e.full_name, sw.full_name) AS display_name").
		Joins("LEFT JOIN employees AS e ON e.id = a.employee_id AND e.deleted_at IS NULL").
		Joins("LEFT JOIN subcontractor_workers AS sw ON sw.id = a.subcontractor_worker_id AND sw.deleted_at IS NULL").
		Joins("LEFT JOIN subcontractors AS sc ON sc.id = sw.subcontractor_id AND sc.deleted_at IS NULL").
		Where(condition, value).Where("a.deleted_at IS NULL AND ((a.employee_id IS NOT NULL AND e.id IS NOT NULL) OR (a.subcontractor_worker_id IS NOT NULL AND sw.id IS NOT NULL AND sc.id IS NOT NULL))").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Account{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Account{}, err
	}
	account := domain.Account{ID: row.ID, LoginID: row.LoginID, PasswordHash: row.PasswordHash, Role: row.Role, SessionVersion: row.SessionVersion, DisplayName: row.DisplayName}
	if row.EmployeeID != nil {
		account.EmployeeID, account.WorkerID, account.WorkerType = *row.EmployeeID, *row.EmployeeID, "employee"
	}
	if row.SubcontractorWorkerID != nil {
		account.SubcontractorWorkerID, account.WorkerID, account.WorkerType = *row.SubcontractorWorkerID, *row.SubcontractorWorkerID, "subcontractor"
	}
	return account, nil
}
