package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	"github.com/unrolled/secure"
)

// SecureHeaders adds security headers to every response.
func SecureHeaders() gin.HandlerFunc {
	secureMiddleware := secure.New(secure.Options{
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ContentSecurityPolicy: "default-src 'self'",
	})

	return func(c *gin.Context) {
		err := secureMiddleware.Process(c.Writer, c.Request)
		if err != nil {
			c.Abort()
			return
		}
		c.Next()
	}
}

// RateLimiter returns a general-purpose rate limiter for authenticated API routes.
// Allows 100 requests per minute per real client IP — enough for normal dashboard usage.
//
// WHY: Without this an attacker can hammer your API thousands of times per second,
// scraping data or brute-forcing IDs. 100/min is generous for a real user but brutal
// for automated attacks.
//
// WithTrustForwardHeader(true): behind a reverse proxy (Railway, Vercel, nginx) the
// raw RemoteAddr is the *proxy's* IP, not the client's — and platforms like Railway
// rotate across several edge IPs, so every request looked like a brand-new client and
// the counter never accumulated. Trusting X-Forwarded-For keys the limiter on the real
// client IP so the limit is actually enforced.
func RateLimiter() gin.HandlerFunc {
	store := memory.NewStore()
	instance := limiter.New(store, limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  100,
	}, limiter.WithTrustForwardHeader(true))
	return mgin.NewMiddleware(instance)
}

// StrictRateLimiter returns a tight rate limiter for sensitive unauthenticated routes
// such as public form submissions and any future auth-adjacent endpoints.
// Allows 10 requests per minute per real client IP.
//
// WHY: Public endpoints don't require a session so they're the easiest targets.
// 10/min stops automated abuse (form spam, enumeration) while being invisible to
// real users who submit a form once every few seconds at most.
//
// NOTE: in-memory store is per-instance. If you scale the backend to more than one
// Railway replica, move both limiters to a shared Redis store (ulule/limiter has a
// redis driver) — otherwise each replica counts independently and the effective limit
// becomes limit × replicas.
func StrictRateLimiter() gin.HandlerFunc {
	store := memory.NewStore()
	instance := limiter.New(store, limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  10,
	}, limiter.WithTrustForwardHeader(true))
	return mgin.NewMiddleware(instance)
}
