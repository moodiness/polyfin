package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/moodiness/polyfin/internal/desktop"
)

// write lays out files, by path, with their contents.
func write(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gone(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s is left: %v", path, err)
		}
	}
}

// Going back puts the previous app, a file such as an AppImage or a
// folder such as Windows' install folder, and the database's copy back
// in place of the new ones, which are removed, then forgets the update.
func TestRestorePutsThePreviousBack(t *testing.T) {
	for _, kind := range []Kind{AppImage, Installer} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			f := desktop.Folders{Data: filepath.Join(dir, "data")}
			cluster := f.Cluster()
			app := filepath.Join(dir, "Polyfin.AppImage")
			exe := app
			files := map[string]string{
				filepath.Join(cluster, "PG_VERSION"):              "18",
				filepath.Join(cluster, "base", "migrated"):        "by the new version",
				filepath.Join(f.PreviousDatabase(), "PG_VERSION"): "18",
				filepath.Join(f.PreviousDatabase(), "base", "1"):  "as the previous version left it",
			}
			if kind == Installer {
				app = filepath.Join(dir, "Polyfin")
				exe = filepath.Join(app, "polyfin-desktop.exe")
				files[exe] = "new app"
				files[filepath.Join(app, "jellyfin-web", "new.js")] = "new"
				files[filepath.Join(app+".old", "polyfin-desktop.exe")] = "previous app"
				files[filepath.Join(app+".old", "jellyfin-web", "old.js")] = "old"
			} else {
				files[app] = "new app"
				files[app+".old"] = "previous app"
			}
			write(t, files)
			pending := Pending{From: "0.42.0", To: "0.43.0", Kind: kind, App: app, Previous: app + ".old", Database: f.PreviousDatabase(), Starts: MaxStarts}
			if err := pending.Write(f); err != nil {
				t.Fatal(err)
			}
			cmd, err := pending.Restore(f, cluster, exe)
			if err != nil {
				t.Fatal(err)
			}
			if got := read(t, exe); got != "previous app" {
				t.Errorf("the app holds %q", got)
			}
			if kind == Installer {
				if got := read(t, filepath.Join(app, "jellyfin-web", "old.js")); got != "old" {
					t.Errorf("the previous app's files: %q", got)
				}
				gone(t, filepath.Join(app, "jellyfin-web", "new.js"))
			}
			if got := read(t, filepath.Join(cluster, "base", "1")); got != "as the previous version left it" {
				t.Errorf("the database holds %q", got)
			}
			gone(t, filepath.Join(cluster, "base", "migrated"), f.PreviousDatabase(), app+".old", app+".broken", cluster+".broken",
				f.PendingUpdate())
			if cmd == nil || filepath.Base(cmd.Path) != filepath.Base(exe) {
				t.Errorf("what starts the previous app: %v", cmd)
			}
			if again, err := ReadPending(f); err != nil || again != nil {
				t.Errorf("the update is still pending: %+v, %v", again, err)
			}
		})
	}
}

// Once the new version answers, what was kept goes, but for a copy that
// holds the running app.
func TestConfirmRemovesWhatWasKept(t *testing.T) {
	dir := t.TempDir()
	f := desktop.Folders{Data: filepath.Join(dir, "data")}
	app := filepath.Join(dir, "Polyfin.app")
	write(t, map[string]string{
		filepath.Join(app, "Contents", "Info.plist"):          "new",
		filepath.Join(app+".old", "Contents", "Info.plist"):   "old",
		filepath.Join(f.PreviousDatabase(), "PG_VERSION"):     "18",
		filepath.Join(f.Cluster(), "PG_VERSION"):              "18",
		filepath.Join(app+".old", "Contents", "MacOS", "app"): "old",
	})
	pending := Pending{From: "0.42.0", To: "0.43.0", Kind: DMG, App: app, Previous: app + ".old", Database: f.PreviousDatabase()}
	if err := pending.Write(f); err != nil {
		t.Fatal(err)
	}
	running := filepath.Join(app+".old", "Contents", "MacOS", "app")
	if err := pending.Confirm(f, running); err != nil {
		t.Fatal(err)
	}
	if got := read(t, running); got != "old" {
		t.Errorf("the running app's bundle was removed: %q", got)
	}
	gone(t, f.PreviousDatabase(), f.PendingUpdate())
	if err := pending.Confirm(f, filepath.Join(app, "Contents", "MacOS", "app")); err != nil {
		t.Fatal(err)
	}
	gone(t, app+".old")
	if got := read(t, filepath.Join(f.Cluster(), "PG_VERSION")); got != "18" {
		t.Errorf("the database: %q", got)
	}
}

// A copy keeps folders, files with their permissions, and links.
func TestCopyTree(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	write(t, map[string]string{filepath.Join(src, "a", "b"): "b"})
	if err := os.Chmod(filepath.Join(src, "a", "b"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a/b", filepath.Join(src, "link")); err != nil {
		t.Skipf("no symbolic links: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	write(t, map[string]string{filepath.Join(dst, "stale"): "x"})
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dst, "link")); got != "b" {
		t.Errorf("through the link: %q", got)
	}
	// Windows has no such permissions.
	if info, err := os.Stat(filepath.Join(dst, "a", "b")); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("the file's mode: %v, %v", info, err)
	}
	gone(t, filepath.Join(dst, "stale"))
}
