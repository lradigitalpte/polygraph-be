package settings

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateLogoDataURL(t *testing.T) {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(append([]byte{}, pngMagic...), 0, 1, 2))
	jpg := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0})
	fakePNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<svg onload=alert(1)>"))
	huge := "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(append([]byte{}, pngMagic...), make([]byte, MaxLogoBytes)...))

	for name, tc := range map[string]struct {
		in    string
		valid bool
	}{
		"empty":      {"", true},
		"png":        {png, true},
		"jpeg":       {jpg, true},
		"svg":        {"data:image/svg+xml;base64,PHN2Zz4=", false},
		"fake png":   {fakePNG, false},
		"too large":  {huge, false},
		"bad base64": {"data:image/png;base64,!!!", false},
		"remote url": {"https://example.com/logo.png", false},
	} {
		if err := validateLogoDataURL(tc.in); (err == nil) != tc.valid {
			t.Errorf("%s: valid=%v, err=%v", name, tc.valid, err)
		}
	}
}

func TestNormalizeWebsite(t *testing.T) {
	for in, want := range map[string]string{
		"":                        "",
		"www.example.com":         "https://www.example.com",
		"http://example.ae/about": "http://example.ae/about",
		"  https://example.com  ": "https://example.com",
	} {
		got, err := normalizeWebsite(in)
		if err != nil || got != want {
			t.Errorf("normalizeWebsite(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "not a site", "ftp://example.com", "https://" + strings.Repeat("a", 260) + ".com"} {
		if _, err := normalizeWebsite(bad); err == nil {
			t.Errorf("normalizeWebsite(%q) expected error", bad)
		}
	}
}
