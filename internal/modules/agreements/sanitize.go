package agreements

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Agreement terms are authored in the TipTap editor and later rendered on the public,
// unauthenticated signing page, so they are reduced to a strict allowlist of formatting
// tags on save. Every attribute is dropped except a text-align style.
var allowedTags = map[string]bool{
	"p": true, "br": true, "h2": true, "h3": true,
	"strong": true, "b": true, "em": true, "i": true, "u": true, "s": true,
	"ul": true, "ol": true, "li": true, "blockquote": true, "hr": true,
}

// Content of these elements is dropped entirely rather than unwrapped.
var droppedTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"noscript": true, "template": true, "svg": true, "math": true, "head": true, "title": true,
}

var textAlignStyle = regexp.MustCompile(`^\s*text-align:\s*(left|center|right|justify)\s*;?\s*$`)

// SanitizeTermsHTML returns the allowlisted subset of the given HTML.
func SanitizeTermsHTML(input string) string {
	nodes, err := html.ParseFragment(strings.NewReader(input), &html.Node{
		Type:     html.ElementNode,
		Data:     "body",
		DataAtom: atom.Body,
	})
	if err != nil {
		return html.EscapeString(input)
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		writeSanitized(&buf, n)
	}
	return strings.TrimSpace(buf.String())
}

func writeSanitized(buf *bytes.Buffer, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		buf.WriteString(html.EscapeString(n.Data))
		return
	case html.ElementNode:
		tag := strings.ToLower(n.Data)
		if droppedTags[tag] {
			return
		}
		if !allowedTags[tag] {
			// Unknown wrapper (div, span, a, ...): keep its text, drop the tag.
			writeChildren(buf, n)
			return
		}
		buf.WriteByte('<')
		buf.WriteString(tag)
		for _, attr := range n.Attr {
			if strings.EqualFold(attr.Key, "style") && textAlignStyle.MatchString(attr.Val) {
				buf.WriteString(` style="`)
				buf.WriteString(html.EscapeString(strings.TrimSpace(attr.Val)))
				buf.WriteByte('"')
			}
		}
		buf.WriteByte('>')
		if tag == "br" || tag == "hr" {
			return
		}
		writeChildren(buf, n)
		buf.WriteString("</")
		buf.WriteString(tag)
		buf.WriteByte('>')
	default:
		writeChildren(buf, n)
	}
}

func writeChildren(buf *bytes.Buffer, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeSanitized(buf, c)
	}
}

// termsPlainText strips tags, used to reject terms that are only empty markup.
func termsPlainText(sanitized string) string {
	doc, err := html.Parse(strings.NewReader(sanitized))
	if err != nil {
		return sanitized
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(b.String())
}
