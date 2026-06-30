package data

import (
	"encoding/csv"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/junfuchang/superflare/config/model"
)

const editorFixedCategory = "[SuperFlare \u5e94\u7528]"

var editorDataRenamePath = os.Rename

// TODO Removed after private link feature support
type _BOOKMARK_REMOVE_PRIVATE struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"link"`
	LocalURL string `yaml:"local_link,omitempty"`
	Icon     string `yaml:"icon,omitempty"`
	Desc     string `yaml:"desc,omitempty"`
	Category string `yaml:"category,omitempty"`
	Subdir   string `yaml:"subdir,omitempty"`
}

func removePrivateProp(input []model.Bookmark) (result []_BOOKMARK_REMOVE_PRIVATE) {
	for _, src := range input {
		var dest _BOOKMARK_REMOVE_PRIVATE
		dest.Name = src.Name
		dest.URL = src.URL
		dest.LocalURL = src.LocalURL
		dest.Icon = src.Icon
		dest.Desc = src.Desc
		dest.Category = src.Category
		dest.Subdir = src.Subdir
		result = append(result, dest)
	}
	return result
}

func restorePrivateProp(input []_BOOKMARK_REMOVE_PRIVATE) (result []model.Bookmark) {
	for _, src := range input {
		var dest model.Bookmark
		dest.Name = src.Name
		dest.URL = src.URL
		dest.LocalURL = src.LocalURL
		dest.Icon = src.Icon
		dest.Desc = src.Desc
		dest.Category = src.Category
		dest.Subdir = src.Subdir
		dest.Private = false
		result = append(result, dest)
	}
	return result
}

func GetBookmarksForEditor() (categories string, bookmarks string, err error) {
	favoriteBookmarks, err := LoadFavoriteBookmarks()
	if err != nil {
		return "", "", fmt.Errorf("load favorite bookmarks for editor failed: %w", err)
	}
	normalBookmarks, err := LoadNormalBookmarks()
	if err != nil {
		return "", "", fmt.Errorf("load normal bookmarks for editor failed: %w", err)
	}

	mixedBookmarks := make([]model.Bookmark, 0, len(favoriteBookmarks.Items)+len(normalBookmarks.Items))
	for _, item := range favoriteBookmarks.Items {
		item.Category = "_FLARE_FIXED_CATEGORY"
		mixedBookmarks = append(mixedBookmarks, item)
	}
	mixedBookmarks = append(mixedBookmarks, normalBookmarks.Items...)

	categories, err = jsonStringify(normalBookmarks.Categories)
	if err != nil {
		return "", "", fmt.Errorf("marshal editor categories failed: %w", err)
	}
	bookmarks, err = jsonStringify(removePrivateProp(mixedBookmarks))
	if err != nil {
		return "", "", fmt.Errorf("marshal editor bookmarks failed: %w", err)
	}
	return categories, bookmarks, nil
}

func getCategoriesFromCSV(input string) (result []model.Category, err error) {
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1

	validItems := make([]model.Category, 0)
	seenIDs := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	row := 0

	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return result, readErr
		}
		row++
		record = normalizeCSVRecord(record)
		if isBlankCSVRecord(record) {
			continue
		}
		if len(record) > 2 {
			return result, fmt.Errorf("category row %d has too many fields", row)
		}

		item := model.Category{
			ID:   csvRecordValue(record, 0),
			Name: csvRecordValue(record, 1),
		}
		if item.ID == "" || item.Name == "" {
			return result, fmt.Errorf("category row %d is incomplete: both ID and Name are required", row)
		}
		if item.Name == editorFixedCategory {
			return result, fmt.Errorf("\u5206\u7c7b\u540d\u79f0 %q \u4e3a\u4fdd\u7559\u540d\u79f0\uff0c\u8bf7\u4f7f\u7528\u5176\u4ed6\u540d\u79f0", item.Name)
		}
		if _, exists := seenIDs[item.ID]; exists {
			return result, fmt.Errorf("\u5206\u7c7b ID %q \u91cd\u590d\uff0c\u8bf7\u4fdd\u6301\u552f\u4e00", item.ID)
		}
		if _, exists := seenNames[item.Name]; exists {
			return result, fmt.Errorf("\u5206\u7c7b\u540d\u79f0 %q \u91cd\u590d\uff0c\u8bf7\u4fdd\u6301\u552f\u4e00", item.Name)
		}

		seenIDs[item.ID] = struct{}{}
		seenNames[item.Name] = struct{}{}
		validItems = append(validItems, item)
	}

	return validItems, nil
}
func getBookmarksFromCSV(input string, categories []model.Category) (favoriteBookmarks []model.Bookmark, normalBookmarks []model.Bookmark, err error) {
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1
	row := 0

	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return favoriteBookmarks, normalBookmarks, readErr
		}
		row++
		record = normalizeCSVRecord(record)
		if isBlankCSVRecord(record) {
			continue
		}

		bookmark, parseErr := parseBookmarkCSVRecord(record, row)
		if parseErr != nil {
			return favoriteBookmarks, normalBookmarks, parseErr
		}
		if bookmark.Name == "" || bookmark.URL == "" {
			return favoriteBookmarks, normalBookmarks, fmt.Errorf("bookmark row %d is incomplete: both Name and URL are required", row)
		}

		if bookmark.Category == editorFixedCategory || bookmark.Category == "" {
			bookmark.Category = ""
			favoriteBookmarks = append(favoriteBookmarks, bookmark)
			continue
		}

		foundCategory := false
		for _, category := range categories {
			if category.Name == bookmark.Category {
				bookmark.Category = category.ID
				foundCategory = true
				break
			}
		}
		if !foundCategory {
			return favoriteBookmarks, normalBookmarks, fmt.Errorf("\u4e66\u7b7e %q \u5f15\u7528\u4e86\u4e0d\u5b58\u5728\u7684\u5206\u7c7b %q", bookmark.Name, bookmark.Category)
		}
		normalBookmarks = append(normalBookmarks, bookmark)
	}

	return favoriteBookmarks, normalBookmarks, nil
}

func normalizeCSVRecord(record []string) []string {
	normalized := make([]string, len(record))
	for i, field := range record {
		normalized[i] = strings.TrimSpace(field)
	}
	return normalized
}

func csvRecordValue(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return record[index]
}

func isBlankCSVRecord(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func parseBookmarkCSVRecord(record []string, row int) (model.Bookmark, error) {
	bookmark := model.Bookmark{}
	switch len(record) {
	case 6:
		bookmark.Name = csvRecordValue(record, 1)
		bookmark.URL = csvRecordValue(record, 2)
		bookmark.Category = csvRecordValue(record, 3)
		bookmark.Icon = csvRecordValue(record, 4)
		bookmark.Desc = csvRecordValue(record, 5)
	case 7:
		bookmark.Name = csvRecordValue(record, 1)
		bookmark.URL = csvRecordValue(record, 2)
		bookmark.Category = csvRecordValue(record, 3)
		bookmark.Subdir = csvRecordValue(record, 4)
		bookmark.Icon = csvRecordValue(record, 5)
		bookmark.Desc = csvRecordValue(record, 6)
	case 8:
		bookmark.Name = csvRecordValue(record, 1)
		bookmark.URL = csvRecordValue(record, 2)
		bookmark.LocalURL = csvRecordValue(record, 3)
		bookmark.Category = csvRecordValue(record, 4)
		bookmark.Subdir = csvRecordValue(record, 5)
		bookmark.Icon = csvRecordValue(record, 6)
		bookmark.Desc = csvRecordValue(record, 7)
	default:
		return bookmark, fmt.Errorf("bookmark row %d has unsupported field count: %d", row, len(record))
	}
	return bookmark, nil
}
func UpdateBookmarksFromEditor(categoriesCSV string, bookmarksCSV string) error {
	return withConfigWriteLock(func() error {
		categories, err := getCategoriesFromCSV(categoriesCSV)
		if err != nil {
			log.Println("editor categories CSV parse error:", err)
			return fmt.Errorf("\u89e3\u6790\u5206\u7c7b\u6570\u636e\u5931\u8d25: %w", err)
		}

		favorite, normal, err := getBookmarksFromCSV(bookmarksCSV, categories)
		if err != nil {
			log.Println("editor bookmarks CSV parse error:", err)
			return fmt.Errorf("\u89e3\u6790\u4e66\u7b7e\u6570\u636e\u5931\u8d25: %w", err)
		}

		var normalBookmarks model.Bookmarks
		normalBookmarks.Items = normal
		normalBookmarks.Categories = categories

		var favoriteBookmarks model.Bookmarks
		favoriteBookmarks.Items = favorite

		if err := saveEditorBookmarkFilesAtomically(map[string]model.Bookmarks{
			"apps":      favoriteBookmarks,
			"bookmarks": normalBookmarks,
		}); err != nil {
			return fmt.Errorf("\u4fdd\u5b58\u7f16\u8f91\u6570\u636e\u5931\u8d25: %w", err)
		}

		return nil
	})
}

type pendingEditorDataFile struct {
	target string
	temp   string
	backup string
}

func saveEditorBookmarkFilesAtomically(files map[string]model.Bookmarks) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	pending := make([]pendingEditorDataFile, 0, len(names))
	for _, name := range names {
		raw, err := yaml.Marshal(files[name])
		if err != nil {
			cleanupPendingEditorDataFiles(pending)
			return err
		}
		item, err := stagePendingEditorDataFile(name, raw)
		if err != nil {
			cleanupPendingEditorDataFiles(pending)
			return err
		}
		pending = append(pending, item)
	}

	for index := range pending {
		item := &pending[index]
		if info, err := osStat(item.target); err == nil {
			if info.IsDir() {
				dirErr := fmt.Errorf("target path %s is a directory, cannot overwrite bookmark config", item.target)
				rollbackErr := rollbackPendingEditorDataFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(dirErr, rollbackErr)
				}
				return dirErr
			}
			backup, err := os.CreateTemp(filepath.Dir(item.target), "."+filepath.Base(item.target)+".backup-*")
			if err != nil {
				rollbackErr := rollbackPendingEditorDataFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
			item.backup = backup.Name()
			_ = backup.Close()
			_ = os.Remove(item.backup)
			if err := editorDataRenamePath(item.target, item.backup); err != nil {
				rollbackErr := rollbackPendingEditorDataFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
		} else if !os.IsNotExist(err) {
			rollbackErr := rollbackPendingEditorDataFiles(pending, index-1)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}

		if err := editorDataRenamePath(item.temp, item.target); err != nil {
			rollbackErr := rollbackPendingEditorDataFiles(pending, index)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}

		invalidateFileCachePath(item.target)
	}

	for _, item := range pending {
		if item.backup != "" {
			_ = os.Remove(item.backup)
		}
	}
	return nil
}

func stagePendingEditorDataFile(name string, raw []byte) (pendingEditorDataFile, error) {
	target, err := configPath(name)
	if err != nil {
		return pendingEditorDataFile{}, err
	}
	dir := filepath.Dir(target)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return pendingEditorDataFile{}, err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := temp.Write(raw); err != nil {
		cleanup()
		return pendingEditorDataFile{}, err
	}
	if err := temp.Chmod(configFileMode); err != nil {
		cleanup()
		return pendingEditorDataFile{}, err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return pendingEditorDataFile{}, err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return pendingEditorDataFile{}, err
	}

	return pendingEditorDataFile{
		target: target,
		temp:   tempPath,
	}, nil
}

func cleanupPendingEditorDataFiles(items []pendingEditorDataFile) {
	for _, item := range items {
		if item.temp != "" {
			_ = os.Remove(item.temp)
		}
	}
}

func rollbackPendingEditorDataFiles(items []pendingEditorDataFile, appliedIndex int) error {
	var rollbackErr error
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if item.temp != "" {
			if err := os.Remove(item.temp); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		if index > appliedIndex {
			if item.backup != "" {
				if err := editorDataRenamePath(item.backup, item.target); err != nil && !os.IsNotExist(err) {
					rollbackErr = errors.Join(rollbackErr, err)
				}
			}
			continue
		}

		if info, err := osStat(item.target); err == nil {
			if info.IsDir() {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("target path %s is a directory, cannot remove generated bookmark config during rollback", item.target))
				continue
			}
			if err := os.Remove(item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
				continue
			}
		} else if !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		if item.backup != "" {
			if err := editorDataRenamePath(item.backup, item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		invalidateFileCachePath(item.target)
	}
	return rollbackErr
}
