package torrent

import (
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A dial or a connection that fails with Windows' socket error tells why,
// as the net package wraps it: a WSA error, which syscall's ECONNREFUSED,
// ECONNRESET and the like do not match on Windows.
func TestOutcomeOfASocketError(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]outcome{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAECONNREFUSED)}: outcomeRefused,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAEHOSTUNREACH)}: outcomeUnreachable,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAENETUNREACH)}:  outcomeUnreachable,
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connectex", windows.WSAETIMEDOUT)}:    outcomeTimedOut,
		&net.OpError{Op: "read", Err: os.NewSyscallError("wsarecv", windows.WSAECONNRESET)}:     outcomeClosed,
		&net.OpError{Op: "write", Err: os.NewSyscallError("wsasend", windows.WSAECONNABORTED)}:  outcomeClosed,
	} {
		if got := outcomeOf(err); got != want {
			t.Errorf("%v: %d, want %d", err, got, want)
		}
	}
}
