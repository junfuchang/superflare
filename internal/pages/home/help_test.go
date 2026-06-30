package home

import (
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
)

func TestGenerateHelpTemplate_RemovesSettingsAndFeedbackEntries(t *testing.T) {
	orig := saveAppFlags()
	origRuntime, origRuntimeSet := saveHelpRuntimeFlags()
	defer restoreAppFlags(orig)
	defer restoreHelpRuntimeFlags(origRuntime, origRuntimeSet)
	define.AppFlags = model.Flags{EnableGuide: true, EnableEditor: true}
	helpRuntimeFlags.Store(helpRuntimeSnapshotFromFlags(define.AppFlags))

	html := string(GenerateHelpTemplate("zh"))
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

func TestGenerateHelpTemplate_DefaultsToChinese(t *testing.T) {
	origRuntime, origRuntimeSet := saveHelpRuntimeFlags()
	defer restoreHelpRuntimeFlags(origRuntime, origRuntimeSet)
	restoreHelpRuntimeFlags(helpRuntimeSnapshot{}, false)
	html := string(GenerateHelpTemplate(""))
	expected := []string{`title="首页"`, `title="帮助"`, `title="应用设置"`, `title="图标库"`}
	for _, token := range expected {
		if !strings.Contains(html, token) {
			t.Fatalf("help template should contain %q in %s", token, html)
		}
	}
}

func TestGenerateHelpTemplate_UsesEnglishWhenLocaleIsEnglish(t *testing.T) {
	origRuntime, origRuntimeSet := saveHelpRuntimeFlags()
	defer restoreHelpRuntimeFlags(origRuntime, origRuntimeSet)
	restoreHelpRuntimeFlags(helpRuntimeSnapshot{}, false)
	html := string(GenerateHelpTemplate("en-US"))
	expected := []string{`title="Home"`, `title="Help"`, `title="Settings"`, `title="Icons"`}
	for _, token := range expected {
		if !strings.Contains(html, token) {
			t.Fatalf("help template should contain %q in %s", token, html)
		}
	}
}

func TestGenerateHelpTemplateFallsBackWhenIconLookupFails(t *testing.T) {
	orig := getHelpIconByName
	origRuntime, origRuntimeSet := saveHelpRuntimeFlags()
	getHelpIconByName = func(string) string { return "" }
	defer func() { getHelpIconByName = orig }()
	defer restoreHelpRuntimeFlags(origRuntime, origRuntimeSet)
	restoreHelpRuntimeFlags(helpRuntimeSnapshot{}, false)

	html := string(GenerateHelpTemplate("zh"))
	if !strings.Contains(html, "<svg") {
		t.Fatalf("expected builtin fallback svg icon, got %s", html)
	}
}

func TestGenerateHelpTemplateKeepsRuntimeFeatureFlagsAfterAppFlagsChange(t *testing.T) {
	orig := saveAppFlags()
	origRuntime, origRuntimeSet := saveHelpRuntimeFlags()
	defer restoreAppFlags(orig)
	defer restoreHelpRuntimeFlags(origRuntime, origRuntimeSet)

	define.AppFlags = model.Flags{EnableGuide: true, EnableEditor: true}
	helpRuntimeFlags.Store(helpRuntimeSnapshotFromFlags(define.AppFlags))

	define.AppFlags = model.Flags{EnableGuide: false, EnableEditor: false}

	html := string(GenerateHelpTemplate("en"))
	for _, token := range []string{`title="Guide"`, `title="Editor"`} {
		if !strings.Contains(html, token) {
			t.Fatalf("expected help template to preserve runtime feature %q in %s", token, html)
		}
	}
}

func saveHelpRuntimeFlags() (helpRuntimeSnapshot, bool) {
	helpRuntimeFlags.mu.RLock()
	defer helpRuntimeFlags.mu.RUnlock()
	return helpRuntimeFlags.cfg, helpRuntimeFlags.set
}

func restoreHelpRuntimeFlags(cfg helpRuntimeSnapshot, set bool) {
	helpRuntimeFlags.mu.Lock()
	helpRuntimeFlags.cfg = cfg
	helpRuntimeFlags.set = set
	helpRuntimeFlags.mu.Unlock()
}
