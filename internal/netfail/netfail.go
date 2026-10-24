// Package netfail tells how a connection failed from the socket error the
// system returned. Systems differ: Unix sockets fail with errno values
// such as ECONNRESET, Windows sockets with WSA errors such as
// WSAECONNRESET (10054). On Windows, syscall's ECONNRESET and the like are
// values Go invents, which no socket returns, so an error compared with
// them there never matches.
package netfail

import (
	"errors"
	"slices"
)

// Refused reports a connection the host refused: nothing listens on its
// port.
func Refused(err error) bool { return isAny(err, refused) }

// Unreachable reports a host or a network that no route leads to.
func Unreachable(err error) bool { return isAny(err, unreachable) }

// TimedOut reports a connection the system gave up on, the host never
// answering. A deadline or a context that ended is not one.
func TimedOut(err error) bool { return isAny(err, timedOut) }

// Reset reports a connection the host reset. On Windows, it is also one
// the system aborted, which is how Windows tells of a reset met while
// sending.
func Reset(err error) bool { return isAny(err, reset) }

// BrokenPipe reports a write to a connection the host had already closed
// or reset. Windows sockets have no such error: there, the write fails as
// Reset tells.
func BrokenPipe(err error) bool { return isAny(err, brokenPipe) }

// isAny reports whether err is one of targets.
func isAny(err error, targets []error) bool {
	return slices.ContainsFunc(targets, func(target error) bool { return errors.Is(err, target) })
}
