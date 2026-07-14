package data

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppConfig(t *testing.T) {
	filePath := getConfigPath("config")
	os.Remove(filePath)

	if err := EnsureAppConfigExists(); err != nil {
		t.Fatalf("EnsureAppConfigExists: %v", err)
	}
	data, err := loadAppConfigFromYaml("config")
	if err != nil {
		t.Fatalf("Load App Config: %v", err)
	}
	if data.Title != "SuperFlare" {
		t.Fatal("Load App Config Failed")
	}
	if err := saveAppConfigToYamlFile("config", data); err != nil {
		t.Fatalf("Save App Config Failed: %v", err)
	}

	os.Remove(filePath)
}

func TestLoadAppConfigFromYamlDefaultValues(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	require.NoError(t, EnsureAppConfigExists())
	data, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.Equal(t, "SuperFlare", data.Title, "default Title")
	assert.Equal(t, "onedark", data.Theme, "default Theme")
	assert.Equal(t, "onedark", data.ThemeBase, "default ThemeBase")
	assert.Equal(t, "zh", data.Locale, "default Locale")
	assert.Equal(t, "bing", data.SearchEngine, "default search engine")
	assert.Equal(t, "new-tab", data.SearchEngineOpenMode, "default search engine open mode")
	assert.True(t, data.ShowFavorites, "default show favorites")
	assert.Equal(t, 5, data.HomeMaxColumns, "default home max columns")
	assert.Equal(t, 1600, data.HomeMaxWidth, "default home max width")
}

func TestLoadAppConfigFromRawDefaultsMissingShowFavoritesToTrue(t *testing.T) {
	options, err := LoadAppConfigFromRaw([]byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\n"))
	require.NoError(t, err)
	assert.True(t, options.ShowFavorites)
}

func TestLoadAppConfigFromRawPreservesExplicitShowFavoritesFalse(t *testing.T) {
	options, err := LoadAppConfigFromRaw([]byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\nShowFavorites: false\n"))
	require.NoError(t, err)
	assert.False(t, options.ShowFavorites)
}

func TestLoadAppConfigFromYamlDefaultsMissingShowFavoritesToTrue(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\n"), 0644))

	options, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.True(t, options.ShowFavorites)
}

func TestLoadAppConfigFromYamlPreservesExplicitShowFavoritesFalse(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\nShowFavorites: false\n"), 0644))

	options, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.False(t, options.ShowFavorites)
}

func TestLoadAppConfigFromYamlInvalidYAML(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	configPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("Title: [invalid\n  broken"), 0644))

	_, err = loadAppConfigFromYaml("config")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config", "should return a parse error")
}

func TestLoadAppConfigFromYamlReturnsErrorWhenConfigMissing(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	_, err = loadAppConfigFromYaml("config")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config config is missing")
}

func TestLoadAppConfigFromYamlCacheSeparatesDifferentWorkingDirs(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	dirA := t.TempDir()
	dirB := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dirA, "config.yml"), []byte("Title: dir-a\nTheme: blackboard\nLocale: zh\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirB, "config.yml"), []byte("Title: dir-b\nTheme: blackboard\nLocale: zh\n"), 0644))

	require.NoError(t, os.Chdir(dirA))
	configA, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.Equal(t, "dir-a", configA.Title)

	require.NoError(t, os.Chdir(dirB))
	configB, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.Equal(t, "dir-b", configB.Title)
}

func TestLoadAppConfigFromYamlReturnsErrorWhenStatFails(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	targetPath := filepath.Join(tmpDir, "config.yml")
	originalStat := osStat
	defer func() { osStat = originalStat }()
	osStat = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(targetPath) {
			return nil, errors.New("forced stat failure")
		}
		return originalStat(path)
	}

	_, err = loadAppConfigFromYaml("config")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat config config failed")
}

func TestLoadAppConfigFromYamlReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := loadAppConfigFromYaml("config")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve config working directory failed")
}

func TestLoadAppConfigFromRawReturnsErrorWhenConfigValuesInvalid(t *testing.T) {
	_, err := LoadAppConfigFromRaw([]byte("Title: SuperFlare\nLocale: zh\nTheme: mystery\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme value: mystery")
}
