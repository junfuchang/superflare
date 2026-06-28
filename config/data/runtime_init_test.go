package data

import (
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
		if _, err := os.Stat(filepath.Join(tmpDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
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
	if _, err := os.Stat(filepath.Join(tmpDir, "config.yml")); err != nil {
		t.Fatalf("expected config.yml to exist: %v", err)
	}
}
