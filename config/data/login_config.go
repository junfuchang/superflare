package data

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

var envAssignmentPattern = regexp.MustCompile(`^(\s*)(export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=`)

func normalizeEnvContentForINI(content []byte) []byte {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for index, line := range lines {
		match := envAssignmentPattern.FindStringSubmatchIndex(line)
		if match == nil {
			continue
		}
		exportStart, exportEnd := match[4], match[5]
		keyStart, keyEnd := match[6], match[7]
		if exportStart < 0 || exportEnd < 0 || keyStart < 0 || keyEnd < 0 {
			continue
		}
		remainder := line[match[1]:]
		lines[index] = line[:match[0]] + line[keyStart:keyEnd] + "=" + remainder
	}
	return []byte(strings.Join(lines, "\n"))
}

func loadEnvFileFromContent(content []byte) (*ini.File, error) {
	envs, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, normalizeEnvContentForINI(content))
	if err != nil {
		return nil, fmt.Errorf("parse env content failed: %w", err)
	}
	return envs, nil
}

func readEnvValueFromContent(content []byte, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	envs, err := loadEnvFileFromContent(content)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(envs.Section("").Key(key).String()), nil
}

func readEnvValueFromFile(filePath string, key string) (string, error) {
	info, err := os.Stat(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("read env file failed: %s is a directory", filePath)
	}
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	return readEnvValueFromContent(data, key)
}

func upsertEnvValueInContent(content []byte, key string, value string) []byte {
	key = strings.TrimSpace(key)
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	updated := false
	for index, line := range lines {
		leading, exportPrefix, matchedKey, ok := matchEnvAssignmentLine(line)
		if ok && matchedKey == key {
			lines[index] = leading + exportPrefix + key + "=" + formatEnvValue(value)
			updated = true
		}
	}
	if !updated {
		nextLine := key + "=" + formatEnvValue(value)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], nextLine, "")
		} else {
			lines = append(lines, nextLine)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func matchEnvAssignmentLine(line string) (leading string, exportPrefix string, key string, ok bool) {
	match := envAssignmentPattern.FindStringSubmatch(line)
	if match == nil {
		return "", "", "", false
	}
	return match[1], match[2], strings.TrimSpace(match[3]), true
}

func formatEnvValue(value string) string {
	if value == "" {
		return ""
	}
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, " \t#;\"'") {
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return value
}

func upsertEnvValue(filePath string, key string, value string) error {
	content, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		content = nil
	}
	next := upsertEnvValueInContent(content, key, value)
	return saveFile(filePath, next)
}

func upsertEnvValueIfFileExists(filePath string, key string, value string) error {
	exists, err := pathExists(filePath)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return upsertEnvValue(filePath, key, value)
}

func getEnvFilePath() string {
	rootDir, err := configRootDir()
	if err != nil {
		return ".env"
	}
	return filepath.Join(rootDir, ".env")
}

func envFilePath() (string, error) {
	rootDir, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, ".env"), nil
}

func ValidateLoginCredentialPair(user string, pass string, source string) error {
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	source = strings.TrimSpace(source)
	if source == "" {
		source = "login config"
	}
	if user == "" && pass == "" {
		return nil
	}
	if user == "" || pass == "" {
		return fmt.Errorf("%s login credentials are incomplete: username and password must both be set or both be empty", source)
	}
	return nil
}

func GetLoginConfig() (user string, pass string, err error) {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return "", "", fmt.Errorf("read login config failed: %w", err)
	}
	user = strings.TrimSpace(options.LoginUser)
	pass = strings.TrimSpace(options.LoginPass)
	if err := ValidateLoginCredentialPair(user, pass, "config.yml"); err != nil {
		return user, pass, fmt.Errorf("read login config failed: %w", err)
	}
	if user != "" && pass != "" {
		return user, pass, nil
	}

	envPath, err := envFilePath()
	if err != nil {
		return user, pass, fmt.Errorf("read login config failed: %w", err)
	}
	envExists, err := pathExists(envPath)
	if err != nil {
		return user, pass, fmt.Errorf("read login config failed: %w", err)
	}
	envUser := ""
	envPass := ""
	if user == "" && envExists {
		var userErr error
		envUser, userErr = readEnvValueFromFile(envPath, "FLARE_USER")
		if userErr != nil {
			return user, pass, fmt.Errorf("read login config failed: %w", userErr)
		}
	}
	if pass == "" && envExists {
		var passErr error
		envPass, passErr = readEnvValueFromFile(envPath, "FLARE_PASS")
		if passErr != nil {
			return user, pass, fmt.Errorf("read login config failed: %w", passErr)
		}
	}
	if envExists {
		if err := ValidateLoginCredentialPair(envUser, envPass, ".env"); err != nil {
			return user, pass, fmt.Errorf("read login config failed: %w", err)
		}
	}
	if user == "" {
		user = strings.TrimSpace(envUser)
	}
	if pass == "" {
		pass = strings.TrimSpace(envPass)
	}
	if user != "" && pass != "" {
		return user, pass, nil
	}

	return user, pass, nil
}

func UpdateLoginConfig(user string, pass string) error {
	if err := ValidateLoginCredentialPair(user, pass, "login config update"); err != nil {
		return err
	}
	if err := EnsureAppConfigExists(); err != nil {
		return err
	}
	_, err := withLockedConfigUpdate("config", func() (model.Application, error) {
		options, err := GetAllSettingsOptions()
		if err != nil {
			return model.Application{}, err
		}
		options.LoginUser = user
		options.LoginPass = pass
		configRaw, err := yaml.Marshal(options)
		if err != nil {
			return model.Application{}, fmt.Errorf("marshal login config failed: %w", err)
		}

		configPath, err := configPath("config")
		if err != nil {
			return model.Application{}, err
		}
		files := map[string][]byte{
			configPath: configRaw,
		}
		envPath, err := envFilePath()
		if err != nil {
			return model.Application{}, fmt.Errorf("stat .env failed: %w", err)
		}
		envExists, envErr := pathExists(envPath)
		if envErr != nil {
			return model.Application{}, fmt.Errorf("stat .env failed: %w", envErr)
		}
		if envExists {
			content, err := os.ReadFile(filepath.Clean(envPath))
			if err != nil {
				return model.Application{}, fmt.Errorf("read .env failed: %w", err)
			}
			next := upsertEnvValueInContent(content, "FLARE_USER", user)
			next = upsertEnvValueInContent(next, "FLARE_PASS", pass)
			files[envPath] = next
		}

		if err := saveFilesAtomicallyLocked(files); err != nil {
			return model.Application{}, fmt.Errorf("save login config failed: %w", err)
		}
		invalidateFileCachePath(configPath)
		return options, nil
	})
	return err
}
