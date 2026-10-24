//go:build !windows

package netfail

import (
	"net"
	"os"
	"syscall"
	"testing"
)

// A dial or a connection that fails with the system's socket error tells
// why, as the net package wraps it.
func TestSocketErrors(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}: "refused",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}: "unreachable",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}:  "unreachable",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ETIMEDOUT)}:    "timed out",
		&net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}:      "reset",
		&net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}:         "broken pipe",
	} {
		if got := failure(err); got != want {
			t.Errorf("%v: %q, want %q", err, got, want)
		}
	}
}
