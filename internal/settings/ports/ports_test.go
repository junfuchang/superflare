package ports

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	portscollector "github.com/junfuchang/superflare/internal/ports"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/labstack/echo/v5"
)

func ensurePortsSettingsConfig(t *testing.T) {
	t.Helper()
	if err := data.EnsureAppConfigExists(); err != nil {
		t.Fatalf("EnsureAppConfigExists: %v", err)
	}
}

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

func TestParsePortBindingsRejectsNonIntegerPort(t *testing.T) {
	if _, err := parsePortBindings(`[{"Port":3060.5,"Protocol":"tcp","Remark":"dev"}]`); err == nil {
		t.Fatal("expected non-integer port error")
	}
}

func TestParsePortBindingsRejectsOutOfRangePort(t *testing.T) {
	if _, err := parsePortBindings(`[{"Port":70000,"Protocol":"tcp","Remark":"dev"}]`); err == nil {
		t.Fatal("expected out-of-range port error")
	}
}

func TestParsePortBindingsRejectsInvalidProtocol(t *testing.T) {
	if _, err := parsePortBindings(`[{"Port":3060,"Protocol":"http","Remark":"dev"}]`); err == nil {
		t.Fatal("expected invalid protocol error")
	}
}

func TestUpdatePortRemarksReadsFormValue(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

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
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile("ports.yaml")
	if err != nil {
		t.Fatalf("read ports.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "3060") || !strings.Contains(string(raw), "dev") {
		t.Fatalf("ports.yaml missing saved remark: %s", string(raw))
	}
}

func TestUpdatePortRemarksReturnsBadRequestPayloadWhenFormDataMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("expected failure payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing form data") {
		t.Fatalf("expected missing form data detail, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksReturnsBadRequestPayloadWhenPortsInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("ports", `[{invalid-json}]`)
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("expected failure payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "parse ports payload failed") {
		t.Fatalf("expected parse ports payload failed detail, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksReturnsBadRequestPayloadWhenPortsMissing(t *testing.T) {
	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("expected failure payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing ports payload") {
		t.Fatalf("expected missing ports payload detail, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksReturnsBadRequestPayloadWhenPortRowInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("ports", `[{"Port":70000,"Protocol":"http","Remark":"dev"}]`)
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("expected failure payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "parse ports payload failed") {
		t.Fatalf("expected parse ports payload failed detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed invalid row payload, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksKeepsHiddenPortsWhenNotShown(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
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

func TestUpdatePortRemarksReturnsConfigErrorWhenExistingBindingsBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := os.WriteFile("ports.yaml", []byte("ports: [broken"), 0644); err != nil {
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
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ports config error") {
		t.Fatalf("expected ports config error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed config error payload, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksReturnsConfigErrorWhenSettingsConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile("config.yml", []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	form := url.Values{}
	form.Set("ports", `[{"Port":3060,"Protocol":"tcp","Remark":"dev"}]`)
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "settings config error") {
		t.Fatalf("expected settings config error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed settings config error payload, got %s", rec.Body.String())
	}
}

func TestUpdatePortRemarksReturnsServerErrorWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := os.Mkdir("ports.yaml", 0755); err != nil {
		t.Fatalf("mkdir ports.yaml: %v", err)
	}

	form := url.Values{}
	form.Set("ports", `[{"Port":3060,"Protocol":"tcp","Remark":"dev"}]`)
	form.Set("includeHidden", "1")
	req := httptest.NewRequest(http.MethodPost, "/settings/ports", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updatePortRemarks(c); err != nil {
		t.Fatalf("updatePortRemarks: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("expected failure payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "save ports failed") {
		t.Fatalf("expected detailed save error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected save failure detail payload, got %s", rec.Body.String())
	}
}

func TestPagePortsReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePorts(c); err != nil {
		t.Fatalf("pagePorts: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPagePortsDataReturnsServerErrorWhenBindingsBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := os.WriteFile("ports.yaml", []byte("ports: [broken"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports/data", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePortsData(c); err != nil {
		t.Fatalf("pagePortsData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ports config error") {
		t.Fatalf("expected ports config error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed ports data error payload, got %s", rec.Body.String())
	}
}

func TestPagePortsDataReturnsServerErrorWhenSettingsConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile("config.yml", []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports/data", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePortsData(c); err != nil {
		t.Fatalf("pagePortsData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "settings config error") {
		t.Fatalf("expected settings config error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed settings config error payload, got %s", rec.Body.String())
	}
}

func TestPagePortsDataReturnsServerErrorWhenRuntimeCollectFails(t *testing.T) {
	originalCollector := portscollectorCollectReportWithBindingsErr
	portscollectorCollectReportWithBindingsErr = func(bindings map[string]model.PortBinding, includeHidden bool) (portscollector.CollectionReport, error) {
		return portscollector.CollectionReport{}, os.ErrPermission
	}
	defer func() { portscollectorCollectReportWithBindingsErr = originalCollector }()

	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports/data", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePortsData(c); err != nil {
		t.Fatalf("pagePortsData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ports runtime collect error") {
		t.Fatalf("expected runtime collect error payload, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("expected detailed runtime collect error payload, got %s", rec.Body.String())
	}
}

func TestPagePortsDataReturnsWarningsWhenOwnerResolutionIncomplete(t *testing.T) {
	originalCollector := portscollectorCollectReportWithBindingsErr
	portscollectorCollectReportWithBindingsErr = func(bindings map[string]model.PortBinding, includeHidden bool) (portscollector.CollectionReport, error) {
		return portscollector.CollectionReport{
			Items: []model.PortInfo{
				{Port: 5668, Protocol: "tcp", Running: true, PID: 321},
			},
			Warnings: []portscollector.CollectionWarning{
				{Code: "owner_resolution_partial", MissingOwners: 1, RuntimePorts: 1, Detail: "lookup denied"},
			},
		}, nil
	}
	defer func() { portscollectorCollectReportWithBindingsErr = originalCollector }()

	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	ensurePortsSettingsConfig(t)
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports/data", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePortsData(c); err != nil {
		t.Fatalf("pagePortsData: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK       bool             `json:"ok"`
		Items    []model.PortInfo `json:"items"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !payload.OK || len(payload.Items) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.Warnings) != 1 || !strings.Contains(payload.Warnings[0], "lookup denied") {
		t.Fatalf("expected detailed warning payload, got %#v", payload.Warnings)
	}
}

func TestPagePortsRendersWithoutRuntimeCollection(t *testing.T) {
	originalCollector := portscollectorCollectReportWithBindingsErr
	called := false
	portscollectorCollectReportWithBindingsErr = func(bindings map[string]model.PortBinding, includeHidden bool) (portscollector.CollectionReport, error) {
		called = true
		return portscollector.CollectionReport{}, os.ErrPermission
	}
	defer func() { portscollectorCollectReportWithBindingsErr = originalCollector }()

	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = portsPageRenderer{
		t: t,
		assert: func(m map[string]any) {
			portsData, ok := m["PortsData"].(template.HTML)
			if !ok {
				t.Fatalf("unexpected PortsData type %T", m["PortsData"])
			}
			if string(portsData) != "[]" {
				t.Fatalf("expected empty initial ports data, got %q", string(portsData))
			}
			dataURI, ok := m["PortsDataURI"].(string)
			if !ok || dataURI != "/settings/ports/data" {
				t.Fatalf("unexpected PortsDataURI: %#v", m["PortsDataURI"])
			}
		},
	}
	c := e.NewContext(req, rec)

	if err := pagePorts(c); err != nil {
		t.Fatalf("pagePorts: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected pagePorts to skip runtime port collection during initial render")
	}
}

func TestPagePortsReturnsStyledErrorWhenBindingsBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile("ports.yaml", []byte("ports: [broken"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pagePorts(c); err != nil {
		t.Fatalf("pagePorts: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPagePortsKeepsStoredRuntimeDebugModeAfterAppFlagsChange(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	origFlags := define.AppFlags
	defer func() {
		define.AppFlags = origFlags
		settingsroot.SetRuntimeFlags(origFlags)
	}()

	define.AppFlags = model.Flags{DebugMode: true}
	settingsroot.SetRuntimeFlags(define.AppFlags)
	define.AppFlags = model.Flags{DebugMode: false}

	req := httptest.NewRequest(http.MethodGet, "/settings/ports", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = portsPageRenderer{
		t: t,
		assert: func(m map[string]any) {
			if got, _ := m["DebugMode"].(bool); !got {
				t.Fatalf("expected stored DebugMode=true, got %#v", m["DebugMode"])
			}
			if got, _ := m["DebugAssetVersion"].(string); got != "?v=dev" {
				t.Fatalf("expected stored debug asset version, got %#v", m["DebugAssetVersion"])
			}
		},
	}
	c := e.NewContext(req, rec)

	if err := pagePorts(c); err != nil {
		t.Fatalf("pagePorts: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

type testRenderer struct{}

func (testRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	return nil
}

type portsWarningRenderer struct {
	t *testing.T
}

func (r portsWarningRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	warnings, ok := m["RenderWarnings"].([]string)
	if !ok || len(warnings) == 0 {
		r.t.Fatalf("expected render warnings, got %#v", m["RenderWarnings"])
	}
	found := false
	for _, item := range warnings {
		if strings.Contains(item, "lookup denied") {
			found = true
			break
		}
	}
	if !found {
		r.t.Fatalf("expected owner resolution warning detail, got %#v", warnings)
	}
	return nil
}

type portsPageRenderer struct {
	t      *testing.T
	assert func(map[string]any)
}

func (r portsPageRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	if r.assert != nil {
		r.assert(m)
	}
	return nil
}
