package home

import (
	"net/url"
	"testing"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
)

func TestRenderBookmarkHrefPrefersLocalURL(t *testing.T) {
	requestURL := &fn.DynamicURL{Hostname: "192.168.1.20"}
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.1.10/app", true, false, requestURL)
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parse href: %v", err)
	}
	if parsed.Path != define.MiscPages.RedirLocal.Path {
		t.Fatalf("expected local redir path, got %q", parsed.Path)
	}
	source, err := data.Base64DecodeUrl(parsed.Query().Get("go"))
	if err != nil {
		t.Fatalf("decode source: %v", err)
	}
	local, err := data.Base64DecodeUrl(parsed.Query().Get("local"))
	if err != nil {
		t.Fatalf("decode local: %v", err)
	}
	if string(source) != "https://public.example.com/app" || string(local) != "http://192.168.1.10/app" {
		t.Fatalf("unexpected redirect params: source=%q local=%q", source, local)
	}
}

func TestRenderBookmarkHrefFallsBackWhenNotLocalNetwork(t *testing.T) {
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.1.10/app", false, false, nil)
	if href != "https://public.example.com/app" {
		t.Fatalf("expected source URL, got %q", href)
	}
}

func TestRenderBookmarkHrefFallsBackWhenLocalURLCannotShareNetwork(t *testing.T) {
	requestURL := &fn.DynamicURL{Hostname: "192.168.0.10"}
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.10.1/app", true, false, requestURL)
	if href != "https://public.example.com/app" {
		t.Fatalf("expected source URL for different local network, got %q", href)
	}
}

func TestRenderBookmarkHrefFallsBackWhenForwardedClientCannotShareNetwork(t *testing.T) {
	requestURL := &fn.DynamicURL{Hostname: "superflare.example.com", RemoteHost: "192.168.0.88"}
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.10.1/app", true, false, requestURL)
	if href != "https://public.example.com/app" {
		t.Fatalf("expected source URL for different forwarded client network, got %q", href)
	}
}

func TestRenderBookmarkHrefKeepsEncryptedSourceWhenNoLocalURL(t *testing.T) {
	href := renderBookmarkHref("https://public.example.com/app", "", true, true, nil)
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parse href: %v", err)
	}
	if parsed.Path != define.MiscPages.RedirHelper.Path {
		t.Fatalf("expected encrypted redir path, got %q", parsed.Path)
	}
}

func TestRenderBookmarkHrefDoesNotUseLocalRedirWhenSourceURLIsNonHTTP(t *testing.T) {
	href := renderBookmarkHref("chrome-extension://abc/index.html", "http://192.168.1.10/app", true, false, nil)
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parse href: %v", err)
	}
	if parsed.Path != define.MiscPages.RedirHelper.Path {
		t.Fatalf("expected helper redir path, got %q", parsed.Path)
	}
	source, err := data.Base64DecodeUrl(parsed.Query().Get("go"))
	if err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if string(source) != "chrome-extension://abc/index.html" {
		t.Fatalf("unexpected source redirect param: %q", source)
	}
}
