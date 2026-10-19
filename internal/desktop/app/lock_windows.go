package app

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile holds file's lock while the app runs; the system releases it
// when the app ends, however it ends.
func lockFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrRunning
	}
	return err
}

// unlockFile releases file's lock.
func unlockFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}
