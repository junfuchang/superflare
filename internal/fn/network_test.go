package fn

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetHTMLRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("a", maxHTMLResponseBytes+1)))
	}))
	defer server.Close()

	_, err := GetHTML(server.URL)
	if err == nil {
		t.Fatal("expected oversized html response to fail")
	}
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("expected errResponseTooLarge, got %v", err)
	}
}

func TestGetJSONRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	var payload map[string]any
	err := GetJSON(server.URL, &payload)
	if err == nil {
		t.Fatal("expected non-success status to fail")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestGetJSONRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"payload": strings.Repeat("x", maxJSONResponseBytes),
		})
	}))
	defer server.Close()

	var payload map[string]any
	err := GetJSON(server.URL, &payload)
	if err == nil {
		t.Fatal("expected oversized json response to fail")
	}
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("expected errResponseTooLarge, got %v", err)
	}
}
