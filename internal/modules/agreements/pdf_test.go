package agreements

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"my-app/internal/modules/appointments"
)

func TestSignedPDFIsStoredFiledAndStable(t *testing.T) {
	s, _ := setupService(t)
	client := seedClient(t, s, "Individual")
	res := sendBoth(t, s, client.ID, seedTemplates(t, s))
	id := fmt.Sprint(res.Request.ID)

	_, _, err := s.StaffPDF(id)
	assert.ErrorIs(t, err, ErrNotSigned, "no PDF before signing")

	view, err := s.GetPublicView(res.Request.Token)
	require.NoError(t, err)
	_, err = s.Sign(res.Request.Token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "1.2.3.4", "ua")
	require.NoError(t, err)

	var stored AgreementRequest
	require.NoError(t, s.db.First(&stored, res.Request.ID).Error)
	require.True(t, bytes.HasPrefix(stored.PDFData, []byte("%PDF-")), "PDF generated at signing")
	sum := sha256.Sum256(stored.PDFData)
	assert.Equal(t, hex.EncodeToString(sum[:]), stored.PDFSHA256)

	// Filed in the Document Vault with the same bytes.
	require.NotNil(t, stored.ClientDocumentID)
	var doc appointments.ClientDocument
	require.NoError(t, s.db.First(&doc, *stored.ClientDocumentID).Error)
	assert.Equal(t, "agreement", doc.Type)
	assert.Equal(t, client.ID, doc.ClientID)
	assert.Equal(t, stored.PDFSHA256, doc.Hash)
	assert.Contains(t, doc.Name, agreementReference(stored.ID))
	uploaded := s.storage.(*memStorage).files
	require.Len(t, uploaded, 1)
	for _, data := range uploaded {
		assert.Equal(t, stored.PDFData, data)
	}

	// Staff and public downloads serve the identical stored file.
	staffPDF, name, err := s.StaffPDF(id)
	require.NoError(t, err)
	assert.Equal(t, stored.PDFData, staffPDF)
	assert.Equal(t, signedPDFFilename(stored.ID), name)
	publicPDF, _, err := s.PublicPDF(res.Request.Token)
	require.NoError(t, err)
	assert.Equal(t, stored.PDFData, publicPDF)

	// Changing branding afterwards does not alter the signed copy.
	require.NoError(t, s.db.Exec("UPDATE organization_settings SET name = 'Renamed Lab' WHERE id = 1").Error)
	again, _, err := s.StaffPDF(id)
	require.NoError(t, err)
	assert.Equal(t, stored.PDFData, again)
}

func TestPDFRegeneratedWhenMissing(t *testing.T) {
	s, _ := setupService(t)
	client := seedClient(t, s, "Individual")
	res := sendBoth(t, s, client.ID, seedTemplates(t, s))
	view, err := s.GetPublicView(res.Request.Token)
	require.NoError(t, err)
	_, err = s.Sign(res.Request.Token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "", "")
	require.NoError(t, err)

	// Simulate a failed generation at signing time.
	require.NoError(t, s.db.Model(&AgreementRequest{}).Where("id = ?", res.Request.ID).
		Updates(map[string]interface{}{"pdf_data": nil, "pdf_sha256": ""}).Error)
	data, _, err := s.PublicPDF(res.Request.Token)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
}

func testPNGDataURL(w, h int) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 70, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func richRequest(logo string) pdfInput {
	now := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC)
	body := SanitizeTermsHTML(`<h2>Payment terms</h2>
<p>By signing you accept the <strong>fees</strong>, <em>deadlines</em> and <u>conditions</u> below — “in full”.</p>
<ol><li><p>The fee is payable before the session → no exceptions.</p></li>
<li><p>Accepted methods:</p><ul><li><p>Bank transfer</p></li><li><p>Card (€, £, $)</p></li></ul></li>
<li>Plain item without paragraph</li></ol>
<blockquote><p>Refunds are processed within 14 days.</p></blockquote><hr><h3>Notes</h3><p>Line one<br>Line two</p>`)
	items := []AgreementRequestItem{}
	for i := 1; i <= 4; i++ {
		items = append(items, AgreementRequestItem{
			ID: uint(i), TemplateID: uint(i), TemplateVersion: i, Title: fmt.Sprintf("Agreement %d", i),
			BodyHTML: body, SortOrder: i, AcceptedAt: &now,
		})
	}
	req := AgreementRequest{
		ID: 42, RecipientName: "Jane Doe", RecipientEmail: "jane@example.com", Status: StatusSigned,
		SentAt: now.Add(-time.Hour), ViewedAt: &now, SignedAt: &now, SignedName: "Jane Doe",
		SignatureDataURL: testSignature(), SignerIP: "203.0.113.9", SignerUserAgent: strings.Repeat("Mozilla/5.0 ", 12),
		ContentHash: strings.Repeat("a", 64), SignatureHash: strings.Repeat("b", 64), Items: items,
	}
	scheduled := now.Add(72 * time.Hour)
	return pdfInput{
		Request: req,
		Organization: PublicOrganization{
			Name: "Polygraph Forensic Labs", Website: "https://www.example-lab.ae", LogoDataURL: logo,
			Phone: "+971 4 000 0000", SupportEmail: "hello@example-lab.ae", Address: "Dubai, UAE",
		},
		Booking: &PublicBooking{ScheduledAt: scheduled, ExamType: "Fidelity examination", ExamFee: 2500, Currency: "AED"},
	}
}

func TestGenerateAgreementPDFRichContent(t *testing.T) {
	for name, logo := range map[string]string{
		"with logo":        testPNGDataURL(240, 80),
		"no logo":          "",
		"unreadable logo":  "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not really a png")),
		"svg is not a png": "data:image/svg+xml;base64,PHN2Zz4=",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := generateAgreementPDF(richRequest(logo))
			require.NoError(t, err)
			assert.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
			if out := os.Getenv("AGREEMENT_PDF_OUT"); out != "" && name == "with logo" {
				require.NoError(t, os.WriteFile(out, data, 0o600))
			}
		})
	}

	// Stable output: the same record always yields the same bytes.
	a, err := generateAgreementPDF(richRequest(""))
	require.NoError(t, err)
	b, err := generateAgreementPDF(richRequest(""))
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func TestFormatMoney(t *testing.T) {
	assert.Equal(t, "AED 2,500.00", formatMoney(2500, "aed"))
	assert.Equal(t, "USD 1,234,567.89", formatMoney(1234567.89, "USD"))
	assert.Equal(t, "AED 0.50", formatMoney(0.5, "AED"))
	assert.Equal(t, "AED 10.00", formatMoney(9.999, "AED"))
}
