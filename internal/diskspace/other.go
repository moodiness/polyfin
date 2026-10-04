//go:build !linux && !darwin

// Package diskspace measures the disks Polyfin's folders are on.
package diskspace

// Measure cannot measure disks here: folders are described unmeasured.
func Measure(string) (free, used int64, mount string, ok bool) {
	return 0, 0, "", false
}
