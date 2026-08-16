package exams

import (
	"strings"

	"github.com/jung-kurt/gofpdf"
)

func richTextToHTMLFromRaw(input string) string {
	var b strings.Builder
	i := 0
	for i < len(input) {
		if strings.HasPrefix(input[i:], "**") {
			end := strings.Index(input[i+2:], "**")
			if end >= 0 {
				b.WriteString("<b>")
				b.WriteString(escapePDFHTMLText(input[i+2 : i+2+end]))
				b.WriteString("</b>")
				i += 2 + end + 2
				continue
			}
		}
		if input[i] == '_' && i+1 < len(input) && input[i+1] != ' ' {
			end := strings.Index(input[i+1:], "_")
			if end >= 0 {
				b.WriteString("<i>")
				b.WriteString(escapePDFHTMLText(input[i+1 : i+1+end]))
				b.WriteString("</i>")
				i += 1 + end + 1
				continue
			}
		}
		if input[i] == '\n' {
			b.WriteString("<br>")
			i++
			continue
		}
		next := len(input)
		if at := strings.Index(input[i:], "**"); at >= 0 {
			next = i + at
		}
		if at := strings.Index(input[i:], "_"); at >= 0 && i+at < next {
			next = i + at
		}
		if next == i {
			b.WriteByte(input[i])
			i++
			continue
		}
		b.WriteString(escapePDFHTMLText(input[i:next]))
		i = next
	}
	return b.String()
}

func writeRichReportParagraph(pdf *gofpdf.Fpdf, text string, fontSize float64, style string, afterLn float64) {
	if strings.TrimSpace(text) == "" {
		return
	}
	// TipTap HTML (preferred) or legacy **bold** / _italic_ markup.
	htmlBody := credentialsHTMLToPDFBasic(text)
	if htmlBody == "" {
		return
	}
	pdf.SetFont("Helvetica", style, fontSize)
	html := pdf.HTMLBasicNew()
	_, lineHt := pdf.GetFontSize()
	html.Write(lineHt*1.22, htmlBody)
	if afterLn > 0 {
		pdf.Ln(afterLn)
	}
}
