package i18n

import "testing"

func TestNormalizeLocale(t *testing.T) {
	cases := map[string]string{
		"":           "zh",
		"zh":         "zh",
		"zh-CN":      "zh",
		"zh-Hans-CN": "zh",
		"en":         "en",
		"en-US":      "en",
		"EN-gb":      "en",
		"fr":         "zh",
	}
	for input, expected := range cases {
		if got := NormalizeLocale(input); got != expected {
			t.Fatalf("NormalizeLocale(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestDateFormatDefaultsToChinese(t *testing.T) {
	if got := DateFormat(""); got != "2006年1月2日" {
		t.Fatalf("DateFormat default = %q", got)
	}
	if got := DateFormat("fr"); got != "2006年1月2日" {
		t.Fatalf("DateFormat unknown locale = %q", got)
	}
}

func TestFavoritesModuleTranslationsAreNonEmpty(t *testing.T) {
	translations := map[string]string{
		"zh": "收藏",
		"en": "Favorites",
	}
	for locale, want := range translations {
		if got := T(locale, "favorites"); got != want {
			t.Fatalf("T(%q, favorites) = %q, want %q", locale, got, want)
		}
	}

	for _, locale := range []string{"zh", "en"} {
		for _, key := range []string{"show_favorites", "custom_favorites_title"} {
			if got := T(locale, key); got == "" || got == key {
				t.Fatalf("T(%q, %q) = %q, want a non-empty translation", locale, key, got)
			}
		}
	}
}
