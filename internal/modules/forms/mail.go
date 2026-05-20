package forms

import (
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

func sendSMTPMail(toEmail string, subject string, body string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if host == "" || port == "" {
		return errors.New("SMTP_HOST and SMTP_PORT must be configured")
	}

	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if from == "" {
		from = "noreply@polygraph.local"
	}

	addr := host + ":" + port
	message := []byte("Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n" +
		body + "\r\n")

	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	pass := os.Getenv("SMTP_PASS")
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	if err := smtp.SendMail(addr, auth, from, []string{toEmail}, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
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
