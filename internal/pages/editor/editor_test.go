package editor

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
)

type stubReadCloser struct {
	reader   *bytes.Reader
	closeErr error
}

func newStubReadCloser(raw []byte, closeErr error) *stubReadCloser {
	return &stubReadCloser{reader: bytes.NewReader(raw), closeErr: closeErr}
}

func (s *stubReadCloser) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *stubReadCloser) Close() error {
	return s.closeErr
}

func TestMarshalEditorPortsOnlyIncludesRemarkedPorts(t *testing.T) {
	got, err := marshalEditorPorts([]model.PortBinding{
		{Port: 3060, Protocol: "tcp", Remark: "dev"},
		{Port: 8080, Protocol: "tcp"},
		{Port: 5353, Protocol: "udp", Remark: "dns"},
		{Port: 9090, Protocol: "tcp", Remark: "hidden", Hidden: true},
	})
	if err != nil {
		t.Fatalf("marshalEditorPorts: %v", err)
	}
	if !strings.Contains(got, `"Port":3060`) || !strings.Contains(got, `"Remark":"dev"`) {
		t.Fatalf("expected remarked port in %s", got)
	}
	if strings.Contains(got, "8080") {
		t.Fatalf("unexpected unremarked port in %s", got)
	}
	if strings.Contains(got, "5353") {
		t.Fatalf("unexpected udp port in %s", got)
	}
	if strings.Contains(got, "9090") {
		t.Fatalf("unexpected hidden port in %s", got)
	}
}

func TestMarshalEditorPortsReturnsErrorWhenJSONMarshalFails(t *testing.T) {
	original := marshalEditorPortsJSON
	marshalEditorPortsJSON = func(v interface{}) ([]byte, error) {
		return nil, errors.New("forced marshal failure")
	}
	defer func() { marshalEditorPortsJSON = original }()

	_, err := marshalEditorPorts([]model.PortBinding{{Port: 3060, Protocol: "tcp", Remark: "dev"}})
	if err == nil {
		t.Fatal("expected marshalEditorPorts to fail")
	}
	if !strings.Contains(err.Error(), "marshal editor ports failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderHidesRemarkedPortsWhenLoginDisabled(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	writeEditorRenderFiles(t, tmpDir)

	origRuntime, runtimeSet := define.SnapshotAppRuntimeFlags()
	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origAuth := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() {
		if runtimeSet {
			define.StoreAppRuntimeFlags(origRuntime.Source, origRuntime.Base, origRuntime.Current)
		} else {
			define.ResetAppRuntimeFlags()
		}
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreAuthRuntimeConfig(origAuth)
	})

	flags := model.Flags{
		Port:             3636,
		CookieName:       "superflare",
		CookieSecret:     "editor-secret",
		DisableLoginMode: true,
		User:             "admin",
		Pass:             "admin",
	}
	define.StoreAppRuntimeCurrentFlags(flags)

	e := echo.New()
	e.Renderer = editorPortsCaptureRenderer{t: t, want: "[]"}
	auth.RequestHandleWithFlags(e, flags)
	e.GET("/editor", render)

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCheckOneLink_UsesGETInsteadOfHEAD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result := checkOneLink(context.Background(), server.Client(), linkCheckItem{Row: 3, URL: server.URL})
	if result.Status != "ok" {
		t.Fatalf("expected GET-based link check to pass, got %+v", result)
	}
}

func TestCheckOneLink_NotFoundStillInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer server.Close()

	result := checkOneLink(context.Background(), server.Client(), linkCheckItem{Row: 5, URL: server.URL})
	if result.Status != "invalid" {
		t.Fatalf("expected 404 to remain invalid, got %+v", result)
	}
	if !strings.Contains(result.Reason, "404") {
		t.Fatalf("expected 404 reason, got %+v", result)
	}
}

func TestCheckOneLink_RestrictedStatusIsNotMarkedInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	result := checkOneLink(context.Background(), server.Client(), linkCheckItem{Row: 7, URL: server.URL})
	if result.Status != "restricted" {
		t.Fatalf("expected 403 to be treated as restricted, got %+v", result)
	}
	if !strings.Contains(result.Reason, "403") {
		t.Fatalf("expected 403 reason, got %+v", result)
	}
}

func TestCheckOneLink_ServerErrorIsMarkedUnstable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	result := checkOneLink(context.Background(), server.Client(), linkCheckItem{Row: 8, URL: server.URL})
	if result.Status != "unstable" {
		t.Fatalf("expected 502 to be treated as unstable, got %+v", result)
	}
	if !strings.Contains(result.Reason, "502") {
		t.Fatalf("expected 502 reason, got %+v", result)
	}
}

func TestCheckOneLink_RetriesTransientFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	result := checkOneLink(context.Background(), server.Client(), linkCheckItem{Row: 9, URL: server.URL})
	if result.Status != "ok" {
		t.Fatalf("expected second probe to recover, got %+v", result)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts.Load())
	}
}

func TestCheckOneLink_TimeoutIsMarkedUnstable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow ok"))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 20 * time.Millisecond

	result := checkOneLink(context.Background(), client, linkCheckItem{Row: 10, URL: server.URL})
	if result.Status != "unstable" {
		t.Fatalf("expected timeout to be unstable, got %+v", result)
	}
}

func TestCheckOneLink_StopsRetryWaitWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- waitForRetry(ctx, 2*time.Second)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
		if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("expected cancel reason, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected retry wait to stop soon after context cancellation")
	}
}

func TestValidateRestorePayloadRejectsBrokenConfig(t *testing.T) {
	err := validateRestorePayload("config", []byte("Title: [broken"))
	if err == nil {
		t.Fatal("expected invalid config payload to fail validation")
	}
	if !strings.Contains(err.Error(), "parse config restore payload failed") {
		t.Fatalf("expected stable config restore error, got %v", err)
	}
}

func TestValidateRestorePayloadRejectsConfigWithInvalidValues(t *testing.T) {
	err := validateRestorePayload("config", []byte("Title: SuperFlare\nLocale: zh\nTheme: mystery\n"))
	if err == nil {
		t.Fatal("expected invalid config values to fail validation")
	}
	if !strings.Contains(err.Error(), "parse config restore payload failed") {
		t.Fatalf("expected stable config restore error, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid theme value: mystery") {
		t.Fatalf("expected invalid theme detail, got %v", err)
	}
}

func TestValidateRestorePayloadRejectsBookmarksWithUnknownCategory(t *testing.T) {
	err := validateRestorePayload("bookmarks", []byte("categories:\n- id: default\n  title: 分类1\nlinks:\n- name: Bookmark A\n  category: missing\n  link: https://bookmark.example.com\n"))
	if err == nil {
		t.Fatal("expected invalid bookmark payload to fail validation")
	}
	if !strings.Contains(err.Error(), "parse bookmarks restore payload failed") {
		t.Fatalf("expected stable bookmarks restore error, got %v", err)
	}
	if !strings.Contains(err.Error(), "references unknown category id") {
		t.Fatalf("expected invalid bookmark category detail, got %v", err)
	}
}

func TestRestoreDataRejectsOversizedBackupUpload(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("backup", "config.yml")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("a"), restoreUploadMaxBytes+1)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/restore", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := restoreData(c); err != nil {
		t.Fatalf("restoreData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "backup file too large") {
		t.Fatalf("expected oversize error message, got %s", rec.Body.String())
	}
}

func TestRestoreDataMissingFileReturnsStyledErrorPage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/restore", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := restoreData(c); err != nil {
		t.Fatalf("restoreData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing backup file") {
		t.Fatalf("expected missing file detail, got %s", rec.Body.String())
	}
}

func TestRestoreDataRedirectsWithSuccessNoticeAfterZipRestore(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	files := map[string]string{
		"config.yml":    "Title: SuperFlare\nLocale: zh\nTheme: blackboard\n",
		"bookmarks.yml": "categories:\n- id: default\n  title: Default\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n",
		"apps.yml":      "links:\n- name: App A\n  link: https://public.example.com/app\n",
		"ports.yaml":    "ports:\n- port: 3636\n  protocol: tcp\n  remark: superflare\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(raw), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	zipEntries := map[string]string{
		"config.yml":    "Title: SuperFlare\nLocale: zh\nTheme: blackboard\n",
		"bookmarks.yml": "categories:\n- id: changed\n  title: Changed\nlinks:\n- name: Bookmark B\n  category: changed\n  link: https://changed.example.com\n",
	}
	for name, raw := range zipEntries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(raw)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("backup", "editor-backup.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(zipBuf.Bytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/restore", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := restoreData(c); err != nil {
		t.Fatalf("restoreData: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/editor?notice=") {
		t.Fatalf("expected success notice redirect, got %q", location)
	}
	if !strings.Contains(location, "restore_success") {
		t.Fatalf("expected restore success notice key, got %q", location)
	}
}

func TestBackupDataReturnsStyledErrorWhenSourceReadFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.Mkdir("config.yml", 0755); err != nil {
		t.Fatalf("mkdir config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor/backup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := backupData(c); err != nil {
		t.Fatalf("backupData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "superflare-backup-") {
		t.Fatalf("expected backup export to fail instead of returning an attachment, headers=%v body=%s", rec.Header(), rec.Body.String())
	}
}

func TestBackupDataReturnsStyledErrorWhenSourceMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor/backup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := backupData(c); err != nil {
		t.Fatalf("backupData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "backup source file is missing") {
		t.Fatalf("expected missing source detail, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "superflare-backup-") {
		t.Fatalf("expected backup export to fail instead of returning an attachment, headers=%v body=%s", rec.Header(), rec.Body.String())
	}
}

func TestBackupDataReturnsStyledErrorWhenSourceEmpty(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	files := map[string]string{
		"config.yml":    "",
		"bookmarks.yml": "categories:\n- id: default\n  title: Default\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n",
		"apps.yml":      "links:\n- name: App A\n  link: https://public.example.com/app\n",
		"ports.yaml":    "ports:\n- port: 3636\n  protocol: tcp\n  remark: superflare\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(raw), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor/backup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := backupData(c); err != nil {
		t.Fatalf("backupData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "backup source file is empty") {
		t.Fatalf("expected empty source detail, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "superflare-backup-") {
		t.Fatalf("expected backup export to fail instead of returning an attachment, headers=%v body=%s", rec.Header(), rec.Body.String())
	}
}

func TestUpdateDataReturnsStyledErrorPageWhenPayloadInvalid(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor", strings.NewReader("categories=1,Links&bookmarks=broken"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := updateData(c); err != nil {
		t.Fatalf("updateData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "解析书签数据失败") {
		t.Fatalf("expected bookmark parse detail, got %s", rec.Body.String())
	}
}

func TestUpdateDataReturnsJSONWhenPayloadInvalidAndJSONAccepted(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor", strings.NewReader("categories=1,Links&bookmarks=broken"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := updateData(c); err != nil {
		t.Fatalf("updateData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected JSON error, got styled page: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "解析书签数据失败") {
		t.Fatalf("expected bookmark parse detail, got %s", rec.Body.String())
	}
	if contentType := rec.Header().Get(echo.HeaderContentType); !strings.Contains(contentType, echo.MIMEApplicationJSON) {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}
}

func TestUpdateDataReturnsStyledErrorPageWhenCategoryIDsDuplicate(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor", strings.NewReader("categories=1,Links%0A1,Links2&bookmarks="))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := updateData(c); err != nil {
		t.Fatalf("updateData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ID") {
		t.Fatalf("expected duplicate category id detail, got %s", rec.Body.String())
	}
}

func TestUpdateDataRedirectsWithSuccessNoticeWhenSaveSucceeds(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	files := map[string]string{
		"config.yml":    "Title: SuperFlare\nLocale: zh\nTheme: blackboard\n",
		"bookmarks.yml": "categories:\n- id: default\n  title: Default\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n",
		"apps.yml":      "links:\n- name: App A\n  link: https://public.example.com/app\n",
		"ports.yaml":    "ports:\n- port: 3636\n  protocol: tcp\n  remark: superflare\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(raw), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	e := echo.New()
	form := strings.NewReader("categories=1,Links&bookmarks=1,Bookmark%20A,https://bookmark.example.com,,Links,,,")
	req := httptest.NewRequest(http.MethodPost, "/editor", form)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := updateData(c); err != nil {
		t.Fatalf("updateData: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/editor?notice=") {
		t.Fatalf("expected success notice redirect, got %q", location)
	}
	if !strings.Contains(location, "save_success") {
		t.Fatalf("expected save success notice key, got %q", location)
	}
}

func TestUpdateDataSurfacesSettingsConfigErrorAlongsideValidationError(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := updateData(c); err != nil {
		t.Fatalf("updateData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing form data") {
		t.Fatalf("expected missing form data detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "settings config error") {
		t.Fatalf("expected explicit settings config error detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "parse config config failed") {
		t.Fatalf("expected broken config detail, got %s", rec.Body.String())
	}
}

func TestCheckLinksReturnsStructuredErrorWhenPayloadInvalid(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/check-links", strings.NewReader(`{"bookmarks":"a,b,\""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := checkLinks(c); err != nil {
		t.Fatalf("checkLinks: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "parse links payload failed") {
		t.Fatalf("expected parse links payload failure, got %s", rec.Body.String())
	}
}

func TestCheckLinksReturnsImmediateInvalidResultForMalformedPublicURL(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/check-links", strings.NewReader(`{"bookmarks":"1,Bookmark A,http://,,Links,,icon,desc"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := checkLinks(c); err != nil {
		t.Fatalf("checkLinks: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"invalid"`) {
		t.Fatalf("expected invalid result payload, got %s", body)
	}
	if !strings.Contains(body, `"row":1`) {
		t.Fatalf("expected invalid result row payload, got %s", body)
	}
}

func TestCheckLinksAcceptsCurrentTenFieldEditorRows(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/check-links", strings.NewReader(`{"bookmarks":"1,Bookmark A,http://,,Links,,icon,desc,true,false"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := checkLinks(c); err != nil {
		t.Fatalf("checkLinks: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected current editor row to be accepted, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"invalid"`) {
		t.Fatalf("expected malformed public URL result, got %s", rec.Body.String())
	}
}

func TestParseLinksForCheckRejectsIncompleteNonBlankRow(t *testing.T) {
	_, err := parseLinksForCheck("1,Bookmark A")
	if err == nil {
		t.Fatal("expected incomplete link-check row to fail")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLinksForCheckRejectsInvalidRowNumber(t *testing.T) {
	_, err := parseLinksForCheck("abc,Bookmark A,https://example.com,,Links,,icon,desc")
	if err == nil {
		t.Fatal("expected invalid row number to fail")
	}
	if !strings.Contains(err.Error(), "invalid row number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLinksForCheckStillSkipsBlankAndLocalRows(t *testing.T) {
	payload, err := parseLinksForCheck(",,,,,,,\n1,Bookmark A,http://192.168.0.10,,Links,,icon,desc\n2,Bookmark B,https://example.com,,Links,,icon,desc")
	if err != nil {
		t.Fatalf("parseLinksForCheck: %v", err)
	}
	if len(payload.ImmediateResult) != 0 {
		t.Fatalf("expected no immediate results, got %#v", payload.ImmediateResult)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected exactly one public item, got %#v", payload.Items)
	}
	if payload.Items[0].Row != 2 || payload.Items[0].URL != "https://example.com" {
		t.Fatalf("unexpected parsed item: %#v", payload.Items[0])
	}
}

func TestParseLinksForCheckReturnsImmediateInvalidResultForMalformedHTTPURL(t *testing.T) {
	payload, err := parseLinksForCheck("1,Bookmark A,http://,,Links,,icon,desc")
	if err != nil {
		t.Fatalf("parseLinksForCheck: %v", err)
	}
	if len(payload.Items) != 0 {
		t.Fatalf("expected no network-check items, got %#v", payload.Items)
	}
	if len(payload.ImmediateResult) != 1 {
		t.Fatalf("expected one immediate invalid result, got %#v", payload.ImmediateResult)
	}
	if payload.ImmediateResult[0].Status != "invalid" {
		t.Fatalf("expected invalid status, got %#v", payload.ImmediateResult[0])
	}
	if !strings.Contains(strings.ToLower(payload.ImmediateResult[0].Reason), "host") {
		t.Fatalf("expected missing host reason, got %#v", payload.ImmediateResult[0])
	}
}

func TestParseLinksForCheckSkipsNonHTTPURLSchemes(t *testing.T) {
	payload, err := parseLinksForCheck("1,Bookmark A,chrome-extension://abc,,Links,,icon,desc")
	if err != nil {
		t.Fatalf("parseLinksForCheck: %v", err)
	}
	if len(payload.Items) != 0 || len(payload.ImmediateResult) != 0 {
		t.Fatalf("expected non-http scheme to be skipped, got %#v", payload)
	}
}

func TestRestoreZipRejectsOversizedEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("config.yml")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("a"), restoreZipEntryMaxBytes+1)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	err = restoreZip(buf.Bytes())
	if err == nil {
		t.Fatal("expected oversized zip entry to be rejected")
	}
	if !strings.Contains(err.Error(), "oversized") {
		t.Fatalf("expected oversized message, got %v", err)
	}
}

func TestRestoreZipRejectsDuplicateRestorableFiles(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	first, err := zw.Create("config.yml")
	if err != nil {
		t.Fatalf("create first zip entry: %v", err)
	}
	if _, err := first.Write([]byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n")); err != nil {
		t.Fatalf("write first zip entry: %v", err)
	}
	second, err := zw.Create("config.yaml")
	if err != nil {
		t.Fatalf("create second zip entry: %v", err)
	}
	if _, err := second.Write([]byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n")); err != nil {
		t.Fatalf("write second zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	err = restoreZip(buf.Bytes())
	if err == nil {
		t.Fatal("expected duplicate restorable files to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate message, got %v", err)
	}
}

func TestReadRestoreZipEntryReturnsCloseError(t *testing.T) {
	raw, tooLarge, err := readRestoreZipEntry(newStubReadCloser([]byte("Title: SuperFlare\n"), errors.New("zip crc mismatch")), 1024)
	if err == nil {
		t.Fatal("expected close error to surface")
	}
	if tooLarge {
		t.Fatal("did not expect tooLarge on close error")
	}
	if raw != nil {
		t.Fatalf("expected no payload when close fails, got %q", string(raw))
	}
	if !strings.Contains(err.Error(), "zip crc mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRollbackPendingRestoreFilesRestoresBackups(t *testing.T) {
	tmpDir := t.TempDir()
	targetA := filepath.Join(tmpDir, "config.yml")
	targetB := filepath.Join(tmpDir, "bookmarks.yml")
	backupA := filepath.Join(tmpDir, "config.yml.backup")
	backupB := filepath.Join(tmpDir, "bookmarks.yml.backup")
	tempA := filepath.Join(tmpDir, "config.yml.restore")
	tempB := filepath.Join(tmpDir, "bookmarks.yml.restore")

	if err := os.WriteFile(backupA, []byte("original-a"), 0644); err != nil {
		t.Fatalf("write backupA: %v", err)
	}
	if err := os.WriteFile(backupB, []byte("original-b"), 0644); err != nil {
		t.Fatalf("write backupB: %v", err)
	}
	if err := os.WriteFile(targetA, []byte("new-a"), 0644); err != nil {
		t.Fatalf("write targetA: %v", err)
	}
	if err := os.WriteFile(tempA, []byte("temp-a"), 0644); err != nil {
		t.Fatalf("write tempA: %v", err)
	}
	if err := os.WriteFile(tempB, []byte("temp-b"), 0644); err != nil {
		t.Fatalf("write tempB: %v", err)
	}

	err := rollbackPendingRestoreFiles([]pendingRestoreFile{
		{target: targetA, temp: tempA, backup: backupA, name: "config"},
		{target: targetB, temp: tempB, backup: backupB, name: "bookmarks"},
	}, 0)
	if err != nil {
		t.Fatalf("rollbackPendingRestoreFiles: %v", err)
	}

	gotA, err := os.ReadFile(targetA)
	if err != nil {
		t.Fatalf("read targetA: %v", err)
	}
	if string(gotA) != "original-a" {
		t.Fatalf("targetA not restored, got %q", string(gotA))
	}
	gotB, err := os.ReadFile(targetB)
	if err != nil {
		t.Fatalf("read targetB: %v", err)
	}
	if string(gotB) != "original-b" {
		t.Fatalf("targetB not restored, got %q", string(gotB))
	}
}

func TestRegisterAssetRouting_ExposesRuntimeAsset(t *testing.T) {
	e := echo.New()
	RegisterAssetRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/assets/table/runtime.js", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime asset route to return 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("unexpected content-type %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "regeneratorRuntime") {
		t.Fatal("runtime asset does not look like regenerator runtime")
	}
}

func TestRegisterAssetRoutingKeepsStoredDebugModeAfterAppFlagsChange(t *testing.T) {
	origFlags := define.AppFlags
	origRuntime, origRuntimeSet := saveEditorRuntimeFlags()
	defer func() {
		define.AppFlags = origFlags
		restoreEditorRuntimeFlags(origRuntime, origRuntimeSet)
	}()

	define.AppFlags = model.Flags{DebugMode: true}
	editorRuntimeFlags.Store(editorRuntimeSnapshotFromFlags(define.AppFlags))

	e := echo.New()
	RegisterAssetRouting(e)

	define.AppFlags = model.Flags{DebugMode: false}

	if got := getDebugAssetVersion(); got != "?v=dev" {
		t.Fatalf("expected debug asset version to stay bound, got %q", got)
	}
}

func TestRenderReturnsStyledErrorPageWhenEditorDataBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	origFlags := define.AppFlags
	define.AppFlags = model.Flags{DisableLoginMode: true}
	defer func() { define.AppFlags = origFlags }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("items:\n- name: app\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("Categories: [broken\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SuperFlare") {
		t.Fatalf("expected status page body to mention site brand, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenEditorPortsMarshalFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	originalMarshal := marshalEditorPortsJSON
	marshalEditorPortsJSON = func(v interface{}) ([]byte, error) {
		return nil, errors.New("forced marshal failure")
	}
	defer func() { marshalEditorPortsJSON = originalMarshal }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("items:\n- name: app\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: 默认\nlinks:\n- name: bookmark\n  category: default\n  link: https://bookmark.example.com\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("ports:\n- port: 3060\n  protocol: tcp\n  remark: dev\n"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderWarnsAndSanitizesInvalidSiteIcon(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nSiteIcon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: app\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: Default\nlinks:\n- name: bookmark\n  category: default\n  link: https://bookmark.example.com\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("ports:\n- port: 3060\n  protocol: tcp\n  remark: dev\n"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	e := echo.New()
	e.Renderer = editorCaptureRenderer{t: t}
	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRenderBindsOperationNoticeFromQuery(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	files := map[string]string{
		"config.yml":    "Title: SuperFlare\nLocale: zh\nTheme: blackboard\n",
		"bookmarks.yml": "categories:\n- id: default\n  title: Default\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n",
		"apps.yml":      "links:\n- name: App A\n  link: https://public.example.com/app\n",
		"ports.yaml":    "ports:\n- port: 3636\n  protocol: tcp\n  remark: superflare\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(raw), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	e := echo.New()
	e.Renderer = editorNoticeCaptureRenderer{t: t}
	req := httptest.NewRequest(http.MethodGet, "/editor?notice=save_success", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

type editorCaptureRenderer struct {
	t *testing.T
}

func (r editorCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	if got, _ := m["OptionSiteIcon"].(string); got != "" {
		r.t.Fatalf("expected sanitized OptionSiteIcon, got %q", got)
	}
	warnings, ok := m["RenderWarnings"].([]string)
	if !ok || len(warnings) != 1 {
		r.t.Fatalf("expected one render warning, got %#v", m["RenderWarnings"])
	}
	if !strings.Contains(warnings[0], "Site icon config error") {
		r.t.Fatalf("unexpected render warning: %v", warnings[0])
	}
	return nil
}

type editorNoticeCaptureRenderer struct {
	t *testing.T
}

func (r editorNoticeCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	notice, ok := m["OperationNotice"].(map[string]string)
	if !ok {
		r.t.Fatalf("expected OperationNotice map, got %#v", m["OperationNotice"])
	}
	if notice["Type"] != "success" {
		r.t.Fatalf("expected success notice type, got %#v", notice)
	}
	if notice["Text"] != "书签与分类数据保存成功。" {
		r.t.Fatalf("expected localized save notice text, got %#v", notice)
	}
	return nil
}

type editorPortsCaptureRenderer struct {
	t    *testing.T
	want string
}

func (r editorPortsCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	got := strings.TrimSpace(fmt.Sprint(m["DataPorts"]))
	if got != r.want {
		r.t.Fatalf("expected DataPorts %s, got %s", r.want, got)
	}
	if got, _ := m["ShowEditorPortPicker"].(bool); got {
		r.t.Fatal("expected editor port picker to be disabled")
	}
	if got, _ := m["LocalLANHost"].(string); got != "" {
		r.t.Fatalf("expected LocalLANHost to be hidden, got %q", got)
	}
	return nil
}

func writeEditorRenderFiles(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"config.yml":    "Title: SuperFlare\nLocale: zh\nTheme: blackboard\n",
		"apps.yml":      "links:\n- name: app\n  link: https://app.example.com\n",
		"bookmarks.yml": "categories:\n- id: default\n  title: Default\nlinks:\n- name: bookmark\n  category: default\n  link: https://bookmark.example.com\n",
		"ports.yaml":    "ports:\n- port: 3060\n  protocol: tcp\n  remark: dev\n",
	}
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestRunLinkChecksWithCheckerBoundsConcurrency(t *testing.T) {
	items := make([]linkCheckItem, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, linkCheckItem{Row: i + 1, URL: fmt.Sprintf("https://example.com/%d", i+1)})
	}

	var current atomic.Int32
	var peak atomic.Int32
	checker := func(ctx context.Context, client *http.Client, item linkCheckItem) linkCheckResult {
		now := current.Add(1)
		for {
			prev := peak.Load()
			if now <= prev || peak.CompareAndSwap(prev, now) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		current.Add(-1)
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: "ok"}
	}

	results := make([]linkCheckResult, 0)
	resultsMu := sync.Mutex{}
	runLinkChecksWithChecker(context.Background(), &http.Client{}, items, 4, &results, &resultsMu, checker)

	if peak.Load() > 4 {
		t.Fatalf("expected peak concurrency <= 4, got %d", peak.Load())
	}
}

func TestRunLinkChecksWithCheckerStopsAfterContextCanceled(t *testing.T) {
	items := make([]linkCheckItem, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, linkCheckItem{Row: i + 1, URL: fmt.Sprintf("https://example.com/%d", i+1)})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var started atomic.Int32
	checker := func(ctx context.Context, client *http.Client, item linkCheckItem) linkCheckResult {
		count := started.Add(1)
		if count == 3 {
			cancel()
		}
		select {
		case <-ctx.Done():
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "unstable", Reason: ctx.Err().Error()}
		case <-time.After(20 * time.Millisecond):
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "ok"}
		}
	}

	results := make([]linkCheckResult, 0)
	resultsMu := sync.Mutex{}
	runLinkChecksWithChecker(ctx, &http.Client{}, items, 4, &results, &resultsMu, checker)

	if started.Load() >= int32(len(items)) {
		t.Fatalf("expected context cancellation to stop scheduling early, started=%d total=%d", started.Load(), len(items))
	}
}

func TestRunLinkChecksWithCheckerMarksPendingItemsWhenDeadlineExpires(t *testing.T) {
	items := make([]linkCheckItem, 0, 6)
	for i := 0; i < 6; i++ {
		items = append(items, linkCheckItem{Row: i + 1, URL: fmt.Sprintf("https://example.com/%d", i+1)})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	checker := func(ctx context.Context, client *http.Client, item linkCheckItem) linkCheckResult {
		select {
		case <-ctx.Done():
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "unstable", Reason: ctx.Err().Error()}
		case <-time.After(200 * time.Millisecond):
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "ok"}
		}
	}

	start := time.Now()
	results := make([]linkCheckResult, 0)
	resultsMu := sync.Mutex{}
	runLinkChecksWithChecker(ctx, &http.Client{}, items, 2, &results, &resultsMu, checker)
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Fatalf("expected deadline-bounded link check run, took %v", elapsed)
	}
	if len(results) != len(items) {
		t.Fatalf("expected every item to receive a result after timeout, got %d of %d: %#v", len(results), len(items), results)
	}
	timeoutPendingCount := 0
	for _, result := range results {
		if result.Status != "unstable" {
			t.Fatalf("expected timeout result to be unstable, got %#v", result)
		}
		if strings.Contains(result.Reason, "before this URL could be verified") {
			timeoutPendingCount++
		}
	}
	if timeoutPendingCount == 0 {
		t.Fatalf("expected unscheduled items to receive explicit timeout reason, got %#v", results)
	}
}

func TestWriteRestoreFilesAtomicallyRefreshesRuntimeLoginConfig(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	}()

	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "old-user",
		Pass:         "old-pass",
	}
	define.AppBaseFlags = define.AppFlags
	define.AppSourceFlags = define.AppFlags
	auth.StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: restored-user\nLoginPass: restored-pass\n")
	if err := writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig}); err != nil {
		t.Fatalf("writeRestoreFilesAtomically: %v", err)
	}

	snapshot := auth.SnapshotLoginRuntimeConfigForSessionName(auth.RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port))
	if snapshot.User != "restored-user" || snapshot.Pass != "restored-pass" {
		t.Fatalf("expected runtime login snapshot to refresh after restore, got user=%q pass=%q", snapshot.User, snapshot.Pass)
	}
	if define.AppFlags.User != "old-user" || define.AppFlags.Pass != "old-pass" {
		t.Fatalf("expected global app flags unchanged after restore refresh, got user=%q pass=%q", define.AppFlags.User, define.AppFlags.Pass)
	}
	if define.AppBaseFlags.User != "old-user" || define.AppBaseFlags.Pass != "old-pass" {
		t.Fatalf("expected global base flags unchanged after restore refresh, got user=%q pass=%q", define.AppBaseFlags.User, define.AppBaseFlags.Pass)
	}
}

func TestWriteRestoreFilesAtomicallyRollsBackWhenRuntimeLoginRefreshFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n"), 0644); err != nil {
		t.Fatalf("write original config: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".env"), 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	}()

	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "old-user",
		Pass:         "old-pass",
	}
	define.AppBaseFlags = define.AppFlags
	define.AppSourceFlags = define.AppFlags
	auth.StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n")
	err = writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig})
	if err == nil {
		t.Fatal("expected writeRestoreFilesAtomically to fail")
	}
	if !strings.Contains(err.Error(), "refresh runtime login config failed") {
		t.Fatalf("expected runtime refresh failure, got %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(tmpDir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	configText := string(configRaw)
	if !strings.Contains(configText, "LoginUser: old-user") || !strings.Contains(configText, "LoginPass: old-pass") {
		t.Fatalf("expected config rollback, got %s", configText)
	}

	snapshot := auth.SnapshotLoginRuntimeConfigForSessionName(auth.RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port))
	if snapshot.User != "old-user" || snapshot.Pass != "old-pass" {
		t.Fatalf("expected runtime snapshot to remain original, got user=%q pass=%q", snapshot.User, snapshot.Pass)
	}
}

func TestRefreshRuntimeLoginConfigReturnsErrorWhenConfigBrokenEvenIfEnvComplete(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write broken config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=env-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	}()

	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "old-user",
		Pass:         "old-pass",
	}
	define.AppBaseFlags = define.AppFlags
	define.AppSourceFlags = define.AppFlags
	auth.StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	err = refreshRuntimeLoginConfig()
	if err == nil {
		t.Fatal("expected refreshRuntimeLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), "read login config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "parse config config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if define.AppFlags.User != "old-user" || define.AppFlags.Pass != "old-pass" {
		t.Fatalf("runtime flags should remain unchanged, got user=%q pass=%q", define.AppFlags.User, define.AppFlags.Pass)
	}
}

func TestWriteRestoreFilesAtomicallyRepairsDefaultLoginWhenBackupClearsConfigAccount(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	}()

	define.AppSourceFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "source-user",
		Pass:         "source-pass",
	}
	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "old-user",
		Pass:         "old-pass",
	}
	define.AppBaseFlags = define.AppFlags
	auth.StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n")
	if err := writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig}); err != nil {
		t.Fatalf("writeRestoreFilesAtomically: %v", err)
	}

	snapshot := auth.SnapshotLoginRuntimeConfigForSessionName(auth.RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port))
	if snapshot.User != define.DEFAULT_LOGIN_USER || snapshot.Pass != define.DEFAULT_LOGIN_PASS {
		t.Fatalf("expected runtime login snapshot to use repaired defaults, got user=%q pass=%q", snapshot.User, snapshot.Pass)
	}
	if define.AppFlags.User != "old-user" || define.AppFlags.Pass != "old-pass" {
		t.Fatalf("expected global app flags unchanged after default repair, got user=%q pass=%q", define.AppFlags.User, define.AppFlags.Pass)
	}
	if define.AppBaseFlags.User != "old-user" || define.AppBaseFlags.Pass != "old-pass" {
		t.Fatalf("expected global base flags unchanged after default repair, got user=%q pass=%q", define.AppBaseFlags.User, define.AppBaseFlags.Pass)
	}
	configRaw, err := os.ReadFile(filepath.Join(tmpDir, "config.yml"))
	if err != nil {
		t.Fatalf("read repaired config.yml: %v", err)
	}
	configText := string(configRaw)
	if !strings.Contains(configText, "LoginUser: "+define.DEFAULT_LOGIN_USER) || !strings.Contains(configText, "LoginPass: "+define.DEFAULT_LOGIN_PASS) {
		t.Fatalf("expected restored config credentials to be repaired to defaults, got %s", configText)
	}
}

func TestWriteRestoreFilesAtomicallyRepairsDefaultLoginWithStoredRuntimeSession(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origRuntime, runtimeSet := define.SnapshotAppRuntimeFlags()
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		if runtimeSet {
			define.StoreAppRuntimeFlags(origRuntime.Source, origRuntime.Base, origRuntime.Current)
		} else {
			define.ResetAppRuntimeFlags()
		}
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	}()

	define.StoreAppRuntimeFlags(
		model.Flags{
			Port:         3636,
			CookieName:   "runtime-cookie",
			CookieSecret: "runtime-secret",
			User:         "runtime-source-user",
			Pass:         "runtime-source-pass",
		},
		model.Flags{
			Port:         3636,
			CookieName:   "runtime-cookie",
			CookieSecret: "runtime-secret",
			User:         "runtime-base-user",
			Pass:         "runtime-base-pass",
		},
		model.Flags{
			Port:         3636,
			CookieName:   "runtime-cookie",
			CookieSecret: "runtime-secret",
			User:         "runtime-current-user",
			Pass:         "runtime-current-pass",
		},
	)
	define.AppSourceFlags = model.Flags{
		Port:         3737,
		CookieName:   "stale-cookie",
		CookieSecret: "stale-secret",
		User:         "stale-source-user",
		Pass:         "stale-source-pass",
	}
	define.AppBaseFlags = define.AppSourceFlags
	define.AppFlags = define.AppSourceFlags
	auth.StoreLoginRuntimeConfigFromFlags(model.Flags{
		Port:         3636,
		CookieName:   "runtime-cookie",
		CookieSecret: "runtime-secret",
		User:         "old-user",
		Pass:         "old-pass",
	})

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n")
	if err := writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig}); err != nil {
		t.Fatalf("writeRestoreFilesAtomically: %v", err)
	}

	snapshot := auth.SnapshotLoginRuntimeConfigForSessionName(auth.RequestHandleSessionName("runtime-cookie", 3636))
	if snapshot.User != define.DEFAULT_LOGIN_USER || snapshot.Pass != define.DEFAULT_LOGIN_PASS {
		t.Fatalf("expected stored runtime session to receive repaired defaults, got user=%q pass=%q", snapshot.User, snapshot.Pass)
	}
}

func TestWriteRestoreFilesAtomicallySkipsConfigRefreshWhenRestoringBookmarksOnly(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write broken config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: Old\nlinks:\n- name: Old\n  category: default\n  link: https://old.example.com\n"), 0644); err != nil {
		t.Fatalf("write old bookmarks.yml: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".env"), 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	rawBookmarks := []byte("categories:\n- id: default\n  title: New\nlinks:\n- name: New Bookmark\n  category: default\n  link: https://bookmark.example.com\n")
	if err := writeRestoreFilesAtomically(map[string][]byte{"bookmarks": rawBookmarks}); err != nil {
		t.Fatalf("writeRestoreFilesAtomically bookmarks-only: %v", err)
	}

	bookmarksRaw, err := os.ReadFile(filepath.Join(tmpDir, "bookmarks.yml"))
	if err != nil {
		t.Fatalf("read bookmarks.yml: %v", err)
	}
	bookmarksText := string(bookmarksRaw)
	if !strings.Contains(bookmarksText, "New Bookmark") || !strings.Contains(bookmarksText, "bookmark.example.com") {
		t.Fatalf("expected bookmarks restore to persist new content, got %s", bookmarksText)
	}
}

func TestWriteRestoreFilesAtomicallySurfacesPaletteRefreshFailureAfterRollback(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n"), 0644); err != nil {
		t.Fatalf("write original config: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".env"), 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	origRefresh := refreshPagePaletteCache
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
		refreshPagePaletteCache = origRefresh
	}()

	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "old-user",
		Pass:         "old-pass",
	}
	define.AppBaseFlags = define.AppFlags
	define.AppSourceFlags = define.AppFlags
	auth.StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	refreshCalls := 0
	refreshPagePaletteCache = func() error {
		refreshCalls++
		if refreshCalls == 1 {
			return nil
		}
		return errors.New("forced rollback palette refresh failure")
	}

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n")
	err = writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig})
	if err == nil {
		t.Fatal("expected writeRestoreFilesAtomically to fail")
	}
	if !strings.Contains(err.Error(), "refresh runtime login config failed") {
		t.Fatalf("expected runtime refresh failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "refresh page palette cache after rollback failed") {
		t.Fatalf("expected rollback palette refresh failure detail, got %v", err)
	}
}

func TestWriteRestoreFilesAtomicallyFailsWhenTargetPathIsDirectory(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configPath := filepath.Join(tmpDir, "config.yml")
	if err := os.Mkdir(configPath, 0755); err != nil {
		t.Fatalf("mkdir config.yml: %v", err)
	}

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n")
	err = writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig})
	if err == nil {
		t.Fatal("expected directory target failure")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("unexpected error: %v", err)
	}

	info, statErr := os.Stat(configPath)
	if statErr != nil {
		t.Fatalf("stat config.yml: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("config target directory should remain a directory")
	}
}

func TestWriteRestoreFilesAtomicallyFailsWhenGetwdFails(t *testing.T) {
	originalResolve := resolveRestoreConfigPath
	defer func() { resolveRestoreConfigPath = originalResolve }()

	resolveRestoreConfigPath = func(name string) (string, error) {
		return "", errors.New("forced getwd failure")
	}

	rawConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n")
	err := writeRestoreFilesAtomically(map[string][]byte{"config": rawConfig})
	if err == nil {
		t.Fatal("expected writeRestoreFilesAtomically to fail")
	}
	if !strings.Contains(err.Error(), "forced getwd failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteRestoreFilesAtomicallyCleansPendingTempsWhenResolveFailsMidway(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	originalResolve := resolveRestoreConfigPath
	defer func() { resolveRestoreConfigPath = originalResolve }()

	resolveCalls := 0
	resolveRestoreConfigPath = func(name string) (string, error) {
		resolveCalls++
		if resolveCalls == 1 {
			return filepath.Join(tmpDir, "config.yml"), nil
		}
		return "", errors.New("forced resolve failure")
	}

	err = writeRestoreFilesAtomically(map[string][]byte{
		"config": []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"),
		"ports":  []byte("ports: []\n"),
	})
	if err == nil {
		t.Fatal("expected writeRestoreFilesAtomically to fail")
	}
	if !strings.Contains(err.Error(), "forced resolve failure") {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("readdir temp dir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".restore-") {
			t.Fatalf("expected restore temp files to be cleaned, found %s", entry.Name())
		}
	}
}

func TestWriteRestoreFilesAtomicallyCleansPendingTempsWhenSyncFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	originalSync := syncRestoreTempFile
	syncRestoreTempFile = func(file *os.File) error {
		return errors.New("forced restore temp sync failure")
	}
	defer func() { syncRestoreTempFile = originalSync }()

	err = writeRestoreFilesAtomically(map[string][]byte{
		"config": []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"),
	})
	if err == nil {
		t.Fatal("expected writeRestoreFilesAtomically to fail")
	}
	if !strings.Contains(err.Error(), "forced restore temp sync failure") {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("readdir temp dir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".restore-") {
			t.Fatalf("expected restore temp files to be cleaned after sync failure, found %s", entry.Name())
		}
	}
}

func TestBackupDataReturnsStyledErrorWhenResolvePathFails(t *testing.T) {
	originalResolve := resolveRestoreConfigPath
	defer func() { resolveRestoreConfigPath = originalResolve }()

	resolveRestoreConfigPath = func(name string) (string, error) {
		return "", errors.New("forced getwd failure")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor/backup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := backupData(c); err != nil {
		t.Fatalf("backupData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled error page, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "superflare-backup-") {
		t.Fatalf("expected backup export to fail instead of returning an attachment, headers=%v body=%s", rec.Header(), rec.Body.String())
	}
}

func TestBackupDataSurfacesSettingsConfigErrorAlongsideSourceFailure(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/editor/backup", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := backupData(c); err != nil {
		t.Fatalf("backupData: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "backup source file is missing") {
		t.Fatalf("expected backup source failure detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "settings config error") {
		t.Fatalf("expected explicit settings config error detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "parse config config failed") {
		t.Fatalf("expected broken config detail, got %s", rec.Body.String())
	}
}

func TestRestoreDataSurfacesSettingsConfigErrorAlongsideValidationError(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/editor/restore", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := restoreData(c); err != nil {
		t.Fatalf("restoreData: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing backup file") {
		t.Fatalf("expected missing backup file detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "settings config error") {
		t.Fatalf("expected explicit settings config error detail, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "parse config config failed") {
		t.Fatalf("expected broken config detail, got %s", rec.Body.String())
	}
}

func TestRefreshRequestLoginRuntimeUpdatesCurrentRouterSnapshot(t *testing.T) {
	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	defer func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
	}()

	define.AppFlags = model.Flags{
		Port:         3636,
		CookieName:   "superflare",
		CookieSecret: "restore-secret",
		User:         "restored-user",
		Pass:         "restored-pass",
	}
	define.AppBaseFlags = define.AppFlags

	e := echo.New()
	auth.RequestHandle(e)
	e.GET("/probe", func(c *echo.Context) error {
		auth.StoreLoginRuntimeConfigForRequest(c, auth.SnapshotLoginRuntimeConfigFromFlags(model.Flags{User: "old-user", Pass: "old-pass"}))
		refreshRequestLoginRuntime(c)
		snapshot := auth.SnapshotLoginRuntimeConfigForRequest(c)
		if snapshot.User != "restored-user" || snapshot.Pass != "restored-pass" {
			t.Fatalf("expected current router snapshot to refresh, got user=%q pass=%q", snapshot.User, snapshot.Pass)
		}
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func saveEditorRuntimeFlags() (editorRuntimeSnapshot, bool) {
	editorRuntimeFlags.mu.RLock()
	defer editorRuntimeFlags.mu.RUnlock()
	return editorRuntimeFlags.cfg, editorRuntimeFlags.set
}

func restoreEditorRuntimeFlags(cfg editorRuntimeSnapshot, set bool) {
	editorRuntimeFlags.mu.Lock()
	editorRuntimeFlags.cfg = cfg
	editorRuntimeFlags.set = set
	editorRuntimeFlags.mu.Unlock()
}
