//go:build !windows

package localfiles

import (
	"errors"
	"syscall"
)

// looping reports whether err tells of symbolic links leading round in a
// loop: ELOOP, too many links followed.
func looping(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}
