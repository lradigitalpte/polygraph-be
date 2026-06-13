package forms

import (
	"os"
	"strings"

	"my-app/internal/email"
)

func sendSMTPMail(toEmail string, subject string, body string) error {
	return email.Send(toEmail, subject, body)
}

func publicFormURL(token string) string {
	base := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if base == "" {
		base = "http://localhost:3001"
	}
	return strings.TrimRight(base, "/") + "/forms/fill/" + token
}
