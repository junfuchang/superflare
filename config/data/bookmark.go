package data

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

func initBookmarks(filePath string, isFavorite bool) (result model.Bookmarks, err error) {
	const exampleName = "示例链接"
	const exampleLink = "https://link.example.com"
	const exampleDesc = "链接描述文本"

	exampleIcons := [28]string{
		"evernote", "FireHydrant", "email", "MicrosoftOnenote",
		"Robber", "EvPlugType1", "FileImage", "Image",
		"checkDecagram", "sofaOutline", "foodCroissant", "musicCircleOutline", "eraser",
		"BowArrow", "KeyboardOutline", "Incognito", "mastodon", "messageCog",
		"alphaFCircleOutline", "alphaLCircleOutline", "alphaACircleOutline", "alphaRCircleOutline", "alphaECircleOutline",
		"accountSupervisorCircle", "flask", "cityVariantOutline", "alphaYCircleOutline", "sproutOutline",
	}

	if isFavorite {
		for i := 0; i < 4; i++ {
			result.Items = append(result.Items, model.Bookmark{
				Name: exampleName,
				URL:  exampleLink,
				Icon: exampleIcons[i],
				Desc: exampleDesc,
			})
		}
		for i := 0; i < 4; i++ {
			result.Items = append(result.Items, model.Bookmark{
				Name: exampleName,
				URL:  exampleLink,
				Icon: exampleIcons[i+4],
			})
		}
	} else {
		const prefix = "cate-id-"
		for i := 0; i < 4; i++ {
			result.Categories = append(result.Categories, model.Category{
				ID:   prefix + strconv.Itoa(i),
				Name: "链接分类" + strconv.Itoa(i+1),
			})
		}
		for i := 0; i < 20; i++ {
			result.Items = append(result.Items, model.Bookmark{
				Name:     exampleName,
				URL:      exampleLink,
				Icon:     exampleIcons[8+i],
				Category: prefix + strconv.Itoa(i%4),
			})
		}
	}

	if err := validateBookmarks(result, isFavorite); err != nil {
		return result, fmt.Errorf("validate default bookmarks failed: %w", err)
	}

	out, err := yaml.Marshal(result)
	if err != nil {
		log.Println("marshal default bookmarks failed")
		return result, fmt.Errorf("marshal default bookmarks failed: %w", err)
	}

	if err := saveFile(filePath, out); err != nil {
		log.Println("save default bookmarks failed")
		return result, fmt.Errorf("save default bookmarks failed: %w", err)
	}

	return result, nil
}

func saveBookmarksToYamlFile(name string, data model.Bookmarks, isFavorite bool) error {
	if err := validateBookmarks(data, isFavorite); err != nil {
		return err
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		log.Println("marshal bookmarks failed", name)
		return fmt.Errorf("marshal bookmarks %s failed: %w", name, err)
	}

	filePath, err := configPath(name)
	if err != nil {
		return err
	}
	if err := saveFile(filePath, out); err != nil {
		log.Println("save bookmarks failed", name)
		return fmt.Errorf("save bookmarks %s failed: %w", name, err)
	}
	invalidateFileCachePath(filePath)
	return nil
}

func loadBookmarksFromYamlFile(name string, isFavorite bool) (model.Bookmarks, error) {
	var result model.Bookmarks
	filePath, err := configPath(name)
	if err != nil {
		return result, err
	}

	exists, err := pathExists(filePath)
	if err != nil {
		return result, fmt.Errorf("stat bookmarks config %s failed: %w", name, err)
	}
	if !exists {
		return result, fmt.Errorf("bookmarks config %s is missing", name)
	}

	configFile, err := readFileCached(filePath, func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, fmt.Errorf("read bookmarks config %s failed: %w", name, err)
	}
	if err := yaml.Unmarshal(configFile, &result); err != nil {
		return result, fmt.Errorf("parse bookmarks config %s failed: %w", name, err)
	}
	if err := validateBookmarks(result, isFavorite); err != nil {
		return result, err
	}
	return result, nil
}

func loadBookmarksFromRaw(raw []byte, isFavorite bool) (model.Bookmarks, error) {
	var result model.Bookmarks
	if err := yaml.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("parse bookmarks raw failed: %w", err)
	}
	if err := validateBookmarks(result, isFavorite); err != nil {
		return result, err
	}
	return result, nil
}

func validateBookmarks(data model.Bookmarks, isFavorite bool) error {
	for index, bookmark := range data.Items {
		if strings.TrimSpace(bookmark.Name) == "" {
			return fmt.Errorf("invalid bookmark at row %d: missing bookmark name", index+1)
		}
		if strings.TrimSpace(bookmark.URL) == "" {
			return fmt.Errorf("invalid bookmark at row %d: missing bookmark link", index+1)
		}
	}

	if isFavorite {
		return nil
	}

	categoryIDs := make(map[string]struct{}, len(data.Categories))
	categoryNames := make(map[string]struct{}, len(data.Categories))
	for index, category := range data.Categories {
		id := strings.TrimSpace(category.ID)
		name := strings.TrimSpace(category.Name)
		if id == "" {
			return fmt.Errorf("invalid bookmark category at row %d: missing category id", index+1)
		}
		if name == "" {
			return fmt.Errorf("invalid bookmark category at row %d: missing category title", index+1)
		}
		if _, exists := categoryIDs[id]; exists {
			return fmt.Errorf("invalid bookmark categories: duplicate category id %q", category.ID)
		}
		if _, exists := categoryNames[name]; exists {
			return fmt.Errorf("invalid bookmark categories: duplicate category title %q", category.Name)
		}
		categoryIDs[id] = struct{}{}
		categoryNames[name] = struct{}{}
	}

	if len(categoryIDs) == 0 {
		for index, bookmark := range data.Items {
			if strings.TrimSpace(bookmark.Category) == "" {
				continue
			}
			return fmt.Errorf("invalid bookmark at row %d: references missing category id %q", index+1, bookmark.Category)
		}
		return nil
	}

	for index, bookmark := range data.Items {
		categoryID := strings.TrimSpace(bookmark.Category)
		if categoryID == "" {
			continue
		}
		if _, exists := categoryIDs[categoryID]; !exists {
			return fmt.Errorf("invalid bookmark at row %d: references unknown category id %q", index+1, bookmark.Category)
		}
	}

	return nil
}

func SaveFavoriteBookmarks(data model.Bookmarks) error {
	return saveBookmarksToYamlFile("apps", data, true)
}

func SaveNormalBookmarks(data model.Bookmarks) error {
	return saveBookmarksToYamlFile("bookmarks", data, false)
}

func LoadFavoriteBookmarks() (model.Bookmarks, error) {
	return loadBookmarksFromYamlFile("apps", true)
}

func LoadNormalBookmarks() (model.Bookmarks, error) {
	return loadBookmarksFromYamlFile("bookmarks", false)
}

func LoadFavoriteBookmarksFromRaw(raw []byte) (model.Bookmarks, error) {
	return loadBookmarksFromRaw(raw, true)
}

func LoadNormalBookmarksFromRaw(raw []byte) (model.Bookmarks, error) {
	return loadBookmarksFromRaw(raw, false)
}
