package define

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProductionCodeDoesNotReachIntoLegacyRuntimeFlagGlobals(t *testing.T) {
	offenders := findProductionGlobalReferences(t, regexp.MustCompile(`\bdefine\.(AppFlags|AppBaseFlags|AppSourceFlags)\b`))
	if len(offenders) > 0 {
		t.Fatalf("production code must use define runtime snapshot APIs instead of legacy globals: %s", strings.Join(offenders, ", "))
	}
}

func TestProductionCodeDoesNotReachIntoLegacyThemeGlobals(t *testing.T) {
	offenders := findProductionGlobalReferences(t, regexp.MustCompile(`\bdefine\.(ThemeCurrent|ThemePrimaryColor|CACHE_APP_CURRENT_THEME_PRIMARY_COLOR)\b`))
	if len(offenders) > 0 {
		t.Fatalf("production code must use define theme snapshot APIs instead of legacy globals: %s", strings.Join(offenders, ", "))
	}
}

func TestProductionGlobalScanIgnoresOnlyMissingNonGoFiles(t *testing.T) {
	fileEntry := productionScanTestEntry(t, false)
	dirEntry := productionScanTestEntry(t, true)
	tests := []struct {
		name  string
		path  string
		entry os.DirEntry
		err   error
		want  bool
	}{
		{name: "runtime config disappeared", path: filepath.Join("config", "data", "bookmarks.yml"), entry: fileEntry, err: os.ErrNotExist, want: true},
		{name: "runtime config temp disappeared", path: filepath.Join("config", "data", ".bookmarks.yml.tmp-123"), entry: fileEntry, err: os.ErrNotExist, want: true},
		{name: "runtime lock disappeared", path: filepath.Join("config", "data", ".superflare-config.lock"), entry: fileEntry, err: os.ErrNotExist, want: true},
		{name: "Go source disappeared", path: filepath.Join("internal", "server", "server.go"), entry: fileEntry, err: os.ErrNotExist, want: false},
		{name: "source directory disappeared", path: filepath.Join("internal", "server"), entry: dirEntry, err: os.ErrNotExist, want: false},
		{name: "runtime-named directory disappeared", path: filepath.Join("internal", "config.yml"), entry: dirEntry, err: os.ErrNotExist, want: false},
		{name: "runtime-temp-named directory disappeared", path: filepath.Join("internal", ".bookmarks.yml.tmp-123"), entry: dirEntry, err: os.ErrNotExist, want: false},
		{name: "entry identity unavailable", path: filepath.Join("config", "data", "bookmarks.yml"), entry: nil, err: os.ErrNotExist, want: false},
		{name: "unrelated text file disappeared", path: filepath.Join("docs", "README.md"), entry: fileEntry, err: os.ErrNotExist, want: false},
		{name: "unrelated YAML temp disappeared", path: filepath.Join("docs", ".notes.yml.tmp-123"), entry: fileEntry, err: os.ErrNotExist, want: false},
		{name: "runtime config permission error", path: filepath.Join("config", "data", "bookmarks.yml"), entry: fileEntry, err: os.ErrPermission, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIgnoreProductionWalkError(tt.path, tt.entry, tt.err); got != tt.want {
				t.Fatalf("shouldIgnoreProductionWalkError(%q, %v) = %v, want %v", tt.path, tt.err, got, tt.want)
			}
		})
	}
}

func productionScanTestEntry(t *testing.T, directory bool) os.DirEntry {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "entry")
	var err error
	if directory {
		err = os.Mkdir(path, 0o755)
	} else {
		err = os.WriteFile(path, []byte("test"), 0o644)
	}
	if err != nil {
		t.Fatalf("create test directory entry: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read test directory entry: entries=%d err=%v", len(entries), err)
	}
	return entries[0]
}

func shouldIgnoreProductionWalkError(path string, entry os.DirEntry, err error) bool {
	if !os.IsNotExist(err) || entry == nil || entry.IsDir() || !entry.Type().IsRegular() {
		return false
	}

	base := filepath.Base(path)
	for _, name := range []string{".env", "config.yml", "apps.yml", "bookmarks.yml", "ports.yaml"} {
		if base == name || strings.HasPrefix(base, "."+name+".tmp-") || strings.HasPrefix(base, "."+name+".backup-") {
			return true
		}
	}
	return base == ".superflare-config.lock"
}

func findProductionGlobalReferences(t *testing.T, pattern *regexp.Regexp) []string {
	t.Helper()
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	var offenders []string
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if shouldIgnoreProductionWalkError(path, entry, err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".tools", "node_modules", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "config/define/") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if pattern.Match(raw) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk production code: %v", err)
	}
	return offenders
}
