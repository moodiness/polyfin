// Package desktop holds what the parts of Polyfin Desktop share: the
// install folder it runs its programs from, the data folder it keeps its
// files in, and the system's ways of opening an address or a folder.
//
// Nothing here needs cgo: only the tray of cmd/polyfin-desktop does, on
// macOS.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Environment variables of the app itself, which the server never reads.
const (
	// InstallDirVariable names an install folder other than the app's
	// own, for development and tests: a folder laid out as Install says.
	InstallDirVariable = "POLYFIN_DESKTOP_INSTALL_DIR"
	// DataDirVariable names a data folder other than the system's usual
	// place, as an absolute path.
	DataDirVariable = "POLYFIN_DESKTOP_DATA_DIR"
)

// Install is the folder the packages put the app's programs in: the
// folder of its executable, or on macOS the bundle's Contents/Resources.
// It holds polyfin (polyfin.exe), postgresql/{bin,lib,share},
// jellyfin-web and fonts.
type Install struct {
	Dir string
}

// Executable is name with the system's executable suffix.
func Executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Server is the polyfin executable the app runs with polyfin serve.
func (i Install) Server() string { return filepath.Join(i.Dir, Executable("polyfin")) }

// PostgresBin is the folder of PostgreSQL's programs: postgres, initdb,
// pg_ctl, and the pg_dump and pg_restore backups use.
func (i Install) PostgresBin() string { return filepath.Join(i.Dir, "postgresql", "bin") }

// WebDir is jellyfin-web, the web client.
func (i Install) WebDir() string { return filepath.Join(i.Dir, "jellyfin-web") }

// FontsDir is the fallback fonts of subtitles and generated images.
func (i Install) FontsDir() string { return filepath.Join(i.Dir, "fonts") }

// Check tells what the install folder lacks for the app to run: the
// server and PostgreSQL's programs. jellyfin-web and the fonts are
// optional, as for the server.
func (i Install) Check() error {
	var missing []string
	for _, path := range []string{i.Server(), filepath.Join(i.PostgresBin(), Executable("pg_ctl")),
		filepath.Join(i.PostgresBin(), Executable("initdb")), filepath.Join(i.PostgresBin(), Executable("postgres"))} {
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the install folder %s lacks %s", i.Dir, strings.Join(missing, ", "))
	}
	return nil
}

// FindInstall returns the install folder: the one InstallDirVariable
// names, else the app's own.
func FindInstall() (Install, error) {
	if dir := os.Getenv(InstallDirVariable); dir != "" {
		abs, err := filepath.Abs(dir)
		return Install{Dir: abs}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return Install{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Install{Dir: installDir(exe, runtime.GOOS)}, nil
}

// installDir is the install folder of the app whose executable is exe:
// on macOS, inside a bundle, its Contents/Resources. macOS and Linux paths
// are read with slashes whatever the host, as tests of every system run
// everywhere.
func installDir(exe, goos string) string {
	if goos == "windows" {
		return filepath.Dir(exe)
	}
	dir := path.Dir(exe)
	if goos == "darwin" && path.Base(dir) == "MacOS" && path.Base(path.Dir(dir)) == "Contents" {
		return path.Join(path.Dir(dir), "Resources")
	}
	return dir
}

// Bundle is the macOS app bundle exe is in, such as
// /Applications/Polyfin.app, or "" when it is in none.
func Bundle(exe string) string {
	dir := path.Dir(exe)
	contents := path.Dir(dir)
	bundle := path.Dir(contents)
	if path.Base(dir) == "MacOS" && path.Base(contents) == "Contents" && strings.HasSuffix(bundle, ".app") {
		return bundle
	}
	return ""
}

// Folders is the data folder and what the app keeps in it. The server's
// POLYFIN_DATA_DIR is the data folder itself: its cache, recordings and
// backups go below it.
type Folders struct {
	Data string
}

// Cluster is PostgreSQL's data folder.
func (f Folders) Cluster() string { return filepath.Join(f.Data, "postgresql") }

// Tools is where FFmpeg and ffprobe are downloaded.
func (f Folders) Tools() string { return filepath.Join(f.Data, "tools") }

// Log is the folder of the logs: the app's, the server's and PostgreSQL's.
func (f Folders) Log() string { return filepath.Join(f.Data, "log") }

// Updates is where a new release is downloaded before it is installed.
func (f Folders) Updates() string { return filepath.Join(f.Data, "updates") }

// State is the file of the app's own choices, such as Start with the
// computer.
func (f Folders) State() string { return filepath.Join(f.Data, "desktop.json") }

// Lock is the file that keeps a second app from running on the same
// data folder.
func (f Folders) Lock() string { return filepath.Join(f.Data, "desktop.lock") }

// ServerPID is the file that names the server process the app started,
// to stop one a crash of the app left running.
func (f Folders) ServerPID() string { return filepath.Join(f.Data, "polyfin.pid") }

// Instance is the file that names the app running on the data folder,
// while it runs: its PID and the token a request to quit must carry.
func (f Folders) Instance() string { return filepath.Join(f.Data, "desktop.pid") }

// QuitRequest is the file polyfin-desktop -quit writes to ask the
// running app to quit.
func (f Folders) QuitRequest() string { return filepath.Join(f.Data, "desktop.quit") }

// Offer is the file the app writes the update it offers to, for the
// server to offer it in the admin app (see Offered).
func (f Folders) Offer() string { return filepath.Join(f.Data, "desktop-update.json") }

// InstallRequest is the file the server writes to ask the running app to
// install the update it offers (see RequestInstall).
func (f Folders) InstallRequest() string { return filepath.Join(f.Data, "desktop.install") }

// PendingUpdate is the file of an update installed whose new version has
// not answered yet: where the previous app and the copy of the database
// are kept until it does (see update.Pending).
func (f Folders) PendingUpdate() string { return filepath.Join(f.Data, "desktop-update-pending.json") }

// RolledBack is the file that tells the app went back to its previous
// version (see RolledBack).
func (f Folders) RolledBack() string { return filepath.Join(f.Data, "desktop-update-rollback.json") }

// PreviousDatabase is the copy of the cluster's folder an update keeps
// until the new version answers.
func (f Folders) PreviousDatabase() string { return filepath.Join(f.Updates(), "postgresql.previous") }

// Make creates the data folder and the log folder, readable by the user
// only.
func (f Folders) Make() error {
	for _, dir := range []string{f.Data, f.Log()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// FindFolders returns the data folder: the one DataDirVariable names,
// else the system's usual place.
func FindFolders() (Folders, error) {
	if dir := os.Getenv(DataDirVariable); dir != "" {
		if !filepath.IsAbs(dir) {
			return Folders{}, fmt.Errorf("%s must be an absolute path, not %q", DataDirVariable, dir)
		}
		return Folders{Data: filepath.Clean(dir)}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Folders{}, err
	}
	dir, err := dataDir(runtime.GOOS, os.Getenv, home)
	return Folders{Data: dir}, err
}

// dataDir is the system's usual place for the data folder:
// %LOCALAPPDATA%\Polyfin on Windows, ~/Library/Application Support/Polyfin
// on macOS, $XDG_DATA_HOME/polyfin, else ~/.local/share/polyfin, on Linux.
func dataDir(goos string, env func(string) string, home string) (string, error) {
	switch goos {
	case "windows":
		local := env("LOCALAPPDATA")
		if local == "" {
			return "", errors.New("LOCALAPPDATA is not set")
		}
		return filepath.Join(local, "Polyfin"), nil
	case "darwin":
		return path.Join(home, "Library", "Application Support", "Polyfin"), nil
	default:
		// The XDG specification ignores a relative XDG_DATA_HOME.
		if xdg := env("XDG_DATA_HOME"); xdg != "" && path.IsAbs(xdg) {
			return path.Join(xdg, "polyfin"), nil
		}
		return path.Join(home, ".local", "share", "polyfin"), nil
	}
}
