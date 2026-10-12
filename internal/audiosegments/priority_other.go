//go:build !linux && !darwin

package audiosegments

// lowerPriority leaves a process's priority as it is where Polyfin cannot
// change it.
func lowerPriority(int) {}
