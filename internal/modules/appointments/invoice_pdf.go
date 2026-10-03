package appointments

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"

	"my-app/internal/money"
)

// InvoicePDFOrg is branding/contact info for generated invoice PDFs.
type InvoicePDFOrg struct {
	Name         string
	Address      string
	Phone        string
	SupportEmail string
	LogoDataURL  string
}

var invoiceAccent = [3]int{201, 100, 66}

// BuildInvoicePDF creates a branded invoice PDF matching the dashboard print layout.
func BuildInvoicePDF(quote *Quotation, org InvoicePDFOrg) ([]byte, error) {
	if quote == nil {
		return nil, fmt.Errorf("quotation is required")
	}

	code := strings.TrimSpace(quote.Code)
	if code == "" {
		code = fmt.Sprintf("INV-%d", quote.ID)
	}
	currency := strings.ToUpper(strings.TrimSpace(quote.Currency))
	if currency == "" {
		currency = "AED"
	}

	orgName := strings.TrimSpace(org.Name)
	if orgName == "" {
		orgName = "Polygraph UAE"
	}

	title := strings.TrimSpace(quote.Title)
	if title == "" {
		title = "Polygraph services"
	}

	clientName := strings.TrimSpace(quote.Client.Name)
	if clientName == "" {
		clientName = fmt.Sprintf("Client #%d", quote.ClientID)
	}

	totals := computeInvoiceTotals(quote)
	paid := money.CeilWhole(quote.CollectedAmount)
	if paid < 0 {
		paid = 0
	}
	total := totals.Total
	balance := total - paid
	if balance < 0 {
		balance = 0
	}

	issueDate := quote.CreatedAt.UTC()
	if issueDate.IsZero() {
		issueDate = time.Now().UTC()
	}
	validUntil := issueDate.Add(30 * 24 * time.Hour)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(14, 14, 14)
	pdf.AddPage()

	leftX := 14.0
	rightX := 196.0
	contentW := rightX - leftX

	// Logo + org block (left)
	logoY := 14.0
	imgBytes, imgType, ok := decodeLogoImage(org.LogoDataURL)
	if !ok {
		imgBytes, imgType, ok = readLogoFile("assets/logo-print.png")
	}
	if ok && len(imgBytes) > 0 {
		imgName := "org-logo"
		opt := gofpdf.ImageOptions{ImageType: imgType, ReadDpi: true}
		pdf.RegisterImageOptionsReader(imgName, opt, bytes.NewReader(imgBytes))
		pdf.Image(imgName, leftX, logoY, 42, 0, false, "", 0, "")
		logoY += 18
	}

	pdf.SetXY(leftX, logoY)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(17, 17, 17)
	pdf.Cell(0, 6, orgName)
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(102, 102, 102)
	for _, line := range []string{
		strings.TrimSpace(org.Address),
		phoneLine(org.Phone),
		strings.TrimSpace(org.SupportEmail),
	} {
		if line == "" {
			continue
		}
		pdf.Cell(0, 4.5, line)
		pdf.Ln(4.5)
	}

	// Title + meta table (right)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(17, 17, 17)
	pdf.SetXY(120, 14)
	pdf.CellFormat(contentW-106, 10, "Invoice", "", 1, "R", false, 0, "")

	metaRows := [][2]string{
		{"Invoice No.", code},
		{"Date", issueDate.Format("02-01-2006")},
		{"Amount", formatMoneyPDF(total, currency)},
		{"Valid until", validUntil.Format("02-01-2006")},
	}
	metaW := 78.0
	metaX := rightX - metaW
	rowY := 26.0
	for _, row := range metaRows {
		pdf.SetXY(metaX, rowY)
		pdf.SetFillColor(invoiceAccent[0], invoiceAccent[1], invoiceAccent[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(32, 7, strings.ToUpper(row[0]), "0", 0, "L", true, 0, "")
		pdf.SetFillColor(246, 239, 235)
		pdf.SetTextColor(17, 17, 17)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(46, 7, row[1], "0", 1, "R", true, 0, "")
		rowY += 7
	}

	// Bill to + payment summary boxes
	boxY := 72.0
	boxH := 32.0
	boxW := (contentW - 8) / 2
	drawBox(pdf, leftX, boxY, boxW, boxH)
	drawBox(pdf, leftX+boxW+8, boxY, boxW, boxH)

	pdf.SetXY(leftX+4, boxY+4)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(17, 17, 17)
	pdf.Cell(0, 5, "BILL TO")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(0, 5, clientName)
	pdf.Ln(5)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(85, 85, 85)
	if email := strings.TrimSpace(quote.Client.Email); email != "" {
		pdf.Cell(0, 4.5, "Email: "+email)
		pdf.Ln(4.5)
	}
	if phone := strings.TrimSpace(quote.Client.Phone); phone != "" {
		pdf.Cell(0, 4.5, "Phone: "+phone)
	}

	payX := leftX + boxW + 8 + 4
	pdf.SetXY(payX, boxY+4)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(17, 17, 17)
	pdf.Cell(0, 5, "PAYMENT SUMMARY")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(85, 85, 85)
	pdf.SetX(payX)
	pdf.Cell(0, 4.5, "Total: "+formatMoneyPDF(total, currency))
	pdf.Ln(4.5)
	pdf.SetX(payX)
	pdf.Cell(0, 4.5, "Paid: "+formatMoneyPDF(paid, currency))
	pdf.Ln(4.5)
	pdf.SetX(payX)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(17, 17, 17)
	pdf.Cell(0, 4.5, "Balance due: "+formatMoneyPDF(balance, currency))

	// Line items table
	tableY := boxY + boxH + 10
	colSN := 10.0
	colDesc := 88.0
	colQty := 16.0
	colUnit := 34.0
	colTotal := 34.0

	pdf.SetXY(leftX, tableY)
	pdf.SetFillColor(invoiceAccent[0], invoiceAccent[1], invoiceAccent[2])
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.CellFormat(colSN, 8, "S/N", "0", 0, "C", true, 0, "")
	pdf.CellFormat(colDesc, 8, "DESCRIPTION", "0", 0, "L", true, 0, "")
	pdf.CellFormat(colQty, 8, "QTY", "0", 0, "R", true, 0, "")
	pdf.CellFormat(colUnit, 8, "UNIT PRICE", "0", 0, "R", true, 0, "")
	pdf.CellFormat(colTotal, 8, "TOTAL PRICE", "0", 1, "R", true, 0, "")

	pdf.SetTextColor(42, 42, 42)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetX(leftX)
	pdf.CellFormat(colSN, 8, "1", "B", 0, "C", false, 0, "")
	pdf.CellFormat(colDesc, 8, title, "B", 0, "L", false, 0, "")
	pdf.CellFormat(colQty, 8, "1", "B", 0, "R", false, 0, "")
	lineAmount := totals.Subtotal
	if lineAmount <= 0 {
		lineAmount = total
	}
	pdf.CellFormat(colUnit, 8, formatMoneyPDF(lineAmount, currency), "B", 0, "R", false, 0, "")
	pdf.CellFormat(colTotal, 8, formatMoneyPDF(lineAmount, currency), "B", 1, "R", false, 0, "")

	// Notes + totals breakdown
	bottomY := tableY + 24
	notesW := contentW - 78
	drawBox(pdf, leftX, bottomY, notesW, 28)
	pdf.SetXY(leftX+4, bottomY+4)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(17, 17, 17)
	pdf.Cell(0, 5, "NOTES")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(68, 68, 68)
	pdf.SetX(leftX + 8)
	pdf.MultiCell(notesW-10, 4.5, "• This quotation is valid for 30 days from the issue date.", "", "L", false)

	totalBarX := leftX + notesW + 8
	totalBarW := 70.0
	drawInvoiceTotalsBox(pdf, totalBarX, bottomY, totalBarW, totals, currency)

	pdf.SetY(280)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(153, 153, 153)
	pdf.MultiCell(contentW, 4, fmt.Sprintf("Thank you for choosing %s. This invoice is subject to acceptance within its validity period.", orgName), "", "C", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type invoiceTotals struct {
	Subtotal  float64
	Discount  float64
	VAT       float64
	VatRate   float64
	Total     float64
	ShowBreak bool
}

func computeInvoiceTotals(q *Quotation) invoiceTotals {
	subtotal := money.CeilWhole(q.SubtotalAmount)
	if subtotal <= 0 {
		subtotal = money.CeilWhole(q.Amount)
	}
	discount := money.CeilWhole(q.DiscountAmount)
	if discount > subtotal {
		discount = subtotal
	}
	after := subtotal - discount
	if after < 0 {
		after = 0
	}
	rate := q.VatRate
	if rate < 0 {
		rate = 0
	}
	showVAT := money.CeilWhole(q.VatAmount) > 0 || rate > 0
	vat := 0.0
	if showVAT && after > 0 && rate > 0 {
		vat = money.CeilWhole(after * rate / 100)
	}
	total := after + vat
	if total <= 0 {
		total = money.CeilWhole(q.Amount)
	}
	showBreak := discount > 0 || vat > 0
	return invoiceTotals{
		Subtotal:  subtotal,
		Discount:  discount,
		VAT:       vat,
		VatRate:   rate,
		Total:     total,
		ShowBreak: showBreak,
	}
}

func drawInvoiceTotalsBox(pdf *gofpdf.Fpdf, x, y, w float64, totals invoiceTotals, currency string) {
	rowH := 7.0
	rows := 1
	if totals.ShowBreak {
		rows++
	}
	if totals.Discount > 0 {
		rows++
	}
	if totals.VAT > 0 {
		rows++
	}
	boxH := float64(rows)*rowH + 4
	drawBox(pdf, x, y, w, boxH)

	curY := y + 2
	writeTotalRow := func(label, value string, accent bool) {
		pdf.SetXY(x+3, curY)
		if accent {
			pdf.SetFillColor(invoiceAccent[0], invoiceAccent[1], invoiceAccent[2])
			pdf.SetTextColor(255, 255, 255)
			pdf.SetFont("Helvetica", "B", 10)
		} else {
			pdf.SetFillColor(255, 255, 255)
			if strings.HasPrefix(label, "Discount") {
				pdf.SetTextColor(192, 57, 43)
			} else {
				pdf.SetTextColor(42, 42, 42)
			}
			pdf.SetFont("Helvetica", "", 9)
		}
		pdf.CellFormat(w/2-3, rowH, label, "", 0, "L", accent, 0, "")
		pdf.CellFormat(w/2-3, rowH, value, "", 1, "R", accent, 0, "")
		curY += rowH
	}

	if totals.ShowBreak {
		writeTotalRow("Net Subtotal", formatMoneyPDF(totals.Subtotal, currency), false)
	}
	if totals.Discount > 0 {
		writeTotalRow("Discount", "-"+formatMoneyPDF(totals.Discount, currency), false)
	}
	if totals.VAT > 0 {
		label := fmt.Sprintf("VAT (%.0f%%)", totals.VatRate)
		if math.Mod(totals.VatRate, 1) != 0 {
			label = fmt.Sprintf("VAT (%.2f%%)", totals.VatRate)
		}
		writeTotalRow(label, formatMoneyPDF(totals.VAT, currency), false)
	}
	totalLabel := "Total Amount"
	if totals.VAT > 0 {
		totalLabel = "Total Amount (Incl. VAT)"
	}
	writeTotalRow(totalLabel, formatMoneyPDF(totals.Total, currency), true)
}

func drawBox(pdf *gofpdf.Fpdf, x, y, w, h float64) {
	pdf.SetDrawColor(226, 226, 226)
	pdf.SetLineWidth(0.2)
	pdf.RoundedRect(x, y, w, h, 2, "1234", "D")
}

func phoneLine(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(phone), "phone:") {
		return phone
	}
	return "Phone: " + phone
}

func readLogoFile(relPath string) ([]byte, string, bool) {
	candidates := []string{relPath}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), relPath))
	}
	for _, p := range candidates {
		raw, err := os.ReadFile(p)
		if err != nil || len(raw) == 0 {
			continue
		}
		imgType := "PNG"
		if strings.HasSuffix(strings.ToLower(p), ".jpg") || strings.HasSuffix(strings.ToLower(p), ".jpeg") {
			imgType = "JPG"
		}
		return raw, imgType, true
	}
	return nil, "", false
}

func decodeLogoImage(dataURL string) ([]byte, string, bool) {
	dataURL = strings.TrimSpace(dataURL)
	if dataURL == "" {
		return nil, "", false
	}
	const prefix = "data:"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, "", false
	}
	rest := strings.TrimPrefix(dataURL, prefix)
	sep := strings.Index(rest, ",")
	if sep < 0 {
		return nil, "", false
	}
	meta := rest[:sep]
	payload := rest[sep+1:]
	imgType := "PNG"
	switch {
	case strings.Contains(meta, "image/jpeg"), strings.Contains(meta, "image/jpg"):
		imgType = "JPG"
	case strings.Contains(meta, "image/png"):
		imgType = "PNG"
	default:
		return nil, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 {
		return nil, "", false
	}
	return raw, imgType, true
}

func formatMoneyPDF(amount float64, currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = "AED"
	}
	rounded := money.CeilWhole(amount)
	return fmt.Sprintf("%s %s", currency, formatAmountCommas(rounded))
}

func formatAmountCommas(amount float64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	whole := int64(amount)
	frac := int64(math.Round((amount - float64(whole)) * 100))
	if frac == 100 {
		whole++
		frac = 0
	}
	wholeStr := fmt.Sprintf("%d", whole)
	if len(wholeStr) > 3 {
		var parts []string
		for len(wholeStr) > 3 {
			parts = append([]string{wholeStr[len(wholeStr)-3:]}, parts...)
			wholeStr = wholeStr[:len(wholeStr)-3]
		}
		if wholeStr != "" {
			parts = append([]string{wholeStr}, parts...)
		}
		wholeStr = strings.Join(parts, ",")
	}
	return fmt.Sprintf("%s%s.%02d", sign, wholeStr, frac)
}
