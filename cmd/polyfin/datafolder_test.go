//go:build !windows

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/config"
)

// chosenFolder is a data folder of the mode perm, as an administrator may
// have set one up.
func chosenFolder(t *testing.T, perm os.FileMode) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, perm); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A data folder that was chosen never keeps Polyfin from starting for its
// mode or for being a link. One its group or others can write into is
// told in the log, with the command that mends it, and given to System ›
// Health; a link to a folder of Polyfin's own is told nowhere.
func TestServeTellsAChosenDataFolderOthersCanWriteInto(t *testing.T) {
	t.Parallel()
	for _, perm := range []os.FileMode{0o777, 0o775} {
		dir := chosenFolder(t, perm)
		var logs syncBuffer
		fault, err := prepareDataFolder(config.Config{DataDir: dir}, slog.New(slog.NewTextHandler(&logs, nil)))
		if err != nil {
			t.Fatalf("the start in a folder of mode %04o is refused: %v", perm, err)
		}
		if fault == nil || fault.Dir != dir || fault.OtherOwner() || fault.Mode != perm {
			t.Errorf("mode %04o: the fault for System › Health is %+v", perm, fault)
		}
		told := logs.String()
		if strings.Count(told, "level=WARN") != 1 || !strings.Contains(told, dir) || !strings.Contains(told, "chmod go-w") {
			t.Errorf("mode %04o: want one warning that names %s and the command:\n%s", perm, dir, told)
		}
	}

	link := filepath.Join(t.TempDir(), "data")
	if err := os.Symlink(chosenFolder(t, 0o755), link); err != nil {
		t.Fatal(err)
	}
	var logs syncBuffer
	fault, err := prepareDataFolder(config.Config{DataDir: link}, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil || fault != nil || logs.String() != "" {
		t.Errorf("a link to a folder of its own: %+v, %v, and in the log:\n%s", fault, err, logs.String())
	}
}

// A data folder Polyfin makes is for its user alone and told nowhere; the
// default one of earlier versions, kept as it is there, is told in the
// log with the folder to move it to; what is no folder stops the start.
func TestServeStartsOnlyInADataFolderOfItsOwn(t *testing.T) {
	t.Parallel()
	var logs syncBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	own := filepath.Join(t.TempDir(), "config", "polyfin-server")
	if fault, err := prepareDataFolder(config.Config{DataDir: own}, logger); err != nil || fault != nil {
		t.Fatalf("a missing folder: %+v, %v", fault, err)
	}
	if info, err := os.Stat(own); err != nil {
		t.Error(err)
	} else if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("the data folder Polyfin made has mode %04o, want 0700", perm)
	}
	if logs.String() != "" {
		t.Errorf("a data folder of its own is told in the log:\n%s", logs.String())
	}

	file := filepath.Join(own, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDataFolder(config.Config{DataDir: file}, logger); err == nil || !strings.Contains(err.Error(), file) {
		t.Errorf("a file: %v; want a refusal that names %s", err, file)
	}

	earlier := chosenFolder(t, 0o700)
	if fault, err := prepareDataFolder(config.Config{DataDir: earlier, MoveDataDirTo: own}, logger); err != nil || fault != nil {
		t.Fatalf("the earlier default folder: %+v, %v", fault, err)
	}
	if told := logs.String(); strings.Count(told, "level=WARN") != 1 || !strings.Contains(told, earlier) || !strings.Contains(told, own) {
		t.Errorf("the earlier default folder is not told with the one to move it to:\n%s", told)
	}
}

// The database Polyfin runs itself is in the data folder: it is neither
// made nor started where the data folder is no folder.
func TestTheEmbeddedDatabaseIsNotStartedWhereTheDataFolderIsNoFolder(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// No server program is looked for: the folder is refused first.
	_, err := startEmbedded(context.Background(), t.TempDir(), file, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("the start: %v; want a refusal that names %s", err, file)
	}
}
