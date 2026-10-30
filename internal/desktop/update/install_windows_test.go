package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// fakeInstaller is a copy of the test binary which, started, writes how
// it was asked to show its window, then ends.
const fakeInstaller = "Polyfin-Desktop-0.43.0-windows-x64-setup.exe"

func init() {
	exe, err := os.Executable()
	if err != nil || filepath.Base(exe) != fakeInstaller {
		return
	}
	var info windows.StartupInfo
	_ = windows.GetStartupInfo(&info)
	report := fmt.Appendf(nil, "%d %d", info.Flags, info.ShowWindow)
	if os.WriteFile(exe+".part", report, 0o600) == nil {
		_ = os.Rename(exe+".part", exe+".txt")
	}
	os.Exit(0)
}

// The installer starts at once, its wizard shown: started hidden, it
// would wait unseen for a click once the app has quit. The install folder
// is kept beside it first.
func TestInstallerIsShown(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "polyfin-installer-")
	if err != nil {
		t.Fatal(err)
	}
	// Windows refuses to remove a program that runs.
	t.Cleanup(func() {
		deadline := time.Now().Add(30 * time.Second)
		for os.RemoveAll(dir) != nil && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
		_ = os.RemoveAll(dir + ".old")
	})
	pkg := filepath.Join(dir, fakeInstaller)
	if err := copyFile(self, pkg); err != nil {
		t.Fatal(err)
	}
	installed, err := Install(context.Background(), pkg, Installer, filepath.Join(dir, "polyfin-desktop.exe"))
	if err != nil || installed.Start != nil {
		t.Fatalf("Install = %+v, %v: the installer should have started", installed, err)
	}
	if _, err := os.Stat(filepath.Join(installed.Previous, fakeInstaller)); err != nil || installed.App != dir {
		t.Errorf("the install folder was not kept: %+v, %v", installed, err)
	}
	deadline := time.Now().Add(time.Minute)
	for {
		data, err := os.ReadFile(pkg + ".txt")
		if err == nil {
			var flags uint32
			var show uint16
			if _, err := fmt.Sscan(string(data), &flags, &show); err != nil {
				t.Fatalf("report %q: %v", data, err)
			}
			if flags&windows.STARTF_USESHOWWINDOW != 0 && show == windows.SW_HIDE {
				t.Errorf("the installer started hidden: flags %#x, show state %d", flags, show)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the installer did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}
