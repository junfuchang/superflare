package home

import (
	"fmt"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

var loadFavoriteBookmarks = data.LoadFavoriteBookmarks
var loadNormalBookmarks = data.LoadNormalBookmarks

func appendConfiguredIconWarnings(locale string, iconMode string, showApps bool, showBookmarks bool, warnings []string) []string {
	iconMode = strings.ToUpper(strings.TrimSpace(iconMode))
	if iconMode == define.IconModeHidden {
		return warnings
	}

	if showApps {
		if appsData, err := loadFavoriteBookmarks(); err == nil {
			warnings = appendConfiguredIconWarningsForItems(locale, iconMode, true, appsData.Items, false, nil, warnings)
		} else {
			warnings = append(warnings, formatConfiguredIconLoadWarning(locale, "apps", err))
		}
	}

	if showBookmarks {
		if bookmarksData, err := loadNormalBookmarks(); err == nil {
			warnings = appendConfiguredIconWarningsForItems(locale, iconMode, false, nil, true, bookmarksData.Items, warnings)
		} else {
			warnings = append(warnings, formatConfiguredIconLoadWarning(locale, "bookmarks", err))
		}
	}

	return warnings
}

func appendConfiguredIconWarningsForItems(locale string, iconMode string, showApps bool, apps []model.Bookmark, showBookmarks bool, bookmarks []model.Bookmark, warnings []string) []string {
	iconMode = strings.ToUpper(strings.TrimSpace(iconMode))
	if iconMode == define.IconModeHidden {
		return warnings
	}
	if showApps {
		if count := countInvalidExplicitIcons(apps); count > 0 {
			warnings = append(warnings, formatConfiguredIconWarning(locale, "apps", count, iconMode))
		}
	}
	if showBookmarks {
		if count := countInvalidExplicitIcons(bookmarks); count > 0 {
			warnings = append(warnings, formatConfiguredIconWarning(locale, "bookmarks", count, iconMode))
		}
	}
	return warnings
}

func countInvalidExplicitIcons(items []model.Bookmark) int {
	count := 0
	for _, item := range items {
		if hasInvalidExplicitIcon(item.Icon) {
			count++
		}
	}
	return count
}

func hasInvalidExplicitIcon(icon string) bool {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return false
	}
	if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		return false
	}
	return !mdi.IconExists(icon)
}

func formatConfiguredIconWarning(locale string, scope string, count int, iconMode string) string {
	if count <= 0 {
		return ""
	}
	labelEn := "bookmark"
	titleEn := "Bookmark"
	titleZh := "\u4e66\u7b7e"
	if scope == "apps" {
		labelEn = "app"
		titleEn = "App"
		titleZh = "\u5e94\u7528"
	}

	if locale == "en" {
		suffix := "entries"
		verb := "use"
		fallbackDetail := "are temporarily rendering without icons."
		if count == 1 {
			suffix = "entry"
			verb = "uses"
			fallbackDetail = "is temporarily rendering without icons."
		}
		if iconMode == define.IconModeMissingFill {
			fallbackDetail = "are temporarily using fallback icons."
			if count == 1 {
				fallbackDetail = "is temporarily using fallback icons."
			}
		}
		return fmt.Sprintf("%s icon config fallback: %d %s %s %s invalid custom icon names and %s", titleEn, count, labelEn, suffix, verb, fallbackDetail)
	}
	if iconMode == define.IconModeMissingFill {
		return fmt.Sprintf("%s\u56fe\u6807\u914d\u7f6e\u5b58\u5728\u56de\u9000\uff1a%d \u6761%s\u4f7f\u7528\u4e86\u65e0\u6548\u7684\u81ea\u5b9a\u4e49\u56fe\u6807\u540d\uff0c\u5f53\u524d\u5df2\u4e34\u65f6\u4f7f\u7528\u56de\u9000\u56fe\u6807\u3002", titleZh, count, titleZh)
	}
	return fmt.Sprintf("%s\u56fe\u6807\u914d\u7f6e\u5b58\u5728\u56de\u9000\uff1a%d \u6761%s\u4f7f\u7528\u4e86\u65e0\u6548\u7684\u81ea\u5b9a\u4e49\u56fe\u6807\u540d\uff0c\u5f53\u524d\u5df2\u4e34\u65f6\u4e0d\u5c55\u793a\u56fe\u6807\u3002", titleZh, count, titleZh)
}

func formatConfiguredIconLoadWarning(locale string, scope string, err error) string {
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return ""
	}
	titleEn := "Bookmark"
	titleZh := "\u4e66\u7b7e"
	if scope == "apps" {
		titleEn = "App"
		titleZh = "\u5e94\u7528"
	}
	if locale == "en" {
		return fmt.Sprintf("%s icon config could not be read: %s. Icon fallback diagnostics were skipped.", titleEn, detail)
	}
	return fmt.Sprintf("%s\u56fe\u6807\u914d\u7f6e\u8bfb\u53d6\u5931\u8d25\uff1a%s\u3002\u56fe\u6807\u56de\u9000\u8bca\u65ad\u5df2\u8df3\u8fc7\u3002", titleZh, detail)
}
