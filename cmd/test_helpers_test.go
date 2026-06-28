package cmd_test

import (
	"os"
	"testing"
)

var cmdTestEnvKeys = []string{
	"FLARE_DEBUG",
	"FLARE_PORT",
	"FLARE_GUIDE",
	"FLARE_EDITOR",
	"FLARE_MINI_REQUEST",
	"FLARE_DISABLE_CSP",
	"FLARE_VISIBILITY",
	"FLARE_DISABLE_LOGIN",
	"FLARE_USER",
	"FLARE_PASS",
	"FLARE_COOKIE_NAME",
	"FLARE_COOKIE_SECRET",
}

func resetCmdEnv(t *testing.T) {
	t.Helper()
	for _, key := range cmdTestEnvKeys {
		value, exists := os.LookupEnv(key)
		if exists {
			t.Cleanup(func(k string, v string) func() {
				return func() { _ = os.Setenv(k, v) }
			}(key, value))
		} else {
			t.Cleanup(func(k string) func() {
				return func() { _ = os.Unsetenv(k) }
			}(key))
		}
		_ = os.Unsetenv(key)
	}
}

func withTempCmdWorkDir(t *testing.T) string {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	return tmpDir
}
