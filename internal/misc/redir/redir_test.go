package redir

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/labstack/echo/v5"
)

func TestBookmarksContainLocalPairWithDynamicURL(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://192.168.1.10:3636/redir/local", nil)
	req.Host = "192.168.1.10:3636"
	requestURL := fn.ParseRequestURLTo(req)
	bookmarks := []model.Bookmark{
		{URL: "{origin}/app", LocalURL: "http://192.168.1.20/app"},
	}
	if !bookmarksContainLocalPair(bookmarks, &requestURL, "http://192.168.1.10:3636/app", "http://192.168.1.20/app") {
		t.Fatal("expected bookmark local URL pair to match")
	}
}

func TestRenderLocalRedirectPageContainsFallbackAndLocalURL(t *testing.T) {
	page := string(renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{Locale: "en"}))
	for _, token := range []string{
		"https://public.example.com/app",
		"http://192.168.1.20/app",
		"Use source address",
		"fetch(localURL",
		"var seconds=3",
		"startFallbackCountdown",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRenderLocalRedirectPageDefaultsToChinese(t *testing.T) {
	page := string(renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{}))
	for _, token := range []string{
		`lang="zh-CN"`,
		"正在连接内网地址",
		"打开源书签地址",
		"秒后打开源书签地址",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRenderLocalRedirectPageUsesEnglishLocale(t *testing.T) {
	page := string(renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{Locale: "en"}))
	for _, token := range []string{
		`lang="en"`,
		"Connecting to the local address",
		"Opening the source bookmark in ",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRedirHelperInvalidTargetUsesStyledStatusPage(t *testing.T) {
	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/url", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, token := range []string{"跳转地址无效", "status-panel", "返回首页"} {
		if !strings.Contains(body, token) {
			t.Fatalf("redirect error page missing %q in %s", token, body)
		}
	}
}
