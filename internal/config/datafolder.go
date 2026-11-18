package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// dataFolderName is the default data folder's name in the configuration
// folder of Polyfin's user. Not polyfin: on macOS that folder is
// Application Support, where Polyfin Desktop keeps Polyfin, its own data
// folder, and the file system does not tell the two names apart.
const dataFolderName = "polyfin-server"

// DefaultDataDir is the folder Polyfin keeps its files in when
// POLYFIN_DATA_DIR is not set: polyfin-server in the configuration folder
// of the user it runs as, which the other accounts of the system cannot
// write into, as they can into the system's temporary folder.
func DefaultDataDir() (string, error) {
	dir, err := ownConfigDir(runtime.GOOS, os.UserConfigDir, user.Current)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dataFolderName), nil
}

// ownConfigDir is the configuration folder of the user Polyfin runs as on
// the system goos: the one fromEnvironment tells, os.UserConfigDir, and,
// where the environment does not tell it, as for a service started
// without HOME, the same folder of the home account gives, the user's in
// the system's user database. Windows has no such second source.
func ownConfigDir(goos string, fromEnvironment func() (string, error), account func() (*user.User, error)) (string, error) {
	dir, err := fromEnvironment()
	if err == nil && !filepath.IsAbs(dir) {
		err = fmt.Errorf("%q is not an absolute path", dir)
	}
	if err == nil {
		return dir, nil
	}
	if goos == "windows" {
		return "", err
	}
	current, lookupErr := account()
	if lookupErr != nil || !filepath.IsAbs(current.HomeDir) {
		return "", err
	}
	if goos == "darwin" {
		return filepath.Join(current.HomeDir, "Library", "Application Support"), nil
	}
	return filepath.Join(current.HomeDir, ".config"), nil
}

// EarlierDataDir is the default data folder of earlier versions: polyfin
// in the system's temporary folder, which every account of the system
// writes into.
func EarlierDataDir() string {
	return filepath.Join(os.TempDir(), "polyfin")
}

// chooseDataDir is the data folder of a Polyfin started without
// POLYFIN_DATA_DIR: the one own tells, DefaultDataDir, or earlier,
// EarlierDataDir, while something is there and nothing is at own, so that
// a server that kept its files there does not start without them. moveTo
// is then own, where its files are to be moved. Nothing is made nor
// judged here: PrepareDataFolder refuses an earlier folder that is not
// Polyfin's user's own.
func chooseDataDir(own func() (string, error), earlier string) (dir, moveTo string, err error) {
	dir, err = own()
	if err != nil {
		return "", "", fmt.Errorf("POLYFIN_DATA_DIR is not set, and the folder of Polyfin's user cannot be told: %w", err)
	}
	if isThere(dir) || !isThere(earlier) {
		return dir, "", nil
	}
	return earlier, dir, nil
}

// isThere tells whether something has the name path, a link that leads
// nowhere included; what cannot be told is taken to be there.
func isThere(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// DataFolderFault tells why a data folder is not the folder of Polyfin's
// user alone: another account of the system can change the backups
// Polyfin restores from it, the programs it runs from it and the files
// it serves from it.
type DataFolderFault struct {
	// Dir is the data folder.
	Dir string
	// Owner is the user that owns it and User the one Polyfin runs as:
	// the fault when they differ.
	Owner, User int
	// Mode is its permission bits, which let its group or others write
	// into it: the fault when Owner is User.
	Mode fs.FileMode
}

// OtherOwner tells whether the fault is the folder's owner, another user
// than the one Polyfin runs as, rather than its mode.
func (f *DataFolderFault) OtherOwner() bool { return f.Owner != f.User }

// String tells the fault in words, for the log and for a refusal.
func (f *DataFolderFault) String() string {
	if f.OtherOwner() {
		return fmt.Sprintf("belongs to another user (user %d; Polyfin runs as user %d)", f.Owner, f.User)
	}
	return fmt.Sprintf("can be written by its group or by others (mode %04o)", f.Mode)
}

// Command is the command that mends the fault.
func (f *DataFolderFault) Command() string {
	if f.OtherOwner() {
		return fmt.Sprintf("chown %d %q", f.User, f.Dir)
	}
	return fmt.Sprintf("chmod go-w %q", f.Dir)
}

// refuseChosen tells whether Polyfin refuses to start in a data folder an
// administrator chose, or in the default one found there, when it is a
// symbolic link, another user's, or writable by its group or by others,
// as it refuses the default folder of earlier versions. False: such a
// folder was set up on purpose, often on a system with no other account,
// so its fault is told in the log and in System › Health, and a link is
// followed. True makes every data folder as strict as the earlier
// default.
const refuseChosen = false

// PrepareDataFolder makes the data folder dir when it is missing, for
// Polyfin's user alone, and looks at one that is there. It must be a
// folder, or lead to one. A folder that is there keeps its mode: another
// account may read the recordings or the backups in it on purpose.
//
// A folder another user owns, or that its group or others can write
// into, is returned as a fault, for the log and System › Health to tell.
// The default folder of earlier versions is refused instead, and when it
// is a symbolic link: it is in the system's temporary folder, where
// another account can make it first, and nobody chose it.
//
// inContainer tells that Polyfin runs in a container. Its volume has the
// owner and the mode its host gave it, and the accounts that reach it are
// the host's, which Polyfin cannot tell: no fault is looked for there.
// Nor is one on Windows, which tells neither an owner nor a mode this way.
func PrepareDataFolder(dir string, inContainer bool) (*DataFolderFault, error) {
	earlier := dir == EarlierDataDir()
	fault, err := prepareDataFolder(dir, earlier || refuseChosen, os.Geteuid(), inContainer)
	if err == nil || !earlier {
		return fault, err
	}
	own, ownErr := DefaultDataDir()
	if ownErr != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w; it is the default data folder of earlier versions, which Polyfin uses until %q exists", err, own)
}

// prepareDataFolder is PrepareDataFolder for a Polyfin that runs as the
// user self; strict refuses a link and a fault, as for the default
// folder of earlier versions.
func prepareDataFolder(dir string, strict bool, self int, inContainer bool) (*DataFolderFault, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("the data folder %q cannot be made: %w", dir, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return nil, fmt.Errorf("the data folder %q cannot be looked at: %w", dir, err)
	}
	if strict && info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("the data folder %q is a symbolic link: name the folder it leads to in POLYFIN_DATA_DIR", dir)
	}
	// What a link leads to is the data folder; os.Stat tells a folder as
	// os.MkdirAll does, on Windows too.
	if info, err = os.Stat(dir); err != nil {
		return nil, fmt.Errorf("the data folder %q cannot be looked at: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("the data folder %q is not a folder", dir)
	}
	if inContainer {
		return nil, nil
	}
	fault := writableByAnother(dir, info, self)
	if fault != nil && strict {
		return nil, fmt.Errorf("the data folder %q %s: mend it with %s, or name another folder in POLYFIN_DATA_DIR", dir, fault, fault.Command())
	}
	return fault, nil
}
