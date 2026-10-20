package localfiles

import (
	"errors"
	"syscall"
)

// unwatchedReason tells why inotify could not watch a folder: its limit of
// watches (ENOSPC, fs.inotify.max_user_watches), its limit of watchers or
// of open files (EMFILE, ENFILE, fs.inotify.max_user_instances), or
// another failure.
func unwatchedReason(err error) string {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return unwatchedWatchLimit
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return unwatchedInstanceLimit
	}
	return unwatchedFailed
}

// rewatchCreated: inotify keeps the watch of a folder created as added
// (see watchlimit_kqueue.go).
const rewatchCreated = false
