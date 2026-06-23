package home

import (
	"net/url"
	"testing"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
)

func TestRenderBookmarkHrefPrefersLocalURL(t *testing.T) {
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.1.10/app", true, false)
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
	href := renderBookmarkHref("https://public.example.com/app", "http://192.168.1.10/app", false, false)
	if href != "https://public.example.com/app" {
		t.Fatalf("expected source URL, got %q", href)
	}
}

func TestRenderBookmarkHrefKeepsEncryptedSourceWhenNoLocalURL(t *testing.T) {
	href := renderBookmarkHref("https://public.example.com/app", "", true, true)
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parse href: %v", err)
	}
	if parsed.Path != define.MiscPages.RedirHelper.Path {
		t.Fatalf("expected encrypted redir path, got %q", parsed.Path)
	}
}
