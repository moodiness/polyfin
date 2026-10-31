package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/moodiness/polyfin/internal/desktop"
)

// MaxStarts is how many times the new version's server may start without
// answering /healthz before the app goes back to the previous version.
const MaxStarts = 3

// Pending is an update installed whose new version has not answered yet:
// the app keeps the previous app and a copy of the database, taken while
// it was stopped, until the new server answers /healthz, and puts them
// back once it started MaxStarts times without answering. A database the
// new version migrated would not start the previous one, which refuses a
// schema newer than its own: the copy is the database as it left it.
type Pending struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind Kind   `json:"kind"`
	// App is where the app is (its bundle, its AppImage, or its install
	// folder on Windows), Previous the previous app kept beside it.
	App      string `json:"app"`
	Previous string `json:"previous"`
	// Database is the copy of the cluster's folder.
	Database string `json:"database"`
	// Starts counts the new version's server starts so far.
	Starts int `json:"starts"`
}

// ReadPending reads the pending update of the data folder, nil when
// there is none.
func ReadPending(f desktop.Folders) (*Pending, error) {
	data, err := os.ReadFile(f.PendingUpdate())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p Pending
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Write keeps p in the data folder.
func (p Pending) Write(f desktop.Folders) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	temporary := f.PendingUpdate() + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, f.PendingUpdate())
}

// Confirm removes the previous app and the copy of the database, then the
// record of p: the new version answered. A copy that holds exe, the
// running app, is left alone, as after an install that was cancelled.
func (p Pending) Confirm(f desktop.Folders, exe string) error {
	var errs []error
	for _, kept := range []string{p.Previous, p.Database} {
		if kept == "" || within(exe, kept) {
			continue
		}
		errs = append(errs, os.RemoveAll(kept))
	}
	if err := os.Remove(f.PendingUpdate()); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// within tells whether path is dir or below it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Restore puts the previous app and the copy of the database back in
// place of the new ones, cluster being the cluster's folder, once the
// server and the database have stopped, then removes the record of p. It
// returns what starts the previous app, exe being the running app's
// executable. What is replaced is removed; on Windows, the running
// executable cannot be: RemoveLeftovers removes it at the next start.
func (p Pending) Restore(f desktop.Folders, cluster, exe string) (*exec.Cmd, error) {
	if p.Previous == "" || p.App == "" {
		return nil, errors.New("no previous app was kept")
	}
	if p.Database != "" {
		if err := putBack(p.Database, cluster); err != nil {
			return nil, fmt.Errorf("put the copy of the database back: %w", err)
		}
	}
	var err error
	if p.Kind == Installer {
		err = putBackEntries(p.Previous, p.App)
	} else {
		err = putBack(p.Previous, p.App)
	}
	if err != nil {
		return nil, fmt.Errorf("put the previous app back: %w", err)
	}
	if err := os.Remove(f.PendingUpdate()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	switch p.Kind {
	case DMG:
		return desktop.WindowCommand(context.Background(), "/usr/bin/open", "-n", p.App), nil
	case AppImage:
		return desktop.WindowCommand(context.Background(), p.App), nil
	}
	return desktop.WindowCommand(context.Background(), exe), nil
}

// broken is where what Restore replaces goes before it is removed.
func broken(path string) string { return path + ".broken" }

// putBack puts the file or folder kept in place of current.
func putBack(kept, current string) error {
	if _, err := os.Lstat(kept); err != nil {
		return err
	}
	aside := broken(current)
	if err := os.RemoveAll(aside); err != nil {
		return err
	}
	if err := os.Rename(current, aside); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(kept, current); err != nil {
		_ = os.Rename(aside, current)
		return err
	}
	return os.RemoveAll(aside)
}

// putBackEntries puts the entries of the folder kept in place of those of
// the folder current, which holds the running executable: Windows lets a
// running executable be renamed, never its folder.
func putBackEntries(kept, current string) error {
	entries, err := os.ReadDir(kept)
	if err != nil {
		return err
	}
	aside := broken(current)
	if err := os.RemoveAll(aside); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(aside, 0o755); err != nil {
		return err
	}
	present, err := os.ReadDir(current)
	if err != nil {
		return err
	}
	for _, entry := range present {
		if err := os.Rename(filepath.Join(current, entry.Name()), filepath.Join(aside, entry.Name())); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(kept, entry.Name()), filepath.Join(current, entry.Name())); err != nil {
			return err
		}
	}
	if err := os.Remove(kept); err != nil {
		return err
	}
	// The running executable stays until RemoveLeftovers.
	_ = os.RemoveAll(aside)
	return nil
}

// RemoveLeftovers removes what Restore could not, beside the app whose
// executable is exe, installed from a package of kind.
func RemoveLeftovers(kind Kind, exe string) error {
	if kind != Installer {
		return nil
	}
	err := os.RemoveAll(broken(filepath.Dir(exe)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CopyTree copies the folder src to dst, which it replaces: its folders,
// files and links, with their permissions. A failure leaves no dst.
func CopyTree(src, dst string) (err error) {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dst)
		}
	}()
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyKept(path, target, info.Mode().Perm())
		}
		// Sockets and the like are not part of an app or a stopped
		// database.
		return nil
	})
}

// copyKept copies the file src to dst, which must not exist, with mode.
func copyKept(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}
