package repository

import (
	"context"
	"errors"

	"github.com/royle/construction/backend/internal/domain"
)

var ErrNotFound = errors.New("record not found")

type AccountRepository interface {
	FindByLoginID(ctx context.Context, loginID string) (domain.Account, error)
	FindByID(ctx context.Context, id uint64) (domain.Account, error)
}
