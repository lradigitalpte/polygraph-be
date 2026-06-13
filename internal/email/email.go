// Package email is the single SMTP sender for the app. It sends a
// multipart/alternative message (plain text + branded HTML with the logo) and
// uses connection timeouts so a stuck SMTP server fails fast instead of hanging
// the request (which otherwise surfaces as a 502 behind the Railway proxy).
package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"net"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	dialTimeout = 10 * time.Second
	opDeadline  = 30 * time.Second
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// sanitizeHeader strips CR/LF so values cannot inject extra SMTP headers.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// logoURL resolves the public URL of the logo for use in HTML emails.
func logoURL() string {
	base := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if base == "" {
		base = "https://polygraph-fe-web.vercel.app"
	}
	return strings.TrimRight(base, "/") + "/logo.png"
}

// textToHTML escapes a plain-text body, linkifies URLs, and converts newlines to <br>.
func textToHTML(body string) string {
	escaped := html.EscapeString(body)
	linked := urlRe.ReplaceAllStringFunc(escaped, func(u string) string {
		return fmt.Sprintf(`<a href="%s" style="color:#c0392b">%s</a>`, u, u)
	})
	return strings.ReplaceAll(linked, "\n", "<br>")
}

// htmlBody wraps the message body in a branded template with the logo on a dark banner.
func htmlBody(textBody string) string {
	return `<!DOCTYPE html><html><body style="margin:0;padding:24px;background:#f4f4f5">` +
		`<div style="max-width:600px;margin:0 auto;font-family:Arial,Helvetica,sans-serif;color:#1a1a1a">` +
		`<div style="background:#000;padding:18px;text-align:center;border-radius:8px 8px 0 0">` +
		`<img src="` + logoURL() + `" alt="Polygraph Forensic System" style="height:40px;width:auto" />` +
		`</div>` +
		`<div style="padding:24px;background:#ffffff;border:1px solid #e5e5e5;border-top:none;line-height:1.6;font-size:15px">` +
		textToHTML(textBody) +
		`</div>` +
		`<p style="text-align:center;color:#999;font-size:12px;margin:16px 0">Polygraph Forensic System</p>` +
		`</div></body></html>`
}

// Send delivers a branded email (plain-text + HTML alternative) to one recipient.
func Send(toEmail, subject, textBody string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if host == "" || port == "" {
		return errors.New("SMTP_HOST and SMTP_PORT must be configured")
	}
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if from == "" {
		from = strings.TrimSpace(os.Getenv("FROM_ADDRESS"))
	}
	if from == "" {
		from = "noreply@polygraph.ae"
	}

	subject = sanitizeHeader(subject)
	toEmail = sanitizeHeader(toEmail)
	from = sanitizeHeader(from)

	boundary := fmt.Sprintf("bnd_%d", time.Now().UnixNano())
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", toEmail)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(textBody + "\r\n")
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(htmlBody(textBody) + "\r\n")
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	msg := []byte(b.String())

	addr := net.JoinHostPort(host, port)
	// #nosec G704 -- the SMTP host/port come from operator-set env (SMTP_HOST/PORT),
	// not user input, so this is not attacker-controlled SSRF.
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(opDeadline))

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if err := c.Hello("polygraph.ae"); err != nil {
		return fmt.Errorf("smtp helo: %w", err)
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if user := strings.TrimSpace(os.Getenv("SMTP_USER")); user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, os.Getenv("SMTP_PASS"), host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	// #nosec G707 -- from and toEmail are CRLF-stripped via sanitizeHeader above,
	// so they cannot inject additional SMTP headers or commands.
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(toEmail); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return c.Quit()
}
