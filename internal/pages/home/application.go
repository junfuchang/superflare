package home

import (
	"html/template"
	"strings"
	"sync"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
)

var builderPool = sync.Pool{
	New: func() any { return &strings.Builder{} },
}

func GenerateApplicationsTemplateErr(filter string, options *model.Application) (template.HTML, error) {
	return GenerateApplicationsTemplateWithLocalAndURLErr(filter, options, false, nil)
}

func GenerateApplicationsTemplateWithLocalErr(filter string, options *model.Application, preferLocal bool) (template.HTML, error) {
	return GenerateApplicationsTemplateWithLocalAndURLErr(filter, options, preferLocal, nil)
}

func GenerateApplicationsTemplateWithLocalAndURLErr(filter string, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL) (template.HTML, error) {
	if options == nil {
		options = &model.Application{}
	}
	appsData, err := data.LoadFavoriteBookmarks()
	if err != nil {
		return template.HTML(""), err
	}
	b, ok := builderPool.Get().(*strings.Builder)
	if !ok {
		b = &strings.Builder{}
	}
	b.Reset()
	defer builderPool.Put(b)

	n := len(appsData.Items)
	parseApps := make([]model.Bookmark, 0, n)
	for _, app := range appsData.Items {
		app.URL = fn.ParseDynamicUrlWith(app.URL, requestURL)
		app.LocalURL = fn.ParseDynamicUrlWith(app.LocalURL, requestURL)
		parseApps = append(parseApps, app)
	}

	var apps []model.Bookmark
	if filter != "" {
		apps = make([]model.Bookmark, 0, n)
	}

	if filter != "" {
		filterLower := strings.ToLower(filter)
		for _, bookmark := range parseApps {
			if strings.Contains(strings.ToLower(bookmark.Name), filterLower) || strings.Contains(strings.ToLower(bookmark.URL), filterLower) || strings.Contains(strings.ToLower(bookmark.LocalURL), filterLower) || strings.Contains(strings.ToLower(bookmark.Desc), filterLower) {
				apps = append(apps, bookmark)
			}
		}
	} else {
		apps = parseApps
	}

	for _, app := range apps {
		desc := app.Desc
		if desc == "" {
			desc = app.URL
		}
		templateURL := renderBookmarkHref(app.URL, app.LocalURL, preferLocal, options.EnableEncryptedLink)
		templateIcon := renderBookmarkIcon(app.Icon, app.URL, options.IconMode)
		hasIcon := strings.TrimSpace(templateIcon) != ""
		escapedID := template.HTMLEscapeString(app.Icon)
		escapedURL := template.HTMLEscapeString(templateURL)
		escapedName := template.HTMLEscapeString(app.Name)
		escapedDesc := template.HTMLEscapeString(desc)
		if options.OpenAppNewTab {
			b.WriteString(`<div class="app-container" data-id="`)
			b.WriteString(escapedID)
			b.WriteString(`"><a target="_blank" rel="noopener" href="`)
			b.WriteString(escapedURL)
			b.WriteString(`" class="app-item" title="`)
			b.WriteString(escapedName)
			if hasIcon {
				b.WriteString(`"><div class="app-icon">`)
				b.WriteString(templateIcon)
				b.WriteString(`</div><div class="app-text has-icon"><p class="app-title">`)
			} else {
				b.WriteString(`"><div class="app-text"><p class="app-title">`)
			}
			b.WriteString(escapedName)
			b.WriteString(`</p><p class="app-desc">`)
			b.WriteString(escapedDesc)
			b.WriteString(`</p></div></a></div>`)
		} else {
			b.WriteString(`<div class="app-container" data-id="`)
			b.WriteString(escapedID)
			b.WriteString(`"><a rel="noopener" href="`)
			b.WriteString(escapedURL)
			b.WriteString(`" class="app-item" title="`)
			b.WriteString(escapedName)
			if hasIcon {
				b.WriteString(`"><div class="app-icon">`)
				b.WriteString(templateIcon)
				b.WriteString(`</div><div class="app-text has-icon"><p class="app-title">`)
			} else {
				b.WriteString(`"><div class="app-text"><p class="app-title">`)
			}
			b.WriteString(escapedName)
			b.WriteString(`</p><p class="app-desc">`)
			b.WriteString(escapedDesc)
			b.WriteString(`</p></div></a></div>`)
		}
	}
	return template.HTML(b.String()), nil
}
