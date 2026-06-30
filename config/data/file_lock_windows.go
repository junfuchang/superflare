//go:build windows

package data

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockOpenedConfigFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		return fmt.Errorf("lock config files failed: %w", err)
	}
	return nil
}

func unlockOpenedConfigFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		return fmt.Errorf("unlock config files failed: %w", err)
	}
	return nil
}
