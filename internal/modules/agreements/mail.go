package agreements

import (
	"fmt"
	"os"
	"strings"

	"my-app/internal/email"
)

// Outbound mail senders are package variables so tests can stub them.
var (
	sendMail               = email.Send
	sendMailWithAttachment = email.SendWithAttachment
)

func publicAgreementURL(token string) string {
	base := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if base == "" {
		base = "http://localhost:3001"
	}
	return strings.TrimRight(base, "/") + "/agreements/" + token
}

func itemTitles(items []AgreementRequestItem) string {
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, "  • "+it.Title)
	}
	return strings.Join(lines, "\n")
}

func sendRequestEmail(req *AgreementRequest, orgName string, reminder bool) error {
	subject := fmt.Sprintf("Please review and sign: your agreements with %s", orgName)
	intro := "Before your session, please read and sign the following agreements:"
	if reminder {
		subject = fmt.Sprintf("Reminder: please sign your agreements with %s", orgName)
		intro = "This is a reminder to read and sign the following agreements before your session:"
	}
	body := fmt.Sprintf(
		"Hello %s,\n\n%s\n\n%s\n\nOpen the secure link below to review and sign:\n%s\n\nThis link expires on %s.\n\nThank you,\n%s",
		req.RecipientName,
		intro,
		itemTitles(req.Items),
		publicAgreementURL(req.Token),
		req.ExpiresAt.Format("January 2, 2006"),
		orgName,
	)
	return sendMail(req.RecipientEmail, subject, body)
}

// notifyStaff tells the person who sent the request that the client responded.
// Best effort: a failed notification never fails the client's signing.
func notifyStaff(req *AgreementRequest, outcome string) {
	to := strings.TrimSpace(req.SentByEmail)
	if to == "" {
		return
	}
	subject := fmt.Sprintf("Agreements %s by %s", outcome, req.RecipientName)
	body := fmt.Sprintf(
		"%s has %s the following agreements:\n\n%s\n",
		req.RecipientName, outcome, itemTitles(req.Items),
	)
	if outcome == StatusDeclined && req.DeclineReason != "" {
		body += "\nReason given: " + req.DeclineReason + "\n"
	}
	body += "\nOpen the client's Approvals tab in the dashboard for details."
	_ = sendMail(to, subject, body)
}

// sendSignedCopyEmail sends the client their signed PDF. Best effort, like notifyStaff.
func sendSignedCopyEmail(req *AgreementRequest, orgName string, pdfBytes []byte) {
	subject := fmt.Sprintf("Your signed agreements with %s", orgName)
	body := fmt.Sprintf(
		"Hello %s,\n\nThank you for signing the following agreements:\n\n%s\n\nA PDF copy is attached for your records. You can also download it again from your signing link:\n%s\n\nThank you,\n%s",
		req.RecipientName,
		itemTitles(req.Items),
		publicAgreementURL(req.Token),
		orgName,
	)
	_ = sendMailWithAttachment(req.RecipientEmail, subject, body, signedPDFFilename(req.ID), pdfBytes)
}
