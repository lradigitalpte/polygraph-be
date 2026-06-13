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

	// Strip CR/LF from header-bound values to prevent SMTP header/command
	// injection (gosec G707). The body may legitimately contain newlines.
	subject = sanitizeHeader(subject)
	toEmail = sanitizeHeader(toEmail)
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

	// #nosec G707 -- subject and recipient are CRLF-stripped via sanitizeHeader;
	// the body sits after the header separator and cannot inject SMTP headers.
	if err := smtp.SendMail(addr, auth, from, []string{toEmail}, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

// sanitizeHeader removes CR/LF so attacker-influenced values cannot inject
// additional SMTP headers or commands.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
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
