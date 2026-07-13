package cmd_test

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/logger"
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
	options := flags.NewFlagSet("test", flags.ContinueOnError)
	options.Int("port", define.DEFAULT_PORT, "listen port")

	exit := false
	output := captureOutput(func() {
		exit = cmd.ExecuteCLI(cliFlags, options)
	})

	assert.Contains(t, output, "SuperFlare v")
	assert.True(t, exit)
}

func TestExecuteCLI_ShowVersion(t *testing.T) {
	cliFlags := &model.Flags{ShowVersion: true}
	options := flags.NewFlagSet("test", flags.ContinueOnError)

	exit := false
	output := captureOutput(func() {
		exit = cmd.ExecuteCLI(cliFlags, options)
	})

	assert.Regexp(t, versionDatePattern, output)
	assert.True(t, exit)
}

func TestExecuteCLI_NoFlags(t *testing.T) {
	cliFlags := &model.Flags{}
	options := flags.NewFlagSet("test", flags.ContinueOnError)
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

func TestParseERepairsDotEnvLoginCredentialsWhenIncomplete(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FLARE_USER=env-user\nFLARE_PASS=\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	resolved, err := cmd.ParseE()
	if err != nil {
		t.Fatalf("ParseE: %v", err)
	}
	if resolved.User != define.DEFAULT_LOGIN_USER || resolved.Pass != define.DEFAULT_LOGIN_PASS {
		t.Fatalf("expected repaired default credentials, got %q/%q", resolved.User, resolved.Pass)
	}
}

func TestParseERepairsConfigLoginCredentialsWhenIncomplete(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: config-user\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"superflare"}

	resolved, err := cmd.ParseE()
	if err != nil {
		t.Fatalf("ParseE: %v", err)
	}
	if resolved.User != define.DEFAULT_LOGIN_USER || resolved.Pass != define.DEFAULT_LOGIN_PASS {
		t.Fatalf("expected repaired default credentials, got %q/%q", resolved.User, resolved.Pass)
	}
}

func TestParseEEmptyConfigCredentialsUseDefaultPair(t *testing.T) {
	resetCmdEnv(t)
	tmpDir := withTempCmdWorkDir(t)

	origArgs := os.Args
	origLogger := logger.GetLogger()
	origDefaults := define.DefaultEnvVars
	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	defer func() {
		os.Args = origArgs
		logger.SetLogger(origLogger)
		define.DefaultEnvVars = origDefaults
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	}()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	define.DefaultEnvVars.Pass = ""
	os.Args = []string{"superflare"}

	var out bytes.Buffer
	logger.SetLogger(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{})))

	resolved, err := cmd.ParseE()
	if err != nil {
		t.Fatalf("ParseE: %v", err)
	}
	if resolved.User != define.DEFAULT_LOGIN_USER || resolved.Pass != define.DEFAULT_LOGIN_PASS {
		t.Fatalf("expected default credentials after repair, got %q/%q", resolved.User, resolved.Pass)
	}
	if resolved.UserIsGenerated || resolved.PassIsGenerated {
		t.Fatalf("expected repaired credentials to be explicit defaults, got generated user=%v pass=%v", resolved.UserIsGenerated, resolved.PassIsGenerated)
	}
	if !strings.Contains(out.String(), "Default admin/admin login credentials are still active") {
		t.Fatalf("expected default credential warning, got %s", out.String())
	}
}

func TestParseEWarnsWhenDefaultAdminCredentialsRemainActive(t *testing.T) {
	resetCmdEnv(t)
	withTempCmdWorkDir(t)
	origArgs := os.Args
	origLogger := logger.GetLogger()
	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	defer func() {
		os.Args = origArgs
		logger.SetLogger(origLogger)
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	}()

	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: admin\nLoginPass: admin\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	os.Args = []string{"superflare"}

	var out bytes.Buffer
	logger.SetLogger(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{})))

	resolved, err := cmd.ParseE()
	if err != nil {
		t.Fatalf("ParseE: %v", err)
	}
	if resolved.User != "admin" || resolved.Pass != "admin" {
		t.Fatalf("expected admin/admin defaults to remain active, got %q/%q", resolved.User, resolved.Pass)
	}
	if !strings.Contains(out.String(), "Default admin/admin login credentials are still active") {
		t.Fatalf("expected default credential warning, got %s", out.String())
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
	if !strings.Contains(text, "SuperFlare v") {
		t.Fatalf("expected help output, got %s", text)
	}
	if strings.Contains(text, "resolve .env path failed") {
		t.Fatalf("help path should not attempt .env resolution, got %s", text)
	}
}
