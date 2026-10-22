//go:build !windows

package torrent

import (
	"net"
	"os"
	"syscall"
	"testing"
)

// A dial or a connection that fails with the system's socket error tells
// why, as the net package wraps it.
func TestOutcomeOfASocketError(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]outcome{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}: outcomeRefused,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}: outcomeUnreachable,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}:  outcomeUnreachable,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}:    outcomeTimedOut,
		&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}:      outcomeClosed,
		&net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}:         outcomeClosed,
	} {
		if got := outcomeOf(err); got != want {
			t.Errorf("%v: %d, want %d", err, got, want)
		}
	}
}
