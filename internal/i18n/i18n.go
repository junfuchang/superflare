package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

//go:embed locales/*.json
var localesFS embed.FS

var (
	mu      sync.RWMutex
	bundles = make(map[string]map[string]string)
)

func init() {
	load("zh", "locales/zh.json")
	load("en", "locales/en.json")
}

func load(lang, path string) {
	data, err := localesFS.ReadFile(path)
	if err != nil {
		return
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	mu.Lock()
	bundles[lang] = m
	mu.Unlock()
}

func NormalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch {
	case locale == "", strings.HasPrefix(locale, "zh"):
		return "zh"
	case strings.HasPrefix(locale, "en"):
		return "en"
	default:
		return "zh"
	}
}

func T(locale, key string) string {
	locale = NormalizeLocale(locale)
	mu.RLock()
	m := bundles[locale]
	if m == nil {
		m = bundles["zh"]
	}
	mu.RUnlock()
	if m == nil {
		return key
	}
	if s, ok := m[key]; ok {
		return s
	}
	return key
}

func Tf(locale, key string, args ...any) string {
	return fmt.Sprintf(T(locale, key), args...)
}

func Weekday(locale string, w time.Weekday) string {
	keys := []string{"weekday_sun", "weekday_mon", "weekday_tue", "weekday_wed", "weekday_thu", "weekday_fri", "weekday_sat"}
	if int(w) < 0 || int(w) > 6 {
		return ""
	}
	return T(locale, keys[w])
}

func DateFormat(locale string) string {
	switch NormalizeLocale(locale) {
	case "en":
		return "Jan 02, 2006"
	default:
		return "2006年1月2日"
	}
}
