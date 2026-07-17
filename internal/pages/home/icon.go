package home

import (
	"html/template"
	"strings"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

var getSiteFaviconAssetURL = fn.GetSiteFaviconAssetURL

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
		return iconFallbackForLink(link, defaultBookmarkIcon)
	}

	if iconMode == define.IconModeMissingFill {
		return iconFallbackForLink(link, defaultBookmarkIcon)
	}

	return ""
}

func iconFallbackForLink(link string, fallback string) string {
	iconURL := strings.TrimSpace(getSiteFaviconAssetURL(link))
	if iconURL == "" {
		return fallback
	}
	return markFallbackIconForAsyncSiteFavicon(fallback, iconURL)
}

func markFallbackIconForAsyncSiteFavicon(fallback string, iconURL string) string {
	if fallback == "" || iconURL == "" {
		return fallback
	}
	escaped := template.HTMLEscapeString(iconURL)
	if strings.Contains(fallback, `data-site-icon-src=`) {
		return fallback
	}
	if strings.HasPrefix(fallback, "<img ") {
		return strings.Replace(fallback, "<img ", `<img data-site-icon-src="`+escaped+`" `, 1)
	}
	if strings.HasPrefix(fallback, "<svg ") {
		return strings.Replace(fallback, "<svg ", `<svg data-site-icon-src="`+escaped+`" `, 1)
	}
	return fallback
}
