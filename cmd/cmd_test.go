package cmd_test

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	flags "github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var versionDatePattern = regexp.MustCompile(`\d{8}`)

// Mock dependencies
type EnvParserMock struct {
	mock.Mock
}

func (m *EnvParserMock) ParseEnvVars() map[string]string {
	args := m.Called()
	return args.Get(0).(map[string]string)
}

func (m *EnvParserMock) ParseEnvFile(envVars map[string]string) map[string]string {
	args := m.Called(envVars)
	return args.Get(0).(map[string]string)
}

type CLIParserMock struct {
	mock.Mock
}

// parseCLI is used by testify/mock when On("parseCLI", ...) is set.
//
//nolint:unused
func (m *CLIParserMock) parseCLI(envs map[string]string) model.Flags {
	args := m.Called(envs)
	return args.Get(0).(model.Flags)
}

func TestParse(t *testing.T) {
	envParser := new(EnvParserMock)
	cliParser := new(CLIParserMock)

	envVars := map[string]string{}
	parsedEnvs := map[string]string{}
	expectedFlags := model.Flags{}

	defaults := define.DefaultEnvVars
	expectedFlags.User = defaults.User
	expectedFlags.Port = defaults.Port
	expectedFlags.EnableGuide = defaults.EnableGuide
	expectedFlags.EnableEditor = defaults.EnableEditor
	expectedFlags.Visibility = defaults.Visibility
	expectedFlags.EnableMinimumRequest = defaults.EnableMinimumRequest
	expectedFlags.DisableLoginMode = defaults.DisableLoginMode
	expectedFlags.CookieName = defaults.CookieName
	expectedFlags.CookieSecret = defaults.CookieSecret

	envParser.On("ParseEnvVars").Return(envVars)
	envParser.On("ParseEnvFile", envVars).Return(parsedEnvs)
	cliParser.On("parseCLI", parsedEnvs).Return(expectedFlags)

	actualFlags := cmd.Parse()

	actualFlags.Pass = ""
	actualFlags.PassIsGenerated = false

	assert.Equal(t, expectedFlags, actualFlags)
}

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()
	f()
	w.Close()
	os.Stdout = old
	return <-outC
}

func TestExecuteCLI_ShowHelp(t *testing.T) {
	cliFlags := &model.Flags{ShowHelp: true}
	options := &flags.FlagSet{}

	output := captureOutput(func() {
		_ = cmd.ExecuteCLI(cliFlags, options)
	})

	assert.Contains(t, output, "支持命令")
	assert.True(t, cmd.ExecuteCLI(cliFlags, options))
}

func TestExecuteCLI_ShowVersion(t *testing.T) {
	cliFlags := &model.Flags{ShowVersion: true}
	options := &flags.FlagSet{}

	output := captureOutput(func() {
		_ = cmd.ExecuteCLI(cliFlags, options)
	})

	assert.Regexp(t, versionDatePattern, output)
	assert.True(t, cmd.ExecuteCLI(cliFlags, options))
}

func TestExecuteCLI_NoFlags(t *testing.T) {
	cliFlags := &model.Flags{}
	options := &flags.FlagSet{}
	assert.False(t, cmd.ExecuteCLI(cliFlags, options))
}

func TestGetVersionEcho(t *testing.T) {
	ver := cmd.GetVersion(true)
	assert.Regexp(t, versionDatePattern, ver)
}

func TestGetVersionMute(t *testing.T) {
	ver := ""
	output := captureOutput(func() {
		ver = cmd.GetVersion(false)
	})
	assert.Regexp(t, versionDatePattern, ver)
	assert.NotContains(t, output, "Challenge all bookmarking apps and websites directories")
}
