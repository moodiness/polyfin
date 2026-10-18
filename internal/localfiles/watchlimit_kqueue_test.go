//go:build !linux && !windows

package localfiles

import "syscall"

// limitError is how kqueue fails at the limit of open files, and
// limitReason what a folder's Unwatched then tells.
var limitError, limitReason = error(syscall.EMFILE), unwatchedOpenFilesLimit

// failureReasons are the failures to watch a folder, and what each tells:
// there is no limit of watches apart from that of open files.
var failureReasons = []struct {
	err  error
	want string
}{
	{syscall.EMFILE, unwatchedOpenFilesLimit},
	{syscall.ENFILE, unwatchedOpenFilesLimit},
	{syscall.ENOSPC, unwatchedFailed},
	{syscall.EACCES, unwatchedFailed},
}
