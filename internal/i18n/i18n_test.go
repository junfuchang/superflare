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
