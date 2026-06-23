package home

import (
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
)

func TestGenerateHelpTemplate_RemovesSettingsAndFeedbackEntries(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags = model.Flags{EnableGuide: true, EnableEditor: true}

	html := string(GenerateHelpTemplate())
	blocked := []string{
		`title="Theme"`,
		`title="Search"`,
		`title="Appearance"`,
		`title="Application"`,
		`title="Feedback"`,
		define.SettingPages.Theme.Path,
		define.SettingPages.Search.Path,
		define.SettingPages.Appearance.Path,
		define.SettingPages.Others.Path,
		"https://github.com/junfuchang/superflare/issues",
	}
	for _, token := range blocked {
		if strings.Contains(html, token) {
			t.Fatalf("help template should not contain %q in %s", token, html)
		}
	}
}
