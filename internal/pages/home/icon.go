package home

import (
	"html/template"
	"strings"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func renderBookmarkIcon(icon string, link string, iconMode string) string {
	iconMode = strings.ToUpper(strings.TrimSpace(iconMode))
	if iconMode == define.IconModeHidden {
		return ""
	}
	defaultBookmarkIcon := mdi.GetIconByName("bookmark")

	icon = strings.TrimSpace(icon)
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return `<img src="` + template.HTMLEscapeString(icon) + `" alt="">`
	}
	if icon != "" {
		if builtInIcon := mdi.GetIconByName(icon); builtInIcon != "" {
			return builtInIcon
		}
		if iconMode == define.IconModeMissingFill {
			if favicon := fn.GetSiteFaviconFast(link, defaultBookmarkIcon); favicon != "" {
				return favicon
			}
			return defaultBookmarkIcon
		}
		return ""
	}

	if iconMode == define.IconModeMissingFill {
		if favicon := fn.GetSiteFaviconFast(link, defaultBookmarkIcon); favicon != "" {
			return favicon
		}
		return defaultBookmarkIcon
	}

	return ""
}
