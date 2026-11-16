package hls

import (
	"errors"

	"golang.org/x/sys/windows"
)

// heldElsewhere reports whether err tells that another program holds the
// file: Windows refuses to open a file another handle shares no access
// to, and to rename over one held without FILE_SHARE_DELETE.
func heldElsewhere(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
