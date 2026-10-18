//go:build !linux && !darwin && !windows

package media

// LowerPriority leaves a process's priority as it is where Polyfin cannot
// change it.
func LowerPriority(int) {}
