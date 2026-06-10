package middleware

import (
	"os"
	"strings"
)

// AllowedOrigins returns CORS origins from env plus local dev defaults.
//
// Set CORS_ORIGINS to a comma-separated list (e.g. https://app.vercel.app,https://www.example.com).
// FRONTEND_URL and APP_PUBLIC_URL are also accepted as single origins when set.
func AllowedOrigins() []string {
	seen := make(map[string]struct{})
	var origins []string

	add := func(origin string) {
		origin = strings.TrimSpace(origin)
		origin = strings.TrimRight(origin, "/")
		if origin == "" {
			return
		}
		if _, ok := seen[origin]; ok {
			return
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}

	for _, key := range []string{"CORS_ORIGINS", "FRONTEND_URL", "APP_PUBLIC_URL"} {
		raw := os.Getenv(key)
		if raw == "" {
			continue
		}
		if key == "CORS_ORIGINS" {
			for _, part := range strings.Split(raw, ",") {
				add(part)
			}
			continue
		}
		add(raw)
	}

	// Allow local dev servers only outside of production.
	// In production, all allowed origins must be explicitly set via env vars
	// (CORS_ORIGINS, FRONTEND_URL, or APP_PUBLIC_URL) — no implicit localhost.
	if os.Getenv("APP_ENV") != "production" {
		add("http://localhost:3000")
		add("http://localhost:3001")
	}

	return origins
}
