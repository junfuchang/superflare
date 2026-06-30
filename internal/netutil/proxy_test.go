package netutil

import (
	"net/http"
	"net/url"
	"testing"
)

func TestProxyFromCurrentEnvironmentReadsLatestEnvironment(t *testing.T) {
	req := &http.Request{URL: mustParseURL(t, "http://example.com/resource")}

	t.Setenv("HTTP_PROXY", "http://127.0.0.1:8080")
	t.Setenv("NO_PROXY", "")
	first, err := ProxyFromCurrentEnvironment(req)
	if err != nil {
		t.Fatalf("ProxyFromCurrentEnvironment first: %v", err)
	}
	if first == nil || first.Host != "127.0.0.1:8080" {
		t.Fatalf("expected first proxy 127.0.0.1:8080, got %v", first)
	}

	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9090")
	second, err := ProxyFromCurrentEnvironment(req)
	if err != nil {
		t.Fatalf("ProxyFromCurrentEnvironment second: %v", err)
	}
	if second == nil || second.Host != "127.0.0.1:9090" {
		t.Fatalf("expected updated proxy 127.0.0.1:9090, got %v", second)
	}
}

func TestProxyDialAllowListMatchesRememberedProxyAddress(t *testing.T) {
	var list ProxyDialAllowList
	list.Remember(mustParseURL(t, "http://127.0.0.1:8080"))

	if !list.Contains("127.0.0.1:8080") {
		t.Fatal("expected remembered proxy address to be allowed")
	}
	if list.Contains("127.0.0.1:9090") {
		t.Fatal("unexpected proxy address should not be allowed")
	}
}

func TestProxyDialAllowListUsesDefaultPorts(t *testing.T) {
	var list ProxyDialAllowList
	list.Remember(mustParseURL(t, "https://proxy.example.com"))

	if !list.Contains("proxy.example.com:443") {
		t.Fatal("expected default https proxy port to be remembered")
	}
	if list.Contains("proxy.example.com:80") {
		t.Fatal("unexpected proxy port should not be allowed")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}
