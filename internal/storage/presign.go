package storage

import (
	"context"
	"strings"
)

// SignedURLForStored converts a stored object URL into a short-lived, presigned
// URL so a private S3 object can be viewed/downloaded directly by the browser.
//
// Documents save the canonical upload URL (e.g.
// https://bucket.s3.region.amazonaws.com/key). For a private bucket that URL is
// not directly fetchable, so we extract the key and presign it at read time.
// Non-S3 values (local-dev "/uploads/..." paths) and anything we can't parse are
// returned unchanged, and presign failures fall back to the original URL.
func SignedURLForStored(ctx context.Context, store Storage, storedURL string) string {
	if store == nil || storedURL == "" {
		return storedURL
	}
	const marker = ".amazonaws.com/"
	idx := strings.Index(storedURL, marker)
	if idx == -1 {
		return storedURL
	}
	key := storedURL[idx+len(marker):]
	if key == "" {
		return storedURL
	}
	signed, err := store.GetSignedURL(ctx, key)
	if err != nil || signed == "" {
		return storedURL
	}
	return signed
}
