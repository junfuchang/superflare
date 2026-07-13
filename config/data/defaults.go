package data

import (
	"fmt"

	configdefaults "github.com/junfuchang/superflare/config/defaults"
	"github.com/junfuchang/superflare/config/model"
)

func readDefaultFile(name string) ([]byte, error) {
	raw, err := configdefaults.Files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read default file %s failed: %w", name, err)
	}
	return raw, nil
}

func loadDefaultAppConfig() (model.Application, []byte, error) {
	raw, err := readDefaultFile("config.yml")
	if err != nil {
		return model.Application{}, nil, err
	}
	result, err := LoadAppConfigFromRaw(raw)
	if err != nil {
		return model.Application{}, nil, fmt.Errorf("validate default app config failed: %w", err)
	}
	return result, raw, nil
}

func loadDefaultBookmarksConfig(name string, isFavorite bool) (model.Bookmarks, []byte, error) {
	raw, err := readDefaultFile(name)
	if err != nil {
		return model.Bookmarks{}, nil, err
	}
	result, err := loadBookmarksFromRaw(raw, isFavorite)
	if err != nil {
		return model.Bookmarks{}, nil, fmt.Errorf("validate default bookmarks failed: %w", err)
	}
	return result, raw, nil
}

func loadDefaultPortsConfig() (model.Ports, []byte, error) {
	raw, err := readDefaultFile("ports.yaml")
	if err != nil {
		return model.Ports{}, nil, err
	}
	result, err := LoadPortBindingsFromRaw(raw)
	if err != nil {
		return model.Ports{}, nil, fmt.Errorf("validate default ports failed: %w", err)
	}
	return result, raw, nil
}
