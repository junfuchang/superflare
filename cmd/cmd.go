package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	flags "github.com/spf13/pflag"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/appver"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/logger"
	version "github.com/soulteary/version-kit"
)

func Parse() model.Flags {
	resolved, err := ParseE()
	if err != nil {
		panic(err)
	}
	return resolved
}

func ParseE() (model.Flags, error) {
	cliFlags, fs, err := GetCliFlags()
	if err != nil {
		return model.Flags{}, err
	}
	if ExecuteCLI(cliFlags, fs) {
		os.Exit(0)
	}

	envs, err := ParseEnvVarsE()
	if err != nil {
		return model.Flags{}, err
	}
	envs, err = ParseEnvFileE(envs)
	if err != nil {
		return model.Flags{}, err
	}
	resolved, err := resolveCLIFlags(envs, cliFlags, fs)
	if err != nil {
		return model.Flags{}, err
	}
	define.AppSourceFlags = resolved
	resolved, err = applyAccountConfig(resolved)
	if err != nil {
		return model.Flags{}, err
	}
	define.AppBaseFlags = resolved

	log := logger.GetLogger()
	log.Info("程序服务端口", slog.Int(_KEY_PORT, resolved.Port))
	log.Info("页面请求合并", slog.Bool(_KEY_MINI_REQUEST, resolved.EnableMinimumRequest))
	if resolved.DisableLoginMode {
		log.Info("已禁用登录模式，用户可直接调整应用设置。")
	} else {
		log.Info("启用登录模式，调整应用设置需要先进行登录。")
		log.Info("当前内容整体可见性为", slog.String(_KEY_VISIBILITY, resolved.Visibility))

		if resolved.UserIsGenerated {
			log.Info("用户未指定 `FLARE_USER`，使用默认用户名", slog.String("username", define.DEFAULT_USER_NAME))
		} else {
			log.Info("应用用户设置为", slog.String("username", resolved.User))
		}

		if resolved.PassIsGenerated {
			log.Info("用户未指定 `FLARE_PASS`，自动生成应用密码", slog.String("password", resolved.Pass))
		} else {
			log.Info("应用登录密码已设置为", slog.String("password", data.MaskTextWithStars(resolved.Pass)))
		}
	}

	define.AppFlags = resolved
	auth.StoreLoginRuntimeConfigFromFlags(resolved)
	return resolved, nil
}

func ApplyAccountConfigToFlags(flags model.Flags, options model.Application) model.Flags {
	resolved, err := ApplyAccountConfigToFlagsE(flags, options)
	if err != nil {
		panic(err)
	}
	return resolved
}

func ApplyAccountConfigToFlagsE(flags model.Flags, options model.Application) (model.Flags, error) {
	if err := data.ValidateLoginCredentialPair(options.LoginUser, options.LoginPass, "config.yml"); err != nil {
		return flags, err
	}
	if user := strings.TrimSpace(options.LoginUser); user != "" {
		flags.User = user
		flags.UserIsGenerated = false
	}
	if pass := strings.TrimSpace(options.LoginPass); pass != "" {
		flags.Pass = pass
		flags.PassIsGenerated = false
	}
	return flags, nil
}

func applyAccountConfig(flags model.Flags) (model.Flags, error) {
	if err := data.EnsureAppConfigExists(); err != nil {
		return flags, fmt.Errorf("read account config failed: %w", err)
	}
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return flags, fmt.Errorf("read account config failed: %w", err)
	}
	flags, err = ApplyAccountConfigToFlagsE(flags, options)
	if err != nil {
		return flags, fmt.Errorf("read account config failed: %w", err)
	}
	return flags, nil
}

// ExecuteCLI handles --help and --version; returns true if the program should exit.
func ExecuteCLI(cliFlags *model.Flags, options *flags.FlagSet) (exit bool) {
	programVersion := GetVersion(false)
	if cliFlags.ShowHelp {
		fmt.Println(programVersion)
		fmt.Println()
		fmt.Println("支持命令：")
		options.PrintDefaults()
		return true
	}
	if cliFlags.ShowVersion {
		fmt.Println(appver.DisplayVersion())
		return true
	}
	return false
}

func GetVersion(echo bool) string {
	info := version.Default()
	displayVersion := appver.DisplayVersionFromInfo(info)
	programVersion := appver.ProgramVersionString()
	if echo {
		log := logger.GetLogger()
		log.Info("SuperFlare version info",
			slog.String("version", displayVersion),
			slog.String("commit", strings.ToUpper(info.Commit)),
			slog.String("platform", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)),
			slog.String("date", info.BuildDate),
		)
	}
	return programVersion
}
