//go:build !windows

package data

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockOpenedConfigFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock config files failed: %w", err)
	}
	return nil
}

func unlockOpenedConfigFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock config files failed: %w", err)
	}
	return nil
}
