package fn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGetWorkDir(t *testing.T) {
	dir := GetWorkDir()
	if dir == "" {
		t.Fatal("GetWorkDir should not return empty string")
	}
}

func TestGetWorkDirFile(t *testing.T) {
	out := GetWorkDirFile("any")
	if out == "" {
		t.Fatal("GetWorkDirFile should not return empty")
	}
	if filepath.Base(out) != "any" {
		t.Errorf("GetWorkDirFile should use filename: want base %q, got %q", "any", filepath.Base(out))
	}
	outEnv := GetWorkDirFile(".env")
	if filepath.Base(outEnv) != ".env" {
		t.Errorf("GetWorkDirFile(.env) should end with .env, got %q", outEnv)
	}
}

func TestGetWorkDirFileUsesFilepathJoinSemantics(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	want := filepath.Join(dir, "nested", "config.yml")
	got := GetWorkDirFile(filepath.Join("nested", "config.yml"))
	if got != want {
		t.Fatalf("GetWorkDirFile should use filepath semantics: want %q, got %q", want, got)
	}
}

func TestGetWorkDirEReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := GetWorkDirE()
	if err == nil {
		t.Fatal("expected GetWorkDirE to fail")
	}
}

func TestGetWorkDirFileEReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := GetWorkDirFileE(".env")
	if err == nil {
		t.Fatal("expected GetWorkDirFileE to fail")
	}
}
