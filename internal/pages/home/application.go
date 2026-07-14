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
	return generateApplicationsTemplateWithLocalAndURLErr(filter, options, preferLocal, requestURL, true)
}

func generateApplicationsTemplateWithLocalAndURLErr(filter string, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL, canViewPrivate bool) (template.HTML, error) {
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

	filterLower := strings.ToLower(filter)
	apps := make([]model.Bookmark, 0, len(appsData.Items))
	for _, app := range appsData.Items {
		if !bookmarkVisible(app, canViewPrivate) {
			continue
		}
		app.URL = fn.ParseDynamicUrlWith(app.URL, requestURL)
		app.LocalURL = fn.ParseDynamicUrlWith(app.LocalURL, requestURL)
		if filter != "" &&
			!strings.Contains(strings.ToLower(app.Name), filterLower) &&
			!strings.Contains(strings.ToLower(app.URL), filterLower) &&
			!strings.Contains(strings.ToLower(app.LocalURL), filterLower) &&
			!strings.Contains(strings.ToLower(app.Desc), filterLower) {
			continue
		}
		apps = append(apps, app)
	}

	for _, app := range apps {
		desc := app.Desc
		if desc == "" {
			desc = app.URL
		}
		templateURL := renderBookmarkHref(app.URL, app.LocalURL, preferLocal, options.EnableEncryptedLink, requestURL)
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
