package cmd_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	flags "github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

var versionDatePattern = regexp.MustCompile(`\d{8}`)

func TestParse(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	origArgs := os.Args
	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	defer func() {
		os.Args = origArgs
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	}()
	os.Args = []string{"superflare"}

	actualFlags := cmd.Parse()

	expectedFlags := model.Flags{
		Port:                 define.DEFAULT_PORT,
		EnableGuide:          define.DEFAULT_ENABLE_GUIDE,
		EnableEditor:         define.DEFAULT_ENABLE_EDITOR,
		EnableMinimumRequest: define.DEFAULT_ENABLE_MINI_REQUEST,
		DisableCSP:           define.DEFAULT_DISABLE_CSP,
		Visibility:           define.DEFAULT_VISIBILITY,
		DisableLoginMode:     define.DEFAULT_DISABLE_LOGIN,
		User:                 define.DEFAULT_LOGIN_USER,
		Pass:                 define.DEFAULT_LOGIN_PASS,
		UserIsGenerated:      false,
		PassIsGenerated:      false,
		CookieName:           define.DEFAULT_COOKIE_NAME,
		CookieSecret:         define.DEFAULT_COOKIE_SECRET,
	}
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

func TestParseLogsWarningWhenAccountConfigReadFails(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	_, err := cmd.ParseE()
	if err == nil {
		t.Fatal("expected ParseE to fail")
	}
	if !strings.Contains(err.Error(), "read account config failed") {
		t.Fatalf("expected account config error, got %v", err)
	}
}

func TestParseKeepsBaseFlagsInSyncWhenConfigOverridesLoginAccount(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: config-user\nLoginPass: config-pass\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	defer func() {
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	}()

	flags := cmd.Parse()
	if flags.User != "config-user" || flags.Pass != "config-pass" {
		t.Fatalf("expected parsed flags to use config login account, got user=%q pass=%q", flags.User, flags.Pass)
	}
	if define.AppBaseFlags.User != "config-user" || define.AppBaseFlags.Pass != "config-pass" {
		t.Fatalf("expected base flags to stay in sync, got user=%q pass=%q", define.AppBaseFlags.User, define.AppBaseFlags.Pass)
	}
	if define.AppFlags.User != "config-user" || define.AppFlags.Pass != "config-pass" {
		t.Fatalf("expected app flags to stay in sync, got user=%q pass=%q", define.AppFlags.User, define.AppFlags.Pass)
	}
}

func TestParsePreservesSourceFlagsBeforeConfigOverridesLoginAccount(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=env-pass\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: config-user\nLoginPass: config-pass\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	defer func() {
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	}()

	flags := cmd.Parse()
	if flags.User != "config-user" || flags.Pass != "config-pass" {
		t.Fatalf("expected parsed flags to use config login account, got user=%q pass=%q", flags.User, flags.Pass)
	}
	if define.AppSourceFlags.User != "env-user" || define.AppSourceFlags.Pass != "env-pass" {
		t.Fatalf("expected source flags to preserve pre-config account, got user=%q pass=%q", define.AppSourceFlags.User, define.AppSourceFlags.Pass)
	}
}

func TestParseEFailsWhenDotEnvContainsInvalidConfiguredValues(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_PORT=9999999\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	_, err := cmd.ParseE()
	if err == nil {
		t.Fatal("expected ParseE to fail")
	}
	if !strings.Contains(err.Error(), "FLARE_PORT must be between 1 and 65535") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEFailsWhenDotEnvLoginCredentialsIncomplete(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	_, err := cmd.ParseE()
	if err == nil {
		t.Fatal("expected ParseE to fail")
	}
	if !strings.Contains(err.Error(), "parse .env login config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "login credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEFailsWhenConfigLoginCredentialsIncomplete(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: config-user\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	_, err := cmd.ParseE()
	if err == nil {
		t.Fatal("expected ParseE to fail")
	}
	if !strings.Contains(err.Error(), "read account config failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "login credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEHelpBypassesDotEnvResolutionError(t *testing.T) {
	if os.Getenv("SUPERFLARE_PARSE_HELP_HELPER") == "1" {
		originalResolve := cmd.ResolveDotEnvPathForTest()
		defer cmd.SetResolveDotEnvPathForTest(originalResolve)
		cmd.SetResolveDotEnvPathForTest(func() (string, error) {
			return "", os.ErrPermission
		})

		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()
		os.Args = []string{"superflare", "--help"}

		_, _ = cmd.ParseE()
		t.Fatal("ParseE returned without exiting for --help")
	}

	proc := exec.Command(os.Args[0], "-test.run=TestParseEHelpBypassesDotEnvResolutionError$")
	proc.Env = append(os.Environ(), "SUPERFLARE_PARSE_HELP_HELPER=1")
	output, err := proc.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "支持命令") {
		t.Fatalf("expected help output, got %s", text)
	}
	if strings.Contains(text, "resolve .env path failed") {
		t.Fatalf("help path should not attempt .env resolution, got %s", text)
	}
}
