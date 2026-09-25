package settings

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
)

// MaxLogoBytes caps the decoded logo size; it is stored inline and sent on public pages.
const MaxLogoBytes = 500 * 1024

var (
	pngMagic  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	jpegMagic = []byte{0xff, 0xd8, 0xff}
)

// validateLogoDataURL accepts "" (no logo) or a base64 PNG/JPEG data URL whose bytes
// really are that format. Only PNG and JPEG are allowed because gofpdf can embed them.
func validateLogoDataURL(logo string) error {
	if logo == "" {
		return nil
	}
	var mime, payload string
	switch {
	case strings.HasPrefix(logo, "data:image/png;base64,"):
		mime, payload = "png", strings.TrimPrefix(logo, "data:image/png;base64,")
	case strings.HasPrefix(logo, "data:image/jpeg;base64,"):
		mime, payload = "jpeg", strings.TrimPrefix(logo, "data:image/jpeg;base64,")
	default:
		return errors.New("logo must be a PNG or JPEG image")
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > MaxLogoBytes+3 {
		return errors.New("logo must be 500 KB or smaller")
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return errors.New("logo image data is invalid")
	}
	if len(raw) > MaxLogoBytes {
		return errors.New("logo must be 500 KB or smaller")
	}
	if (mime == "png" && !bytes.HasPrefix(raw, pngMagic)) || (mime == "jpeg" && !bytes.HasPrefix(raw, jpegMagic)) {
		return errors.New("logo file does not match its image type")
	}
	return nil
}

// normalizeWebsite accepts "" or an http(s) URL; a bare domain gets https:// prepended.
func normalizeWebsite(raw string) (string, error) {
	site := strings.TrimSpace(raw)
	if site == "" {
		return "", nil
	}
	if !strings.Contains(site, "://") {
		site = "https://" + site
	}
	u, err := url.Parse(site)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.Contains(u.Host, ".") {
		return "", errors.New("website must be a valid web address, e.g. www.example.com")
	}
	if len(site) > 255 {
		return "", errors.New("website must be 255 characters or fewer")
	}
	return site, nil
}
