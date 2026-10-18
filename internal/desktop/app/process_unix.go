//go:build !windows

package app

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// prepareServer puts the server in a process group of its own, so that
// one that does not stop in time is killed with its children.
func prepareServer(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	parentDeath(cmd.SysProcAttr)
}

// adopt needs nothing on Unix: the process group is the server's.
func adopt(*os.Process) error { return nil }

// interrupt asks the server to stop, as Docker does.
func interrupt(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

// kill ends the server's process group at once.
func kill(process *os.Process) error {
	err := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// alive tells whether pid runs.
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
