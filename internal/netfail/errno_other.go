//go:build !windows

package netfail

import "syscall"

// The socket errors a dial or a connection fails with, by how it ended.
var (
	refused     = []error{syscall.ECONNREFUSED}
	unreachable = []error{syscall.EHOSTUNREACH, syscall.ENETUNREACH}
	timedOut    = []error{syscall.ETIMEDOUT}
	reset       = []error{syscall.ECONNRESET}
	brokenPipe  = []error{syscall.EPIPE}
)
