package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/royle/construction/backend/internal/domain"
	"github.com/royle/construction/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type AuthUsecase struct{ accounts repository.AccountRepository }

func NewAuthUsecase(accounts repository.AccountRepository) *AuthUsecase {
	return &AuthUsecase{accounts: accounts}
}

func (u *AuthUsecase) Login(ctx context.Context, loginID, password string) (domain.AuthenticatedUser, error) {
	account, err := u.accounts.FindByLoginID(ctx, strings.TrimSpace(loginID))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)) != nil {
		return domain.AuthenticatedUser{}, ErrInvalidCredentials
	}
	return account.Public(), nil
}

func (u *AuthUsecase) Me(ctx context.Context, accountID uint64) (domain.AuthenticatedUser, error) {
	account, err := u.accounts.FindByID(ctx, accountID)
	if err != nil {
		return domain.AuthenticatedUser{}, err
	}
	return account.Public(), nil
}
