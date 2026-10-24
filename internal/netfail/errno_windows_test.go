package netfail

import (
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A dial or a connection that fails with Windows' socket error tells why,
// as the net package wraps it: a WSA error, which syscall's ECONNREFUSED,
// ECONNRESET and the like do not match on Windows.
func TestSocketErrors(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAECONNREFUSED)}: "refused",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAEHOSTUNREACH)}: "unreachable",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAENETUNREACH)}:  "unreachable",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAETIMEDOUT)}:    "timed out",
		&net.OpError{Op: "read", Err: os.NewSyscallError("wsarecv", windows.WSAECONNRESET)}:     "reset",
		&net.OpError{Op: "write", Err: os.NewSyscallError("wsasend", windows.WSAECONNABORTED)}:  "reset",
	} {
		if got := failure(err); got != want {
			t.Errorf("%v: %q, want %q", err, got, want)
		}
	}
}
