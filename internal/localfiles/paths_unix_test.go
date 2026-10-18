//go:build !windows

package localfiles

import "testing"

// On Linux and macOS, a local folder is an absolute path other than /; a
// Windows path is none.
func TestUnixFolderPaths(t *testing.T) {
	for path, want := range map[string]string{
		"/media/movies":             "/media/movies",
		" /media/movies/classics/ ": "/media/movies/classics",
		"/media/../srv":             "/srv",
	} {
		if got, err := validLocation(path); err != nil || got != want {
			t.Errorf("%q: %q, %v, want %q", path, got, err, want)
		}
	}
	for _, path := range []string{"/", "/media/..", "media", `D:\Movies`, `\\nas\media`, "/media\x00"} {
		if got, err := validLocation(path); err != ErrInvalidPath {
			t.Errorf("%q: %q, %v, want ErrInvalidPath", path, got, err)
		}
	}
}
