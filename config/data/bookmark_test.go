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

func TestFavoriteBookmarks(t *testing.T) {

	filePath := getConfigPath("apps")
	os.Remove(filePath)
	if err := EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	data, err := LoadFavoriteBookmarks()
	if err != nil {
		t.Fatalf("LoadFavoriteBookmarks: %v", err)
	}
	if len(data.Categories) != 0 || len(data.Items) == 0 {
		t.Fatal("Load Favorite Bookmarks Failed")
	}
	if err := SaveFavoriteBookmarks(data); err != nil {
		t.Fatalf("Save Favorite Bookmarks Failed: %v", err)
	}

	os.Remove(filePath)

}

func TestSaveFavoriteBookmarksUsesConfigWriteLock(t *testing.T) {
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
		done <- SaveFavoriteBookmarks(model.Bookmarks{Items: []model.Bookmark{{Name: "App", URL: "https://example.com"}}})
	}()

	select {
	case err := <-done:
		configWriteMu.Unlock()
		t.Fatalf("SaveFavoriteBookmarks bypassed config write lock, err=%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	configWriteMu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SaveFavoriteBookmarks after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SaveFavoriteBookmarks did not finish after config write lock was released")
	}
}

func TestNormalBookmarks(t *testing.T) {

	filePath := getConfigPath("bookmarks")
	os.Remove(filePath)
	if err := EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	data, err := LoadNormalBookmarks()
	if err != nil {
		t.Fatalf("LoadNormalBookmarks: %v", err)
	}
	if len(data.Categories) == 0 || len(data.Items) == 0 {
		t.Fatal("Load Normal Bookmarks Failed")
	}
	if err := SaveNormalBookmarks(data); err != nil {
		t.Fatalf("Save Normal Bookmarks Failed: %v", err)
	}

	os.Remove(filePath)

}

func TestLoadFavoriteBookmarksReturnsErrorWhenConfigMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	_, err = LoadFavoriteBookmarks()
	if err == nil {
		t.Fatal("expected LoadFavoriteBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "bookmarks config apps is missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksReturnsErrorWhenConfigMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	_, err = LoadNormalBookmarks()
	if err == nil {
		t.Fatal("expected LoadNormalBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "bookmarks config bookmarks is missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveFavoriteBookmarksReturnsErrorWhenTargetIsDirectory(t *testing.T) {
	filePath := getConfigPath("apps")
	_ = os.RemoveAll(filePath)
	if err := os.Mkdir(filePath, 0755); err != nil {
		t.Fatalf("mkdir apps target: %v", err)
	}

	err := SaveFavoriteBookmarks(model.Bookmarks{})
	if err == nil {
		t.Fatal("expected SaveFavoriteBookmarks to fail when target path is a directory")
	}

	_ = os.RemoveAll(filePath)
}

func TestSaveNormalBookmarksReturnsErrorWhenTargetIsDirectory(t *testing.T) {
	filePath := getConfigPath("bookmarks")
	_ = os.RemoveAll(filePath)
	if err := os.Mkdir(filePath, 0755); err != nil {
		t.Fatalf("mkdir bookmarks target: %v", err)
	}

	err := SaveNormalBookmarks(model.Bookmarks{})
	if err == nil {
		t.Fatal("expected SaveNormalBookmarks to fail when target path is a directory")
	}

	_ = os.RemoveAll(filePath)
}

func TestLoadFavoriteBookmarksReturnsErrorWhenStatFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	targetPath := filepath.Join(tmpDir, "apps.yml")
	originalStat := osStat
	defer func() { osStat = originalStat }()
	osStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(targetPath) {
			return nil, errors.New("forced stat failure")
		}
		return originalStat(path)
	}

	_, err = LoadFavoriteBookmarks()
	if err == nil {
		t.Fatal("expected LoadFavoriteBookmarks to fail")
	}
	if got := err.Error(); !strings.Contains(got, "stat bookmarks config apps failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksReturnsErrorWhenCategoryIDsDuplicate(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("categories:\n- id: default\n  title: 分类1\n- id: default\n  title: 分类2\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), raw, 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	_, err = LoadNormalBookmarks()
	if err == nil {
		t.Fatal("expected LoadNormalBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "duplicate category id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksReturnsErrorWhenBookmarkReferencesUnknownCategory(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("categories:\n- id: default\n  title: 分类1\nlinks:\n- name: Bookmark A\n  category: missing\n  link: https://bookmark.example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), raw, 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	_, err = LoadNormalBookmarks()
	if err == nil {
		t.Fatal("expected LoadNormalBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "references unknown category id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksFromRawReturnsErrorWhenBookmarkCategoryInvalid(t *testing.T) {
	_, err := LoadNormalBookmarksFromRaw([]byte("categories:\n- id: default\n  title: 分类1\nlinks:\n- name: Bookmark A\n  category: missing\n  link: https://bookmark.example.com\n"))
	if err == nil {
		t.Fatal("expected LoadNormalBookmarksFromRaw to fail")
	}
	if !strings.Contains(err.Error(), "references unknown category id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFavoriteBookmarksReturnsErrorWhenBookmarkNameMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("links:\n- name: \"\"\n  link: https://app.example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), raw, 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	_, err = LoadFavoriteBookmarks()
	if err == nil {
		t.Fatal("expected LoadFavoriteBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "missing bookmark name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFavoriteBookmarksReturnsErrorWhenBookmarkLinkMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("links:\n- name: App A\n  link: \"\"\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), raw, 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	_, err = LoadFavoriteBookmarks()
	if err == nil {
		t.Fatal("expected LoadFavoriteBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "missing bookmark link") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksReturnsErrorWhenBookmarkNameMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("categories:\n- id: default\n  title: 分类1\nlinks:\n- name: \"\"\n  category: default\n  link: https://bookmark.example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), raw, 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	_, err = LoadNormalBookmarks()
	if err == nil {
		t.Fatal("expected LoadNormalBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "missing bookmark name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalBookmarksReturnsErrorWhenBookmarkLinkMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	raw := []byte("categories:\n- id: default\n  title: 分类1\nlinks:\n- name: Bookmark A\n  category: default\n  link: \"\"\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), raw, 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	_, err = LoadNormalBookmarks()
	if err == nil {
		t.Fatal("expected LoadNormalBookmarks to fail")
	}
	if !strings.Contains(err.Error(), "missing bookmark link") {
		t.Fatalf("unexpected error: %v", err)
	}
}
