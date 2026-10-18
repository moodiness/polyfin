//go:build !windows

package process

import (
	"os"
	"os/exec"
)

// Interruptible prepares FFmpeg's cmd, not started yet, to be interrupted
// as Ctrl-C would: interrupt sends it SIGINT, each time it is called. FFmpeg
// stops at its first interrupt, writing what it holds, and its second ends
// a read that waits for its source.
func Interruptible(cmd *exec.Cmd) (interrupt func() error, err error) {
	return func() error { return cmd.Process.Signal(os.Interrupt) }, nil
}
