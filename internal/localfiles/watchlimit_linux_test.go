package localfiles

import "syscall"

// limitError is how inotify fails at its limit of watches, and limitReason
// what a folder's Unwatched then tells.
var limitError, limitReason = error(syscall.ENOSPC), unwatchedWatchLimit

// failureReasons are the failures to watch a folder, and what each tells.
var failureReasons = []struct {
	err  error
	want string
}{
	{syscall.ENOSPC, unwatchedWatchLimit},
	{syscall.EMFILE, unwatchedInstanceLimit},
	{syscall.ENFILE, unwatchedInstanceLimit},
	{syscall.EACCES, unwatchedFailed},
}
