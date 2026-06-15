package appointments

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"time"
)

// DocumentShare is a tokenised link that delivers an already-stored document
// (a ClientDocument or SubjectDocument) to the client by email. Unlike form
// requests — which collect answers — this is one-way: the recipient only views
// or downloads the file. Status moves sent -> viewed when the link is opened.
type DocumentShare struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time  `json:"created_at"`
	Token          string     `gorm:"size:64;uniqueIndex;not null" json:"token"`
	ClientID       uint       `gorm:"index;not null" json:"client_id"`
	SubjectID      *uint      `gorm:"index" json:"subject_id,omitempty"`
	Scope          string     `gorm:"size:20;not null" json:"scope"` // client | subject
	DocumentID     uint       `gorm:"not null" json:"document_id"`
	Name           string     `gorm:"size:255;not null" json:"name"`
	URL            string     `gorm:"size:500" json:"url,omitempty"`
	RecipientEmail string     `gorm:"size:255;not null" json:"recipient_email"`
	RecipientName  string     `gorm:"size:255" json:"recipient_name,omitempty"`
	Status         string     `gorm:"size:20;default:'sent'" json:"status"` // sent | viewed
	SentAt         time.Time  `json:"sent_at"`
	ViewedAt       *time.Time `json:"viewed_at,omitempty"`
	ExpiresAt      time.Time  `gorm:"index" json:"expires_at"`
	SentByEmail    string     `gorm:"size:255" json:"sent_by_email,omitempty"`
}

func generateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// publicShareURL builds the public link the client opens to view the document.
// Mirrors the resolution order used by the forms/intake modules.
func publicShareURL(token string) string {
	base := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if base == "" {
		base = "http://localhost:3001"
	}
	return strings.TrimRight(base, "/") + "/shared/" + token
}
