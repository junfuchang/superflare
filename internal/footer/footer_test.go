package footer

import (
	"html/template"
	"strings"
	"testing"
)

func TestSanitizeAllowsLimitedHTMLAndSafeLinks(t *testing.T) {
	input := `备案 <strong>信息</strong><br><a href="https://example.com" title="官网" target="_blank" onclick="alert(1)">官网</a><span> 版本 </span><code>20260628</code>`

	got := Sanitize(input)

	for _, expected := range []string{
		`<strong>信息</strong>`,
		`<br>`,
		`<a href="https://example.com" title="官网" target="_blank" rel="noopener noreferrer">官网</a>`,
		`<span> 版本 </span>`,
		`<code>20260628</code>`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected sanitized footer to contain %q, got %q", expected, got)
		}
	}
	if strings.Contains(got, "onclick") {
		t.Fatalf("expected dangerous event handlers to be removed, got %q", got)
	}
}

func TestSanitizeDropsDangerousMarkupAndKeepsSafeText(t *testing.T) {
	input := `<script>alert(1)</script><a href="javascript:alert(1)">bad</a><img src=x onerror=alert(1)><em>ok</em>`

	got := Sanitize(input)

	for _, broken := range []string{
		`<script`,
		`javascript:`,
		`<img`,
		`onerror`,
		`alert(1)`,
	} {
		if strings.Contains(strings.ToLower(got), broken) {
			t.Fatalf("expected sanitized footer to remove %q, got %q", broken, got)
		}
	}
	if !strings.Contains(got, `bad`) {
		t.Fatalf("expected unsafe link text to remain visible, got %q", got)
	}
	if !strings.Contains(got, `<em>ok</em>`) {
		t.Fatalf("expected allowed emphasis tag to remain, got %q", got)
	}
}

func TestSanitizeAllowsRelativeLinks(t *testing.T) {
	got := Sanitize(`<a href="/help">帮助</a>`)
	if !strings.Contains(got, `<a href="/help">帮助</a>`) {
		t.Fatalf("expected relative link to remain, got %q", got)
	}
}

func TestBindTemplateDataSeparatesRawAndRenderedFooter(t *testing.T) {
	m := map[string]any{}
	raw := `</textarea><script>alert(1)</script><strong>ok</strong>`

	BindTemplateData(m, raw)

	gotRaw, ok := m["OptionFooter"].(string)
	if !ok {
		t.Fatalf("expected raw footer to stay a string, got %T", m["OptionFooter"])
	}
	if gotRaw != raw {
		t.Fatalf("expected raw footer to be preserved, got %q", gotRaw)
	}
	rendered, ok := m["RenderedFooter"].(template.HTML)
	if !ok {
		t.Fatalf("expected rendered footer to be trusted html, got %T", m["RenderedFooter"])
	}
	renderedText := string(rendered)
	if strings.Contains(renderedText, `<script`) {
		t.Fatalf("expected rendered footer to be sanitized, got %q", renderedText)
	}
	if !strings.Contains(renderedText, `<strong>ok</strong>`) {
		t.Fatalf("expected rendered footer to keep safe markup, got %q", renderedText)
	}
}
