package validation

import (
	"strings"
	"testing"
)

func TestSafeCSSColorAcceptsSupportedColors(t *testing.T) {
	for _, input := range []string{
		"#fff",
		"#ffffff",
		"#ffffffff",
		"rgb(255, 253, 234)",
		"rgba(255, 253, 234, 1)",
		"rgba(10%, 20%, 30%, 50%)",
		" rgba(1, 2, 3, 0.5) ",
	} {
		if got := SafeCSSColor(input, "fallback"); got != strings.TrimSpace(input) {
			t.Fatalf("expected %q to be accepted, got %q", input, got)
		}
	}
}

func TestSafeCSSColorRejectsUnsafeOrUnsupportedColors(t *testing.T) {
	for _, input := range []string{
		"red",
		"url(javascript:alert(1))",
		"rgb(256, 0, 0)",
		"rgba(0, 0, 0, 2)",
		"rgb(0, 0)",
		"#ggg",
	} {
		if got := SafeCSSColor(input, "fallback"); got != "fallback" {
			t.Fatalf("expected %q to fall back, got %q", input, got)
		}
	}
}
