//go:build linux || darwin

package media

import "syscall"

// LowerPriority gives a process the lowest CPU priority, so that playback
// and the server's requests go first: FFmpeg reading in the background
// runs so.
func LowerPriority(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, 19)
}
