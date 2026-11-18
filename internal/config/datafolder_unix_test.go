//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// mode is the permission bits of the folder path.
func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// folderOfMode makes a folder of the mode perm, whatever the umask.
func folderOfMode(t *testing.T, perm os.FileMode) string {
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

// linkTo makes a link to target, and returns it.
func linkTo(t *testing.T, target string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "data")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// A data folder an administrator chose, or the default one found there,
// never keeps Polyfin from starting for its owner, its mode or for being a
// link: it was set up on purpose. When another user owns it, or its group
// or others can write into it, the start tells what is wrong and the
// command that mends it; a link is followed, and what it leads to judged.
func TestAChosenDataFolderAnotherAccountCanChangeIsToldAndStarts(t *testing.T) {
	t.Parallel()
	self := os.Geteuid()
	for _, test := range []struct {
		name string
		dir  string
		self int
		// told and command are what the fault says and the command it
		// gives, less the folder; empty for no fault.
		told, command string
	}{
		{"a folder everyone writes into", folderOfMode(t, 0o777), self, "mode 0777", "chmod go-w "},
		{"a folder its group writes into", folderOfMode(t, 0o775), self, "mode 0775", "chmod go-w "},
		{"a folder others write into", folderOfMode(t, 0o757), self, "mode 0757", "chmod go-w "},
		{"a folder of another user", folderOfMode(t, 0o700), self + 1, "another user", "chown "},
		{"a folder of another user, for Polyfin as root", folderOfMode(t, 0o755), 0, "another user", "chown 0 "},
		{"a link to a folder of its own", linkTo(t, folderOfMode(t, 0o755)), self, "", ""},
		{"a link to a folder everyone writes into", linkTo(t, folderOfMode(t, 0o777)), self, "mode 0777", "chmod go-w "},
		{"a folder of its own", folderOfMode(t, 0o755), self, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			before, err := os.Stat(test.dir)
			if err != nil {
				t.Fatal(err)
			}
			fault, err := prepareDataFolder(test.dir, false, test.self, false)
			if err != nil {
				t.Fatalf("the start in %s is refused: %v", test.dir, err)
			}
			switch {
			case test.told == "" && fault != nil:
				t.Errorf("a fault is told of %s: %s", test.dir, fault)
			case test.told != "" && fault == nil:
				t.Errorf("no fault is told of %s; want %q", test.dir, test.told)
			case test.told != "" && (fault.Dir != test.dir || !strings.Contains(fault.String(), test.told) ||
				!strings.HasPrefix(fault.Command(), test.command) || !strings.Contains(fault.Command(), test.dir)):
				t.Errorf("the fault of %s: %q, mended with %q; want %q and %q", fault.Dir, fault, fault.Command(), test.told, test.command)
			}
			if after, err := os.Stat(test.dir); err != nil || after.Mode() != before.Mode() {
				t.Errorf("the folder was changed: %v, %v; it was %v", after.Mode(), err, before.Mode())
			}
		})
	}
}

// The strict rule, which the default folder of earlier versions gets and
// which refuseChosen gives every data folder: Polyfin does not start in a
// link, in a folder another user owns, or in one its group or others can
// write into. The message names the folder and what is wrong with it.
func TestTheDataFolderIsRefusedWhenAnotherAccountCouldChangeIt(t *testing.T) {
	t.Parallel()
	self := os.Geteuid()
	theirs := t.TempDir()
	for _, test := range []struct {
		name string
		dir  string
		self int
		want string
	}{
		{"a link to a folder", linkTo(t, theirs), self, "symbolic link"},
		{"a link that leads nowhere", linkTo(t, filepath.Join(theirs, "missing")), self, "symbolic link"},
		{"a folder everyone writes into", folderOfMode(t, 0o777), self, "mode 0777"},
		{"a folder its group writes into", folderOfMode(t, 0o775), self, "mode 0775"},
		{"a folder others write into", folderOfMode(t, 0o757), self, "mode 0757"},
		{"a folder of another user", folderOfMode(t, 0o700), self + 1, "another user"},
		{"a folder of another user, for Polyfin as root", folderOfMode(t, 0o755), 0, "another user"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fault, err := prepareDataFolder(test.dir, true, test.self, false)
			if err == nil || fault != nil || !strings.Contains(err.Error(), test.dir) || !strings.Contains(err.Error(), test.want) {
				t.Errorf("the start in %s: %v, %v; want a refusal that names it and tells %q", test.dir, fault, err, test.want)
			}
		})
	}
	if entries, err := os.ReadDir(theirs); err != nil || len(entries) != 0 {
		t.Errorf("the folder the link leads to was written into: %v %v", entries, err)
	}
}

// A data folder Polyfin makes is for its user alone, with the folders it
// makes above it. One that is there keeps the read bits it has: another
// account may read the recordings in it on purpose. What is no folder, and
// a link that leads to none, stop the start whoever chose them.
func TestADataFolderPolyfinMakesIsItsUsersAlone(t *testing.T) {
	t.Parallel()
	self := os.Geteuid()
	base := t.TempDir()
	dir := filepath.Join(base, "config", "polyfin-server")
	if fault, err := prepareDataFolder(dir, false, self, false); err != nil || fault != nil {
		t.Fatalf("a missing folder: %v, %v", fault, err)
	}
	for _, made := range []string{dir, filepath.Dir(dir)} {
		if got := mode(t, made); got != 0o700 {
			t.Errorf("%s was made with mode %04o, want 0700", made, got)
		}
	}
	for _, strict := range []bool{false, true} {
		for _, perm := range []os.FileMode{0o755, 0o750, 0o700} {
			existing := folderOfMode(t, perm)
			if fault, err := prepareDataFolder(existing, strict, self, false); err != nil || fault != nil {
				t.Errorf("a folder of mode %04o, the user's own: %v, %v (strict %v)", perm, fault, err, strict)
			}
			if got := mode(t, existing); got != perm {
				t.Errorf("a folder of mode %04o became %04o", perm, got)
			}
		}
		file := filepath.Join(t.TempDir(), "data")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		for name, path := range map[string]string{"a file": file, "a link to a file": linkTo(t, file), "a link that leads nowhere": linkTo(t, filepath.Join(base, "missing"))} {
			if fault, err := prepareDataFolder(path, strict, self, false); err == nil || fault != nil || !strings.Contains(err.Error(), path) {
				t.Errorf("%s: %v, %v; want a refusal that names %s (strict %v)", name, fault, err, path, strict)
			}
		}
	}
	if isThere(filepath.Join(base, "missing")) {
		t.Error("the folder a link leads to was made")
	}
}

// In a container the volume has the owner and the mode its host gave it:
// neither is judged there, so nothing is told and the Docker image starts
// on any volume. The default folder of earlier versions is still not used
// through a link.
func TestInAContainerTheDataFolderIsNotJudged(t *testing.T) {
	t.Parallel()
	self := os.Geteuid()
	for _, strict := range []bool{false, true} {
		for _, perm := range []os.FileMode{0o777, 0o775, 0o755} {
			dir := folderOfMode(t, perm)
			if fault, err := prepareDataFolder(dir, strict, self+1, true); err != nil || fault != nil {
				t.Errorf("a volume of mode %04o and of another owner: %v, %v (strict %v)", perm, fault, err, strict)
			}
			if got := mode(t, dir); got != perm {
				t.Errorf("a volume of mode %04o became %04o", perm, got)
			}
		}
	}
	if fault, err := prepareDataFolder(linkTo(t, folderOfMode(t, 0o777)), false, self, true); err != nil || fault != nil {
		t.Errorf("a link to a volume: %v, %v", fault, err)
	}
	if _, err := prepareDataFolder(linkTo(t, t.TempDir()), true, self, true); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("the earlier default, a link: %v", err)
	}
}

// The default folder of earlier versions is in the temporary folder, where
// another account can make it first, and nobody chose it. Polyfin started
// without POLYFIN_DATA_DIR then neither starts in it, nor writes into what
// it leads to, nor starts on an empty folder of its own without a word.
func TestAnEarlierDataFolderOfAnotherAccountStopsTheStart(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		make func(t *testing.T, earlier, theirs string)
		self int
		want string
	}{
		{"a link to a folder of theirs", func(t *testing.T, earlier, theirs string) {
			if err := os.Symlink(theirs, earlier); err != nil {
				t.Fatal(err)
			}
		}, os.Geteuid(), "symbolic link"},
		{"a folder of theirs", func(t *testing.T, earlier, _ string) {
			if err := os.Mkdir(earlier, 0o755); err != nil {
				t.Fatal(err)
			}
		}, os.Geteuid() + 1, "another user"},
		{"a folder everyone writes into", func(t *testing.T, earlier, _ string) {
			if err := os.Mkdir(earlier, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(earlier, 0o777); err != nil {
				t.Fatal(err)
			}
		}, os.Geteuid(), "mode 0777"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// shared stands for the temporary folder, home for the
			// folder of Polyfin's user.
			shared, theirs, home := t.TempDir(), t.TempDir(), t.TempDir()
			earlier, own := filepath.Join(shared, "polyfin"), filepath.Join(home, "polyfin-server")
			test.make(t, earlier, theirs)
			dir, moveTo, err := chooseDataDir(func() (string, error) { return own, nil }, earlier)
			if err != nil || dir != earlier || moveTo != own {
				t.Fatalf("the data folder: %q, to move to %q, %v; want %q, to move to %q", dir, moveTo, err, earlier, own)
			}
			// What the start does with the data folder, then with its
			// default backups folder.
			_, err = prepareDataFolder(dir, true, test.self, false)
			if err == nil {
				err = PrepareFolder(accounts.DefaultBackupFolder(dir), true)
				t.Fatalf("Polyfin starts in %s (its backups folder: %v)", dir, err)
			}
			if !strings.Contains(err.Error(), earlier) || !strings.Contains(err.Error(), test.want) {
				t.Errorf("the refusal: %v; want one that names %s and tells %q", err, earlier, test.want)
			}
			if entries, err := os.ReadDir(theirs); err != nil || len(entries) != 0 {
				t.Errorf("the other account's folder was written into: %v %v", entries, err)
			}
			if isThere(own) {
				t.Error("a new, empty data folder was made instead")
			}
		})
	}
}

// The default backups folder is for Polyfin's user alone when Polyfin
// makes it: the backups hold the whole database. One that is there, and a
// folder an administrator chose, keep their mode.
func TestTheDefaultBackupsFolderIsItsUsersAlone(t *testing.T) {
	t.Parallel()
	data := folderOfMode(t, 0o755)
	backups := accounts.DefaultBackupFolder(data)
	if err := PrepareFolder(backups, true); err != nil {
		t.Fatal(err)
	}
	if got := mode(t, backups); got != 0o700 {
		t.Errorf("the default backups folder was made with mode %04o, want 0700", got)
	}
	if got := mode(t, data); got != 0o755 {
		t.Errorf("the data folder above it became %04o", got)
	}
	for _, create := range []bool{true, false} {
		chosen := folderOfMode(t, 0o755)
		if err := PrepareFolder(chosen, create); err != nil {
			t.Fatal(err)
		}
		if got := mode(t, chosen); got != 0o755 {
			t.Errorf("a folder that was there became %04o (create %v)", got, create)
		}
	}
}
