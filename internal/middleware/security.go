package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	"github.com/unrolled/secure"
)

// SecureHeaders adds security headers to every response
func SecureHeaders() gin.HandlerFunc {
	secureMiddleware := secure.New(secure.Options{
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ContentSecurityPolicy: "default-src 'self'",
		// In production, set this to true
		// IsDevelopment: false,
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

// RateLimiter returns a rate limiting middleware
// Default: 100 requests per minute per IP
func RateLimiter() gin.HandlerFunc {
	// Define the rate (100 requests per minute)
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  100,
	}

	// Use in-memory store (use Redis for production with multiple instances)
	store := memory.NewStore()

	// Create the limiter
	instance := limiter.New(store, rate)

	// Return the middleware
	return mgin.NewMiddleware(instance)
}
