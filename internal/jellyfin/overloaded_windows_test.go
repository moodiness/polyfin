package jellyfin

import (
	"fmt"
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/moodiness/polyfin/internal/stremio"
)

// An artwork server that resets the connection shows it gets too many
// requests, with Windows' socket errors as stremio.Client.Image returns
// them: WSA errors, which syscall's ECONNRESET does not match on Windows.
func TestOverloadedByAResetConnection(t *testing.T) {
	t.Parallel()
	for _, reset := range []error{
		&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("wsarecv", windows.WSAECONNRESET)},
		&net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("wsasend", windows.WSAECONNABORTED)},
	} {
		if err := fmt.Errorf("%w: %w", stremio.ErrUnreachable, reset); !overloaded(err) {
			t.Errorf("%v: not overloaded", err)
		}
	}
}
