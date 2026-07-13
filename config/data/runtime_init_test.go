package data

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRuntimeDataFilesCreatesMissingFiles(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	for _, name := range []string{"apps.yml", "bookmarks.yml", "ports.yaml"} {
		target := filepath.Join(tmpDir, name)
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		want, err := readDefaultFile(name)
		if err != nil {
			t.Fatalf("read default %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s should match repository defaults", name)
		}
	}
}

func TestEnsureAppConfigExistsCreatesMissingFile(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := EnsureAppConfigExists(); err != nil {
		t.Fatalf("EnsureAppConfigExists: %v", err)
	}
	target := filepath.Join(tmpDir, "config.yml")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected config.yml to exist: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	want, err := readDefaultFile("config.yml")
	if err != nil {
		t.Fatalf("read default config.yml: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("config.yml should match repository defaults")
	}
}
