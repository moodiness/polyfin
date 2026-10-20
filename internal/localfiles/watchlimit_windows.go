package localfiles

// unwatchedReason tells why Windows could not watch a folder: it has no
// limit of watches to raise, so any failure is one.
func unwatchedReason(error) string {
	return unwatchedFailed
}

// rewatchCreated: Windows keeps the watch of a folder created as added
// (see watchlimit_kqueue.go).
const rewatchCreated = false
