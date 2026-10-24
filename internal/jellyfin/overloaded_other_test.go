//go:build !windows

package jellyfin

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// An artwork server that resets the connection shows it gets too many
// requests, with the system's socket error as stremio.Client.Image
// returns it.
func TestOverloadedByAResetConnection(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("%w: %w", stremio.ErrUnreachable,
		&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)})
	if !overloaded(err) {
		t.Errorf("%v: not overloaded", err)
	}
}
