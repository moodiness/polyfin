//go:build !windows

package torrent

import "syscall"

// The socket errors a dial or a connection fails with, by how it ended
// (see outcomeOf).
var (
	refusedErrors     = []error{syscall.ECONNREFUSED}
	unreachableErrors = []error{syscall.EHOSTUNREACH, syscall.ENETUNREACH}
	timedOutErrors    = []error{syscall.ETIMEDOUT}
	closedErrors      = []error{syscall.ECONNRESET, syscall.EPIPE}
)
