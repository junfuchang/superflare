package cmd_test

import (
	"os"
	"testing"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/stretchr/testify/assert"
)

func TestCheckDotEnvFileExist(t *testing.T) {
	withTempCmdWorkDir(t)
	envPath := ".env"

	// test .env not exist
	_ = os.Remove(envPath)
	exists, err := cmd.CheckDotEnvFileExist(envPath)
	assert.NoError(t, err)
	assert.Equal(t, false, exists)

	// test .env exist
	f, _ := os.Create(envPath)
	filename := f.Name()
	assert.NoError(t, f.Close())
	defer os.Remove(filename)
	exists, err = cmd.CheckDotEnvFileExist(envPath)
	assert.NoError(t, err)
	assert.Equal(t, true, exists)

	assert.NoError(t, os.Remove(envPath))
	assert.NoError(t, os.Mkdir(envPath, 0755))
	exists, err = cmd.CheckDotEnvFileExist(envPath)
	assert.Error(t, err)
	assert.Equal(t, false, exists)
}

func TestParseEnvFile_NotExist(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"

	// test .env not exist
	_ = os.Remove(envPath)
	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.NoError(t, err)
	assert.Equal(t, flags, envParsed)
}

func TestParseEnvFile_DirectoryPathReturnsError(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"

	// test .env not exist
	_ = os.Mkdir(envPath, 0755)
	defer os.Remove(envPath)
	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.Equal(t, flags, envParsed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func TestParseEnvFile_ParseErr(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"
	envParsed.Port = 1234
	envParsed.User = "123"
	envParsed.UserIsGenerated = true
	envParsed.Pass = "123"
	envParsed.PassIsGenerated = true

	// test .env auto correct
	defer os.Remove(envPath)
	_ = os.WriteFile(envPath, []byte("FLARE_PORT=true\nFLARE_USER=\nFLARE_PASS=\nFLARE_GUIDE=1111"), 0644)
	_, err := cmd.ParseEnvFileE(envParsed)
	assert.Error(t, err)
}

func TestParseEnvFile_ParseOverwrite(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"
	envParsed.Port = 1234
	envParsed.User = "123"
	envParsed.UserIsGenerated = true
	envParsed.Pass = "123"
	envParsed.PassIsGenerated = true

	// test .env auto correct
	defer os.Remove(envPath)
	_ = os.WriteFile(envPath, []byte("FLARE_PORT=2345\nFLARE_USER=\nFLARE_PASS=\nFLARE_GUIDE=false"), 0644)
	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.NoError(t, err)

	envParsed.Port = 2345
	envParsed.EnableGuide = false

	envParsed.User = define.DEFAULT_LOGIN_USER
	envParsed.Pass = define.DEFAULT_LOGIN_PASS
	envParsed.UserIsGenerated = false
	envParsed.PassIsGenerated = false

	assert.Equal(t, flags, envParsed)
}

func TestParseEnvFile_PortError(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"
	envParsed.Port = 1234
	envParsed.User = "123"
	envParsed.UserIsGenerated = true
	envParsed.Pass = "123"
	envParsed.PassIsGenerated = true

	// test .env auto correct
	defer os.Remove(envPath)
	_ = os.WriteFile(envPath, []byte("FLARE_PORT=9999999\nFLARE_USER=\nFLARE_PASS=\nFLARE_GUIDE=false"), 0644)
	_, err := cmd.ParseEnvFileE(envParsed)
	assert.Error(t, err)
}

func TestParseEnvFile_RepairsCredentialsWhenOnlyUserIsConfigured(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	defaults := define.DefaultEnvVars

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"
	envParsed.User = defaults.User
	envParsed.Pass = defaults.Pass
	envParsed.UserIsGenerated = false
	envParsed.PassIsGenerated = false

	// test .env auto correct
	defer os.Remove(envPath)
	_ = os.WriteFile(envPath, []byte("FLARE_PORT=3636\nFLARE_USER=superflare\nFLARE_PASS=\n"), 0644)
	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.NoError(t, err)
	assert.Equal(t, define.DEFAULT_LOGIN_USER, flags.User)
	assert.Equal(t, define.DEFAULT_LOGIN_PASS, flags.Pass)
	assert.False(t, flags.UserIsGenerated)
	assert.False(t, flags.PassIsGenerated)
}

func TestParseEnvFile_PasswordWithCommentChars(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"
	envParsed.User = "fallback-user"
	envParsed.Pass = "fallback-pass"
	envParsed.UserIsGenerated = true
	envParsed.PassIsGenerated = true

	f, _ := os.Create(envPath)
	defer os.Remove(envPath)
	defer f.Close()
	_, _ = f.Write([]byte("FLARE_USER=test-user\nFLARE_PASS=abc#123;xyz\n"))

	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.NoError(t, err)
	assert.Equal(t, "test-user", flags.User)
	assert.Equal(t, "abc#123;xyz", flags.Pass)
	assert.Equal(t, false, flags.UserIsGenerated)
	assert.Equal(t, false, flags.PassIsGenerated)
}

func TestParseEnvFile_RepairsIncompleteLoginCredentials(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	t.Setenv("FLARE_DEBUG", "true")

	envParsed := cmd.ParseEnvVars()
	envPath := ".env"

	defer os.Remove(envPath)
	_ = os.WriteFile(envPath, []byte("FLARE_USER=superflare\nFLARE_PASS=\n"), 0644)

	flags, err := cmd.ParseEnvFileE(envParsed)
	assert.NoError(t, err)
	assert.Equal(t, define.DEFAULT_LOGIN_USER, flags.User)
	assert.Equal(t, define.DEFAULT_LOGIN_PASS, flags.Pass)
	assert.False(t, flags.UserIsGenerated)
	assert.False(t, flags.PassIsGenerated)
}
