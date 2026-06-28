package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/logger"
	"gopkg.in/ini.v1"
)

var resolveDotEnvPath = func() (string, error) {
	return fn.GetWorkDirFileE(".env")
}

func CheckDotEnvFileExist(filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log := logger.GetLogger()
		log.Debug("default .env file does not exist, skip parsing")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat .env failed: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("stat .env failed: %s is a directory", filePath)
	}
	return true, nil
}

func GetDotEnvFileStringOrDefault(envs *ini.File, key string, def string) (string, error) {
	value := strings.TrimSpace(envs.Section("").Key(key).String())
	if value == "" {
		log := logger.GetLogger()
		log.Debug(fmt.Sprintf("%s is empty in .env, keep default value", key))
		return def, nil
	}
	return value, nil
}

func GetDotEnvFileBoolOrDefault(envs *ini.File, key string, def bool) (bool, error) {
	raw := strings.TrimSpace(envs.Section("").Key(key).String())
	if raw == "" {
		log := logger.GetLogger()
		log.Debug(fmt.Sprintf("%s is empty in .env, keep default value", key))
		return def, nil
	}
	value, err := envs.Section("").Key(key).Bool()
	if err != nil {
		return def, fmt.Errorf("parse %s as bool failed: %w", key, err)
	}
	return value, nil
}

func ParseEnvFile(baseFlags model.Flags) model.Flags {
	resolved, err := ParseEnvFileE(baseFlags)
	if err != nil {
		panic(err)
	}
	return resolved
}

func ParseEnvFileE(baseFlags model.Flags) (model.Flags, error) {
	log := logger.GetLogger()

	envPath, err := resolveDotEnvPath()
	if err != nil {
		log.Error("resolve .env path failed", "error", err.Error())
		return baseFlags, fmt.Errorf("resolve .env path failed: %w", err)
	}

	exists, err := CheckDotEnvFileExist(envPath)
	if err != nil {
		return baseFlags, err
	}
	if !exists {
		return baseFlags, nil
	}

	envs, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, envPath)
	if err != nil {
		return baseFlags, fmt.Errorf("parse .env file failed: %w", err)
	}

	if err := envs.MapTo(&model.Envs{}); err != nil {
		return baseFlags, fmt.Errorf("map .env values failed: %w", err)
	}
	rawUser := strings.TrimSpace(envs.Section("").Key("FLARE_USER").String())
	rawPass := strings.TrimSpace(envs.Section("").Key("FLARE_PASS").String())
	if err := data.ValidateLoginCredentialPair(rawUser, rawPass, ".env"); err != nil {
		return baseFlags, fmt.Errorf("parse .env login config failed: %w", err)
	}

	rawPort := strings.TrimSpace(envs.Section("").Key("FLARE_PORT").String())
	if rawPort != "" {
		port, err := envs.Section("").Key("FLARE_PORT").Int()
		if err != nil {
			return baseFlags, fmt.Errorf("parse FLARE_PORT as int failed: %w", err)
		}
		if port < 1 || port > 65535 {
			return baseFlags, fmt.Errorf("FLARE_PORT must be between 1 and 65535: %d", port)
		}
		baseFlags.Port = port
	}

	user, err := GetDotEnvFileStringOrDefault(envs, "FLARE_USER", baseFlags.User)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.User = user
	if rawUser != "" {
		baseFlags.UserIsGenerated = false
	}

	pass, err := GetDotEnvFileStringOrDefault(envs, "FLARE_PASS", baseFlags.Pass)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.Pass = pass
	if rawPass != "" {
		baseFlags.PassIsGenerated = false
	}

	baseFlags.DisableLoginMode, err = GetDotEnvFileBoolOrDefault(envs, "FLARE_DISABLE_LOGIN", baseFlags.DisableLoginMode)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.DisableCSP, err = GetDotEnvFileBoolOrDefault(envs, "FLARE_DISABLE_CSP", baseFlags.DisableCSP)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.EnableMinimumRequest, err = GetDotEnvFileBoolOrDefault(envs, "FLARE_MINI_REQUEST", baseFlags.EnableMinimumRequest)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.EnableEditor, err = GetDotEnvFileBoolOrDefault(envs, "FLARE_EDITOR", baseFlags.EnableEditor)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.EnableGuide, err = GetDotEnvFileBoolOrDefault(envs, "FLARE_GUIDE", baseFlags.EnableGuide)
	if err != nil {
		return baseFlags, err
	}

	baseFlags.Visibility, err = GetDotEnvFileStringOrDefault(envs, "FLARE_VISIBILITY", baseFlags.Visibility)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.CookieName, err = GetDotEnvFileStringOrDefault(envs, "FLARE_COOKIE_NAME", baseFlags.CookieName)
	if err != nil {
		return baseFlags, err
	}
	baseFlags.CookieSecret, err = GetDotEnvFileStringOrDefault(envs, "FLARE_COOKIE_SECRET", baseFlags.CookieSecret)
	if err != nil {
		return baseFlags, err
	}

	return baseFlags, nil
}
