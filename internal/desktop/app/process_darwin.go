package app

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// parentDeath has no equivalent on macOS: the server a crash of the app
// left running is stopped at the next start.
func parentDeath(*syscall.SysProcAttr) {}

// startTime is when pid started, in microseconds since 1970.
func startTime(pid int) (int64, bool) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(info.Proc.P_pid) != pid {
		return 0, false
	}
	return info.Proc.P_starttime.Sec*1_000_000 + int64(info.Proc.P_starttime.Usec), true
}
