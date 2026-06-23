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
		if builtInIcon := mdi.GetIconByName(icon); builtInIcon != "" {
			return builtInIcon
		}
	}
	if favicon := fn.GetSiteFavicon(link, ""); favicon != "" {
		return favicon
	}
	if iconMode == "FILLING" {
		if fillingIcon := fn.GetYandexFavicon(link, ""); fillingIcon != "" {
			return fillingIcon
		}
	}
	return mdi.GetIconByName("bookmark")
}
