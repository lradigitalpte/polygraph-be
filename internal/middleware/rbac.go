package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RBACMiddleware checks if the user has one of the required roles
func RBACMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Role not found in context"})
			c.Abort()
			return
		}

		userRole := role.(string)
		authorized := false
		for _, r := range requiredRoles {
			if userRole == r {
				authorized = true
				break
			}
		}

		if !authorized {
			c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to perform this action"})
			c.Abort()
			return
		}

		c.Next()
	}
}
