package appointments

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

// BuildInvoicePDF creates a simple branded invoice PDF for a quotation.
func BuildInvoicePDF(quote *Quotation) ([]byte, error) {
	if quote == nil {
		return nil, fmt.Errorf("quotation is required")
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(0, 10, "INVOICE")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 11)
	code := strings.TrimSpace(quote.Code)
	if code == "" {
		code = fmt.Sprintf("INV-%d", quote.ID)
	}
	currency := strings.ToUpper(strings.TrimSpace(quote.Currency))
	if currency == "" {
		currency = "USD"
	}

	pdf.Cell(0, 7, fmt.Sprintf("Invoice: %s", code))
	pdf.Ln(6)
	pdf.Cell(0, 7, fmt.Sprintf("Date: %s", quote.CreatedAt.UTC().Format("02-01-2006")))
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 7, "Bill to")
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 11)
	clientName := strings.TrimSpace(quote.Client.Name)
	if clientName == "" {
		clientName = fmt.Sprintf("Client #%d", quote.ClientID)
	}
	pdf.Cell(0, 6, clientName)
	pdf.Ln(5)
	if email := strings.TrimSpace(quote.Client.Email); email != "" {
		pdf.Cell(0, 6, email)
		pdf.Ln(5)
	}
	if phone := strings.TrimSpace(quote.Client.Phone); phone != "" {
		pdf.Cell(0, 6, phone)
		pdf.Ln(5)
	}
	pdf.Ln(4)

	title := strings.TrimSpace(quote.Title)
	if title == "" {
		title = "Polygraph services"
	}

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(120, 8, "Description", "1", 0, "L", false, 0, "")
	pdf.CellFormat(50, 8, "Amount", "1", 1, "R", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(120, 8, title, "1", 0, "L", false, 0, "")
	pdf.CellFormat(50, 8, formatMoneyPDF(quote.Amount, currency), "1", 1, "R", false, 0, "")

	balance := quote.Amount - quote.CollectedAmount
	if balance < 0 {
		balance = 0
	}

	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(120, 7, "Total", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 7, formatMoneyPDF(quote.Amount, currency), "", 1, "R", false, 0, "")
	pdf.CellFormat(120, 7, "Paid", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 7, formatMoneyPDF(quote.CollectedAmount, currency), "", 1, "R", false, 0, "")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(120, 8, "Balance due", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 8, formatMoneyPDF(balance, currency), "", 1, "R", false, 0, "")

	if desc := strings.TrimSpace(quote.Description); desc != "" {
		pdf.Ln(10)
		pdf.SetFont("Helvetica", "B", 11)
		pdf.Cell(0, 7, "Notes")
		pdf.Ln(6)
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(0, 5, desc, "", "L", false)
	}

	pdf.Ln(12)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.MultiCell(0, 5, "Thank you for choosing Polygraph Forensic System. A secure payment link may be included in the accompanying email.", "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatMoneyPDF(amount float64, currency string) string {
	return fmt.Sprintf("%s %.2f", currency, amount)
}
