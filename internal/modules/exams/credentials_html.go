package exams

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

var (
	reTag        = regexp.MustCompile(`(?is)<(/?)([a-z0-9]+)([^>]*)>`)
	reStyleAlign = regexp.MustCompile(`(?i)text-align\s*:\s*(left|center|right)`)
	reWhitespace = regexp.MustCompile(`\s+`)
	reManyBreaks = regexp.MustCompile(`(?i)(<br\s*/?>\s*){3,}`)
)

// credentialsContentEmpty reports whether HTML/plain credentials have visible text.
func credentialsContentEmpty(input string) bool {
	return strings.TrimSpace(stripHTMLToText(input)) == ""
}

func stripHTMLToText(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	s := input
	s = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`(?i)</(p|div|li|h[1-6]|tr)>`).ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

func escapePDFHTMLText(raw string) string {
	s := html.UnescapeString(raw)
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// credentialsHTMLToPDFBasic converts TipTap HTML into gofpdf HTMLBasic-friendly markup.
func credentialsHTMLToPDFBasic(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "<") {
		return richTextToHTMLFromRaw(trimmed)
	}

	var b strings.Builder
	pos := 0
	orderedDepth := 0
	orderedIndex := 0
	centerOpen := false
	headingOpen := false

	closeCenter := func() {
		if centerOpen {
			b.WriteString("</center>")
			centerOpen = false
		}
	}
	closeHeading := func() {
		if headingOpen {
			b.WriteString("</b>")
			headingOpen = false
		}
	}
	writeText := func(raw string) {
		if raw == "" {
			return
		}
		b.WriteString(escapePDFHTMLText(raw))
	}

	for pos < len(trimmed) {
		loc := reTag.FindStringSubmatchIndex(trimmed[pos:])
		if loc == nil {
			writeText(trimmed[pos:])
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
		if start > pos {
			writeText(trimmed[pos:start])
		}

		closing := trimmed[pos+loc[2]:pos+loc[3]] == "/"
		tag := strings.ToLower(trimmed[pos+loc[4] : pos+loc[5]])
		attrs := ""
		if loc[6] >= 0 {
			attrs = trimmed[pos+loc[6] : pos+loc[7]]
		}

		switch tag {
		case "br":
			b.WriteString("<br>")
		case "b", "strong":
			if closing {
				b.WriteString("</b>")
			} else {
				b.WriteString("<b>")
			}
		case "i", "em":
			if closing {
				b.WriteString("</i>")
			} else {
				b.WriteString("<i>")
			}
		case "u":
			if closing {
				b.WriteString("</u>")
			} else {
				b.WriteString("<u>")
			}
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
			isHeading := strings.HasPrefix(tag, "h")
			if closing {
				closeHeading()
				closeCenter()
				b.WriteString("<br><br>")
			} else {
				align := ""
				if m := reStyleAlign.FindStringSubmatch(attrs); len(m) == 2 {
					align = strings.ToLower(m[1])
				}
				if align == "center" {
					b.WriteString("<center>")
					centerOpen = true
				}
				if isHeading {
					b.WriteString("<b>")
					headingOpen = true
				}
			}
		case "ul":
			if closing {
				b.WriteString("<br>")
			}
		case "ol":
			if closing {
				if orderedDepth > 0 {
					orderedDepth--
				}
				orderedIndex = 0
				b.WriteString("<br>")
			} else {
				orderedDepth++
				orderedIndex = 0
			}
		case "li":
			if closing {
				b.WriteString("<br><br>")
			} else if orderedDepth > 0 {
				orderedIndex++
				b.WriteString(strconv.Itoa(orderedIndex) + ". ")
			} else {
				b.WriteString("• ")
			}
		}

		pos = end
	}

	closeHeading()
	closeCenter()

	result := strings.TrimSpace(reManyBreaks.ReplaceAllString(b.String(), "<br><br>"))
	for strings.HasSuffix(strings.ToLower(result), "<br>") || strings.HasSuffix(strings.ToLower(result), "<br/>") {
		lower := strings.ToLower(result)
		if strings.HasSuffix(lower, "<br/>") {
			result = strings.TrimSpace(result[:len(result)-5])
			continue
		}
		result = strings.TrimSpace(result[:len(result)-4])
	}
	return result
}

func writeCredentialsPage(pdf *gofpdf.Fpdf, credentialsHTML string) {
	body := credentialsHTMLToPDFBasic(credentialsHTML)
	if strings.TrimSpace(body) == "" {
		return
	}

	pdf.AddPage()
	pdf.SetTextColor(180, 100, 40)
	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(0, 8, "POLYGRAPH EXAMINER CREDENTIALS", "", 1, "C", false, 0, "")
	pdf.Ln(8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	htmlWriter := pdf.HTMLBasicNew()
	_, lineHt := pdf.GetFontSize()
	htmlWriter.Write(lineHt*1.55, body)
}
