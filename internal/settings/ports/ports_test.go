package ports

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestParsePortBindings(t *testing.T) {
	got, err := parsePortBindings(`[{"Port":3060,"Protocol":"tcp","Remark":" dev "},{"Port":"8080","Protocol":"udp","Remark":"dns"},{"Port":9090,"Protocol":"tcp","Remark":""},{"Port":5005,"Protocol":"tcp","Remark":"","Hidden":true}]`)
	if err != nil {
		t.Fatalf("parsePortBindings: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 bindings, got %#v", got)
	}
	if got[0].Port != 3060 || got[0].Protocol != "tcp" || got[0].Remark != "dev" {
		t.Fatalf("unexpected first binding: %#v", got[0])
	}
	if got[1].Port != 8080 || got[1].Protocol != "udp" || got[1].Remark != "dns" {
		t.Fatalf("unexpected second binding: %#v", got[1])
	}
	if got[2].Port != 5005 || got[2].Protocol != "tcp" || !got[2].Hidden || got[2].Remark != "" {
		t.Fatalf("unexpected hidden binding: %#v", got[2])
	}
}

func TestParsePortBindingsRejectsInvalidJSON(t *testing.T) {
	if _, err := parsePortBindings(`[{\"Port\":3060}]`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestUpdatePortRemarksReadsFormValue(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	form := url.Values{}
	form.Set("ports", `[{"Port":3060,"Protocol":"tcp","Remark":"dev"}]`)
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	e.Renderer = testRenderer{}

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	raw, err := os.ReadFile("ports.yaml")
	if err != nil {
		t.Fatalf("read ports.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "3060") || !strings.Contains(string(raw), "dev") {
		t.Fatalf("ports.yaml missing saved remark: %s", string(raw))
	}
}

func TestUpdatePortRemarksKeepsHiddenPortsWhenNotShown(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile("ports.yaml", []byte("ports:\n- port: 8080\n  protocol: tcp\n  hidden: true\n"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	form := url.Values{}
	form.Set("ports", `[{"Port":3060,"Protocol":"tcp","Remark":"dev"}]`)
	form.Set("includeHidden", "0")
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	raw, err := os.ReadFile("ports.yaml")
	if err != nil {
		t.Fatalf("read ports.yaml: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "3060") || !strings.Contains(text, "8080") || !strings.Contains(text, "hidden: true") {
		t.Fatalf("ports.yaml did not preserve hidden binding: %s", text)
	}
}

type testRenderer struct{}

func (testRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	return nil
}
