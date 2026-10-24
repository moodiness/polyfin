package netfail

import "golang.org/x/sys/windows"

// The socket errors a dial or a connection fails with, by how it ended.
// Windows' sockets fail with WSA errors. A write to a connection the host
// reset fails as a reset does, with WSAECONNRESET or WSAECONNABORTED: no
// socket error there is a broken pipe.
var (
	refused     = []error{windows.WSAECONNREFUSED}
	unreachable = []error{windows.WSAEHOSTUNREACH, windows.WSAENETUNREACH}
	timedOut    = []error{windows.WSAETIMEDOUT}
	reset       = []error{windows.WSAECONNRESET, windows.WSAECONNABORTED}
	brokenPipe  []error
)
