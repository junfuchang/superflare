package home

import (
	"html/template"
	"strconv"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
)

func GenerateBookmarkTemplate(filter string, options *model.Application) template.HTML {
	return GenerateBookmarkTemplateWithLocal(filter, options, false)
}

func GenerateBookmarkTemplateWithLocal(filter string, options *model.Application, preferLocal bool) template.HTML {
	if options == nil {
		op, err := data.GetAllSettingsOptions()
		if err != nil {
			op = model.Application{}
		}
		options = &op
	}
	bookmarksData, err := data.LoadNormalBookmarks()
	if err != nil {
		return template.HTML("")
	}
	b, ok := builderPool.Get().(*strings.Builder)
	if !ok {
		b = &strings.Builder{}
	}
	b.Reset()
	defer builderPool.Put(b)

	n := len(bookmarksData.Items)
	parseBookmarks := make([]model.Bookmark, 0, n)
	for _, bookmark := range bookmarksData.Items {
		bookmark.URL = fn.ParseDynamicUrl(bookmark.URL)
		bookmark.LocalURL = fn.ParseDynamicUrl(bookmark.LocalURL)
		parseBookmarks = append(parseBookmarks, bookmark)
	}

	bookmarks := parseBookmarks
	if filter != "" {
		bookmarks = make([]model.Bookmark, 0, n)
	}

	if filter != "" {
		filterLower := strings.ToLower(filter)
		for _, bookmark := range parseBookmarks {
			if strings.Contains(strings.ToLower(bookmark.Name), filterLower) || strings.Contains(strings.ToLower(bookmark.URL), filterLower) || strings.Contains(strings.ToLower(bookmark.LocalURL), filterLower) {
				bookmarks = append(bookmarks, bookmark)
			}
		}
	}

	if len(bookmarksData.Categories) > 0 {
		defaultCategory := bookmarksData.Categories[0]
		b.WriteString(`<div class="bookmark-groups">`)
		for _, category := range bookmarksData.Categories {
			categoryCopy := category
			renderBookmarksWithCategories(b, &bookmarks, &categoryCopy, &defaultCategory, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal)
		}
		b.WriteString(`</div>`)
	} else {
		b.WriteString(`<div class="bookmark-groups">`)
		renderBookmarksWithoutCategories(b, &bookmarks, options.OpenBookmarkNewTab, options.EnableEncryptedLink, options.IconMode, preferLocal)
		b.WriteString(`</div>`)
	}

	return template.HTML(b.String())
}

func renderBookmarksWithoutCategories(b *strings.Builder, bookmarks *[]model.Bookmark, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool) {
	b.WriteString(`<div class="bookmark-group-container pull-left"><ul class="bookmark-list">`)
	for _, bookmark := range *bookmarks {
		renderBookmarkItem(b, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal)
	}
	b.WriteString(`</ul></div>`)
}

func renderBookmarksWithCategories(b *strings.Builder, bookmarks *[]model.Bookmark, category *model.Category, defaultCategory *model.Category, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool) {
	var itemBuf strings.Builder
	subdirs := make(map[string][]model.Bookmark)
	subdirOrder := make([]string, 0)
	for _, bookmark := range *bookmarks {
		matched := false
		if bookmark.Category != "" {
			matched = bookmark.Category == category.ID
		} else {
			matched = category.ID == defaultCategory.ID
		}
		if !matched {
			continue
		}
		if bookmark.Subdir != "" {
			if _, ok := subdirs[bookmark.Subdir]; !ok {
				subdirOrder = append(subdirOrder, bookmark.Subdir)
			}
			subdirs[bookmark.Subdir] = append(subdirs[bookmark.Subdir], bookmark)
			continue
		}
		renderBookmarkItem(&itemBuf, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal)
	}
	if itemBuf.Len() == 0 && len(subdirOrder) == 0 {
		return
	}
	b.WriteString(`<div class="bookmark-group-container pull-left"><h3 class="bookmark-group-title">`)
	b.WriteString(category.Name)
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
				renderBookmarkItem(b, bookmark, OpenBookmarkNewTab, EnableEncryptedLink, IconMode, preferLocal)
			}
			b.WriteString(`</ul></details>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<ul class="bookmark-list">`)
	b.WriteString(itemBuf.String())
	b.WriteString(`</ul></div>`)
}

func renderBookmarkItem(b *strings.Builder, bookmark model.Bookmark, OpenBookmarkNewTab bool, EnableEncryptedLink bool, IconMode string, preferLocal bool) {
	templateURL := renderBookmarkHref(bookmark.URL, bookmark.LocalURL, preferLocal, EnableEncryptedLink)
	templateIcon := renderBookmarkIcon(bookmark.Icon, bookmark.URL, IconMode)
	if OpenBookmarkNewTab {
		b.WriteString(`<li><a target="_blank" rel="noopener" href="`)
	} else {
		b.WriteString(`<li><a rel="noopener" href="`)
	}
	b.WriteString(template.HTMLEscapeString(templateURL))
	b.WriteString(`" class="bookmark">`)
	b.WriteString(templateIcon)
	b.WriteString(`<span>`)
	b.WriteString(template.HTMLEscapeString(bookmark.Name))
	b.WriteString(`</span></a></li>`)
}
