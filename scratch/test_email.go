// Email connectivity test script.
// Usage:
//
//	MAIL_HOST=polygraph.ae MAIL_USER=admin@polygraph.ae MAIL_PASS=<password> go run scratch/test_email.go
//
// Required env vars:
//
//	MAIL_HOST  - mail server hostname (e.g. polygraph.ae)
//	MAIL_USER  - IMAP/SMTP username (full email address)
//	MAIL_PASS  - account password
//
// Optional env vars (defaults shown):
//
//	IMAP_PORT  - IMAP TLS port  (default: 993)
//	SMTP_PORT  - SMTP TLS port  (default: 465)

package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"time"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: environment variable %s is required\n", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// dialTLS opens a TLS connection and returns it, or an error.
func dialTLS(host, port string) (net.Conn, error) {
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	return tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
}

// testIMAP connects to the IMAP server, reads the greeting, and logs in.
func testIMAP(host, port, user, pass string) error {
	fmt.Printf("[IMAP] Connecting to %s:%s ...\n", host, port)

	conn, err := dialTLS(host, port)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}
	defer conn.Close()
	fmt.Println("[IMAP] TLS handshake OK")

	// Read server greeting (IMAP sends an untagged * OK line on connect).
	buf := make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("reading greeting: %w", err)
	}
	fmt.Printf("[IMAP] Server greeting: %s", buf[:n])

	// Send LOGIN command.
	tag := "a001"
	cmd := fmt.Sprintf("%s LOGIN %q %q\r\n", tag, user, pass)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprint(conn, cmd); err != nil {
		return fmt.Errorf("sending LOGIN: %w", err)
	}

	// Read LOGIN response.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err = conn.Read(buf)
	if err != nil {
		return fmt.Errorf("reading LOGIN response: %w", err)
	}
	resp := string(buf[:n])
	fmt.Printf("[IMAP] LOGIN response: %s", resp)

	// Send LOGOUT.
	fmt.Fprintf(conn, "a002 LOGOUT\r\n")

	if len(resp) >= len(tag)+4 && resp[len(tag)+1:len(tag)+3] == "OK" {
		fmt.Println("[IMAP] LOGIN OK")
		return nil
	}
	return fmt.Errorf("unexpected LOGIN response: %s", resp)
}

// testSMTP connects to the SMTP server over TLS (implicit TLS on port 465)
// and authenticates using AUTH LOGIN / PLAIN.
func testSMTP(host, port, user, pass string) error {
	fmt.Printf("[SMTP] Connecting to %s:%s ...\n", host, port)

	conn, err := dialTLS(host, port)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}
	defer conn.Close()
	fmt.Println("[SMTP] TLS handshake OK")

	addr := net.JoinHostPort(host, port)
	smtpClient, err := smtp.NewClient(conn, addr)
	if err != nil {
		return fmt.Errorf("smtp.NewClient: %w", err)
	}
	defer smtpClient.Quit()

	auth := smtp.PlainAuth("", user, pass, host)
	if err := smtpClient.Auth(auth); err != nil {
		return fmt.Errorf("AUTH failed: %w", err)
	}

	fmt.Println("[SMTP] AUTH OK")
	return nil
}

func main() {
	host := mustEnv("MAIL_HOST")
	user := mustEnv("MAIL_USER")
	pass := mustEnv("MAIL_PASS")
	imapPort := envOr("IMAP_PORT", "993")
	smtpPort := envOr("SMTP_PORT", "465")

	fmt.Printf("=== Email Connectivity Test ===\nHost : %s\nUser : %s\n\n", host, user)

	var failed bool

	if err := testIMAP(host, imapPort, user, pass); err != nil {
		fmt.Printf("[IMAP] FAILED: %v\n", err)
		failed = true
	} else {
		fmt.Println("[IMAP] PASSED")
	}

	fmt.Println()

	if err := testSMTP(host, smtpPort, user, pass); err != nil {
		fmt.Printf("[SMTP] FAILED: %v\n", err)
		failed = true
	} else {
		fmt.Println("[SMTP] PASSED")
	}

	fmt.Println()
	if failed {
		fmt.Println("Result: one or more checks FAILED")
		os.Exit(1)
	}
	fmt.Println("Result: all checks PASSED - safe to configure in production")
}
