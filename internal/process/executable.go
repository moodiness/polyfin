package process

import (
	"io"
	"os"
	"syscall"
)

// WriteExecutable writes what r reads to the file name, created with perm
// or truncated, for a program to be run from it: one downloaded, or a
// test's fake one.
//
// Some systems refuse to run a file, "text file busy", while any process
// holds it open for writing. A program started meanwhile holds every
// descriptor of Polyfin until its own exec closes them, this one too, and
// Polyfin starts programs all the time. So no program starts from the
// open of the file to its close: they wait on the lock os/exec takes to
// start one. r must not wait on a program starting.
func WriteExecutable(name string, r io.Reader, perm os.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	file, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, r)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
