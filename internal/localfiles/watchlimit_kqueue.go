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

// rewatchCreated: a folder created in a folder watched is watched twice
// at once, by the watch, which adds it, and by fsnotify itself, which
// watches each entry of a folder; fsnotify may then register its own watch
// of the folder over the one just added, without the writes in it, so
// that the files later added to the folder go unseen. The watch adds the
// folder again as its scan begins, when fsnotify is done with it; the scan
// lists the files written meanwhile.
const rewatchCreated = true
