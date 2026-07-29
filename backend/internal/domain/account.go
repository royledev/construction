package domain

import "time"

// Account is the minimal authenticated principal exposed to delivery code.
// PasswordHash deliberately never leaves the repository/usecase boundary.
type Account struct {
	ID                    uint64
	EmployeeID            uint64
	SubcontractorWorkerID uint64
	WorkerType            string
	WorkerID              uint64
	LoginID               string
	PasswordHash          string
	Role                  string
	SessionVersion        uint64
	DisplayName           string
	CreatedAt             time.Time
}

type AuthenticatedUser struct {
	ID                    uint64 `json:"id"`
	EmployeeID            uint64 `json:"employeeId,omitempty"`
	SubcontractorWorkerID uint64 `json:"subcontractorWorkerId,omitempty"`
	WorkerType            string `json:"workerType"`
	WorkerID              uint64 `json:"workerId"`
	LoginID               string `json:"loginId"`
	DisplayName           string `json:"displayName"`
	Role                  string `json:"role"`
	SessionVersion        uint64 `json:"-"`
}

func (a Account) Public() AuthenticatedUser {
	return AuthenticatedUser{ID: a.ID, EmployeeID: a.EmployeeID, SubcontractorWorkerID: a.SubcontractorWorkerID, WorkerType: a.WorkerType, WorkerID: a.WorkerID, LoginID: a.LoginID, DisplayName: a.DisplayName, Role: a.Role, SessionVersion: a.SessionVersion}
}
