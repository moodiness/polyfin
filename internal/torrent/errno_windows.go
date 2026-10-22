package torrent

import "golang.org/x/sys/windows"

// The socket errors a dial or a connection fails with, by how it ended
// (see outcomeOf). Windows' sockets fail with WSA errors: syscall's
// ECONNREFUSED, ECONNRESET and the like are values Go invents on
// Windows, which no socket returns.
var (
	refusedErrors     = []error{windows.WSAECONNREFUSED}
	unreachableErrors = []error{windows.WSAEHOSTUNREACH, windows.WSAENETUNREACH}
	timedOutErrors    = []error{windows.WSAETIMEDOUT}
	closedErrors      = []error{windows.WSAECONNRESET, windows.WSAECONNABORTED}
)
