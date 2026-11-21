package localfiles

import (
	"errors"

	"golang.org/x/sys/windows"
)

// looping reports whether err tells of symbolic links leading round in a
// loop: Windows gives up following them with ERROR_CANT_RESOLVE_FILENAME,
// never with ELOOP.
func looping(err error) bool {
	return errors.Is(err, windows.ERROR_CANT_RESOLVE_FILENAME)
}
