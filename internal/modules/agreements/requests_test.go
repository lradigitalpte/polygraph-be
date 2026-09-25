package agreements

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/modules/appointments"
	"my-app/internal/modules/exams"
	"my-app/internal/modules/settings"
	"my-app/internal/modules/subjects"
)

type sentMail struct{ to, subject, body, attachment string }

// memStorage is an in-memory storage.Storage for Document Vault uploads.
type memStorage struct{ files map[string][]byte }

func (m *memStorage) UploadFile(_ context.Context, key string, body io.Reader, _ string) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	m.files[key] = data
	return "mem://" + key, nil
}
func (m *memStorage) DeleteFile(_ context.Context, key string) error {
	delete(m.files, key)
	return nil
}
func (m *memStorage) GetSignedURL(_ context.Context, key string) (string, error) {
	return "mem://" + key, nil
}

func setupService(t *testing.T) (*Service, *[]sentMail) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	dbName := fmt.Sprintf("file:agreements_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&subjects.Subject{}, &appointments.Client{}, &appointments.Appointment{},
		&exams.ExamType{}, &settings.OrganizationSettings{}, &appointments.ClientDocument{},
		&AgreementTemplate{}, &AgreementRequest{}, &AgreementRequestItem{},
	))
	require.NoError(t, db.Create(&settings.OrganizationSettings{ID: 1, Name: "Test Lab", Website: "https://lab.test"}).Error)

	mails := &[]sentMail{}
	orig, origAttach := sendMail, sendMailWithAttachment
	sendMail = func(to, subject, body string) error {
		*mails = append(*mails, sentMail{to: to, subject: subject, body: body})
		return nil
	}
	sendMailWithAttachment = func(to, subject, body, name string, _ []byte) error {
		*mails = append(*mails, sentMail{to: to, subject: subject, body: body, attachment: name})
		return nil
	}
	t.Cleanup(func() { sendMail, sendMailWithAttachment = orig, origAttach })
	return &Service{db: db, storage: &memStorage{files: map[string][]byte{}}}, mails
}

var clientSeq int

func seedClient(t *testing.T, s *Service, clientType string) appointments.Client {
	t.Helper()
	clientSeq++
	c := appointments.Client{
		Name:       "Jane Doe",
		ClientType: clientType,
		Email:      fmt.Sprintf("jane-%d@example.com", clientSeq),
	}
	require.NoError(t, s.db.Create(&c).Error)
	return c
}

func seedTemplates(t *testing.T, s *Service) []AgreementTemplate {
	t.Helper()
	pay, err := s.CreateTemplate(TemplateInput{Title: "Payment", Kind: KindPayment, BodyHTML: "<p>Pay first.</p>"}, "admin@lab.test")
	require.NoError(t, err)
	cancel, err := s.CreateTemplate(TemplateInput{Title: "Cancellation", Kind: KindCancellation, BodyHTML: "<p>48h notice.</p>"}, "admin@lab.test")
	require.NoError(t, err)
	return []AgreementTemplate{*pay, *cancel}
}

// testSignature is a real (tiny) PNG so the PDF can embed it.
func testSignature() string {
	img := image.NewNRGBA(image.Rect(0, 0, 60, 20))
	for x := 5; x < 55; x++ {
		img.Set(x, 10+(x%7)-3, color.NRGBA{A: 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func itemIDs(items []PublicItem) []uint {
	ids := make([]uint, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func sendBoth(t *testing.T, s *Service, clientID uint, templates []AgreementTemplate) *SendResult {
	t.Helper()
	res, err := s.SendRequest(clientID, SendInput{TemplateIDs: []uint{templates[0].ID, templates[1].ID}}, "staff@lab.test")
	require.NoError(t, err)
	return res
}

func TestSendViewSignFlow(t *testing.T) {
	s, mails := setupService(t)
	client := seedClient(t, s, "Individual")
	templates := seedTemplates(t, s)

	res := sendBoth(t, s, client.ID, templates)
	assert.Empty(t, res.EmailError)
	require.Len(t, *mails, 1)
	assert.Equal(t, client.Email, (*mails)[0].to)
	assert.Contains(t, (*mails)[0].body, res.Link)
	token := res.Request.Token

	view, err := s.GetPublicView(token)
	require.NoError(t, err)
	assert.Equal(t, StatusViewed, view.Status)
	assert.Equal(t, "Test Lab", view.Organization.Name)
	require.Len(t, view.Items, 2)
	assert.Equal(t, "Payment", view.Items[0].Title)

	// Editing the template after sending must not change the client's copy.
	_, err = s.UpdateTemplate(fmt.Sprint(templates[0].ID), TemplateInput{Title: "Payment", Kind: KindPayment, BodyHTML: "<p>New terms.</p>"}, "admin@lab.test")
	require.NoError(t, err)
	view, err = s.GetPublicView(token)
	require.NoError(t, err)
	assert.Equal(t, "<p>Pay first.</p>", view.Items[0].BodyHTML)

	// Every agreement must be ticked.
	_, err = s.Sign(token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items[:1])}, "1.2.3.4", "ua")
	assert.ErrorContains(t, err, "Cancellation")

	_, err = s.Sign(token, SignInput{SignedName: " ", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "1.2.3.4", "ua")
	assert.ErrorContains(t, err, "full name")

	_, err = s.Sign(token, SignInput{SignedName: "Jane Doe", SignatureDataURL: "data:image/png;base64,bm90IGEgcG5n", AcceptedItemIDs: itemIDs(view.Items)}, "1.2.3.4", "ua")
	assert.ErrorContains(t, err, "signature")

	signed, err := s.Sign(token, SignInput{SignedName: "  Jane   Doe ", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "1.2.3.4", "ua")
	require.NoError(t, err)
	assert.Equal(t, StatusSigned, signed.Status)
	assert.Equal(t, "Jane Doe", signed.SignedName)
	for _, it := range signed.Items {
		assert.NotNil(t, it.AcceptedAt)
	}

	var stored AgreementRequest
	require.NoError(t, s.db.First(&stored, res.Request.ID).Error)
	assert.Equal(t, "1.2.3.4", stored.SignerIP)
	assert.Len(t, stored.SignatureHash, 64)

	// The client gets their signed PDF, and the staff member who sent it is notified.
	require.Len(t, *mails, 3)
	assert.Equal(t, client.Email, (*mails)[1].to)
	assert.Equal(t, signedPDFFilename(res.Request.ID), (*mails)[1].attachment)
	assert.Equal(t, "staff@lab.test", (*mails)[2].to)

	// A second submission is rejected.
	_, err = s.Sign(token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "1.2.3.4", "ua")
	assert.Error(t, err)

	// Signed agreements cannot be voided.
	_, err = s.VoidRequest(fmt.Sprint(res.Request.ID), "staff@lab.test")
	assert.Error(t, err)
}

func TestSendRejectsOrganizationClients(t *testing.T) {
	s, _ := setupService(t)
	templates := seedTemplates(t, s)
	for _, clientType := range []string{"Corporate", "Law Firm"} {
		client := seedClient(t, s, clientType)
		_, err := s.SendRequest(client.ID, SendInput{TemplateIDs: []uint{templates[0].ID}}, "staff@lab.test")
		assert.ErrorContains(t, err, "individual", clientType)
	}
}

func TestSendRejectsInactiveTemplateAndDuplicateOpenBooking(t *testing.T) {
	s, _ := setupService(t)
	client := seedClient(t, s, "Individual")
	templates := seedTemplates(t, s)

	_, err := s.SetTemplateActive(fmt.Sprint(templates[1].ID), false)
	require.NoError(t, err)
	_, err = s.SendRequest(client.ID, SendInput{TemplateIDs: []uint{templates[1].ID}}, "")
	assert.ErrorContains(t, err, "inactive")

	appt := appointments.Appointment{ClientID: client.ID, ScheduledAt: time.Now().Add(48 * time.Hour), Status: "confirmed"}
	require.NoError(t, s.db.Create(&appt).Error)
	_, err = s.SendRequest(client.ID, SendInput{AppointmentID: &appt.ID, TemplateIDs: []uint{templates[0].ID}}, "")
	require.NoError(t, err)
	_, err = s.SendRequest(client.ID, SendInput{AppointmentID: &appt.ID, TemplateIDs: []uint{templates[0].ID}}, "")
	assert.ErrorContains(t, err, "already has agreements")

	other := seedClient(t, s, "Individual")
	_, err = s.SendRequest(other.ID, SendInput{AppointmentID: &appt.ID, TemplateIDs: []uint{templates[0].ID}}, "")
	assert.ErrorContains(t, err, "does not belong")
}

func TestVoidedAndExpiredLinksAreUnavailable(t *testing.T) {
	s, _ := setupService(t)
	client := seedClient(t, s, "Individual")
	templates := seedTemplates(t, s)

	voided := sendBoth(t, s, client.ID, templates)
	_, err := s.VoidRequest(fmt.Sprint(voided.Request.ID), "staff@lab.test")
	require.NoError(t, err)
	_, err = s.GetPublicView(voided.Request.Token)
	assert.ErrorIs(t, err, ErrLinkUnavailable)

	expired := sendBoth(t, s, client.ID, templates)
	require.NoError(t, s.db.Model(&AgreementRequest{}).Where("id = ?", expired.Request.ID).
		Update("expires_at", time.Now().Add(-time.Hour)).Error)
	_, err = s.GetPublicView(expired.Request.Token)
	assert.ErrorIs(t, err, ErrLinkUnavailable)
	rows, err := s.ListClientRequests(client.ID)
	require.NoError(t, err)
	for _, r := range rows {
		if r.ID == expired.Request.ID {
			assert.Equal(t, StatusExpired, r.Status)
			assert.Empty(t, r.Link)
		}
	}

	_, err = s.GetPublicView("not-a-real-token")
	assert.ErrorIs(t, err, ErrRequestNotFound)
}

func TestTamperedTermsCannotBeSigned(t *testing.T) {
	s, _ := setupService(t)
	client := seedClient(t, s, "Individual")
	res := sendBoth(t, s, client.ID, seedTemplates(t, s))
	view, err := s.GetPublicView(res.Request.Token)
	require.NoError(t, err)

	require.NoError(t, s.db.Model(&AgreementRequestItem{}).Where("id = ?", view.Items[0].ID).
		Update("body_html", "<p>Altered.</p>").Error)
	_, err = s.Sign(res.Request.Token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "", "")
	assert.ErrorContains(t, err, "could not be verified")
}

func TestDecline(t *testing.T) {
	s, mails := setupService(t)
	client := seedClient(t, s, "Individual")
	res := sendBoth(t, s, client.ID, seedTemplates(t, s))

	view, err := s.Decline(res.Request.Token, "Fee too high")
	require.NoError(t, err)
	assert.Equal(t, StatusDeclined, view.Status)
	assert.Contains(t, (*mails)[len(*mails)-1].body, "Fee too high")

	_, err = s.Sign(res.Request.Token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "", "")
	assert.Error(t, err)
}
