package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppConfig(t *testing.T) {
	filePath := getConfigPath("config")
	os.Remove(filePath)

	data, err := loadAppConfigFromYaml("config")
	if err != nil {
		t.Fatalf("Load App Config: %v", err)
	}
	if data.Title != "superflare" {
		t.Fatal("Load App Config Failed")
	}
	ok := saveAppConfigToYamlFile("config", data)
	if !ok {
		t.Fatal("Save App Config Failed")
	}

	os.Remove(filePath)
}

func TestLoadAppConfigFromYamlDefaultValues(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origWd) }()

	data, err := loadAppConfigFromYaml("config")
	require.NoError(t, err)
	assert.Equal(t, "superflare", data.Title, "default Title")
	assert.Equal(t, "blackboard", data.Theme, "default Theme")
	assert.Equal(t, "zh", data.Locale, "default Locale")
	assert.Equal(t, "rgba(26, 26, 26, 1)", data.CustomThemeBackground, "default custom background")
	assert.Equal(t, "rgba(255, 253, 234, 1)", data.CustomThemePrimary, "default custom primary")
	assert.Equal(t, "rgba(92, 92, 92, 1)", data.CustomThemeAccent, "default custom accent")
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
