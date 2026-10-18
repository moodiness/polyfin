package localfiles

// unwatchedReason tells why Windows could not watch a folder: it has no
// limit of watches to raise, so any failure is one.
func unwatchedReason(error) string {
	return unwatchedFailed
}
