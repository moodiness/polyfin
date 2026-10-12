//go:build linux || darwin

package audiosegments

import "syscall"

// lowerPriority gives a process the lowest CPU priority, so that playback
// and the server's requests go first.
func lowerPriority(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, 19)
}
