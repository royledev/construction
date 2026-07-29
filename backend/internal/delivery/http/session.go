package httpdelivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/royle/construction/backend/internal/domain"
)

const sessionCookieName = "construction_session"

type sessionClaims struct {
	AccountID      uint64 `json:"sub"`
	WorkerType     string `json:"wt"`
	WorkerID       uint64 `json:"wid"`
	SessionVersion uint64 `json:"sv"`
	ExpiresAt      int64  `json:"exp"`
}

type SessionManager struct {
	secret []byte
	secure bool
}

func NewSessionManager(secret string, secure bool) *SessionManager {
	return &SessionManager{secret: []byte(secret), secure: secure}
}

// Issue is retained only for tests and deliberately produces a legacy
// account-only token. Parse rejects it, so pre-upgrade cookies cannot acquire
// a changed subject's permissions.
func (m *SessionManager) Issue(accountID uint64) (string, time.Time, error) {
	return m.issue(sessionClaims{AccountID: accountID})
}

func (m *SessionManager) IssueUser(user domain.AuthenticatedUser) (string, time.Time, error) {
	return m.issue(sessionClaims{AccountID: user.ID, WorkerType: user.WorkerType, WorkerID: user.WorkerID, SessionVersion: user.SessionVersion})
}

func (m *SessionManager) issue(claims sessionClaims) (string, time.Time, error) {
	expiresAt := time.Now().Add(12 * time.Hour)
	claims.ExpiresAt = expiresAt.Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + m.sign(encoded), expiresAt, nil
}

func (m *SessionManager) Parse(raw string) (sessionClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(m.sign(parts[0])), []byte(parts[1])) {
		return sessionClaims{}, errors.New("invalid session")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, err
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.AccountID == 0 || claims.WorkerID == 0 || !oneOf(claims.WorkerType, "employee", "subcontractor") || claims.SessionVersion == 0 || claims.ExpiresAt <= time.Now().Unix() {
		return sessionClaims{}, errors.New("expired session")
	}
	return claims, nil
}

func (m *SessionManager) sign(payload string) string {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (m *SessionManager) Cookie(value string, expires time.Time) string {
	return sessionCookieName + "=" + value + "; Path=/; HttpOnly; SameSite=Lax; Max-Age=" + strconv.FormatInt(int64(time.Until(expires).Seconds()), 10) + secureSuffix(m.secure)
}

func (m *SessionManager) ClearCookie() string {
	return sessionCookieName + "=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0" + secureSuffix(m.secure)
}
func (m *SessionManager) CookieName() string { return sessionCookieName }
func secureSuffix(secure bool) string {
	if secure {
		return "; Secure"
	}
	return ""
}
