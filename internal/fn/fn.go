package fn

import (
	"fmt"
	"os"
	"path/filepath"
)

var osGetwd = os.Getwd
var testGetwdExport = func() func() (string, error) { return osGetwd }
var testSetGetwdExport = func(next func() (string, error)) { osGetwd = next }

// GetWorkDir returns the current working directory, or empty string on error.
// Deprecated: use GetWorkDirE to handle errors.
func GetWorkDir() string {
	dir, err := osGetwd()
	if err != nil {
		return ""
	}
	return dir
}

// GetWorkDirE returns the current working directory or an error.
func GetWorkDirE() (string, error) {
	dir, err := osGetwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory failed: %w", err)
	}
	return dir, nil
}

// GetWorkDirFile returns the path joining current working directory and filename.
func GetWorkDirFile(filename string) string {
	dir := GetWorkDir()
	return filepath.Join(dir, filename)
}

// GetWorkDirFileE returns the path joining current working directory and filename or an error.
func GetWorkDirFileE(filename string) (string, error) {
	dir, err := GetWorkDirE()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}
