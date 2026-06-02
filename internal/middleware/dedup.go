package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// dedupWindow is how long an identical create is treated as a duplicate.
const dedupWindow = 5 * time.Second

type cachedResponse struct {
	status    int
	body      []byte
	expiresAt time.Time
}

var (
	dedupCache sync.Map // key -> cachedResponse
	dedupLocks sync.Map // key -> *sync.Mutex
)

type bufferedWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w bufferedWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

// Deduplicate prevents accidental double-submits (e.g. a double-clicked button or a
// retried request): an identical mutating request — same user, path, and body —
// repeated within a short window returns the first response instead of running the
// handler again. Identical requests are serialized so even simultaneous double-clicks
// only create one record.
func Deduplicate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only guard create-style requests.
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		userID, _ := c.Get("user_id")
		sum := sha256.Sum256(append([]byte(fmt.Sprintf("%v|%s|", userID, c.FullPath())), bodyBytes...))
		key := hex.EncodeToString(sum[:])

		// Serialize identical requests so concurrent double-clicks don't both run.
		muIface, _ := dedupLocks.LoadOrStore(key, &sync.Mutex{})
		mu := muIface.(*sync.Mutex)
		mu.Lock()
		defer mu.Unlock()

		if v, ok := dedupCache.Load(key); ok {
			entry := v.(cachedResponse)
			if time.Now().Before(entry.expiresAt) {
				c.Data(entry.status, "application/json; charset=utf-8", entry.body)
				c.Abort()
				return
			}
			dedupCache.Delete(key)
		}

		buf := &bytes.Buffer{}
		c.Writer = bufferedWriter{ResponseWriter: c.Writer, buf: buf}
		c.Next()

		// Cache only successful creations so a failed attempt can be retried.
		if status := c.Writer.Status(); status >= 200 && status < 300 {
			dedupCache.Store(key, cachedResponse{
				status:    status,
				body:      buf.Bytes(),
				expiresAt: time.Now().Add(dedupWindow),
			})
		}
	}
}
