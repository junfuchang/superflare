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
	tests := []struct {
		name string
		path string
		err  error
		want bool
	}{
		{name: "runtime config disappeared", path: filepath.Join("config", "data", "bookmarks.yml"), err: os.ErrNotExist, want: true},
		{name: "Go source disappeared", path: filepath.Join("internal", "server", "server.go"), err: os.ErrNotExist, want: false},
		{name: "runtime config permission error", path: filepath.Join("config", "data", "bookmarks.yml"), err: os.ErrPermission, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIgnoreProductionWalkError(tt.path, tt.err); got != tt.want {
				t.Fatalf("shouldIgnoreProductionWalkError(%q, %v) = %v, want %v", tt.path, tt.err, got, tt.want)
			}
		})
	}
}

func shouldIgnoreProductionWalkError(path string, err error) bool {
	return os.IsNotExist(err) && !strings.HasSuffix(path, ".go")
}

func findProductionGlobalReferences(t *testing.T, pattern *regexp.Regexp) []string {
	t.Helper()
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	var offenders []string
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if shouldIgnoreProductionWalkError(path, err) {
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
