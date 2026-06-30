package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/junfuchang/superflare/config/model"
)

func TestNormalizePortBindings(t *testing.T) {
	got := normalizePortBindings([]model.PortBinding{
		{Port: 3060, Protocol: "", Remark: " app "},
		{Port: 80, Protocol: "udp", Remark: "dns"},
		{Port: 1234, Protocol: "tcp", Hidden: true},
		{Port: 4321, Protocol: "tcp"},
		{Port: 0, Protocol: "tcp", Remark: "bad"},
		{Port: 3060, Protocol: "tcp", Remark: "new"},
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 bindings, got %#v", got)
	}
	if got[0].Port != 80 || got[0].Protocol != "udp" || got[0].Remark != "dns" {
		t.Fatalf("unexpected first binding: %#v", got[0])
	}
	if got[1].Port != 1234 || got[1].Protocol != "tcp" || !got[1].Hidden {
		t.Fatalf("unexpected hidden binding: %#v", got[1])
	}
	if got[2].Port != 3060 || got[2].Protocol != "tcp" || got[2].Remark != "new" {
		t.Fatalf("unexpected third binding: %#v", got[2])
	}
}

func TestSavePortBindingsUsesConfigWriteLock(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configWriteMu.Lock()
	done := make(chan error, 1)
	go func() {
		done <- SavePortBindings(model.Ports{Items: []model.PortBinding{{Port: 3636, Protocol: "tcp", Remark: "superflare"}}})
	}()

	select {
	case err := <-done:
		configWriteMu.Unlock()
		t.Fatalf("SavePortBindings bypassed config write lock, err=%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	configWriteMu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SavePortBindings after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SavePortBindings did not finish after config write lock was released")
	}
}

func TestGetPortRemarkMapWithErrorReturnsErrorWhenPortsConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("Items: [broken"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	_, err = GetPortRemarkMapWithError()
	if err == nil {
		t.Fatal("expected GetPortRemarkMapWithError to fail")
	}
}

func TestLoadPortBindingsReturnsErrorWhenConfigMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	_, err = LoadPortBindings()
	if err == nil {
		t.Fatal("expected LoadPortBindings to fail")
	}
	if !strings.Contains(err.Error(), "ports config is missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPortBindingsReturnsErrorWhenStatFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	targetPath := filepath.Join(tmpDir, "ports.yaml")
	originalStat := osStat
	defer func() { osStat = originalStat }()
	osStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(targetPath) {
			return nil, errors.New("forced stat failure")
		}
		return originalStat(path)
	}

	_, err = LoadPortBindings()
	if err == nil {
		t.Fatal("expected LoadPortBindings to fail")
	}
	if !strings.Contains(err.Error(), "stat ports config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPortBindingsReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := LoadPortBindings()
	if err == nil {
		t.Fatal("expected LoadPortBindings to fail")
	}
	if !strings.Contains(err.Error(), "resolve config working directory failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPortBindingsReturnsErrorWhenProtocolInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("ports:\n- port: 8080\n  protocol: http\n  remark: bad\n"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	_, err = LoadPortBindings()
	if err == nil {
		t.Fatal("expected LoadPortBindings to fail for invalid protocol")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadPortBindingsReturnsErrorWhenPortOutOfRange(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("ports:\n- port: 70000\n  protocol: tcp\n  remark: bad\n"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	_, err = LoadPortBindings()
	if err == nil {
		t.Fatal("expected LoadPortBindings to fail for out-of-range port")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}
