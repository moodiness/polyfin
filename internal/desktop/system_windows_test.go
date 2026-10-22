package desktop

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

// startupReporter is a copy of the test binary which, started, writes how
// it was asked to show its window, then ends.
const startupReporter = "startup-reporter.exe"

func init() {
	exe, err := os.Executable()
	if err != nil || filepath.Base(exe) != startupReporter {
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

// Programs with windows of their own show them: what Open opens, such as
// the log folder in Explorer or the admin app in the browser, and what
// WindowCommand starts, such as the update's installer. Console programs,
// such as clip, pg_ctl and the server, open no window.
func TestWindowsShown(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name  string
		start func(exe string) error
		shown bool
	}{
		{"Open", Open, true},
		{"WindowCommand", func(exe string) error { return WindowCommand(ctx, exe).Run() }, true},
		{"Command", func(exe string) error { return Command(ctx, exe).Run() }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			exe := reporter(t)
			if err := c.start(exe); err != nil {
				t.Fatal(err)
			}
			flags, show := startup(t, exe)
			hidden := flags&windows.STARTF_USESHOWWINDOW != 0 && show == windows.SW_HIDE
			if hidden == c.shown {
				t.Errorf("started with flags %#x and show state %d: shown %v, want %v", flags, show, !hidden, c.shown)
			}
		})
	}
}

// reporter copies the test binary as a startup reporter, in a folder
// removed once the copy has ended: Windows refuses to remove a program
// that runs.
func reporter(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "polyfin-shown-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(30 * time.Second)
		for os.RemoveAll(dir) != nil && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
	})
	exe := filepath.Join(dir, startupReporter)
	in, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(exe)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// startup waits for the report of the reporter exe, for up to a minute:
// CI runners can be slow.
func startup(t *testing.T, exe string) (flags uint32, show uint16) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(exe + ".txt"); err == nil {
			if _, err := fmt.Sscan(string(data), &flags, &show); err != nil {
				t.Fatalf("report %q: %v", data, err)
			}
			return flags, show
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the program did not start")
	return 0, 0
}
