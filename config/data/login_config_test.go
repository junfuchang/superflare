package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateLoginConfigUpdatesConfigAndEnvTogether(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-old-user\nFLARE_PASS=env-old-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := UpdateLoginConfig("new-user", "new-pass"); err != nil {
		t.Fatalf("expected UpdateLoginConfig to succeed: %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(tmpDir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	configText := string(configRaw)
	if !strings.Contains(configText, "LoginUser: new-user") || !strings.Contains(configText, "LoginPass: new-pass") {
		t.Fatalf("config.yml was not updated with new login config: %s", configText)
	}

	envRaw, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envText := string(envRaw)
	if !strings.Contains(envText, "FLARE_USER=new-user") || !strings.Contains(envText, "FLARE_PASS=new-pass") {
		t.Fatalf(".env was not updated with new login config: %s", envText)
	}
}

func TestUpdateLoginConfigRejectsIncompleteCredentialsBeforeWriting(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	originalConfig := []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), originalConfig, 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	originalEnv := []byte("FLARE_USER=env-old-user\nFLARE_PASS=env-old-pass\n")
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), originalEnv, 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err = UpdateLoginConfig("new-user", "")
	if err == nil {
		t.Fatal("expected UpdateLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), "login credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(tmpDir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	if string(configRaw) != string(originalConfig) {
		t.Fatalf("config.yml should stay unchanged, got: %s", string(configRaw))
	}

	envRaw, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(envRaw) != string(originalEnv) {
		t.Fatalf(".env should stay unchanged, got: %s", string(envRaw))
	}
}

func TestUpdateLoginConfigRollsBackConfigWhenEnvReplaceFails(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-old-user\nFLARE_PASS=env-old-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	originalRename := osRename
	defer func() { osRename = originalRename }()

	renameCalls := 0
	osRename = func(oldPath string, newPath string) error {
		renameCalls++
		if renameCalls == 4 {
			return os.ErrPermission
		}
		return originalRename(oldPath, newPath)
	}

	if err := UpdateLoginConfig("new-user", "new-pass"); err == nil {
		t.Fatal("expected UpdateLoginConfig to fail")
	}

	configRaw, err := os.ReadFile(filepath.Join(tmpDir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	configText := string(configRaw)
	if !strings.Contains(configText, "LoginUser: old-user") || !strings.Contains(configText, "LoginPass: old-pass") {
		t.Fatalf("config.yml should be rolled back, got: %s", configText)
	}

	envRaw, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envText := string(envRaw)
	if !strings.Contains(envText, "FLARE_USER=env-old-user") || !strings.Contains(envText, "FLARE_PASS=env-old-pass") {
		t.Fatalf(".env should remain original, got: %s", envText)
	}
}

func TestGetLoginConfigReturnsEnvValuesWhenConfigCredentialsEmpty(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=env-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	user, pass, err := GetLoginConfig()
	if err != nil {
		t.Fatalf("GetLoginConfig: %v", err)
	}
	if user != "env-user" || pass != "env-pass" {
		t.Fatalf("expected env credentials, got user=%q pass=%q", user, pass)
	}
}

func TestGetLoginConfigReturnsEnvValuesWhenAssignmentsUseSpacesQuotesAndExport(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("export FLARE_USER = \"env-user\"\nFLARE_PASS = \"env pass\"\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	user, pass, err := GetLoginConfig()
	if err != nil {
		t.Fatalf("GetLoginConfig: %v", err)
	}
	if user != "env-user" || pass != "env pass" {
		t.Fatalf("expected env credentials from flexible assignments, got user=%q pass=%q", user, pass)
	}
}

func TestGetLoginConfigReturnsErrorWhenConfigBrokenEvenIfEnvComplete(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=env-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	_, _, err = GetLoginConfig()
	if err == nil {
		t.Fatal("expected GetLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), "parse config config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetLoginConfigReturnsErrorWhenConfigCredentialsIncomplete(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: config-user\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=env-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	_, _, err = GetLoginConfig()
	if err == nil {
		t.Fatal("expected GetLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), "config.yml login credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetLoginConfigReturnsErrorWhenEnvCredentialsIncomplete(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	_, _, err = GetLoginConfig()
	if err == nil {
		t.Fatal("expected GetLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), ".env login credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetLoginConfigReturnsErrorWhenEnvPathIsDirectoryAndFallbackNeeded(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, ".env"), 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	_, _, err = GetLoginConfig()
	if err == nil {
		t.Fatal("expected GetLoginConfig to fail when .env is a directory")
	}
	if !strings.Contains(err.Error(), "read login config failed") {
		t.Fatalf("expected wrapped read login config error, got %v", err)
	}
}

func TestReadEnvValueFromContentSupportsFlexibleAssignments(t *testing.T) {
	user, err := readEnvValueFromContent([]byte(" export FLARE_USER = \"admin user\" \n"), "FLARE_USER")
	if err != nil {
		t.Fatalf("readEnvValueFromContent: %v", err)
	}
	if user != "admin user" {
		t.Fatalf("expected flexible env assignment to parse, got %q", user)
	}
}

func TestUpdateLoginConfigUpdatesFlexibleEnvAssignments(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: old-user\nLoginPass: old-pass\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("export FLARE_USER = old-user\nFLARE_PASS = \"old pass\"\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := UpdateLoginConfig("new-user", "new pass"); err != nil {
		t.Fatalf("UpdateLoginConfig: %v", err)
	}

	envRaw, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envText := string(envRaw)
	if !strings.Contains(envText, "export FLARE_USER=new-user") {
		t.Fatalf("expected flexible FLARE_USER line to be updated, got: %s", envText)
	}
	if !strings.Contains(envText, `FLARE_PASS="new pass"`) {
		t.Fatalf("expected spaced FLARE_PASS to be quoted and updated, got: %s", envText)
	}
}

func TestGetLoginConfigReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, _, err := GetLoginConfig()
	if err == nil {
		t.Fatal("expected GetLoginConfig to fail")
	}
	if !strings.Contains(err.Error(), "resolve config working directory failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
