//go:build linux || darwin

// Package diskspace measures the disks Polyfin's folders are on.
package diskspace

import (
	"os"
	"path/filepath"
	"syscall"
)

// Measure measures the disk path is on: the bytes free to Polyfin, the
// bytes used, and the folder the disk is mounted on.
func Measure(path string) (free, used int64, mount string, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, "", false
	}
	block := uint64(stat.Bsize)
	return int64(stat.Bavail * block), int64((stat.Blocks - stat.Bfree) * block), mountPoint(path), true
}

// mountPoint is the highest folder above path on the same device.
func mountPoint(path string) string {
	path, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	device := info.Sys().(*syscall.Stat_t).Dev
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		info, err := os.Stat(parent)
		if err != nil || info.Sys().(*syscall.Stat_t).Dev != device {
			return path
		}
		path = parent
	}
}
