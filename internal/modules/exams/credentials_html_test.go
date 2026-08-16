package exams

import (
	"strings"
	"testing"
)

func TestCredentialsContentEmpty(t *testing.T) {
	if !credentialsContentEmpty("") {
		t.Fatal("expected empty string to be empty")
	}
	if !credentialsContentEmpty("<p></p>") {
		t.Fatal("expected empty paragraph to be empty")
	}
	if !credentialsContentEmpty("<p><br></p>") {
		t.Fatal("expected break-only paragraph to be empty")
	}
	if credentialsContentEmpty("<p>QUALIFICATIONS</p>") {
		t.Fatal("expected real content not to be empty")
	}
}

func TestCredentialsHTMLToPDFBasic(t *testing.T) {
	html := `<h2>QUALIFICATIONS</h2><ul><li>Graduated as a <strong>Basic</strong> Polygraph Examiner</li></ul><p style="text-align: center">Centered note</p>`
	out := credentialsHTMLToPDFBasic(html)
	if !strings.Contains(out, "<b>QUALIFICATIONS</b>") {
		t.Fatalf("expected heading bold, got %q", out)
	}
	if !strings.Contains(out, "• ") {
		t.Fatalf("expected bullet, got %q", out)
	}
	if !strings.Contains(out, "<b>Basic</b>") {
		t.Fatalf("expected bold mark, got %q", out)
	}
	if !strings.Contains(out, "<center>") || !strings.Contains(out, "Centered note") {
		t.Fatalf("expected centered paragraph, got %q", out)
	}
}

func TestCredentialsApostrophes(t *testing.T) {
	input := `<p>Lynn Marcy (USA) &#39;00 &amp; &#39;05</p>`
	out := credentialsHTMLToPDFBasic(input)
	if strings.Contains(out, "&#39;") {
		t.Fatalf("unexpected &#39; in PDF HTML output: %q", out)
	}
	if !strings.Contains(out, "'00") || !strings.Contains(out, "'05") {
		t.Fatalf("expected unescaped apostrophes in PDF HTML output: %q", out)
	}
}
