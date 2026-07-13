package data

import (
	"fmt"
	"log"

	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

func initAppConfig(filePath string) (result model.Application, err error) {
	result, out, err := loadDefaultAppConfig()
	if err != nil {
		return result, err
	}

	if err := saveFile(filePath, out); err != nil {
		log.Println("init default app config failed")
		return result, fmt.Errorf("init default app config failed: %w", err)
	}

	return result, nil
}

func saveAppConfigToYamlFile(name string, result model.Application) error {
	return saveAppConfigToYamlFileWith(name, result, saveFile)
}

func saveAppConfigToYamlFileLocked(name string, result model.Application) error {
	return saveAppConfigToYamlFileWith(name, result, saveFileLocked)
}

func saveAppConfigToYamlFileWith(name string, result model.Application, save func(string, []byte) error) error {
	out, err := yaml.Marshal(result)
	if err != nil {
		log.Println("marshal app config failed")
		return fmt.Errorf("marshal app config failed: %w", err)
	}

	filePath, err := configPath(name)
	if err != nil {
		return err
	}
	if err := save(filePath, out); err != nil {
		log.Println("save app config failed")
		return fmt.Errorf("save app config failed: %w", err)
	}
	invalidateFileCachePath(filePath)
	return nil
}

func loadAppConfigFromYaml(name string) (model.Application, error) {
	var result model.Application
	filePath, err := configPath(name)
	if err != nil {
		return result, err
	}

	exists, err := pathExists(filePath)
	if err != nil {
		return result, fmt.Errorf("stat config %s failed: %w", name, err)
	}
	if !exists {
		return result, fmt.Errorf("config %s is missing", name)
	}
	configFile, err := readFileCached(filePath, func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, fmt.Errorf("read config %s failed: %w", name, err)
	}
	parseErr := yaml.Unmarshal(configFile, &result)
	if parseErr != nil {
		return result, fmt.Errorf("parse config %s failed: %w", name, parseErr)
	}
	return result, nil
}

func LoadAppConfigFromRaw(raw []byte) (model.Application, error) {
	var result model.Application
	if err := yaml.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("parse config raw failed: %w", err)
	}
	if err := validateSettingsOptions(result); err != nil {
		return result, err
	}
	return result, nil
}
