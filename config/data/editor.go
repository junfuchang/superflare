package data

import (
	"encoding/csv"
	"io"
	"log"
	"strings"

	"github.com/jszwec/csvutil"

	"github.com/junfuchang/superflare/config/model"
)

const editorFixedCategory = "[SuperFlare \u5e94\u7528]"

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

func GetBookmarksForEditor() (categories string, bookmarks string) {
	favoriteBookmarks, errFav := LoadFavoriteBookmarks()
	if errFav != nil {
		return "", ""
	}
	normalBookmarks, errNorm := LoadNormalBookmarks()
	if errNorm != nil {
		return "", ""
	}

	var mixedBookmarks []model.Bookmark

	var appendFixedCategoryForFavorite []model.Bookmark
	for _, item := range favoriteBookmarks.Items {
		// TODO Defined as a constant, provided for front-end use
		item.Category = "_FLARE_FIXED_CATEGORY"
		appendFixedCategoryForFavorite = append(appendFixedCategoryForFavorite, item)
	}

	mixedBookmarks = append(mixedBookmarks, appendFixedCategoryForFavorite...)
	mixedBookmarks = append(mixedBookmarks, normalBookmarks.Items...)

	categories = jsonStringify(normalBookmarks.Categories)
	bookmarks = jsonStringify(removePrivateProp(mixedBookmarks))

	return categories, bookmarks
}

func getCategoriesFromCSV(input string) (result []model.Category, err error) {
	var fixHead = []byte("ID,Name\n" + input)
	var decode []model.Category
	if err := csvutil.Unmarshal(fixHead, &decode); err != nil {
		return result, err
	}

	var validItem []model.Category

	for _, item := range decode {
		if item.Name != "" && item.ID != "" {
			validItem = append(validItem, item)
		}
	}
	return validItem, nil
}

func getBookmarksFromCSV(input string, categories []model.Category) (favoriteBookmarks []model.Bookmark, normalBookmarks []model.Bookmark, err error) {
	header := getBookmarksCSVHeader(input)
	var fixHead = []byte(header + input)
	var decode []_BOOKMARK_REMOVE_PRIVATE

	if err := csvutil.Unmarshal(fixHead, &decode); err != nil {
		return favoriteBookmarks, normalBookmarks, err
	}

	bookmarks := restorePrivateProp(decode)
	for _, bookmark := range bookmarks {
		if bookmark.Name != "" && bookmark.URL != "" {
			// TODO Defined as a constant, provided for front-end use
			if bookmark.Category == editorFixedCategory || bookmark.Category == "" {
				bookmark.Category = ""
				favoriteBookmarks = append(favoriteBookmarks, bookmark)
			} else {
				for _, category := range categories {
					if category.Name == bookmark.Category {
						bookmark.Category = category.ID
						break
					}
				}
				normalBookmarks = append(normalBookmarks, bookmark)
			}
		}
	}

	return favoriteBookmarks, normalBookmarks, nil
}

func getBookmarksCSVHeader(input string) string {
	const current = "ID,Name,URL,LocalURL,Category,Subdir,Icon,Desc\n"
	const withoutLocalURL = "ID,Name,URL,Category,Subdir,Icon,Desc\n"
	const legacy = "ID,Name,URL,Category,Icon,Desc\n"
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return current
		}
		if err != nil {
			return current
		}
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}
		switch len(record) {
		case 6:
			return legacy
		case 7:
			return withoutLocalURL
		default:
			return current
		}
	}
}

func UpdateBookmarksFromEditor(categoriesCSV string, bookmakrsCSV string) bool {

	categories, err := getCategoriesFromCSV(categoriesCSV)
	if err != nil {
		log.Println("editor categories CSV parse error:", err)
		return false
	}

	favorite, normal, err := getBookmarksFromCSV(bookmakrsCSV, categories)
	if err != nil {
		log.Println("editor bookmarks CSV parse error:", err)
		return false
	}

	var normalBookmarks model.Bookmarks
	normalBookmarks.Items = normal
	normalBookmarks.Categories = categories
	SaveNormalBookmarks(normalBookmarks)

	var favoriteBookmarks model.Bookmarks
	favoriteBookmarks.Items = favorite
	SaveFavoriteBookmarks(favoriteBookmarks)

	return true
}
