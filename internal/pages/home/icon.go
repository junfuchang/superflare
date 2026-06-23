package home

import (
	"html/template"
	"strings"

	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func renderBookmarkIcon(icon string, link string, iconMode string) string {
	icon = strings.TrimSpace(icon)
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return `<img src="` + template.HTMLEscapeString(icon) + `" alt="">`
	}
	if icon != "" {
		return mdi.GetIconByName(icon)
	}
	if favicon := fn.GetSiteFavicon(link, ""); favicon != "" {
		return favicon
	}
	if iconMode == "FILLING" {
		return fn.GetYandexFavicon(link, "")
	}
	return ""
}
