package editor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestMarshalEditorPortsOnlyIncludesRemarkedPorts(t *testing.T) {
	got := marshalEditorPorts([]model.PortBinding{
		{Port: 3060, Protocol: "tcp", Remark: "dev"},
		{Port: 8080, Protocol: "tcp"},
		{Port: 5353, Protocol: "udp", Remark: "dns"},
		{Port: 9090, Protocol: "tcp", Remark: "hidden", Hidden: true},
	})
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

	result := checkOneLink(server.Client(), linkCheckItem{Row: 3, URL: server.URL})
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

	result := checkOneLink(server.Client(), linkCheckItem{Row: 5, URL: server.URL})
	if result.Status != "invalid" {
		t.Fatalf("expected 404 to remain invalid, got %+v", result)
	}
	if !strings.Contains(result.Reason, "404") {
		t.Fatalf("expected 404 reason, got %+v", result)
	}
}
