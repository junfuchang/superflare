package cmd_test

import (
	"testing"

	env "github.com/caarlos0/env/v6"
	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/stretchr/testify/assert"
)

func TestParseEnvVars(t *testing.T) {
	resetCmdEnv(t)
	t.Setenv("FLARE_PORT", "5000")
	t.Setenv("FLARE_GUIDE", "false")
	t.Setenv("FLARE_USER", "test")
	t.Setenv("FLARE_VISIBILITY", "private")
	t.Setenv("FLARE_COOKIE_NAME", "flare_test")
	t.Setenv("FLARE_COOKIE_SECRET", "test_cookie_secret")

	flags := cmd.ParseEnvVars()

	assert.Equal(t, flags.Port, 5000)
	assert.Equal(t, flags.EnableGuide, false)
	assert.Equal(t, flags.Visibility, "private")
	assert.Equal(t, flags.User, "test")
	assert.Equal(t, flags.CookieName, "flare_test")
	assert.Equal(t, flags.CookieSecret, "test_cookie_secret")
}
func TestInitAccountFromEnvVars_normal(t *testing.T) {
	resetCmdEnv(t)
	defaultEnvs := define.DefaultEnvVars

	err := env.Parse(&defaultEnvs)
	assert.Nil(t, err, "TestInitAccountFromEnvVars Faild")
	var target model.Flags

	// 3. update username and password
	cmd.InitAccountFromEnvVars(
		"custom",
		defaultEnvs.Pass,
		&target.User,
		&target.Pass,
		define.DEFAULT_USER_NAME,
		&target.UserIsGenerated,
		&target.PassIsGenerated,
		&target.DisableLoginMode,
	)
	assert.Equal(t, target.User, "custom")
	assert.Equal(t, target.UserIsGenerated, false)
	assert.Equal(t, target.PassIsGenerated, false)
	assert.Equal(t, target.Pass, define.DEFAULT_LOGIN_PASS)
}

func TestInitAccountFromEnvVars_EmptyUser(t *testing.T) {
	resetCmdEnv(t)
	defaultEnvs := define.DefaultEnvVars

	err := env.Parse(&defaultEnvs)
	assert.Nil(t, err, "TestInitAccountFromEnvVars Faild")
	var target model.Flags

	// 4. test empty username and password
	cmd.InitAccountFromEnvVars(
		"",
		defaultEnvs.Pass,
		&target.User,
		&target.Pass,
		define.DEFAULT_USER_NAME,
		&target.UserIsGenerated,
		&target.PassIsGenerated,
		&target.DisableLoginMode,
	)
	assert.Equal(t, target.User, define.DEFAULT_USER_NAME)
	assert.Equal(t, target.UserIsGenerated, true)
	assert.Equal(t, target.PassIsGenerated, false)
	assert.Equal(t, target.Pass, define.DEFAULT_LOGIN_PASS)
}

func TestInitAccountFromEnvVars_EmptyPass(t *testing.T) {
	resetCmdEnv(t)
	defaultEnvs := define.DefaultEnvVars

	err := env.Parse(&defaultEnvs)
	assert.Nil(t, err, "TestInitAccountFromEnvVars Faild")

	var target model.Flags

	// 4. test empty password
	cmd.InitAccountFromEnvVars(
		"custom",
		"",
		&target.User,
		&target.Pass,
		define.DEFAULT_USER_NAME,
		&target.UserIsGenerated,
		&target.PassIsGenerated,
		&target.DisableLoginMode,
	)
	assert.Equal(t, target.User, "custom")
	assert.Equal(t, len(target.Pass), 8)
	assert.Equal(t, target.PassIsGenerated, true)
}

func TestInitAccountFromEnvVars_Pass(t *testing.T) {
	resetCmdEnv(t)
	defaultEnvs := define.DefaultEnvVars

	err := env.Parse(&defaultEnvs)
	assert.Nil(t, err, "TestInitAccountFromEnvVars Faild")
	var target model.Flags

	// 4. test empty password
	cmd.InitAccountFromEnvVars(
		"custom",
		"custom",
		&target.User,
		&target.Pass,
		define.DEFAULT_USER_NAME,
		&target.UserIsGenerated,
		&target.PassIsGenerated,
		&target.DisableLoginMode,
	)
	assert.Equal(t, target.User, "custom")
	assert.Equal(t, target.Pass, "custom")
	assert.Equal(t, target.PassIsGenerated, false)
	assert.Equal(t, target.UserIsGenerated, false)
}
