package home

import (
	"html/template"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func GenerateHelpTemplate() template.HTML {
	apps := []model.Bookmark{
		{
			Name: "Home",
			URL:  define.RegularPages.Home.Path,
			Icon: "homeCircle",
			Desc: "",
		},
		{
			Name: "Help",
			URL:  define.RegularPages.Help.Path,
			Icon: "helpCircle",
			Desc: "",
		},
		{
			Name: "Settings",
			URL:  define.RegularPages.Settings.Path,
			Icon: "fireCircle",
			Desc: "",
		},
	}

	if define.AppFlags.EnableGuide {
		apps = append(apps, model.Bookmark{
			Name: "Guide",
			URL:  define.RegularPages.Guide.Path,
			Icon: "radioactiveCircleOutline",
			Desc: "",
		})
	}

	if define.AppFlags.EnableEditor {
		apps = append(apps, model.Bookmark{
			Name: "Editor",
			URL:  define.RegularPages.Editor.Path,
			Icon: "pencilCircle",
			Desc: "",
		})
	}

	apps = append(apps, []model.Bookmark{
		{
			Name: "Icons",
			URL:  define.RegularPages.Icons.Path,
			Icon: "heartCircle",
			Desc: "",
		},
	}...)

	tpl := ""
	for _, app := range apps {
		desc := app.URL
		if app.Desc != "" {
			desc = app.Desc
		}
		tpl += `
			<div class="app-container" data-id="` + app.Icon + `">
			<a href="` + app.URL + `" class="app-item" title="` + app.Name + `">
			  <div class="app-icon">` + mdi.GetIconByName(app.Icon) + `</div>
			  <div class="app-text">
				<p class="app-title">` + app.Name + `</p>
				<p class="app-desc">` + desc + `</p>
			  </div>
			</a>
			</div>
			`
	}
	return template.HTML(tpl)
}
