package localfiles

import (
	"os"
	"testing"
)

// On Windows, a local folder is a drive's folder or a share's, with
// backslashes or slashes, a drive's root among them but the system's.
func TestWindowsFolderPaths(t *testing.T) {
	for path, want := range map[string]string{
		`D:\Movies`:             `D:\Movies`,
		` d:/Movies/Classics/ `: `d:\Movies\Classics`,
		`D:\Movies\..\Shows`:    `D:\Shows`,
		`E:\`:                   `E:\`,
		`\\nas\media\Movies`:    `\\nas\media\Movies`,
		`//nas/media/Movies/`:   `\\nas\media\Movies`,
	} {
		if got, err := validLocation(path); err != nil || got != want {
			t.Errorf("%q: %q, %v, want %q", path, got, err, want)
		}
	}
	for _, path := range []string{os.Getenv("SystemDrive") + `\`, `\Movies`, `/Movies`, `Movies`, `D:Movies`, `D:`, "D:\\Movies\x00"} {
		if got, err := validLocation(path); err != ErrInvalidPath {
			t.Errorf("%q: %q, %v, want ErrInvalidPath", path, got, err)
		}
	}
	// A share's address stays one.
	if got := shareKind(`smb://nas/media`); got != ShareSMB {
		t.Errorf("an SMB address: %q", got)
	}
	if got := shareKind(`D:\Movies`); got != "" {
		t.Errorf("a drive's folder: %q", got)
	}
}
