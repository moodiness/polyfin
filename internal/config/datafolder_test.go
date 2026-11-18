package config

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// under tells whether path is in the folder dir.
func under(path, dir string) bool {
	relative, err := filepath.Rel(filepath.Clean(dir), path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Without POLYFIN_DATA_DIR, Polyfin keeps its files in a folder of the
// user it runs as, never in the system's temporary folder, which every
// account writes into; nothing is made by choosing it.
func TestTheDefaultDataFolderIsTheUsersOwn(t *testing.T) {
	t.Parallel()
	own, err := DefaultDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(own) || filepath.Base(own) != "polyfin-server" || under(own, os.TempDir()) {
		t.Errorf("the default data folder is %q, with the temporary folder at %q", own, os.TempDir())
	}
	if configuration, err := os.UserConfigDir(); err == nil && !under(own, configuration) {
		t.Errorf("the default data folder, %q, is not in the user's own, %q", own, configuration)
	}

	// Load gives it, unless this system still has the folder of earlier
	// versions and not yet this one.
	cfg, err := Load(with(nil))
	if err != nil {
		t.Fatal(err)
	}
	if kept := !isThere(own) && isThere(EarlierDataDir()); !kept && (cfg.DataDir != own || cfg.MoveDataDirTo != "") {
		t.Errorf("Load: the data folder is %q, to move to %q; want %q", cfg.DataDir, cfg.MoveDataDirTo, own)
	} else if kept && (cfg.DataDir != EarlierDataDir() || cfg.MoveDataDirTo != own) {
		t.Errorf("Load: the data folder is %q, to move to %q; want %q, to move to %q", cfg.DataDir, cfg.MoveDataDirTo, EarlierDataDir(), own)
	}

	// With nothing at either place, as on a new system.
	home, shared := t.TempDir(), t.TempDir()
	fresh := filepath.Join(home, "polyfin-server")
	dir, moveTo, err := chooseDataDir(func() (string, error) { return fresh, nil }, filepath.Join(shared, "polyfin"))
	if err != nil || dir != fresh || moveTo != "" {
		t.Errorf("a new system: %q, to move to %q, %v; want %q", dir, moveTo, err, fresh)
	}
	if isThere(fresh) || isThere(filepath.Join(shared, "polyfin")) {
		t.Error("choosing the data folder made one")
	}

	// A user whose folder cannot be told gets no folder of the temporary
	// one: Polyfin does not start.
	if dir, _, err := chooseDataDir(func() (string, error) { return "", errors.New("$HOME is not defined") }, filepath.Join(shared, "polyfin")); err == nil ||
		!strings.Contains(err.Error(), "POLYFIN_DATA_DIR") {
		t.Errorf("a user whose folder cannot be told: %q, %v", dir, err)
	}
}

// A server that kept its files in the default folder of earlier versions,
// in the temporary folder, keeps them there, with the folder to move them
// to, until the new default folder exists.
func TestAnEarlierDataFolderIsKeptUntilItIsMoved(t *testing.T) {
	t.Parallel()
	home, shared := t.TempDir(), t.TempDir()
	own, earlier := filepath.Join(home, "polyfin-server"), filepath.Join(shared, "polyfin")
	if err := os.Mkdir(earlier, 0o700); err != nil {
		t.Fatal(err)
	}
	choose := func() (string, string, error) {
		return chooseDataDir(func() (string, error) { return own, nil }, earlier)
	}
	dir, moveTo, err := choose()
	if err != nil || dir != earlier || moveTo != own {
		t.Fatalf("with the earlier folder alone: %q, to move to %q, %v; want %q, to move to %q", dir, moveTo, err, earlier, own)
	}
	if fault, err := prepareDataFolder(dir, true, os.Geteuid(), false); err != nil || fault != nil {
		t.Errorf("the earlier folder, the user's own, is refused: %v", err)
	}
	if isThere(own) {
		t.Error("the new folder was made beside the earlier one")
	}

	// Moved, or with a new folder made by hand: the earlier one is no
	// longer looked at.
	if err := os.Mkdir(own, 0o700); err != nil {
		t.Fatal(err)
	}
	if dir, moveTo, err := choose(); err != nil || dir != own || moveTo != "" {
		t.Errorf("with both folders: %q, to move to %q, %v; want %q", dir, moveTo, err, own)
	}
}

// A service started without HOME still has a folder of its own: the one
// of its home in the system's user database.
func TestTheUsersFolderIsToldWithoutItsEnvironment(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	told := func() (string, error) { return filepath.Join(home, "told"), nil }
	untold := func() (string, error) { return "", errors.New("$HOME is not defined") }
	account := func() (*user.User, error) { return &user.User{HomeDir: home}, nil }
	unknown := func() (*user.User, error) { return nil, user.UnknownUserIdError(65532) }
	for _, test := range []struct {
		name            string
		goos            string
		fromEnvironment func() (string, error)
		account         func() (*user.User, error)
		want            string
	}{
		{"told by the environment", "linux", told, unknown, filepath.Join(home, "told")},
		{"a service on Linux", "linux", untold, account, filepath.Join(home, ".config")},
		{"a service on macOS", "darwin", untold, account, filepath.Join(home, "Library", "Application Support")},
		{"Windows has no second source", "windows", untold, account, ""},
		{"a user the system does not know", "linux", untold, unknown, ""},
		{"a home that is not a path", "linux", untold, func() (*user.User, error) { return &user.User{HomeDir: "home"}, nil }, ""},
		{"a folder that is not a path", "linux", func() (string, error) { return filepath.Join("home", ".config"), nil }, account, filepath.Join(home, ".config")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir, err := ownConfigDir(test.goos, test.fromEnvironment, test.account)
			if dir != test.want || (err == nil) != (test.want != "") {
				t.Errorf("%q, %v; want %q", dir, err, test.want)
			}
		})
	}
}
