//go:build darwin || linux

package app

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// A server a crash of the app left running is stopped at the next start;
// a process that took the recorded number later is left alone.
func TestStopLeftover(t *testing.T) {
	dir := t.TempDir()
	start := func() *exec.Cmd {
		cmd := exec.Command("sleep", "120")
		prepareServer(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	leftover := start()
	path := filepath.Join(dir, "polyfin.pid")
	writePID(path, leftover.Process.Pid)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no PID file: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- leftover.Wait() }()
	stopLeftover(path, slog.New(slog.DiscardHandler))
	if err := <-done; err == nil {
		t.Error("the leftover server ended by itself")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the PID file is left")
	}

	other := start()
	recorded, _ := startTime(other.Process.Pid)
	if err := os.WriteFile(path, []byte(strconv.Itoa(other.Process.Pid)+" "+strconv.FormatInt(recorded-1, 10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stopLeftover(path, slog.New(slog.DiscardHandler))
	if !alive(other.Process.Pid) {
		t.Error("a process that started at another time was stopped")
	}
}
