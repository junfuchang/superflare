package data

import (
	"sync"

	"github.com/junfuchang/superflare/config/model"
)

var (
	configWriteMu sync.Mutex

	configUpdateBeforeLoadHook func(name string)
	configUpdateBeforeSaveHook func(name string, next model.Application)
)

func withConfigWriteLock(fn func() error) error {
	configWriteMu.Lock()
	defer configWriteMu.Unlock()
	release, err := lockConfigFiles()
	if err != nil {
		return err
	}
	defer func() {
		_ = release()
	}()
	return fn()
}

func WithConfigWriteLock(fn func() error) error {
	return withConfigWriteLock(fn)
}

func withLockedConfigUpdate(name string, fn func() (model.Application, error)) (model.Application, error) {
	var next model.Application
	err := withConfigWriteLock(func() error {
		if configUpdateBeforeLoadHook != nil {
			configUpdateBeforeLoadHook(name)
		}
		var err error
		next, err = fn()
		return err
	})
	return next, err
}

func lockedSaveAppConfig(name string, next model.Application) error {
	if configUpdateBeforeSaveHook != nil {
		configUpdateBeforeSaveHook(name, next)
	}
	return saveAppConfigToYamlFileLocked(name, next)
}

func updateLockedAppConfig(name string, fn func(options model.Application) (model.Application, error)) error {
	_, err := withLockedConfigUpdate(name, func() (model.Application, error) {
		options, err := GetAllSettingsOptions()
		if err != nil {
			return model.Application{}, err
		}
		next, err := fn(options)
		if err != nil {
			return model.Application{}, err
		}
		if err := lockedSaveAppConfig(name, next); err != nil {
			return model.Application{}, err
		}
		return next, nil
	})
	return err
}
