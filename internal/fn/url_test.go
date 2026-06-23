package fn

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestParseRequestURL_HTTP(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com:8080/foo/bar", nil)
	r.Host = "example.com:8080"
	ParseRequestURL(r)
	if RequestURL.Host != "example.com:8080" {
		t.Errorf("Host: got %q", RequestURL.Host)
	}
	if RequestURL.Hostname != "example.com" {
		t.Errorf("Hostname: got %q", RequestURL.Hostname)
	}
	if RequestURL.Port != "8080" {
		t.Errorf("Port: got %q", RequestURL.Port)
	}
	if RequestURL.Protocol != "http:" {
		t.Errorf("Protocol: got %q", RequestURL.Protocol)
	}
	if RequestURL.Pathname != "/foo/bar" {
		t.Errorf("Pathname: got %q", RequestURL.Pathname)
	}
}

func TestParseRequestURL_HTTPS(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	r.Host = "example.com"
	r.TLS = &tls.ConnectionState{}
	ParseRequestURL(r)
	if RequestURL.Protocol != "https:" {
		t.Errorf("Protocol: got %q", RequestURL.Protocol)
	}
	if RequestURL.Port != "443" {
		t.Errorf("Port: got %q", RequestURL.Port)
	}
}

func TestParseRequestURL_NoPort(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.Host = "example.com"
	ParseRequestURL(r)
	if RequestURL.Hostname != "example.com" || RequestURL.Port != "80" {
		t.Errorf("Hostname=%q Port=%q", RequestURL.Hostname, RequestURL.Port)
	}
}

func TestParseDynamicUrl(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://localhost:5005/", nil)
	r.Host = "localhost:5005"
	ParseRequestURL(r)
	out := ParseDynamicUrl("origin={origin} host={host} path={pathname}")
	if out != "origin=http://localhost:5005 host=localhost:5005 path=/" {
		t.Errorf("ParseDynamicUrl: got %q", out)
	}
	out2 := ParseDynamicUrl("no placeholders")
	if out2 != "no placeholders" {
		t.Errorf("ParseDynamicUrl no placeholders: got %q", out2)
	}
}

func TestHostLooksLocalNetwork(t *testing.T) {
	tests := map[string]bool{
		"localhost:5005":    true,
		"127.0.0.1:5005":    true,
		"192.168.1.20:5005": true,
		"10.0.0.3":          true,
		"[fd00::1]:5005":    true,
		"fnos.local":        true,
		"nas":               true,
		"example.com":       false,
		"superflare.example.com": false,
		"":                  false,
	}
	for host, expected := range tests {
		if got := HostLooksLocalNetwork(host); got != expected {
			t.Fatalf("HostLooksLocalNetwork(%q): expected %v, got %v", host, expected, got)
		}
	}
}
