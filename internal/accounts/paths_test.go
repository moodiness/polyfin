package accounts

import (
	"path/filepath"
	"runtime"
)

// systemPath is an absolute path given with slashes, as the system Polyfin
// runs on writes it: the same on Linux and macOS, on drive C: with
// backslashes on Windows, where /certs is not absolute.
func systemPath(path string) string {
	if runtime.GOOS == "windows" {
		return `C:` + filepath.FromSlash(path)
	}
	return path
}
