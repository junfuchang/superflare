package data

import (
	"fmt"
	"os"
	"path/filepath"
)

const configLockFileName = ".superflare-config.lock"

func configFileLockPath() (string, error) {
	rootDir, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, configLockFileName), nil
}

func lockConfigFiles() (func() error, error) {
	lockPath, err := configFileLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("create config lock directory failed: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open config lock file failed: %w", err)
	}
	if err := lockOpenedConfigFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	released := false
	return func() error {
		if released {
			return nil
		}
		released = true
		unlockErr := unlockOpenedConfigFile(file)
		closeErr := file.Close()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}
