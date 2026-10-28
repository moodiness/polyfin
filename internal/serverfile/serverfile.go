// Package serverfile reads the IPTV lists and XMLTV guides that are files
// on the server, given by their absolute path or a file:// address where
// an address is asked, as Jellyfin's "File or URL" fields take them.
package serverfile

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrMissing reports a path where there is no file.
	ErrMissing = errors.New("there is no file at this path")
	// ErrUnreadable reports a file Polyfin may not read, or a path that
	// is no file, such as a folder.
	ErrUnreadable = errors.New("the file cannot be read")
)

// Path reports the path address names when it is a file of the server:
// an absolute path of the server's system, such as /srv/iptv/list.m3u, or
// C:\IPTV\custom.m3u on Windows, or a file:// address of one. A relative
// path, a web address or anything else is none.
func Path(address string) (string, bool) {
	return pathOf(address, runtime.GOOS == "windows")
}

// pathOf is Path for a server running Windows or not.
func pathOf(address string, windows bool) (string, bool) {
	address = strings.TrimSpace(address)
	if address == "" || strings.ContainsRune(address, 0) {
		return "", false
	}
	if len(address) >= 5 && strings.EqualFold(address[:5], "file:") {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
			parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			return "", false
		}
		address = parsed.Path
		// file:///C:/IPTV/custom.m3u names C:\IPTV\custom.m3u.
		if windows && len(address) >= 3 && address[0] == '/' && drive(address[1:]) {
			address = address[1:]
		}
	}
	if windows {
		if !drive(address) && !strings.HasPrefix(address, `\\`) {
			return "", false
		}
		return strings.ReplaceAll(address, "/", `\`), true
	}
	if !strings.HasPrefix(address, "/") {
		return "", false
	}
	return path.Clean(address), true
}

// drive reports whether p starts with a Windows drive and its root, such
// as C:\ or C:/.
func drive(p string) bool {
	return len(p) >= 3 && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z') && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// Open opens the file at path for reading: a regular file, else
// ErrMissing or ErrUnreadable.
func Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, failure(err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = ErrUnreadable
	}
	if err != nil {
		_ = file.Close()
		return nil, failure(err)
	}
	return file, nil
}

// Check reports whether the file at path can be read, as Open does.
func Check(path string) error {
	file, err := Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// Modified returns when the file at path was last changed.
func Modified(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, failure(err)
	}
	return info.ModTime(), nil
}

// Changed reports whether address, when it is a file of the server, is
// to be read again before its turn: it was modified after checked, when
// it was last read (nil for never), or it can no longer be read while its
// last read did not fail so (unreadable), to tell it at once.
func Changed(address string, checked *time.Time, unreadable bool) bool {
	path, ok := Path(address)
	if !ok {
		return false
	}
	modified, err := Modified(path)
	if err != nil {
		return !unreadable
	}
	return checked == nil || modified.After(*checked)
}

// failure is err as ErrMissing or ErrUnreadable, with what the system
// said.
func failure(err error) error {
	switch {
	case errors.Is(err, ErrMissing), errors.Is(err, ErrUnreadable):
		return err
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %v", ErrMissing, err)
	default:
		return fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
}
