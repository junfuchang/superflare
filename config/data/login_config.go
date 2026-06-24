package data

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readEnvValueFromContent(content []byte, key string) string {
	prefix := key + "="
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		return strings.TrimPrefix(line, prefix)
	}
	return ""
}

func readEnvValueFromFile(filePath string, key string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	return readEnvValueFromContent(data, key), nil
}

func upsertEnvValueInContent(content []byte, key string, value string) []byte {
	prefix := key + "="
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	updated := false
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = prefix + value
			updated = true
		}
	}
	if !updated {
		lines = append(lines, prefix+value)
	}
	return []byte(strings.Join(lines, "\n"))
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
	return os.WriteFile(filePath, next, os.ModePerm)
}

func upsertEnvValueIfFileExists(filePath string, key string, value string) error {
	if !checkExists(filePath) {
		return nil
	}
	return upsertEnvValue(filePath, key, value)
}

func getEnvFilePath() string {
	rootDir, err := os.Getwd()
	if err != nil {
		return ".env"
	}
	return filepath.Join(rootDir, ".env")
}

func GetLoginConfig() (user string, pass string, err error) {
	options, cfgErr := GetAllSettingsOptions()
	if cfgErr == nil {
		user = strings.TrimSpace(options.LoginUser)
		pass = strings.TrimSpace(options.LoginPass)
	}
	if user != "" && pass != "" {
		return user, pass, nil
	}

	envPath := getEnvFilePath()
	envUser, userErr := readEnvValueFromFile(envPath, "FLARE_USER")
	if user == "" && userErr == nil {
		user = strings.TrimSpace(envUser)
	}
	envPass, passErr := readEnvValueFromFile(envPath, "FLARE_PASS")
	if pass == "" && passErr == nil {
		pass = strings.TrimSpace(envPass)
	}

	if cfgErr != nil && userErr != nil && passErr != nil {
		return user, pass, fmt.Errorf("read login config failed: %w", cfgErr)
	}
	return user, pass, nil
}

func UpdateLoginConfig(user string, pass string) bool {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return false
	}
	options.LoginUser = user
	options.LoginPass = pass
	if !saveAppConfigToYamlFile("config", options) {
		return false
	}

	envPath := getEnvFilePath()
	if err := upsertEnvValueIfFileExists(envPath, "FLARE_USER", user); err != nil {
		return false
	}
	if err := upsertEnvValueIfFileExists(envPath, "FLARE_PASS", pass); err != nil {
		return false
	}
	return true
}
