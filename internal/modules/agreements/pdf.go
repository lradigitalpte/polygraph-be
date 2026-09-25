package agreements

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"my-app/internal/timeutil"
)

// pdfInput is everything printed on the signed copy. It is assembled from stored records
// only, so the PDF reflects exactly what was agreed and recorded.
type pdfInput struct {
	Request      AgreementRequest
	Organization PublicOrganization
	Booking      *PublicBooking
}

const (
	pdfMargin      = 18.0
	pdfLineHeight  = 5.0
	pdfBodySize    = 9.5
	pdfMutedGray   = 110
	pdfListIndent  = 6.0
	pdfDateLayout  = "2 Jan 2006, 15:04"
	pdfDateSuffix  = " (Dubai time)"
	agreementRefFm = "AGR-%06d"
)

func agreementReference(id uint) string { return fmt.Sprintf(agreementRefFm, id) }

func formatPDFTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.In(timeutil.ClinicLocation()).Format(pdfDateLayout) + pdfDateSuffix
}

func formatMoney(amount float64, currency string) string {
	whole := int64(amount)
	cents := int64((amount-float64(whole))*100 + 0.5)
	if cents == 100 {
		whole, cents = whole+1, 0
	}
	digits := fmt.Sprint(whole)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s %s.%02d", strings.ToUpper(strings.TrimSpace(currency)), b.String(), cents)
}

// Characters outside Windows-1252 (the core PDF fonts' encoding) are mapped to close
// ASCII equivalents; anything else unsupported is dropped by the translator.
var pdfCharFallbacks = strings.NewReplacer(
	"→", "->", "←", "<-", "✓", "v", "✔", "v", "≥", ">=", "≤", "<=", " ", " ",
)

type pdfWriter struct {
	pdf *gofpdf.Fpdf
	tr  func(string) string
}

func (w *pdfWriter) text(s string) string { return w.tr(pdfCharFallbacks.Replace(s)) }

// decodeImageDataURL returns the bytes and gofpdf image type of a PNG/JPEG data URL.
func decodeImageDataURL(dataURL string) ([]byte, string, bool) {
	for prefix, kind := range map[string]string{"data:image/png;base64,": "PNG", "data:image/jpeg;base64,": "JPG"} {
		if strings.HasPrefix(dataURL, prefix) {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
			return raw, kind, err == nil && len(raw) > 0
		}
	}
	return nil, "", false
}

// imageUsable checks an image on a throwaway document first, because a failed image
// registration puts gofpdf into a sticky error state that would sink the whole PDF
// (e.g. an interlaced PNG logo uploaded in settings).
func imageUsable(raw []byte, kind string) bool {
	probe := gofpdf.New("P", "mm", "A4", "")
	probe.RegisterImageOptionsReader("probe", gofpdf.ImageOptions{ImageType: kind}, bytes.NewReader(raw))
	return probe.Ok()
}

// registerImage embeds an image and returns its natural width/height ratio, or 0 if unusable.
func (w *pdfWriter) registerImage(name, dataURL string) float64 {
	raw, kind, ok := decodeImageDataURL(dataURL)
	if !ok || !imageUsable(raw, kind) {
		return 0
	}
	info := w.pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: kind}, bytes.NewReader(raw))
	if info == nil || info.Height() == 0 {
		return 0
	}
	return info.Width() / info.Height()
}

func generateAgreementPDF(in pdfInput) ([]byte, error) {
	req := in.Request
	pdf := gofpdf.New("P", "mm", "A4", "")
	w := &pdfWriter{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}
	ref := agreementReference(req.ID)

	// Fixed metadata dates keep regenerated copies byte-for-byte stable.
	stamp := req.SentAt
	if req.SignedAt != nil {
		stamp = *req.SignedAt
	}
	pdf.SetCreationDate(stamp)
	pdf.SetModificationDate(stamp)
	pdf.SetCatalogSort(true)
	pdf.SetTitle(fmt.Sprintf("Signed agreement %s - %s", ref, req.RecipientName), true)
	pdf.SetAuthor(in.Organization.Name, true)
	pdf.SetProducer("Polygraph Forensic System", true)

	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AliasNbPages("{nb}")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-13)
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
		pdf.CellFormat(0, 4, w.text(fmt.Sprintf("%s  |  %s  |  Signed copy", in.Organization.Name, ref)), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 4, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()

	w.header(in.Organization)
	w.titleBlock(req, ref)
	w.detailsBlock(req, in.Booking)
	for i, item := range req.Items {
		w.agreementSection(i+1, item)
	}
	w.signatureBlock(req)
	w.auditBlock(req, ref)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (w *pdfWriter) header(org PublicOrganization) {
	pdf := w.pdf
	top := pdf.GetY()
	textX := pdfMargin
	align := "L" // without a logo the name sits on the left, like a letterhead
	if ratio := w.registerImage("org-logo", org.LogoDataURL); ratio > 0 {
		align = "R"
		h := 14.0
		width := h * ratio
		if width > 55 {
			width, h = 55, 55/ratio
		}
		pdf.ImageOptions("org-logo", pdfMargin, top, width, h, false, gofpdf.ImageOptions{}, 0, "")
		textX = pdfMargin + width + 5
	}

	pageW, _ := pdf.GetPageSize()
	right := pageW - pdfMargin
	pdf.SetXY(textX, top)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(right-textX, 6, w.text(org.Name), "", 2, align, false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
	for _, line := range []string{
		strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(org.Website, "https://"), "http://"), "/"),
		strings.Join(nonEmpty(org.Phone, org.SupportEmail), "  |  "),
		org.Address,
	} {
		if strings.TrimSpace(line) != "" {
			pdf.SetX(textX)
			pdf.CellFormat(right-textX, 4, w.text(line), "", 2, align, false, 0, "")
		}
	}
	y := pdf.GetY()
	if align == "R" && y < top+16 { // leave room for the logo
		y = top + 16
	}
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(pdfMargin, y+2, right, y+2)
	pdf.SetY(y + 7)
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func (w *pdfWriter) titleBlock(req AgreementRequest, ref string) {
	pdf := w.pdf
	pdf.SetTextColor(20, 20, 20)
	pdf.SetFont("Helvetica", "B", 16)
	title := "Signed Client Agreement"
	if len(req.Items) > 1 {
		title = "Signed Client Agreements"
	}
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
	pdf.CellFormat(0, 5, w.text(fmt.Sprintf("Reference %s  |  Signed %s", ref, formatPDFTime(req.SignedAt))), "", 1, "L", false, 0, "")
	pdf.Ln(4)
}

func (w *pdfWriter) detailsBlock(req AgreementRequest, booking *PublicBooking) {
	rows := [][2]string{
		{"Client", req.RecipientName},
		{"Email", req.RecipientEmail},
	}
	if booking != nil {
		session := booking.ExamType
		if session == "" {
			session = "Polygraph examination"
		}
		rows = append(rows,
			[2]string{"Session", session},
			[2]string{"Scheduled", formatPDFTime(&booking.ScheduledAt)},
		)
		if booking.ExamFee > 0 {
			rows = append(rows, [2]string{"Fee", formatMoney(booking.ExamFee, booking.Currency)})
		}
	}
	w.keyValueBox(rows)
	w.pdf.Ln(5)
}

func (w *pdfWriter) keyValueBox(rows [][2]string) {
	pdf := w.pdf
	pageW, _ := pdf.GetPageSize()
	width := pageW - 2*pdfMargin
	top := pdf.GetY()
	pdf.SetY(top + 3)
	for _, row := range rows {
		pdf.SetX(pdfMargin + 4)
		pdf.SetFont("Helvetica", "", 8.5)
		pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
		pdf.CellFormat(32, 5.5, w.text(row[0]), "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetTextColor(20, 20, 20)
		pdf.MultiCell(width-40, 5.5, w.text(row[1]), "", "L", false)
	}
	bottom := pdf.GetY() + 3
	pdf.SetDrawColor(215, 215, 215)
	pdf.RoundedRect(pdfMargin, top, width, bottom-top, 2, "1234", "D")
	pdf.SetY(bottom)
}

// keepWithNext starts a new page when fewer than minSpace mm remain, so a heading is
// never stranded at the bottom of a page without the text that follows it.
func (w *pdfWriter) keepWithNext(minSpace float64) {
	_, pageH := w.pdf.GetPageSize()
	if w.pdf.GetY() > pageH-20-minSpace {
		w.pdf.AddPage()
	}
}

func (w *pdfWriter) agreementSection(n int, item AgreementRequestItem) {
	pdf := w.pdf
	w.keepWithNext(30)
	pdf.Ln(2)
	pdf.SetTextColor(20, 20, 20)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.MultiCell(0, 6.5, w.text(fmt.Sprintf("%d. %s", n, item.Title)), "", "L", false)
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
	pdf.CellFormat(0, 4, fmt.Sprintf("Version %d", item.TemplateVersion), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetTextColor(35, 35, 35)
	w.renderTerms(item.BodyHTML)

	pdf.Ln(1)
	pdf.SetTextColor(4, 120, 87)
	accepted := "Not agreed"
	if item.AcceptedAt != nil {
		accepted = "Agreed by the client on " + formatPDFTime(item.AcceptedAt)
		pdf.SetFont("ZapfDingbats", "", 8.5)
		pdf.CellFormat(4.5, 5, "4", "", 0, "L", false, 0, "") // check mark glyph
	}
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.CellFormat(0, 5, w.text(accepted), "", 1, "L", false, 0, "")
	pdf.SetTextColor(20, 20, 20)
	pdf.Ln(3)
}

func (w *pdfWriter) signatureBlock(req AgreementRequest) {
	pdf := w.pdf
	_, pageH := pdf.GetPageSize()
	if pdf.GetY() > pageH-75 {
		pdf.AddPage()
	}
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 7, "Signature", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(70, 70, 70)
	pdf.MultiCell(0, 4.5, w.text("The client confirmed they read and agreed to each agreement above and "+
		"accepted that their electronic signature is the legal equivalent of their handwritten signature."), "", "L", false)
	pdf.Ln(3)

	top := pdf.GetY()
	sigH := 0.0
	if ratio := w.registerImage("client-signature", req.SignatureDataURL); ratio > 0 {
		width, h := 70.0, 70.0/ratio
		if h > 28 {
			h, width = 28, 28*ratio
		}
		pdf.ImageOptions("client-signature", pdfMargin, top, width, h, false, gofpdf.ImageOptions{}, 0, "")
		sigH = h
	}
	lineY := top + sigH + 2
	if sigH == 0 {
		lineY = top + 12
	}
	pdf.SetDrawColor(120, 120, 120)
	pdf.Line(pdfMargin, lineY, pdfMargin+80, lineY)
	pdf.SetY(lineY + 1.5)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 5, w.text(req.SignedName), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
	pdf.CellFormat(0, 4, w.text("Signed electronically on "+formatPDFTime(req.SignedAt)), "", 1, "L", false, 0, "")
	pdf.Ln(6)
}

func (w *pdfWriter) auditBlock(req AgreementRequest, ref string) {
	pdf := w.pdf
	_, pageH := pdf.GetPageSize()
	if pdf.GetY() > pageH-80 {
		pdf.AddPage()
	}
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 6, "Audit trail", "", 1, "L", false, 0, "")
	rows := [][2]string{
		{"Reference", ref},
		{"Sent to", req.RecipientEmail},
		{"Sent", formatPDFTime(&req.SentAt)},
		{"Opened", formatPDFTime(req.ViewedAt)},
		{"Signed", formatPDFTime(req.SignedAt)},
		{"Typed name", req.SignedName},
		{"IP address", fallback(req.SignerIP, "-")},
		{"Device", fallback(req.SignerUserAgent, "-")},
		{"Terms fingerprint", req.ContentHash},
		{"Signature fingerprint", req.SignatureHash},
	}
	for _, row := range rows {
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
		pdf.CellFormat(36, 4.2, w.text(row[0]), "", 0, "L", false, 0, "")
		pdf.SetFont("Courier", "", 7.5)
		pdf.SetTextColor(40, 40, 40)
		pdf.MultiCell(0, 4.2, w.text(row[1]), "", "L", false)
	}
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "I", 7)
	pdf.SetTextColor(pdfMutedGray, pdfMutedGray, pdfMutedGray)
	pdf.MultiCell(0, 3.8, w.text("Fingerprints are SHA-256 hashes. The terms fingerprint identifies the exact wording "+
		"shown to the client; the signature fingerprint binds that wording to the signer's name, signature image and signing time."), "", "L", false)
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// --- Terms rendering ------------------------------------------------------------------

// renderTerms lays out sanitized agreement HTML (see SanitizeTermsHTML): headings,
// paragraphs, nested lists, block quotes and rules, with bold/italic/underline runs.
func (w *pdfWriter) renderTerms(body string) {
	nodes, err := html.ParseFragment(strings.NewReader(body), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		w.pdf.SetFont("Helvetica", "", pdfBodySize)
		w.pdf.MultiCell(0, pdfLineHeight, w.text(termsPlainText(body)), "", "L", false)
		return
	}
	for _, n := range nodes {
		w.renderBlock(n, pdfMargin)
	}
	w.pdf.SetLeftMargin(pdfMargin)
}

func (w *pdfWriter) renderBlock(n *html.Node, left float64) {
	pdf := w.pdf
	if n.Type == html.TextNode {
		if strings.TrimSpace(n.Data) != "" {
			w.paragraph([]*html.Node{n}, left, "")
		}
		return
	}
	if n.Type != html.ElementNode {
		return
	}
	switch n.Data {
	case "h2", "h3":
		size := 11.0
		if n.Data == "h3" {
			size = 10
		}
		w.keepWithNext(18)
		pdf.Ln(1.5)
		pdf.SetFont("Helvetica", "B", size)
		w.paragraphWithSize(children(n), left, "B", size)
	case "p":
		w.paragraph(children(n), left, "")
	case "ul", "ol":
		index := 0
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "li" {
				index++
				marker := "-"
				if n.Data == "ol" {
					marker = fmt.Sprintf("%d.", index)
				} else {
					marker = "\x95" // bullet in Windows-1252
				}
				w.listItem(c, left, marker)
			}
		}
		pdf.Ln(0.8)
	case "blockquote":
		pdf.SetDrawColor(190, 190, 190)
		startY := pdf.GetY()
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.renderBlock(c, left+5)
		}
		pdf.Line(left+1.5, startY, left+1.5, pdf.GetY()-1)
	case "hr":
		pageW, _ := pdf.GetPageSize()
		pdf.SetDrawColor(210, 210, 210)
		pdf.Line(left, pdf.GetY()+1.5, pageW-pdfMargin, pdf.GetY()+1.5)
		pdf.Ln(4)
	case "br":
		pdf.Ln(pdfLineHeight)
	default:
		w.paragraph([]*html.Node{n}, left, "")
	}
}

func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

func (w *pdfWriter) listItem(li *html.Node, left float64, marker string) {
	pdf := w.pdf
	textLeft := left + pdfListIndent
	pdf.SetFont("Helvetica", "", pdfBodySize)
	pdf.SetLeftMargin(left)
	pdf.SetX(left)
	pdf.CellFormat(pdfListIndent, pdfLineHeight, marker, "", 0, "L", false, 0, "")

	// An item holds inline content and/or <p> blocks, plus possibly nested lists.
	var inline []*html.Node
	first := true
	flush := func() {
		if len(inline) > 0 {
			w.writeRuns(inline, textLeft, "", pdfBodySize, !first)
			pdf.Ln(pdfLineHeight + 1)
			inline, first = nil, false
		}
	}
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.ElementNode && c.Data == "p":
			flush()
			w.writeRuns(children(c), textLeft, "", pdfBodySize, !first)
			pdf.Ln(pdfLineHeight + 1)
			first = false
		case c.Type == html.ElementNode && (c.Data == "ul" || c.Data == "ol"):
			flush()
			if first {
				pdf.Ln(pdfLineHeight)
				first = false
			}
			w.renderBlock(c, textLeft)
		default:
			inline = append(inline, c)
		}
	}
	flush()
	if first {
		pdf.Ln(pdfLineHeight + 1)
	}
	pdf.SetLeftMargin(pdfMargin)
}

func (w *pdfWriter) paragraph(nodes []*html.Node, left float64, style string) {
	w.paragraphWithSize(nodes, left, style, pdfBodySize)
}

func (w *pdfWriter) paragraphWithSize(nodes []*html.Node, left float64, style string, size float64) {
	w.writeRuns(nodes, left, style, size, true)
	w.pdf.Ln(pdfLineHeight + 1.5)
	w.pdf.SetLeftMargin(pdfMargin)
}

// writeRuns flows inline text at the given left margin, switching font style per run.
// When resetX is false the text continues from the current position (after a list marker).
func (w *pdfWriter) writeRuns(nodes []*html.Node, left float64, style string, size float64, resetX bool) {
	pdf := w.pdf
	pdf.SetLeftMargin(left)
	if resetX {
		pdf.SetX(left)
	}
	var walk func(n *html.Node, style string)
	walk = func(n *html.Node, style string) {
		switch n.Type {
		case html.TextNode:
			text := strings.Join(strings.Fields(strings.ReplaceAll(n.Data, "\n", " ")), " ")
			if strings.HasPrefix(n.Data, " ") && text != "" {
				text = " " + text
			}
			if strings.HasSuffix(n.Data, " ") && text != "" {
				text += " "
			}
			if text == "" {
				return
			}
			pdf.SetFont("Helvetica", style, size)
			pdf.Write(pdfLineHeight, w.text(text))
		case html.ElementNode:
			next := style
			switch n.Data {
			case "strong", "b":
				next = addStyle(style, "B")
			case "em", "i":
				next = addStyle(style, "I")
			case "u":
				next = addStyle(style, "U")
			case "br":
				pdf.Write(pdfLineHeight, "\n")
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, next)
			}
		}
	}
	for _, n := range nodes {
		walk(n, style)
	}
}

func addStyle(style, s string) string {
	if strings.Contains(style, s) {
		return style
	}
	return style + s
}
