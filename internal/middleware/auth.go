package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"my-app/internal/database"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/rbac"
)

// isEmailAllowed checks whether an email is permitted to access the system.
//
// WHY: Without this check, anyone who registers on the frontend gets auto-created
// in the backend DB with a default "User" role — a foothold in your system.
// This gate ensures only emails from approved domains (set via ALLOWED_EMAIL_DOMAINS)
// or explicitly invited users can get a backend account.
//
// Set ALLOWED_EMAIL_DOMAINS=yourdomain.com,partner.com in your environment.
// Leave it empty to allow all domains (open — not recommended for production).
func isEmailAllowed(email string) bool {
	domains := os.Getenv("ALLOWED_EMAIL_DOMAINS")
	if domains == "" {
		// No restriction configured — allow all (backwards-compatible default).
		// Set ALLOWED_EMAIL_DOMAINS in production to lock this down.
		return true
	}
	emailLower := strings.ToLower(strings.TrimSpace(email))
	atIdx := strings.LastIndex(emailLower, "@")
	if atIdx < 0 {
		return false
	}
	emailDomain := emailLower[atIdx+1:]
	for _, d := range strings.Split(domains, ",") {
		if strings.TrimSpace(strings.ToLower(d)) == emailDomain {
			return true
		}
	}
	return false
}

type cachedAuthUser struct {
	user      auth.User
	expiresAt time.Time
}

var authUserCache sync.Map
var authSingleflight singleflight.Group

const authUserCacheTTL = 3 * time.Second

// Validated session tokens are cached briefly so we don't hit the session table on
// every single request. Kept short so logout / expiry take effect almost immediately.
type cachedSession struct {
	email      string
	cacheUntil time.Time
}

var sessionCache sync.Map

const sessionCacheTTL = 5 * time.Second

// extractSessionToken pulls the raw better-auth session token from a Bearer header
// or a session cookie. Signed cookies are "<token>.<signature>"; the DB stores the
// raw token, so we take the part before the signature.
func extractSessionToken(c *gin.Context) string {
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return stripCookieSignature(strings.TrimSpace(parts[1]))
		}
	}
	for _, name := range []string{
		"better-auth.session_token",
		"__Secure-better-auth.session_token",
		"session_token",
	} {
		if cookie, err := c.Cookie(name); err == nil && cookie != "" {
			return stripCookieSignature(cookie)
		}
	}
	return ""
}

func stripCookieSignature(v string) string {
	if i := strings.IndexByte(v, '.'); i > 0 {
		return v[:i]
	}
	return v
}

// validateSession verifies a better-auth session token against the shared session
// table and returns the authenticated user's email. Missing or expired sessions are
// rejected — the expiry check is what enforces inactivity logout server-side. This is
// the ONLY source of identity; client-supplied identity headers are never trusted.
func validateSession(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	if cached, ok := sessionCache.Load(token); ok {
		entry := cached.(cachedSession)
		if time.Now().Before(entry.cacheUntil) {
			return entry.email, entry.email != ""
		}
		sessionCache.Delete(token)
	}

	var row struct {
		Email     string
		ExpiresAt time.Time
	}
	err := database.GetDB().
		Table("session").
		Select(`"user".email AS email, session."expiresAt" AS expires_at`).
		Joins(`JOIN "user" ON "user".id = session."userId"`).
		Where("session.token = ?", token).
		Limit(1).
		Scan(&row).Error

	if err != nil || row.Email == "" || time.Now().After(row.ExpiresAt) {
		// Cache the negative result briefly to blunt token-guessing / replay floods.
		sessionCache.Store(token, cachedSession{email: "", cacheUntil: time.Now().Add(sessionCacheTTL)})
		return "", false
	}
	sessionCache.Store(token, cachedSession{email: row.Email, cacheUntil: time.Now().Add(sessionCacheTTL)})
	return row.Email, true
}

// AuthMiddleware authenticates the request by validating the better-auth session
// token against the shared session table. Identity comes only from a verified
// session — never from a client-supplied header.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractSessionToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		email, ok := validateSession(token)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired session"})
			c.Abort()
			return
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
					// Not found — auto-create (Just-In-Time provisioning).
					// Gate on domain allowlist before creating the account so
					// arbitrary registrations can't get a backend foothold.
					if !isEmailAllowed(email) {
						return nil, fmt.Errorf("email domain not allowed: %s", email)
					}
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
