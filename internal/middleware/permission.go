package middleware

import (
	"fmt"
	"my-app/internal/database"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

type cachedPermissionResult struct {
	allowed   bool
	expiresAt time.Time
}

var permissionCache sync.Map
var permissionSingleflight singleflight.Group

const permissionCacheTTL = 2 * time.Minute

// PermissionMiddleware checks if the user's role has the required permission
func PermissionMiddleware(permissionName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, exists := c.Get("role_id")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized: No role assigned"})
			c.Abort()
			return
		}

		cacheKey := fmt.Sprintf("%v:%s", roleID, permissionName)
		if cached, ok := permissionCache.Load(cacheKey); ok {
			entry := cached.(cachedPermissionResult)
			if time.Now().Before(entry.expiresAt) {
				if !entry.allowed {
					c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied: requires " + permissionName})
					c.Abort()
					return
				}
				c.Next()
				return
			}
			permissionCache.Delete(cacheKey)
		}

		type sfResult struct{ entry cachedPermissionResult }
		v, _, _ := permissionSingleflight.Do(cacheKey, func() (interface{}, error) {
			var count int64
			err := database.GetDB().
				Table("role_permissions").
				Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
				Where("role_permissions.role_id = ? AND permissions.name = ?", roleID, permissionName).
				Count(&count).Error
			allowed := err == nil && count > 0
			entry := cachedPermissionResult{allowed: allowed, expiresAt: time.Now().Add(permissionCacheTTL)}
			permissionCache.Store(cacheKey, entry)
			return sfResult{entry: entry}, nil
		})
		entry := v.(sfResult).entry
		permissionCache.Store(cacheKey, entry)

		if !entry.allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied: requires " + permissionName})
			c.Abort()
			return
		}

		c.Next()
	}
}
