package httpdelivery

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SessionValidator makes a signed cookie revocable when its account or employee
// has been logically deleted.
type SessionValidator interface {
	IsActive(context.Context, sessionClaims) (bool, error)
}

type accountSessionValidator struct{ db *gorm.DB }

func NewSessionValidator(db *gorm.DB) SessionValidator { return accountSessionValidator{db: db} }

func (v accountSessionValidator) IsActive(ctx context.Context, claims sessionClaims) (bool, error) {
	var count int64
	err := v.db.WithContext(ctx).Table("accounts AS a").
		Joins("LEFT JOIN employees AS e ON e.id = a.employee_id AND e.deleted_at IS NULL").
		Joins("LEFT JOIN subcontractor_workers AS sw ON sw.id=a.subcontractor_worker_id AND sw.deleted_at IS NULL").
		Joins("LEFT JOIN subcontractors AS sc ON sc.id=sw.subcontractor_id AND sc.deleted_at IS NULL").
		Where("a.id = ? AND a.session_version = ? AND a.deleted_at IS NULL AND ((a.employee_id IS NOT NULL AND e.id IS NOT NULL AND e.status = 'active' AND ? = 'employee' AND a.employee_id = ?) OR (a.subcontractor_worker_id IS NOT NULL AND sw.id IS NOT NULL AND sc.id IS NOT NULL AND ? = 'subcontractor' AND a.subcontractor_worker_id = ?))", claims.AccountID, claims.SessionVersion, claims.WorkerType, claims.WorkerID, claims.WorkerType, claims.WorkerID).
		Count(&count).Error
	return count == 1, err
}

func NewRouter(handler *AuthHandler, masters *MasterHandler, sessions *SessionManager, validator SessionValidator, corsOrigin string) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors(corsOrigin))
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	api := router.Group("/api/v1")
	auth := api.Group("/auth")
	auth.POST("/login", handler.Login)
	auth.POST("/logout", handler.Logout)
	auth.GET("/me", requireSession(sessions, validator), handler.Me)
	masters.Register(api, sessions, validator)
	return router
}

func requireSession(sessions *SessionManager, validator SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(sessions.CookieName())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "ログインが必要です。"})
			return
		}
		claims, err := sessions.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "セッションが無効です。"})
			return
		}
		active, err := validator.IsActive(c.Request.Context(), claims)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "セッションの確認中にエラーが発生しました。"})
			return
		}
		if !active {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "セッションが無効です。"})
			return
		}
		c.Set("accountID", claims.AccountID)
		c.Next()
	}
}

func cors(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
