// Package diskspace measures the disks Polyfin's folders are on.
package diskspace

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Measure measures the disk the folder path is on: the bytes free to
// Polyfin, the bytes used, and the folder the disk is mounted on, such as
// C:\ or a folder a volume is mounted in.
func Measure(path string) (free, used int64, mount string, ok bool) {
	path, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, "", false
	}
	// Windows asks a share's folder, \\server\share\, to end with a
	// separator; any folder may.
	if !strings.HasSuffix(path, `\`) {
		path += `\`
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, "", false
	}
	var available, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, &totalFree); err != nil {
		return 0, 0, "", false
	}
	return int64(available), int64(total - totalFree), mountPoint(name), true
}

// mountPoint is the folder the volume of path is mounted on, empty when
// Windows does not tell it.
func mountPoint(path *uint16) string {
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	if err := windows.GetVolumePathName(path, &buffer[0], uint32(len(buffer))); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer)
}
