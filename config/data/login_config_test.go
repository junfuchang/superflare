package data

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLoginConfigPrefersConfigThenEnv(t *testing.T) {
	withTempWorkingDir(t)

	require.True(t, UpdateLoginConfig("config-user", "config#pass;1"))

	user, pass, err := GetLoginConfig()
	require.NoError(t, err)
	assert.Equal(t, "config-user", user)
	assert.Equal(t, "config#pass;1", pass)
}

func TestGetLoginConfigFallsBackToEnv(t *testing.T) {
	withTempWorkingDir(t)

	require.NoError(t, os.WriteFile(getEnvFilePath(), []byte("FLARE_USER=env-user\nFLARE_PASS=env#pass;2\n"), 0644))

	user, pass, err := GetLoginConfig()
	require.NoError(t, err)
	assert.Equal(t, "env-user", user)
	assert.Equal(t, "env#pass;2", pass)
}

func TestUpdateLoginConfigSyncsEnvFile(t *testing.T) {
	withTempWorkingDir(t)

	require.NoError(t, os.WriteFile(getEnvFilePath(), []byte("FLARE_USER=old-user\nFLARE_PASS=old-pass\n"), 0644))
	require.True(t, UpdateLoginConfig("admin", "abc#123;xyz"))

	content, err := os.ReadFile(getEnvFilePath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "FLARE_USER=admin")
	assert.Contains(t, string(content), "FLARE_PASS=abc#123;xyz")
}

func TestUpdateLoginConfigDoesNotCreateEnvFileWhenMissing(t *testing.T) {
	withTempWorkingDir(t)

	require.True(t, UpdateLoginConfig("admin", "abc#123;xyz"))

	_, err := os.Stat(getEnvFilePath())
	assert.True(t, os.IsNotExist(err))
}
