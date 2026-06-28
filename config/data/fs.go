package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const configFileMode = 0644

var osRename = os.Rename
var osStat = os.Stat
var osGetwd = os.Getwd

func checkExists(path string) bool {
	exists, _ := pathExists(path)
	return exists
}

func pathExists(path string) (bool, error) {
	_, err := osStat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func configRootDir() (string, error) {
	rootDir, err := osGetwd()
	if err != nil {
		return "", fmt.Errorf("resolve config working directory failed: %w", err)
	}
	return rootDir, nil
}

func configPath(config string) (string, error) {
	rootDir, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, config+".yml"), nil
}

func getConfigPath(config string) string {
	rootDir, err := configRootDir()
	if err != nil {
		return filepath.Join(".", config+".yml")
	}
	return filepath.Join(rootDir, config+".yml")
}

func GetConfigPath(config string) string {
	return getConfigPath(config)
}

func GetConfigPathErr(config string) (string, error) {
	return configPath(config)
}

type pendingFileCommit struct {
	target string
	temp   string
	backup string
}

func saveFile(filePath string, data []byte) error {
	return saveFilesAtomically(map[string][]byte{filePath: data})
}

func stagePendingFileCommit(filePath string, data []byte) (pendingFileCommit, error) {
	dir := filepath.Dir(filePath)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return pendingFileCommit{}, fmt.Errorf("创建临时文件 %s 失败: %w", filePath, err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := temp.Write(data); err != nil {
		cleanup()
		return pendingFileCommit{}, fmt.Errorf("写入临时文件 %s 失败: %w", filePath, err)
	}
	if err := temp.Chmod(configFileMode); err != nil {
		cleanup()
		return pendingFileCommit{}, fmt.Errorf("设置临时文件权限 %s 失败: %w", filePath, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return pendingFileCommit{}, fmt.Errorf("同步临时文件 %s 失败: %w", filePath, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return pendingFileCommit{}, fmt.Errorf("关闭临时文件 %s 失败: %w", filePath, err)
	}

	return pendingFileCommit{
		target: filePath,
		temp:   tempPath,
	}, nil
}

func saveFilesAtomically(files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}

	type commitEntry struct {
		path string
		data []byte
	}

	entries := make([]commitEntry, 0, len(files))
	for filePath, raw := range files {
		entries = append(entries, commitEntry{
			path: filepath.Clean(filePath),
			data: raw,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})

	pending := make([]pendingFileCommit, 0, len(entries))
	for _, entry := range entries {
		item, err := stagePendingFileCommit(entry.path, entry.data)
		if err != nil {
			cleanupPendingFileCommits(pending)
			return err
		}
		pending = append(pending, item)
	}

	for index := range pending {
		item := &pending[index]
		if info, err := os.Stat(item.target); err == nil {
			if info.IsDir() {
				rollbackErr := rollbackPendingFileCommits(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(fmt.Errorf("目标路径 %s 是目录，不能作为配置文件覆盖", item.target), rollbackErr)
				}
				return fmt.Errorf("目标路径 %s 是目录，不能作为配置文件覆盖", item.target)
			}
			backup, err := os.CreateTemp(filepath.Dir(item.target), "."+filepath.Base(item.target)+".backup-*")
			if err != nil {
				rollbackErr := rollbackPendingFileCommits(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
			item.backup = backup.Name()
			_ = backup.Close()
			_ = os.Remove(item.backup)
			if err := osRename(item.target, item.backup); err != nil {
				rollbackErr := rollbackPendingFileCommits(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
		} else if !os.IsNotExist(err) {
			rollbackErr := rollbackPendingFileCommits(pending, index-1)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}

		if err := osRename(item.temp, item.target); err != nil {
			rollbackErr := rollbackPendingFileCommits(pending, index)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
	}

	for _, item := range pending {
		if item.backup != "" {
			_ = os.Remove(item.backup)
		}
	}
	return nil
}

func cleanupPendingFileCommits(items []pendingFileCommit) {
	for _, item := range items {
		if item.temp != "" {
			_ = os.Remove(item.temp)
		}
	}
}

func rollbackPendingFileCommits(items []pendingFileCommit, appliedIndex int) error {
	var rollbackErr error
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if item.temp != "" {
			if err := os.Remove(item.temp); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		if index > appliedIndex {
			if item.backup != "" {
				if err := osRename(item.backup, item.target); err != nil && !os.IsNotExist(err) {
					rollbackErr = errors.Join(rollbackErr, err)
				}
			}
			continue
		}

		if _, err := os.Stat(item.target); err == nil {
			if err := os.Remove(item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
				continue
			}
		}
		if item.backup != "" {
			if err := osRename(item.backup, item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
	}
	return rollbackErr
}

// readFile reads the file and returns (nil, error) on failure. Callers should handle errors.
func readFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", filePath, err)
	}
	return data, nil
}
