//go:build unix

package backup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// modeRecordingPgDump is the body of a pg_dump that writes a dump to its
// --file, or its standard output without one, then records in
// $record/mode the mode of the partial file of $dir while the dump is
// still going on. As pg_dump does, it opens and empties the file it is
// given the name of, and makes one that is not there for whoever the
// process's umask lets read.
const modeRecordingPgDump = `for arg; do
	case $arg in --file=*) exec > "${arg#--file=}" ;; esac
done
printf 'PGDMP every password hash, token and stored key'
ls -l "$dir"/.polyfin-*.partial | cut -c1-10 > "$record/mode"
`

// widenUmask gives the process, for the length of the test, the umask of
// a service started by a shell or by systemd, which lets everyone read
// the files made without a narrower mode.
func widenUmask(t *testing.T) {
	t.Helper()
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
}

// A backup is readable by its owner alone from its first byte, with a
// passphrase or without: Polyfin makes the file before pg_dump runs.
//
//nolint:paralleltest,tparallel // sets the process's umask
func TestABackupIsItsOwnersAloneWhileItIsWritten(t *testing.T) {
	widenUmask(t)
	for name, passphrase := range map[string]string{"plain": "", "encrypted": testPassphrase} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.passphrase = passphrase
			script := filepath.Join(t.TempDir(), "pg_dump")
			body := "#!/bin/sh\nrecord='" + h.record + "'\ndir='" + h.dir + "'\n" + modeRecordingPgDump
			if err := process.WriteExecutable(script, strings.NewReader(body), 0o700); err != nil {
				t.Fatal(err)
			}
			h.service.cfg.PgDump = script
			h.service.now = func() time.Time { return time.Date(2026, 10, 5, 4, 0, 0, 0, time.Local) }
			if err := h.service.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			during, err := os.ReadFile(filepath.Join(h.record, "mode"))
			if err != nil {
				t.Fatal(err)
			}
			if mode := strings.TrimSpace(string(during)); mode != "-rw-------" {
				t.Errorf("the backup while it is written: %q, want -rw-------", mode)
			}
			files := h.files(t)
			if len(files) != 1 || !strings.HasPrefix(files[0], "polyfin-20261005-040000.dump") {
				t.Fatalf("the folder: %q", files)
			}
			if info, err := os.Stat(filepath.Join(h.dir, files[0])); err != nil || info.Mode().Perm() != 0o600 {
				t.Errorf("the backup made: %v %v", info, err)
			}
		})
	}
}

// What a restore brings into the backups folder (a backup downloaded from
// a destination, one an administrator sends, the decrypted copy of an
// encrypted one) is made through copyTo: its owner's alone while it is
// written and after, and nothing is left of a copy that fails.
//
//nolint:paralleltest // sets the process's umask
func TestAFileCopiedForARestoreIsItsOwnersAlone(t *testing.T) {
	widenUmask(t)
	dir := t.TempDir()
	path := temporaryPath(dir)
	var during os.FileMode
	size, err := copyTo(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "PGDMP"); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err == nil {
			during = info.Mode().Perm()
		}
		return err
	})
	if err != nil || size != 5 {
		t.Fatalf("copy: %d %v", size, err)
	}
	if during != 0o600 {
		t.Errorf("the copy while it is written: %v, want -rw-------", during)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the copy made: %v %v", info, err)
	}
	failed := temporaryPath(dir)
	if _, err := copyTo(failed, func(io.Writer) error { return io.ErrUnexpectedEOF }); err == nil {
		t.Fatal("a copy that fails is reported")
	}
	if _, err := os.Lstat(failed); !os.IsNotExist(err) {
		t.Errorf("a copy that failed left its file: %v", err)
	}
}
