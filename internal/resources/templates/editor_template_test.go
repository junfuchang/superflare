package templates

import (
	"strings"
	"testing"
)

func TestEditorTemplateControlsAreNotBroken(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatalf("read editor template: %v", err)
	}
	page := string(raw)

	for _, expected := range []string{
		`id="check-links"`,
		`FLARE_FIX_CATEGORY = ["[SuperFlare \u5e94\u7528]"]`,
		`\u4e66\u7b7e\u540d\u79f0`,
		`\u5185\u7f51\u5730\u5740`,
		`\u5b50\u76ee\u5f55`,
		`type: 'autocomplete'`,
		`local-url-empty`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing %q", expected)
		}
	}

	for _, broken := range []string{
		"? id=",
		"SuperFlare \ufffd",
	} {
		if strings.Contains(page, broken) {
			t.Fatalf("editor template contains broken marker %q", broken)
		}
	}
}
