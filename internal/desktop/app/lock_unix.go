//go:build !windows

package app

import (
	"errors"
	"os"
	"syscall"
)

// lockFile holds file's lock while the app runs; the system releases it
// when the app ends, however it ends.
func lockFile(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrRunning
	}
	return err
}

// unlockFile releases file's lock.
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
