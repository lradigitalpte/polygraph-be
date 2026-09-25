package agreements

import "testing"

func TestSanitizeTermsHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"keeps formatting", `<p><strong>Pay</strong> <em>now</em></p>`, `<p><strong>Pay</strong> <em>now</em></p>`},
		{"drops list classes", `<ol class="list-decimal pl-5"><li><p>One</p></li></ol>`, `<ol><li><p>One</p></li></ol>`},
		{"keeps text-align", `<p style="text-align: center">Hi</p>`, `<p style="text-align: center">Hi</p>`},
		{"drops other styles", `<p style="background:url(javascript:alert(1))">Hi</p>`, `<p>Hi</p>`},
		{"drops script", `<p>Hi</p><script>alert(1)</script>`, `<p>Hi</p>`},
		{"drops event handlers", `<p onclick="alert(1)">Hi</p>`, `<p>Hi</p>`},
		{"unwraps links", `<p><a href="javascript:alert(1)">click</a></p>`, `<p>click</p>`},
		{"drops img", `<img src=x onerror=alert(1)><p>Hi</p>`, `<p>Hi</p>`},
		{"escapes text", `<p>a &lt;b&gt; c</p>`, `<p>a &lt;b&gt; c</p>`},
		{"void tags", `<p>a<br>b</p><hr>`, `<p>a<br>b</p><hr>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeTermsHTML(tc.in); got != tc.want {
				t.Errorf("SanitizeTermsHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTermsPlainTextRejectsEmptyMarkup(t *testing.T) {
	if got := termsPlainText(SanitizeTermsHTML(`<p></p><p><br></p>`)); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := termsPlainText(SanitizeTermsHTML(`<p>Terms</p>`)); got != "Terms" {
		t.Errorf("expected Terms, got %q", got)
	}
}
