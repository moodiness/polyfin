package localfiles

import "syscall"

// limitError is how Windows could fail to watch one more folder, out of
// memory, and limitReason what a folder's Unwatched then tells: Windows
// has no limit of watches to raise.
var limitError, limitReason = error(syscall.Errno(8)), unwatchedFailed

// failureReasons are the failures to watch a folder, and what each tells:
// a failure, whatever it is.
var failureReasons = []struct {
	err  error
	want string
}{
	{syscall.Errno(8), unwatchedFailed},
	{syscall.ENOSPC, unwatchedFailed},
	{syscall.EMFILE, unwatchedFailed},
	{syscall.ERROR_ACCESS_DENIED, unwatchedFailed},
}
