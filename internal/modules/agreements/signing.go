package agreements

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// MaxSignatureBytes caps the decoded signature PNG drawn on the signing page.
const MaxSignatureBytes = 300 * 1024

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// contentHash fingerprints exactly what the client is asked to agree to. It is fixed at
// send time and re-checked at signing, so any change to the stored terms is detectable.
func contentHash(items []AgreementRequestItem) string {
	type hashed struct {
		TemplateID uint   `json:"template_id"`
		Version    int    `json:"version"`
		Title      string `json:"title"`
		Body       string `json:"body"`
	}
	rows := make([]hashed, 0, len(items))
	for _, it := range items {
		rows = append(rows, hashed{it.TemplateID, it.TemplateVersion, it.Title, it.BodyHTML})
	}
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// signatureHash binds the agreed content to who signed, how, and when.
func signatureHash(contentHash, signedName string, signatureBytes []byte, signedAt time.Time) string {
	sigSum := sha256.Sum256(signatureBytes)
	payload := strings.Join([]string{
		contentHash,
		signedName,
		hex.EncodeToString(sigSum[:]),
		signedAt.UTC().Format(time.RFC3339Nano),
	}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// decodeSignature validates a drawn signature: a base64 PNG data URL within size limits.
func decodeSignature(dataURL string) ([]byte, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("please draw your signature")
	}
	payload := strings.TrimPrefix(dataURL, prefix)
	if base64.StdEncoding.DecodedLen(len(payload)) > MaxSignatureBytes+3 {
		return nil, errors.New("signature image is too large")
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || !bytes.HasPrefix(raw, pngMagic) {
		return nil, errors.New("signature image is invalid")
	}
	if len(raw) > MaxSignatureBytes {
		return nil, errors.New("signature image is too large")
	}
	return raw, nil
}

// normalizeSignedName collapses whitespace and checks the typed legal name.
func normalizeSignedName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if len([]rune(name)) < 2 {
		return "", errors.New("please type your full name")
	}
	if len(name) > 255 {
		return "", errors.New("name must be 255 characters or fewer")
	}
	return name, nil
}

func isOrganizationClientType(clientType string) bool {
	t := strings.ToLower(strings.TrimSpace(clientType))
	return t == "corporate" || t == "law firm" || t == "lawfirm"
}
