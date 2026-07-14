package home

import (
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
)

type bookmarkModules struct {
	Bookmarks                 template.HTML
	Favorites                 template.HTML
	HasFavorites              bool
	HasDescriptions           bool
	BookmarksHaveDescriptions bool
	FavoritesHaveDescriptions bool
	items                     []model.Bookmark
	favoriteItems             []model.Bookmark
}

func bookmarkVisible(item model.Bookmark, canViewPrivate bool) bool {
	return canViewPrivate || !item.Private
}

func GenerateBookmarkTemplateErr(filter string, options *model.Application) (template.HTML, error) {
	return GenerateBookmarkTemplateWithLocalAndURLErr(filter, options, false, nil)
}

func GenerateBookmarkTemplateWithLocalErr(filter string, options *model.Application, preferLocal bool) (template.HTML, error) {
	return GenerateBookmarkTemplateWithLocalAndURLErr(filter, options, preferLocal, nil)
}

func GenerateBookmarkTemplateWithLocalAndURLErr(filter string, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL) (template.HTML, error) {
	modules, err := generateBookmarkModulesWithLocalAndURLErr(filter, options, preferLocal, requestURL, true)
	return modules.Bookmarks, err
}

func generateBookmarkModulesWithLocalAndURLErr(filter string, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL, canViewPrivate bool) (bookmarkModules, error) {
	if options == nil {
		options = &model.Application{}
	}
	bookmarksData, err := loadNormalBookmarks()
	if err != nil {
		return bookmarkModules{}, err
	}
	b, ok := builderPool.Get().(*strings.Builder)
	if !ok {
		b = &strings.Builder{}
	}
	b.Reset()
	defer builderPool.Put(b)

	filterLower := strings.ToLower(filter)
	bookmarks := make([]model.Bookmark, 0, len(bookmarksData.Items))
	favorites := make([]model.Bookmark, 0, len(bookmarksData.Items))
	bookmarksHaveDescriptions := false
	favoritesHaveDescriptions := false
	for _, bookmark := range bookmarksData.Items {
		if !bookmarkVisible(bookmark, canViewPrivate) {
			continue
		}
		bookmark.URL = fn.ParseDynamicUrlWith(bookmark.URL, requestURL)
		bookmark.LocalURL = fn.ParseDynamicUrlWith(bookmark.LocalURL, requestURL)
		if filter != "" &&
			!strings.Contains(strings.ToLower(bookmark.Name), filterLower) &&
			!strings.Contains(strings.ToLower(bookmark.URL), filterLower) &&
			!strings.Contains(strings.ToLower(bookmark.LocalURL), filterLower) {
			continue
		}
		bookmarks = append(bookmarks, bookmark)
		hasDescription := strings.TrimSpace(bookmark.Desc) != ""
		bookmarksHaveDescriptions = bookmarksHaveDescriptions || hasDescription
		if bookmark.Favorite {
			favorites = append(favorites, bookmark)
			favoritesHaveDescriptions = favoritesHaveDescriptions || hasDescription
		}
	}
	sort.SliceStable(favorites, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(favorites[i].Name))
		right := strings.ToLower(strings.TrimSpace(favorites[j].Name))
		if left != right {
			return left < right
		}
		return favorites[i].Name < favorites[j].Name
	})

	if len(bookmarksData.Categories) > 0 {
		b.WriteString(`<div class="bookmark-groups">`)
		for _, category := range bookmarksData.Categories {
			categoryCopy := category
			renderBookmarksWithCategories(b, &bookmarks, &categoryCopy, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal, requestURL)
		}
		renderUngroupedBookmarks(b, &bookmarks, bookmarksData.Categories, options.Locale, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal, requestURL)
		b.WriteString(`</div>`)
	} else {
		b.WriteString(`<div class="bookmark-groups">`)
		renderBookmarksWithoutCategories(b, &bookmarks, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal, requestURL)
		b.WriteString(`</div>`)
	}

	var favoritesHTML template.HTML
	if len(favorites) > 0 {
		var favoritesBuilder strings.Builder
		favoritesBuilder.WriteString(`<div class="bookmark-groups">`)
		renderBookmarksWithoutCategories(&favoritesBuilder, &favorites, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal, requestURL)
		favoritesBuilder.WriteString(`</div>`)
		favoritesHTML = template.HTML(favoritesBuilder.String())
	}
	hasDescriptions := (options.ShowBookmarks && bookmarksHaveDescriptions) ||
		(options.ShowFavorites && len(favorites) > 0 && favoritesHaveDescriptions)

	return bookmarkModules{
		Bookmarks:                 template.HTML(b.String()),
		Favorites:                 favoritesHTML,
		HasFavorites:              len(favorites) > 0,
		HasDescriptions:           hasDescriptions,
		BookmarksHaveDescriptions: bookmarksHaveDescriptions,
		FavoritesHaveDescriptions: favoritesHaveDescriptions,
		items:                     bookmarks,
		favoriteItems:             favorites,
	}, nil
}

func renderBookmarksWithoutCategories(b *strings.Builder, bookmarks *[]model.Bookmark, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool, requestURL *fn.DynamicURL) {
	b.WriteString(`<div class="bookmark-group-container pull-left"><ul class="bookmark-list">`)
	for _, bookmark := range *bookmarks {
		renderBookmarkItem(b, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal, requestURL)
	}
	b.WriteString(`</ul></div>`)
}

func renderBookmarksWithCategories(b *strings.Builder, bookmarks *[]model.Bookmark, category *model.Category, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool, requestURL *fn.DynamicURL) {
	var itemBuf strings.Builder
	subdirs := make(map[string][]model.Bookmark)
	subdirOrder := make([]string, 0)
	for _, bookmark := range *bookmarks {
		if strings.TrimSpace(bookmark.Category) != category.ID {
			continue
		}
		if bookmark.Subdir != "" {
			if _, ok := subdirs[bookmark.Subdir]; !ok {
				subdirOrder = append(subdirOrder, bookmark.Subdir)
			}
			subdirs[bookmark.Subdir] = append(subdirs[bookmark.Subdir], bookmark)
			continue
		}
		renderBookmarkItem(&itemBuf, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal, requestURL)
	}
	if itemBuf.Len() == 0 && len(subdirOrder) == 0 {
		return
	}
	b.WriteString(`<div class="bookmark-group-container pull-left"><h3 class="bookmark-group-title">`)
	b.WriteString(template.HTMLEscapeString(category.Name))
	b.WriteString(`</h3>`)
	if len(subdirOrder) > 0 {
		b.WriteString(`<div class="bookmark-subdirs">`)
		for idx, subdir := range subdirOrder {
			id := "subdir-" + category.ID + "-" + strconv.Itoa(idx)
			b.WriteString(`<details class="bookmark-subdir"><summary title="`)
			b.WriteString(template.HTMLEscapeString(subdir))
			b.WriteString(`">`)
			b.WriteString(template.HTMLEscapeString(subdir))
			b.WriteString(`</summary><ul class="bookmark-list" id="`)
			b.WriteString(template.HTMLEscapeString(id))
			b.WriteString(`">`)
			for _, bookmark := range subdirs[subdir] {
				renderBookmarkItem(b, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal, requestURL)
			}
			b.WriteString(`</ul></details>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<ul class="bookmark-list">`)
	b.WriteString(itemBuf.String())
	b.WriteString(`</ul></div>`)
}

func renderUngroupedBookmarks(b *strings.Builder, bookmarks *[]model.Bookmark, categories []model.Category, locale string, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool, requestURL *fn.DynamicURL) {
	categorized := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		categorized[strings.TrimSpace(category.ID)] = struct{}{}
	}
	var itemBuf strings.Builder
	for _, bookmark := range *bookmarks {
		if strings.TrimSpace(bookmark.Category) == "" {
			renderBookmarkItem(&itemBuf, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal, requestURL)
			continue
		}
		if _, ok := categorized[strings.TrimSpace(bookmark.Category)]; !ok {
			renderBookmarkItem(&itemBuf, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal, requestURL)
		}
	}
	if itemBuf.Len() == 0 {
		return
	}
	b.WriteString(`<div class="bookmark-group-container pull-left"><h3 class="bookmark-group-title">`)
	b.WriteString(template.HTMLEscapeString(localeUngroupedTitle(locale)))
	b.WriteString(`</h3><ul class="bookmark-list">`)
	b.WriteString(itemBuf.String())
	b.WriteString(`</ul></div>`)
}

func localeUngroupedTitle(locale string) string {
	if strings.EqualFold(strings.TrimSpace(locale), "en") {
		return "Ungrouped"
	}
	return "未分类"
}

func renderBookmarkItem(b *strings.Builder, bookmark model.Bookmark, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool, requestURL *fn.DynamicURL) {
	templateURL := renderBookmarkHref(bookmark.URL, bookmark.LocalURL, preferLocal, EnableEncryptedLink, requestURL)
	templateIcon := renderBookmarkIcon(bookmark.Icon, bookmark.URL, IconMode)
	if OpenBookmarkNewTab {
		b.WriteString(`<li><a target="_blank" rel="noopener" href="`)
	} else {
		b.WriteString(`<li><a rel="noopener" href="`)
	}
	b.WriteString(template.HTMLEscapeString(templateURL))
	b.WriteString(`" class="bookmark"`)
	if description := strings.TrimSpace(bookmark.Desc); description != "" {
		b.WriteString(` data-bookmark-description="`)
		b.WriteString(template.HTMLEscapeString(description))
		b.WriteString(`"`)
	}
	b.WriteString(`>`)
	b.WriteString(templateIcon)
	b.WriteString(`<span class="bookmark-label">`)
	b.WriteString(template.HTMLEscapeString(bookmark.Name))
	b.WriteString(`</span></a></li>`)
}
