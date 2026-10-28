package serverfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A file of the server is an absolute path of its system, or a file://
// address of one, on Windows as elsewhere; anything else is none.
func TestPathsAreTheServersAbsoluteOnes(t *testing.T) {
	for _, tc := range []struct {
		address string
		windows bool
		path    string
		ok      bool
	}{
		{"/srv/iptv/custom.m3u", false, "/srv/iptv/custom.m3u", true},
		{"  /srv/iptv/../iptv/custom.m3u ", false, "/srv/iptv/custom.m3u", true},
		{"file:///srv/iptv/my%20list.m3u", false, "/srv/iptv/my list.m3u", true},
		{"FILE://localhost/srv/iptv/custom.m3u", false, "/srv/iptv/custom.m3u", true},
		{`C:\IPTV\custom.m3u`, true, `C:\IPTV\custom.m3u`, true},
		{"c:/IPTV/custom.m3u", true, `c:\IPTV\custom.m3u`, true},
		{"file:///C:/IPTV/custom.m3u", true, `C:\IPTV\custom.m3u`, true},
		{`\\nas\iptv\custom.m3u`, true, `\\nas\iptv\custom.m3u`, true},
		{`C:\IPTV\custom.m3u`, false, "", false},
		{"/srv/iptv/custom.m3u", true, "", false},
		{"iptv/custom.m3u", false, "", false},
		{`IPTV\custom.m3u`, true, "", false},
		{"C:custom.m3u", true, "", false},
		{"file://nas/iptv/custom.m3u", false, "", false},
		{"file:relative.m3u", false, "", false},
		{"https://lists.example/custom.m3u", false, "", false},
		{"", false, "", false},
		{"/srv/iptv/\x00.m3u", false, "", false},
	} {
		if path, ok := pathOf(tc.address, tc.windows); path != tc.path || ok != tc.ok {
			t.Errorf("%q (windows %v): %q %v, want %q %v", tc.address, tc.windows, path, ok, tc.path, tc.ok)
		}
	}
}

// A file is read when it is one Polyfin may read; a missing one, or a
// folder, is told as such. A file is read again once modified after it
// was last read, or once it can no longer be read, then not again until
// it changes.
func TestFilesAreCheckedAndTheirChangesTold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.xml")
	if err := os.WriteFile(path, []byte("<tv/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err != nil {
		t.Errorf("a file: %v", err)
	}
	if err := Check(filepath.Join(dir, "missing.xml")); !errors.Is(err, ErrMissing) {
		t.Errorf("a missing file: %v", err)
	}
	if err := Check(dir); !errors.Is(err, ErrUnreadable) {
		t.Errorf("a folder: %v", err)
	}

	modified := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	before, after := modified.Add(-time.Minute), modified.Add(time.Minute)
	address := "file:///" + strings.TrimPrefix(filepath.ToSlash(path), "/")
	for _, tc := range []struct {
		name       string
		address    string
		checked    *time.Time
		unreadable bool
		want       bool
	}{
		{"never read", path, nil, false, true},
		{"modified since", path, &before, false, true},
		{"read since", path, &after, false, false},
		{"by its address", address, &before, false, true},
		{"gone", filepath.Join(dir, "missing.xml"), &after, false, true},
		{"gone, told", filepath.Join(dir, "missing.xml"), &after, true, false},
		{"a web address", "https://lists.example/guide.xml", nil, false, false},
	} {
		if got := Changed(tc.address, tc.checked, tc.unreadable); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
