//go:build unix

package torrent

import (
	"syscall"
	"time"
)

// processCPU is the processor time the test process used so far, user and
// system.
func processCPU() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}
