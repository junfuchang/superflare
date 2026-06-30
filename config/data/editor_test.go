package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestGetBookmarksDataAsJSON(t *testing.T) {
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
		t.Fatalf("EnsureRuntimeDataFiles returned error: %v", err)
	}

	categories, bookmarks, err := GetBookmarksForEditor()
	if err != nil {
		t.Fatalf("GetBookmarksForEditor returned error: %v", err)
	}
	if len(categories) == 0 || len(bookmarks) == 0 {
		t.Fatal("GetBookmarksForEditor Failed")
	}
}

func TestGetAndUpdateBookmarksFromEditor(t *testing.T) {

	const categories = `1,链接分类1
2,链接分类2
3,链接分类3
4,链接分类4`
	const bookmarks = `1,示例链接,https://link.example.com,[SuperFlare 应用],evernote,链接描述文本
2,示例链接,https://link.example.com,[SuperFlare 应用],FireHydrant,链接描述文本
3,示例链接,https://link.example.com,[SuperFlare 应用],email,链接描述文本
4,示例链接,https://link.example.com,[SuperFlare 应用],MicrosoftOnenote,链接描述文本
5,示例链接,https://link.example.com,[SuperFlare 应用],Robber,
6,示例链接,https://link.example.com,[SuperFlare 应用],EvPlugType1,
7,示例链接,https://link.example.com,[SuperFlare 应用],FileImage,
8,示例链接,https://link.example.com,[SuperFlare 应用],Image,
9,示例链接,https://link.example.com,链接分类1,checkDecagram,
10,示例链接,https://link.example.com,链接分类1,eraser,
11,示例链接,https://link.example.com,链接分类1,mastodon,
12,示例链接,https://link.example.com,链接分类1,alphaACircleOutline,
13,示例链接,https://link.example.com,链接分类1,flask,
14,示例链接,https://link.example.com,链接分类2,sofaOutline,
15,示例链接,https://link.example.com,链接分类2,BowArrow,
16,示例链接,https://link.example.com,链接分类2,messageCog,
17,示例链接,https://link.example.com,链接分类2,alphaRCircleOutline,
18,示例链接,https://link.example.com,链接分类2,cityVariantOutline,
19,示例链接,https://link.example.com,链接分类3,foodCroissant,
20,示例链接,https://link.example.com,链接分类3,KeyboardOutline,
21,示例链接,https://link.example.com,链接分类3,alphaFCircleOutline,
22,示例链接,https://link.example.com,链接分类3,alphaECircleOutline,
23,示例链接,https://link.example.com,链接分类3,alphaYCircleOutline,
24,示例链接,https://link.example.com,链接分类4,musicCircleOutline,
25,示例链接,https://link.example.com,链接分类4,Incognito,
26,示例链接,https://link.example.com,链接分类4,alphaLCircleOutline,
27,示例链接,https://link.example.com,链接分类4,accountSupervisorCircle,
28,示例链接,https://link.example.com,链接分类4,sproutOutline,`

	if err := UpdateBookmarksFromEditor(categories, bookmarks); err != nil {
		t.Fatalf("UpdateBookmarksFromEditor Failed: %v", err)
	}

	bookmarkCategories, ok := getCategoriesFromCSV(categories)
	if ok != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}

	_, _, ok = getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if ok != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
}

func TestGetBookmarksFromCSVRecognizesFixedCategory(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,Links,,link,Bookmark link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	favorite, normal, err := getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
	if len(favorite) != 1 || favorite[0].Category != "" {
		t.Fatal("fixed category should be restored as favorite bookmark")
	}
	if len(normal) != 1 || normal[0].Category != "1" {
		t.Fatal("normal category should be restored by category id")
	}
}

func TestGetBookmarksFromCSVParsesLocalURL(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,http://192.168.1.10:8080,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,http://192.168.1.11:8080,Links,Lab,link,Bookmark link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	favorite, normal, err := getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err != nil {
		t.Fatal("getBookmarksFromCSV Failed")
	}
	if len(favorite) != 1 || favorite[0].LocalURL != "http://192.168.1.10:8080" {
		t.Fatalf("favorite local URL was not parsed: %#v", favorite)
	}
	if len(normal) != 1 || normal[0].LocalURL != "http://192.168.1.11:8080" || normal[0].Subdir != "Lab" {
		t.Fatalf("normal bookmark local URL/subdir was not parsed: %#v", normal)
	}
}

func TestGetBookmarksFromCSVRejectsUnknownCategoryName(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,Broken,https://broken.example.com,,MissingCategory,,link,Broken link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	_, _, err = getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err == nil {
		t.Fatal("expected unknown category name to fail")
	}
	if !strings.Contains(err.Error(), "MissingCategory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCategoriesFromCSVRejectsDuplicateNames(t *testing.T) {
	_, err := getCategoriesFromCSV("1,Links\n2,Links")
	if err == nil {
		t.Fatal("expected duplicate category name to fail")
	}
	if !strings.Contains(err.Error(), "Links") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCategoriesFromCSVRejectsDuplicateIDs(t *testing.T) {
	_, err := getCategoriesFromCSV("1,Links\n1,Links2")
	if err == nil {
		t.Fatal("expected duplicate category ID to fail")
	}
	if !strings.Contains(err.Error(), "1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCategoriesFromCSVRejectsReservedFixedCategoryName(t *testing.T) {
	_, err := getCategoriesFromCSV("1,[SuperFlare 应用]")
	if err == nil {
		t.Fatal("expected reserved category name to fail")
	}
	if !strings.Contains(err.Error(), "[SuperFlare 应用]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPropsRemoveAndRestore(t *testing.T) {
	var input []model.Bookmark
	input = append(input, model.Bookmark{Private: true, LocalURL: "http://192.168.1.10"})

	removed := restorePrivateProp(removePrivateProp(input))
	for i := 0; i < len(removed); i++ {
		if removed[i].Private != false {
			t.Fatal("Remove and restore private prop Failed")
		}
		if removed[i].LocalURL != input[i].LocalURL {
			t.Fatal("Remove and restore local URL Failed")
		}
	}
}

func TestGetBookmarksForEditorReturnsErrorWhenBookmarksConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("items:\n- name: app\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("Categories: [broken\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	_, _, err = GetBookmarksForEditor()
	if err == nil {
		t.Fatal("expected broken bookmarks config to fail")
	}
	if !strings.Contains(err.Error(), "load normal bookmarks") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetBookmarksForEditorReturnsErrorWhenEditorJSONMarshalFails(t *testing.T) {
	original := jsonStringify
	jsonStringify = func(data interface{}) (string, error) {
		return "", errors.New("forced json stringify failure")
	}
	defer func() { jsonStringify = original }()

	_, _, err := GetBookmarksForEditor()
	if err == nil {
		t.Fatal("expected GetBookmarksForEditor to fail")
	}
	if !strings.Contains(err.Error(), "marshal editor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateBookmarksFromEditorRollsBackWhenSecondFileRenameFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	originalRename := editorDataRenamePath
	defer func() { editorDataRenamePath = originalRename }()

	bookmarksPath := filepath.Join(tmpDir, "bookmarks.yml")
	appsPath := filepath.Join(tmpDir, "apps.yml")
	if err := os.WriteFile(bookmarksPath, []byte("items:\n- name: old-bookmark\n"), 0644); err != nil {
		t.Fatalf("write bookmarks: %v", err)
	}
	if err := os.WriteFile(appsPath, []byte("items:\n- name: old-app\n"), 0644); err != nil {
		t.Fatalf("write apps: %v", err)
	}

	renameCalls := 0
	editorDataRenamePath = func(oldPath string, newPath string) error {
		renameCalls++
		if renameCalls == 3 {
			return errors.New("forced rename failure")
		}
		return originalRename(oldPath, newPath)
	}

	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,,Links,,link,Bookmark link"
	err = UpdateBookmarksFromEditor(categories, bookmarks)
	if err == nil {
		t.Fatal("expected rename failure")
	}

	gotBookmarks, err := os.ReadFile(bookmarksPath)
	if err != nil {
		t.Fatalf("read bookmarks after rollback: %v", err)
	}
	if string(gotBookmarks) != "items:\n- name: old-bookmark\n" {
		t.Fatalf("bookmarks should be rolled back, got %q", string(gotBookmarks))
	}

	gotApps, err := os.ReadFile(appsPath)
	if err != nil {
		t.Fatalf("read apps after rollback: %v", err)
	}
	if string(gotApps) != "items:\n- name: old-app\n" {
		t.Fatalf("apps should remain original, got %q", string(gotApps))
	}
}

func TestUpdateBookmarksFromEditorSerializesWithSettingsConfigWrites(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := EnsureAppConfigExists(); err != nil {
		t.Fatalf("EnsureAppConfigExists: %v", err)
	}
	if err := EnsureRuntimeDataFiles(); err != nil {
		t.Fatalf("EnsureRuntimeDataFiles: %v", err)
	}

	const categories = "1,Links"
	const bookmarks = "1,Bookmark,https://bookmark.example.com,,Links,,link,Bookmark link"

	startSettingsUpdate := make(chan struct{}, 1)
	releaseEditorSave := make(chan struct{})
	originalRename := editorDataRenamePath
	defer func() { editorDataRenamePath = originalRename }()
	var once sync.Once
	editorDataRenamePath = func(oldPath string, newPath string) error {
		cleanOld := filepath.Base(filepath.Clean(oldPath))
		if cleanOld == "apps.yml" || cleanOld == "bookmarks.yml" {
			once.Do(func() {
				startSettingsUpdate <- struct{}{}
				<-releaseEditorSave
			})
		}
		return originalRename(oldPath, newPath)
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- UpdateBookmarksFromEditor(categories, bookmarks)
	}()

	<-startSettingsUpdate
	go func() {
		errCh <- UpdateAppearance(model.Application{Title: "Concurrent Title", Locale: "zh", IconMode: "FILLING"})
	}()

	close(releaseEditorSave)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent update failed: %v", err)
		}
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.Title != "Concurrent Title" {
		t.Fatalf("expected settings update to persist, got title %q", options.Title)
	}

	normal, err := LoadNormalBookmarks()
	if err != nil {
		t.Fatalf("LoadNormalBookmarks: %v", err)
	}
	if len(normal.Items) != 1 || normal.Items[0].Name != "Bookmark" {
		t.Fatalf("expected editor bookmark update to persist, got %#v", normal.Items)
	}
}

func TestUpdateBookmarksFromEditorFailsWhenTargetPathIsDirectory(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	bookmarksPath := filepath.Join(tmpDir, "bookmarks.yml")
	appsPath := filepath.Join(tmpDir, "apps.yml")
	if err := os.Mkdir(bookmarksPath, 0755); err != nil {
		t.Fatalf("mkdir bookmarks target: %v", err)
	}
	if err := os.WriteFile(appsPath, []byte("items:\n- name: old-app\n"), 0644); err != nil {
		t.Fatalf("write apps: %v", err)
	}

	const categories = "1,Links"
	const bookmarks = "1,App,https://app.example.com,,[SuperFlare \u5e94\u7528],,home,App link\n2,Bookmark,https://bookmark.example.com,,Links,,link,Bookmark link"
	err = UpdateBookmarksFromEditor(categories, bookmarks)
	if err == nil {
		t.Fatal("expected directory target failure")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("unexpected error: %v", err)
	}

	info, statErr := os.Stat(bookmarksPath)
	if statErr != nil {
		t.Fatalf("stat bookmarks target: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("bookmarks target directory should remain a directory")
	}

	gotApps, err := os.ReadFile(appsPath)
	if err != nil {
		t.Fatalf("read apps after failure: %v", err)
	}
	if string(gotApps) != "items:\n- name: old-app\n" {
		t.Fatalf("apps should remain original, got %q", string(gotApps))
	}
}

func TestGetCategoriesFromCSVRejectsHalfFilledRow(t *testing.T) {
	_, err := getCategoriesFromCSV("1,Links\n2,")
	if err == nil {
		t.Fatal("expected half-filled category row to fail")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetBookmarksFromCSVRejectsHalfFilledRow(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,Bookmark A,https://bookmark.example.com,,Links,,link,Bookmark link\n2,,https://broken.example.com,,Links,,link,Broken link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	_, _, err = getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err == nil {
		t.Fatal("expected half-filled bookmark row to fail")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetBookmarksFromCSVStillAllowsBlankSpacerRows(t *testing.T) {
	const categories = "1,Links"
	const bookmarks = "1,Bookmark A,https://bookmark.example.com,,Links,,link,Bookmark link\n,,,,,,\n2,Bookmark B,https://bookmark-b.example.com,,Links,,link,Bookmark link"

	bookmarkCategories, err := getCategoriesFromCSV(categories)
	if err != nil {
		t.Fatal("getCategoriesFromCSV Failed")
	}
	favorite, normal, err := getBookmarksFromCSV(bookmarks, bookmarkCategories)
	if err != nil {
		t.Fatalf("getBookmarksFromCSV Failed: %v", err)
	}
	if len(favorite) != 0 {
		t.Fatalf("expected no favorites, got %#v", favorite)
	}
	if len(normal) != 2 {
		t.Fatalf("expected 2 normal bookmarks, got %#v", normal)
	}
}
