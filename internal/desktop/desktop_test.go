package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Each system's usual place for the data folder.
func TestDataDir(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	for _, c := range []struct {
		goos string
		env  map[string]string
		want string
	}{
		{"windows", map[string]string{"LOCALAPPDATA": `C:\Users\Alex\AppData\Local`}, filepath.Join(`C:\Users\Alex\AppData\Local`, "Polyfin")},
		{"darwin", nil, "/Users/alex/Library/Application Support/Polyfin"},
		{"linux", map[string]string{"XDG_DATA_HOME": "/data/xdg"}, "/data/xdg/polyfin"},
		{"linux", map[string]string{"XDG_DATA_HOME": "relative"}, "/Users/alex/.local/share/polyfin"},
		{"linux", nil, "/Users/alex/.local/share/polyfin"},
	} {
		got, err := dataDir(c.goos, env(c.env), "/Users/alex")
		if err != nil || got != c.want {
			t.Errorf("dataDir(%s, %v) = %q, %v; want %q", c.goos, c.env, got, err, c.want)
		}
	}
	if _, err := dataDir("windows", env(nil), "/"); err == nil {
		t.Error("Windows without LOCALAPPDATA gave a folder")
	}
}

func TestFindFolders(t *testing.T) {
	t.Setenv(DataDirVariable, "relative/data")
	if _, err := FindFolders(); err == nil {
		t.Error("a relative data folder was taken")
	}
	dir := t.TempDir()
	t.Setenv(DataDirVariable, dir)
	folders, err := FindFolders()
	if err != nil || folders.Data != dir || folders.Cluster() != filepath.Join(dir, "postgresql") || folders.Tools() != filepath.Join(dir, "tools") {
		t.Fatalf("FindFolders = %+v, %v", folders, err)
	}
	if err := folders.Make(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folders.Log()); err != nil {
		t.Errorf("no log folder: %v", err)
	}
}

// The install folder is the executable's, or on macOS its bundle's
// Resources; one that lacks the server or PostgreSQL is told.
func TestInstall(t *testing.T) {
	if got := installDir("/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop", "darwin"); got != "/Applications/Polyfin.app/Contents/Resources" {
		t.Errorf("bundle install folder = %q", got)
	}
	if got := installDir("/opt/polyfin/polyfin-desktop", "linux"); got != "/opt/polyfin" {
		t.Errorf("install folder = %q", got)
	}
	if got := installDir(`C:\Program Files\Polyfin\polyfin-desktop.exe`, "windows"); runtime.GOOS == "windows" && got != `C:\Program Files\Polyfin` {
		t.Errorf("Windows install folder = %q", got)
	}
	if got := Bundle("/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop"); got != "/Applications/Polyfin.app" {
		t.Errorf("Bundle = %q", got)
	}
	if got := Bundle("/Users/alex/dev/polyfin-desktop"); got != "" {
		t.Errorf("Bundle outside one = %q", got)
	}
	dir := t.TempDir()
	t.Setenv(InstallDirVariable, dir)
	install, err := FindInstall()
	if err != nil || install.Dir != dir {
		t.Fatalf("FindInstall = %+v, %v", install, err)
	}
	if err := install.Check(); err == nil || !strings.Contains(err.Error(), Executable("polyfin")) {
		t.Errorf("Check of an empty folder = %v", err)
	}
	for _, path := range []string{install.Server(), filepath.Join(install.PostgresBin(), Executable("pg_ctl")),
		filepath.Join(install.PostgresBin(), Executable("initdb")), filepath.Join(install.PostgresBin(), Executable("postgres"))} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := install.Check(); err != nil {
		t.Errorf("Check of a complete folder = %v", err)
	}
}

func TestOpenerAndClipboard(t *testing.T) {
	for _, c := range []struct {
		goos string
		dir  bool
		want string
	}{
		{"darwin", false, "open http://localhost:8096/admin/"},
		{"windows", false, "rundll32 url.dll,FileProtocolHandler http://localhost:8096/admin/"},
		{"windows", true, "explorer http://localhost:8096/admin/"},
		{"linux", false, "xdg-open http://localhost:8096/admin/"},
	} {
		name, args := opener(c.goos, "http://localhost:8096/admin/", c.dir)
		if got := name + " " + strings.Join(args, " "); got != c.want {
			t.Errorf("opener(%s) = %q", c.goos, got)
		}
	}
	if clipboard("darwin")[0][0] != "pbcopy" || clipboard("windows")[0][0] != "clip" || len(clipboard("linux")) != 3 {
		t.Error("clipboard programs")
	}
	for locale, want := range map[string]string{"fr_FR.UTF-8": "fr", "fr-CA": "fr", "en_US.UTF-8": "en", "de_DE": "en", "": "en"} {
		if got := languageOf(locale); got != want {
			t.Errorf("languageOf(%q) = %q", locale, got)
		}
	}
}
