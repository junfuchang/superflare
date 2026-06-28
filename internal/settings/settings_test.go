package settings

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/labstack/echo/v5"
)

func TestPageHomePreservesQueryStringOnRedirect(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings?session-warning=session-invalid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	expected := define.SettingPages.Theme.Path + "?session-warning=session-invalid"
	if got := rec.Header().Get("Location"); got != expected {
		t.Fatalf("redirect location = %q, want %q", got, expected)
	}
}
