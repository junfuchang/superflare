package data

import "fmt"

func EnsureRuntimeDataFiles() error {
	if err := ensureBookmarksConfigExists("apps", true); err != nil {
		return err
	}
	if err := ensureBookmarksConfigExists("bookmarks", false); err != nil {
		return err
	}
	if err := ensurePortsConfigExists(); err != nil {
		return err
	}
	return nil
}

func EnsureAppConfigExists() error {
	filePath, err := configPath("config")
	if err != nil {
		return err
	}
	exists, err := pathExists(filePath)
	if err != nil {
		return fmt.Errorf("stat config config failed: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := initAppConfig(filePath); err != nil {
		return fmt.Errorf("create app config config failed: %w", err)
	}
	return nil
}

func ensureBookmarksConfigExists(name string, isFavorite bool) error {
	filePath, err := configPath(name)
	if err != nil {
		return err
	}
	exists, err := pathExists(filePath)
	if err != nil {
		return fmt.Errorf("stat bookmarks config %s failed: %w", name, err)
	}
	if exists {
		return nil
	}
	if _, err := initBookmarks(filePath, isFavorite); err != nil {
		return fmt.Errorf("create bookmarks config %s failed: %w", name, err)
	}
	return nil
}

func ensurePortsConfigExists() error {
	filePath, err := portsConfigPath()
	if err != nil {
		return err
	}
	exists, err := pathExists(filePath)
	if err != nil {
		return fmt.Errorf("stat ports config failed: %w", err)
	}
	if exists {
		return nil
	}
	_, out, err := loadDefaultPortsConfig()
	if err != nil {
		return err
	}
	if err := saveFile(filePath, out); err != nil {
		return fmt.Errorf("init ports config failed: %w", err)
	}
	return nil
}
