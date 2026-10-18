//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// startAgain replaces this process with a new Polyfin, started as this
// one was, which reads the restored database: it only returns on failure,
// and Polyfin then exits for its supervisor, such as Docker, to start it.
func startAgain() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := syscall.Exec(executable, os.Args, os.Environ()); err != nil {
		return errors.Join(errors.New("cannot start Polyfin again"), err)
	}
	return nil
}
