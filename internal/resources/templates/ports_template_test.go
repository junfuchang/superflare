package templates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortsTemplateUsesAsyncLoadingAndHiddenControls(t *testing.T) {
	raw, err := TPL.ReadFile("html/settings-ports.html")
	if err != nil {
		t.Fatalf("read ports template: %v", err)
	}
	page := string(raw)
	for _, expected := range []string{
		`{{.PortsDataURI}}`,
		`show-hidden-ports`,
		`hide-all-ports`,
		`unhide-all-ports`,
		`includeHidden`,
		`ports_all_hidden`,
		`ports_load_failed`,
		`parseFailureResponse(response, 'ports request failed')`,
		`parseFailureResponse(response, 'save failed')`,
		`payload.detail`,
		`colWidths: [82, 68, 130, 92, 72, 72, 320]`,
		`--ports-table-surface`,
		`--ports-table-input-text`,
		`.ht_clone_top th .colHeader`,
		`.ports-toggle input`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("ports template missing %q", expected)
		}
	}
}

func TestGeneratedPortsTemplateMatchesSourceTemplate(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", "settings-ports.html")))
	if err != nil {
		t.Fatalf("read source ports template: %v", err)
	}
	gen, err := ReadEmbeddedTemplate("settings-ports.html")
	if err != nil {
		t.Fatalf("read generated ports template: %v", err)
	}
	minifiedSource, err := MinifyTemplateBytes(src)
	if err != nil {
		t.Fatalf("minify source ports template: %v", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(minifiedSource, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(gen, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated ports template is out of sync with embed/templates/settings-ports.html")
	}
}
