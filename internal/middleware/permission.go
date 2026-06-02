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

// InvalidatePermissionCache clears all cached permission decisions. Call after
// changing role permissions or per-user permission overrides.
func InvalidatePermissionCache() {
	permissionCache.Range(func(key, _ any) bool {
		permissionCache.Delete(key)
		return true
	})
}

// HasPermission reports whether the current request's user has the named permission,
// honoring per-user overrides first and then the role. Useful inside handlers for
// conditional response shaping (e.g. hiding financial fields). Shares the same cache
// as PermissionMiddleware.
func HasPermission(c *gin.Context, permissionName string) bool {
	roleID, exists := c.Get("role_id")
	if !exists {
		return false
	}
	userID, _ := c.Get("user_id")
	cacheKey := fmt.Sprintf("%v:%v:%s", userID, roleID, permissionName)

	if cached, ok := permissionCache.Load(cacheKey); ok {
		entry := cached.(cachedPermissionResult)
		if time.Now().Before(entry.expiresAt) {
			return entry.allowed
		}
		permissionCache.Delete(cacheKey)
	}

	db := database.GetDB()
	var override struct{ Granted bool }
	ovr := db.
		Table("user_permissions").
		Select("user_permissions.granted").
		Joins("JOIN permissions ON user_permissions.permission_id = permissions.id").
		Where("user_permissions.user_id = ? AND permissions.name = ?", userID, permissionName).
		Limit(1).
		Scan(&override)

	var allowed bool
	if ovr.Error == nil && ovr.RowsAffected > 0 {
		allowed = override.Granted
	} else {
		var count int64
		db.
			Table("role_permissions").
			Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
			Where("role_permissions.role_id = ? AND permissions.name = ?", roleID, permissionName).
			Count(&count)
		allowed = count > 0
	}

	permissionCache.Store(cacheKey, cachedPermissionResult{allowed: allowed, expiresAt: time.Now().Add(permissionCacheTTL)})
	return allowed
}

// PermissionMiddleware checks if the user's role has the required permission
func PermissionMiddleware(permissionName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, exists := c.Get("role_id")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized: No role assigned"})
			c.Abort()
			return
		}
		userID, _ := c.Get("user_id")

		cacheKey := fmt.Sprintf("%v:%v:%s", userID, roleID, permissionName)
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
			db := database.GetDB()

			// 1. Per-user override takes precedence over the role.
			var override struct{ Granted bool }
			ovr := db.
				Table("user_permissions").
				Select("user_permissions.granted").
				Joins("JOIN permissions ON user_permissions.permission_id = permissions.id").
				Where("user_permissions.user_id = ? AND permissions.name = ?", userID, permissionName).
				Limit(1).
				Scan(&override)

			var allowed bool
			if ovr.Error == nil && ovr.RowsAffected > 0 {
				allowed = override.Granted
			} else {
				// 2. Fall back to the role's permissions.
				var count int64
				err := db.
					Table("role_permissions").
					Joins("JOIN permissions ON role_permissions.permission_id = permissions.id").
					Where("role_permissions.role_id = ? AND permissions.name = ?", roleID, permissionName).
					Count(&count).Error
				allowed = err == nil && count > 0
			}

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
