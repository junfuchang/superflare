package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadFileCachedReloadsWhenFileChangesWithSameSize(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config.yml")

	if err := os.WriteFile(filePath, []byte("a"), 0644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	initialTime := time.Unix(1700000000, 0)
	if err := os.Chtimes(filePath, initialTime, initialTime); err != nil {
		t.Fatalf("set initial modtime: %v", err)
	}

	first, err := readFileCached(filePath, func() ([]byte, error) { return os.ReadFile(filePath) })
	if err != nil {
		t.Fatalf("read initial cache: %v", err)
	}
	if string(first) != "a" {
		t.Fatalf("unexpected initial content: %q", string(first))
	}

	if err := os.WriteFile(filePath, []byte("b"), 0644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}
	updatedTime := initialTime.Add(2 * time.Second)
	if err := os.Chtimes(filePath, updatedTime, updatedTime); err != nil {
		t.Fatalf("set updated modtime: %v", err)
	}

	second, err := readFileCached(filePath, func() ([]byte, error) { return os.ReadFile(filePath) })
	if err != nil {
		t.Fatalf("read updated cache: %v", err)
	}
	if string(second) != "b" {
		t.Fatalf("expected updated content after external file change, got %q", string(second))
	}
}
