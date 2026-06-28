package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/soulteary/cli-kit/configutil"
	version "github.com/soulteary/version-kit"
	flags "github.com/spf13/pflag"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
)

var flagsMapTrimValue = regexp.MustCompile(`=.*`)

func GetCliFlags() (*model.Flags, *flags.FlagSet, error) {
	cliFlags := new(model.Flags)
	options := flags.NewFlagSet("appFlags", flags.ContinueOnError)
	options.SortFlags = false

	options.IntVarP(&cliFlags.Port, _KEY_PORT, _KEY_PORT_SHORT, define.DEFAULT_PORT, "listen port")
	options.BoolVarP(&cliFlags.EnableGuide, _KEY_ENABLE_GUIDE, _KEY_ENABLE_GUIDE_SHORT, define.DEFAULT_ENABLE_GUIDE, "enable guide page")
	options.StringVarP(&cliFlags.Visibility, _KEY_VISIBILITY, _KEY_VISIBILITY_SHORT, define.DEFAULT_VISIBILITY, "site visibility")

	options.BoolVarP(&cliFlags.EnableMinimumRequest, _KEY_MINI_REQUEST, _KEY_MINI_REQUEST_SHORT, define.DEFAULT_ENABLE_MINI_REQUEST, "minimize asset requests")
	options.BoolVar(&cliFlags.EnableMinimumRequest, _KEY_MINI_REQUEST_OLD, define.DEFAULT_ENABLE_MINI_REQUEST, "minimize asset requests")
	_ = options.MarkDeprecated(_KEY_MINI_REQUEST_OLD, "please use --"+_KEY_MINI_REQUEST+" instead")

	options.BoolVarP(&cliFlags.DisableLoginMode, _KEY_DISABLE_LOGIN, _KEY_DISABLE_LOGIN_SHORT, define.DEFAULT_DISABLE_LOGIN, "disable login")
	options.BoolVar(&cliFlags.DisableLoginMode, _KEY_DISABLE_LOGIN_OLD, define.DEFAULT_DISABLE_LOGIN, "disable login")
	_ = options.MarkDeprecated(_KEY_DISABLE_LOGIN_OLD, "please use --"+_KEY_DISABLE_LOGIN+" instead")

	options.BoolVarP(&cliFlags.EnableEditor, _KEY_ENABLE_EDITOR, _KEY_ENABLE_EDITOR_SHORT, define.DEFAULT_ENABLE_EDITOR, "enable editor")
	options.BoolVarP(&cliFlags.DisableCSP, _KEY_DISABLE_CSP, _KEY_DISABLE_CSP_SHORT, define.DEFAULT_DISABLE_CSP, "disable CSP")
	options.BoolVarP(&cliFlags.DebugMode, _KEY_DEBUG, _KEY_DEBUG_SHORT, false, "enable debug mode")

	options.BoolVarP(&cliFlags.ShowVersion, "version", "v", false, "show version")
	options.BoolVarP(&cliFlags.ShowHelp, "help", "h", false, "show help")

	options.StringVarP(&cliFlags.CookieName, _KEY_COOKIE_NAME, _KEY_COOKIE_NAME_SHORT, define.DEFAULT_COOKIE_NAME, "cookie name")
	options.StringVarP(&cliFlags.CookieSecret, _KEY_COOKIE_SECRET, _KEY_COOKIE_SECRET_SHORT, define.DEFAULT_COOKIE_SECRET, "cookie secret")

	if err := options.Parse(os.Args[1:]); err != nil {
		return cliFlags, options, fmt.Errorf("parse cli flags failed: %w", err)
	}
	return cliFlags, options, nil
}

// GetFlagsMaps returns a set of flag names that appear in os.Args.
func GetFlagsMaps() map[string]bool {
	keys := make(map[string]bool)
	if len(os.Args) <= 1 {
		return keys
	}
	for _, key := range os.Args[1:] {
		var name string
		if len(key) >= 2 && key[:2] == "--" {
			name = flagsMapTrimValue.ReplaceAllString(key[2:], "")
		} else if len(key) >= 1 && key[:1] == "-" {
			name = flagsMapTrimValue.ReplaceAllString(key[1:], "")
		}
		if name != "" {
			keys[name] = true
		}
	}
	return keys
}

// CheckFlagsExists returns true if any of keys is present in dict.
func CheckFlagsExists(dict map[string]bool, keys []string) bool {
	if dict == nil {
		return false
	}
	for _, key := range keys {
		if dict[key] {
			return true
		}
	}
	return false
}

func resolveCLIFlags(baseFlags model.Flags, cliFlags *model.Flags, fs *flags.FlagSet) (model.Flags, error) {
	if ExecuteCLI(cliFlags, fs) {
		os.Exit(0)
	}
	GetVersion(true)

	port, err := configutil.ResolvePortPflag(fs, _KEY_PORT, "", baseFlags.Port)
	if err != nil {
		return baseFlags, fmt.Errorf("resolve --%s failed: %w", _KEY_PORT, err)
	}
	baseFlags.Port = port

	baseFlags.EnableMinimumRequest = configutil.ResolveBoolPflag(fs, _KEY_MINI_REQUEST, "", baseFlags.EnableMinimumRequest)
	baseFlags.DisableLoginMode = configutil.ResolveBoolPflag(fs, _KEY_DISABLE_LOGIN, "", baseFlags.DisableLoginMode)
	baseFlags.DisableCSP = configutil.ResolveBoolPflag(fs, _KEY_DISABLE_CSP, "", baseFlags.DisableCSP)

	visibility, err := configutil.ResolveEnumPflag(fs, _KEY_VISIBILITY, "", baseFlags.Visibility, []string{"DEFAULT", "PRIVATE"}, false)
	if err != nil {
		return baseFlags, fmt.Errorf("resolve --%s failed: %w", _KEY_VISIBILITY, err)
	}
	baseFlags.Visibility = strings.ToUpper(visibility)

	baseFlags.EnableGuide = configutil.ResolveBoolPflag(fs, _KEY_ENABLE_GUIDE, "", baseFlags.EnableGuide)
	baseFlags.EnableEditor = configutil.ResolveBoolPflag(fs, _KEY_ENABLE_EDITOR, "", baseFlags.EnableEditor)

	baseFlags.CookieName = configutil.ResolveStringPflag(fs, _KEY_COOKIE_NAME, "", baseFlags.CookieName, true)
	baseFlags.CookieSecret = configutil.ResolveStringPflag(fs, _KEY_COOKIE_SECRET, "", baseFlags.CookieSecret, true)

	if !version.Default().IsDev() {
		baseFlags.DebugMode = false
	} else {
		baseFlags.DebugMode = configutil.ResolveBoolPflag(fs, _KEY_DEBUG, "", baseFlags.DebugMode)
	}

	return baseFlags, nil
}

func parseCLI(baseFlags model.Flags) (model.Flags, error) {
	cliFlags, fs, err := GetCliFlags()
	if err != nil {
		return baseFlags, err
	}
	return resolveCLIFlags(baseFlags, cliFlags, fs)
}
