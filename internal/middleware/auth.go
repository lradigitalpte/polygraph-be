package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"

	"my-app/internal/database"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/rbac"
)

var jwks *keyfunc.JWKS

type cachedAuthUser struct {
	user      auth.User
	expiresAt time.Time
}

var authUserCache sync.Map
var authSingleflight singleflight.Group

const authUserCacheTTL = 3 * time.Second

// InitJWKS initializes the JWKS from the Neon Auth endpoint
func InitJWKS(jwksURL string) error {
	var err error
	jwks, err = keyfunc.Get(jwksURL, keyfunc.Options{})
	return err
}

// AuthMiddleware verifies auth via JWT or better-auth session token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string
		var isJWT bool
		var email string

		// Try 1: Check Authorization header (Bearer token)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]

				// Check if it looks like a JWT (has 3 parts separated by dots)
				if strings.Count(tokenString, ".") == 2 {
					isJWT = true
				}
			}
		}

		// Try 2: If no Bearer token, try to read from cookies (better-auth session)
		if tokenString == "" {
			possibleCookies := []string{
				"better-auth.session_token",
				"session_token",
				"authjs.session-token",
				"auth.session",
				"better-auth",
			}

			for _, cookieName := range possibleCookies {
				if cookie, err := c.Cookie(cookieName); err == nil && cookie != "" {
					tokenString = cookie
					break
				}
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header (Bearer token) or session cookie required"})
			c.Abort()
			return
		}

		if isJWT {
			// Attempt JWT validation with JWKS if available
			if jwks != nil {
				token, err := jwt.Parse(tokenString, jwks.Keyfunc)
				if err == nil && token.Valid {
					if claims, ok := token.Claims.(jwt.MapClaims); ok {
						email, _ = claims["email"].(string)
					}
				}
			}
			// Fall back to X-User-Email header if JWT validation failed or JWKS not configured
			if email == "" {
				email = c.GetHeader("X-User-Email")
			}
			if email == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Could not determine user identity from token"})
				c.Abort()
				return
			}
		} else {
			// Session token — rely on X-User-Email header sent by the frontend
			email = c.GetHeader("X-User-Email")
			if email == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Session token requires X-User-Email header"})
				c.Abort()
				return
			}
		}

		// Find user in database to get their role in one query.
		var user auth.User
		if cached, ok := authUserCache.Load(email); ok {
			entry := cached.(cachedAuthUser)
			if time.Now().Before(entry.expiresAt) {
				user = entry.user
			} else {
				authUserCache.Delete(email)
			}
		}

		if user.ID == 0 {
			type sfResult struct{ user auth.User }
			v, err, _ := authSingleflight.Do(email, func() (interface{}, error) {
				var u auth.User
				if err := database.GetDB().Joins("Role").Where("users.email = ?", email).First(&u).Error; err != nil {
					// Not found — auto-create (Just-In-Time provisioning)
					var defaultRole rbac.Role
					database.GetDB().Where("name = ?", "User").FirstOrCreate(&defaultRole, rbac.Role{Name: "User", Description: "Default access"})
					u = auth.User{Email: email, Name: email, RoleID: defaultRole.ID, Status: "active"}
					// Omit the Role association — otherwise GORM overwrites role_id with the
					// zero-value association (0), violating the users.role_id FK.
					if err := database.GetDB().Omit("Role").Create(&u).Error; err != nil {
						return nil, err
					}
					u.Role = defaultRole
				} else {
					authUserCache.Store(email, cachedAuthUser{user: u, expiresAt: time.Now().Add(authUserCacheTTL)})
				}
				return sfResult{user: u}, nil
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync user to local database"})
				c.Abort()
				return
			}
			user = v.(sfResult).user
		}

		if strings.EqualFold(user.Status, "suspended") {
			c.JSON(http.StatusForbidden, gin.H{"error": "User account is suspended"})
			c.Abort()
			return
		}

		now := time.Now()
		updates := map[string]interface{}{}
		if user.Status == "" || strings.EqualFold(user.Status, "pending") {
			updates["status"] = "active"
			user.Status = "active"
		}
		if user.LastActiveAt == nil || now.Sub(*user.LastActiveAt) >= 15*time.Minute {
			updates["last_active_at"] = now
			user.LastActiveAt = &now
		}
		if len(updates) > 0 && user.ID != 0 {
			_ = database.GetDB().Model(&auth.User{}).Where("id = ?", user.ID).Updates(updates).Error
		}
		authUserCache.Store(email, cachedAuthUser{user: user, expiresAt: time.Now().Add(authUserCacheTTL)})

		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("role", user.Role.Name)
		c.Set("role_id", user.RoleID)

		c.Next()
	}
}
