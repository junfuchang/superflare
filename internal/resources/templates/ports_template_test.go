package templates

import (
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
		`id="show-hidden-ports"`,
		`includeHidden`,
		`ports_load_failed`,
		`colWidths: [82, 68, 130, 92, 72, 72, 320]`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("ports template missing %q", expected)
		}
	}
}
