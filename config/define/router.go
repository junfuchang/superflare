package define

import (
	"github.com/junfuchang/superflare/config/model"
)

func getRegularPages() model.RouteMaps {
	return model.RouteMaps{
		Home: model.Page{
			Name:  "Home",
			Title: "Home",
			Path:  "/",
		},
		Settings: model.Page{
			Name:  "Settings",
			Title: "Settings",
			Path:  "/settings",
		},
		Applications: model.Page{
			Name:  "Applications",
			Title: "Applications",
			Path:  "/applications",
		},
		Bookmarks: model.Page{
			Name:  "Bookmarks",
			Title: "Bookmarks",
			Path:  "/bookmarks",
		},
		Help: model.Page{
			Name:  "Help",
			Title: "Help",
			Path:  "/help",
		},
		Guide: model.Page{
			Name:  "Guide",
			Title: "Guide",
			Path:  "/guide",
		},
		Editor: model.Page{
			Name:  "Editor",
			Title: "Editor",
			Path:  "/editor",
		},
		Icons: model.Page{
			Name:  "Icons",
			Title: "MDI",
			Path:  "/icons",
		},
	}
}

var RegularPages = getRegularPages()

func getSettingPages() model.RouteMaps {
	return model.RouteMaps{
		Theme: model.Page{
			Name:  "Theme",
			Title: "Theme",
			Path:  "/settings/theme",
		},
		Search: model.Page{
			Name:  "Search",
			Title: "Search",
			Path:  "/settings/search",
		},
		Appearance: model.Page{
			Name:  "Appearance",
			Title: "Appearance",
			Path:  "/settings/appearance",
		},
		Ports: model.Page{
			Name:  "Ports",
			Title: "Ports",
			Path:  "/settings/ports",
		},
		Others: model.Page{
			Name:  "Others",
			Title: "Others",
			Path:  "/settings/application",
		},
	}
}

var SettingPages = getSettingPages()

func getSettingAPIs() model.RouteMaps {
	return model.RouteMaps{}
}

var SettingPagesAPI = getSettingAPIs()

func getMiscPages() model.RouteMaps {
	return model.RouteMaps{
		HealthCheck: model.API{
			Name: "HealthCheck",
			Path: "/ping",
		},
		RedirHome: model.Page{
			Title: "Redirecting...",
			Name:  "Redir",
			Path:  "/redir",
		},
		RedirHelper: model.API{
			Name: "RedirHelper",
			Path: "/redir/url",
		},
		RedirLocal: model.API{
			Name: "RedirLocal",
			Path: "/redir/local",
		},
		Login: model.API{
			Name: "Login",
			Path: "/login",
		},
		Logout: model.API{
			Name: "Logout",
			Path: "/logout",
		},
	}
}

var MiscPages = getMiscPages()
