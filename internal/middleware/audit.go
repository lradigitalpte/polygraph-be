package middleware

import (
	"context"
	"my-app/internal/database"
	"my-app/internal/models"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// AuditMiddleware logs every request to the database
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process request first to get status code
		c.Next()

		// Skip noisy non-business traffic.
		if c.Request.URL.Path == "/health" || c.Request.Method == "OPTIONS" {
			return
		}
		if c.Request.Method == "GET" {
			switch c.Request.URL.Path {
			case "/api/users", "/api/users/examiners", "/api/subjects", "/api/clients", "/api/exams/types", "/api/rbac/roles", "/api/rbac/permissions", "/api/audit-logs":
				return
			}
		}

		// Extract user ID from context (set by AuthMiddleware)
		var userID *uint
		if val, exists := c.Get("user_id"); exists {
			if id, ok := val.(uint); ok {
				userID = &id
			}
		}

		// Create audit log entry
		log := models.AuditLog{
			UserID:    userID,
			Action:    c.Request.Method + " " + c.Request.URL.Path,
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			Status:    c.Writer.Status(),
			IP:        getClientIP(c),
			UserAgent: c.Request.UserAgent(),
		}

		// Avoid blocking the response on audit persistence.
		go func(entry models.AuditLog) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = database.GetDB().
				WithContext(ctx).
				Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
				Create(&entry).Error
		}(log)
	}
}

func getClientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	if xrip := strings.TrimSpace(c.GetHeader("X-Real-IP")); xrip != "" {
		return xrip
	}

	return c.ClientIP()
}
