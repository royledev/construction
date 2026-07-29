package httpdelivery

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/royle/construction/backend/internal/usecase"
)

type AuthHandler struct {
	auth     *usecase.AuthUsecase
	sessions *SessionManager
}

func NewAuthHandler(auth *usecase.AuthUsecase, sessions *SessionManager) *AuthHandler {
	return &AuthHandler{auth: auth, sessions: sessions}
}

type loginRequest struct {
	LoginID  string `json:"loginId" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ログインIDとパスワードを入力してください。"})
		return
	}
	user, err := h.auth.Login(c.Request.Context(), request.LoginID, request.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ログインIDまたはパスワードが正しくありません。"})
		return
	}
	cookie, expiresAt, err := h.sessions.IssueUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "セッションを開始できません。"})
		return
	}
	c.Header("Set-Cookie", h.sessions.Cookie(cookie, expiresAt))
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.Header("Set-Cookie", h.sessions.ClearCookie())
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) Me(c *gin.Context) {
	accountID, exists := c.Get("accountID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ログインが必要です。"})
		return
	}
	user, err := h.auth.Me(c.Request.Context(), accountID.(uint64))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "セッションが無効です。"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}
