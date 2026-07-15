package home

import (
	"html/template"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

var builderPool = sync.Pool{
	New: func() any { return &strings.Builder{} },
}

type applicationProjection struct {
	HTML           template.HTML
	Modals         template.HTML
	HasDirectories bool
	items          []model.Bookmark
}

type applicationDirectory struct {
	Name        string
	Items       []model.Bookmark
	SourceIndex int
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
	projection, err := generateApplicationProjectionWithLocalAndURLErr(filter, options, preferLocal, requestURL, canViewPrivate)
	return projection.HTML, err
}

func generateApplicationProjectionWithLocalAndURLErr(filter string, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL, canViewPrivate bool) (applicationProjection, error) {
	if options == nil {
		options = &model.Application{}
	}
	appsData, err := loadFavoriteBookmarks()
	if err != nil {
		return applicationProjection{}, err
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
			!strings.Contains(strings.ToLower(app.Desc), filterLower) &&
			!strings.Contains(strings.ToLower(app.Subdir), filterLower) {
			continue
		}
		apps = append(apps, app)
	}

	directories := make([]applicationDirectory, 0)
	directoryIndexes := make(map[string]int)
	ungrouped := make([]model.Bookmark, 0, len(apps))
	for sourceIndex, app := range apps {
		subdir := strings.TrimSpace(app.Subdir)
		if subdir == "" {
			ungrouped = append(ungrouped, app)
			continue
		}

		index, ok := directoryIndexes[subdir]
		if !ok {
			index = len(directories)
			directoryIndexes[subdir] = index
			directories = append(directories, applicationDirectory{Name: subdir, SourceIndex: sourceIndex})
		}
		directories[index].Items = append(directories[index].Items, app)
	}

	sort.SliceStable(directories, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(directories[i].Name))
		right := strings.ToLower(strings.TrimSpace(directories[j].Name))
		if left != right {
			return left < right
		}
		if directories[i].Name != directories[j].Name {
			return directories[i].Name < directories[j].Name
		}
		return directories[i].SourceIndex < directories[j].SourceIndex
	})

	var modals strings.Builder
	for index, directory := range directories {
		renderApplicationDirectory(b, directory, index)
		renderApplicationDirectoryModal(&modals, directory, index, options, preferLocal, requestURL)
	}
	for _, app := range ungrouped {
		renderApplicationItem(b, app, options, preferLocal, requestURL)
	}

	return applicationProjection{
		HTML:           template.HTML(b.String()),
		Modals:         template.HTML(modals.String()),
		HasDirectories: len(directories) > 0,
		items:          apps,
	}, nil
}

func renderApplicationDirectory(b *strings.Builder, directory applicationDirectory, index int) {
	modalID := "application-subdir-modal-" + strconv.Itoa(index)
	escapedModalID := template.HTMLEscapeString(modalID)
	escapedName := template.HTMLEscapeString(directory.Name)

	b.WriteString(`<div class="app-container application-subdirectory-trigger" data-application-subdirectory="`)
	b.WriteString(escapedName)
	b.WriteString(`"><a rel="noopener" href="#`)
	b.WriteString(escapedModalID)
	b.WriteString(`" class="app-item" title="`)
	b.WriteString(escapedName)
	b.WriteString(`" aria-haspopup="dialog" aria-expanded="false" aria-controls="`)
	b.WriteString(escapedModalID)
	b.WriteString(`"><div class="app-icon">`)
	b.WriteString(mdi.GetIconByName("folder"))
	b.WriteString(`</div><div class="app-text has-icon"><p class="app-title">`)
	b.WriteString(escapedName)
	b.WriteString(`</p></div></a></div>`)
}

func renderApplicationDirectoryModal(b *strings.Builder, directory applicationDirectory, index int, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL) {
	indexString := strconv.Itoa(index)
	modalID := template.HTMLEscapeString("application-subdir-modal-" + indexString)
	titleID := template.HTMLEscapeString("application-subdir-title-" + indexString)
	escapedName := template.HTMLEscapeString(directory.Name)

	b.WriteString(`<div id="`)
	b.WriteString(modalID)
	b.WriteString(`" class="application-subdirectory-modal"><a href="#" class="application-subdirectory-backdrop" aria-label="Close" tabindex="-1" aria-hidden="true"></a><div class="application-subdirectory-panel" tabindex="-1" role="dialog" aria-modal="true" aria-labelledby="`)
	b.WriteString(titleID)
	b.WriteString(`"><div class="application-subdirectory-header"><h2 id="`)
	b.WriteString(titleID)
	b.WriteString(`">`)
	b.WriteString(escapedName)
	b.WriteString(`</h2><a href="#" class="application-subdirectory-close" aria-label="Close">&times;</a></div><div class="application-subdirectory-content apps-surface clearfix">`)
	for _, app := range directory.Items {
		renderApplicationItem(b, app, options, preferLocal, requestURL)
	}
	b.WriteString(`</div></div></div>`)
}

func renderApplicationItem(b *strings.Builder, app model.Bookmark, options *model.Application, preferLocal bool, requestURL *fn.DynamicURL) {
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
	b.WriteString(`<div class="app-container" data-id="`)
	b.WriteString(escapedID)
	if options.OpenAppNewTab {
		b.WriteString(`"><a target="_blank" rel="noopener" href="`)
	} else {
		b.WriteString(`"><a rel="noopener" href="`)
	}
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
