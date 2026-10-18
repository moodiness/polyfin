//go:build !linux && !windows

package localfiles

import (
	"errors"
	"syscall"
)

// unwatchedReason tells why kqueue, on macOS, could not watch a folder:
// fsnotify opens each folder and file it watches, so a large library may
// reach the limit of open files (EMFILE, kern.maxfilesperproc; ENFILE,
// kern.maxfiles); or another failure.
func unwatchedReason(err error) string {
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
		return unwatchedOpenFilesLimit
	}
	return unwatchedFailed
}
