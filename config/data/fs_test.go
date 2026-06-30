package data

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckExists(t *testing.T) {

	exist := checkExists("NOT_EXIST")
	if exist != false {
		t.Fatal("check exist failed")
	}

	workDir, _ := os.Getwd()
	exist = checkExists(workDir)
	if exist != true {
		t.Fatal("check exist failed")
	}

}

func TestPathExistsReturnsErrorWhenStatFailsUnexpectedly(t *testing.T) {
	originalStat := osStat
	defer func() { osStat = originalStat }()

	targetPath := filepath.Join(t.TempDir(), "config.yml")
	osStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(targetPath) {
			return nil, errors.New("forced stat failure")
		}
		return originalStat(path)
	}

	exists, err := pathExists(targetPath)
	if err == nil {
		t.Fatal("expected pathExists to return error")
	}
	if exists {
		t.Fatal("expected pathExists to report path missing on stat failure")
	}
}

func TestConfigPathReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := configPath("config")
	if err == nil {
		t.Fatal("expected configPath to fail when getwd fails")
	}
	if err.Error() == "" {
		t.Fatal("expected configPath error message")
	}
}

func TestSaveAndReadFile(t *testing.T) {

	workDir, _ := os.Getwd()
	filePath := filepath.Join(workDir, "test.yml")
	content := []byte("test")

	if err := saveFile(filePath, content); err != nil {
		t.Fatalf("save file failed: %v", err)
	}

	data, err := readFile(filePath)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}

	res := bytes.Compare(content, data)
	if res != 0 {
		t.Fatal("read file failed")
	}

	os.Remove(filePath)
}

func TestSaveFileAtomicallyReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.yml")
	if err := os.WriteFile(filePath, []byte("old"), 0644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	if err := saveFile(filePath, []byte("new")); err != nil {
		t.Fatalf("save file failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("expected overwritten file content, got %q", string(data))
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".test.yml.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files failed: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no leftover temp files, got %v", matches)
	}
}

func TestSaveFilesAtomicallyRollsBackWhenLaterReplaceFails(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.yml")
	secondPath := filepath.Join(dir, "second.yml")

	if err := os.WriteFile(firstPath, []byte("old-first"), 0644); err != nil {
		t.Fatalf("seed first file failed: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("old-second"), 0644); err != nil {
		t.Fatalf("seed second file failed: %v", err)
	}

	originalRename := osRename
	defer func() { osRename = originalRename }()

	renameCalls := 0
	osRename = func(oldPath string, newPath string) error {
		renameCalls++
		if renameCalls == 4 {
			return errors.New("forced rename failure")
		}
		return originalRename(oldPath, newPath)
	}

	err := saveFilesAtomically(map[string][]byte{
		firstPath:  []byte("new-first"),
		secondPath: []byte("new-second"),
	})
	if err == nil {
		t.Fatal("expected saveFilesAtomically to fail")
	}

	firstRaw, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first file failed: %v", err)
	}
	if string(firstRaw) != "old-first" {
		t.Fatalf("expected first file rollback, got %q", string(firstRaw))
	}

	secondRaw, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second file failed: %v", err)
	}
	if string(secondRaw) != "old-second" {
		t.Fatalf("expected second file rollback, got %q", string(secondRaw))
	}
}

func TestSaveFilesAtomicallyRejectsDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "ports.yaml")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}

	err := saveFilesAtomically(map[string][]byte{
		targetDir: []byte("ports: []\n"),
	})
	if err == nil {
		t.Fatal("expected saveFilesAtomically to reject directory target")
	}

	info, statErr := os.Stat(targetDir)
	if statErr != nil {
		t.Fatalf("stat target dir failed: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("directory target should remain a directory")
	}
}

func TestSaveFileWaitsForExternalConfigFileLock(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	release, err := lockConfigFiles()
	if err != nil {
		t.Fatalf("lockConfigFiles: %v", err)
	}

	target := filepath.Join(dir, "config.yml")
	done := make(chan error, 1)
	go func() {
		done <- saveFile(target, []byte("locked-save"))
	}()

	select {
	case err := <-done:
		t.Fatalf("saveFile completed while external lock was still held: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := release(); err != nil {
		t.Fatalf("release config lock: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("saveFile after release failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("saveFile did not complete after external lock was released")
	}
}
