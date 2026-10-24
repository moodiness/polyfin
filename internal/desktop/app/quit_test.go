package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
)

// fakeApp holds the data folder of POLYFIN_DESKTOP_DATA_DIR as the app
// does, then says ready. With FAKE_DESKTOP_APP=listen, it answers a
// request to quit and ends; with deaf, it listens to none, and ends when
// its input does.
func fakeApp() {
	folders := desktop.Folders{Data: os.Getenv(desktop.DataDirVariable)}
	release, err := Lock(folders.Lock())
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake app:", err)
		os.Exit(1)
	}
	if os.Getenv("FAKE_DESKTOP_APP") != "listen" {
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	requested, stop, err := ListenQuit(folders)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake app:", err)
		os.Exit(1)
	}
	fmt.Println("ready")
	<-requested
	stop()
	release()
}

// startFakeApp starts the test binary as an app holding data, once it
// is ready.
func startFakeApp(t *testing.T, mode, data string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "FAKE_DESKTOP_APP="+mode, desktop.DataDirVariable+"="+data)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	if line, err := bufio.NewReader(stdout).ReadString('\n'); line != "ready\n" {
		t.Fatalf("the fake app said %q: %v", line, err)
	}
	return cmd
}

// The quit command asks the app of the data folder to quit, and returns
// once the app's process has ended, the app's record gone.
func TestQuit(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	app := startFakeApp(t, "listen", folders.Data)
	if pid, _, ok := desktop.ReadInstance(folders.Instance()); !ok || pid != app.Process.Pid {
		t.Fatalf("the app's record names %d, not %d", pid, app.Process.Pid)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := RequestQuit(ctx, folders); err != nil {
		t.Fatalf("RequestQuit = %v", err)
	}
	// The app ended: on Unix it is a zombie until waited for.
	if running(app.Process.Pid) {
		t.Error("the app still runs")
	}
	if err := app.Wait(); err != nil {
		t.Errorf("the app ended with %v", err)
	}
	for _, path := range []string{folders.Instance(), folders.QuitRequest()} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is left: %v", filepath.Base(path), err)
		}
	}
}

// With no app on the data folder, the command says so at once, and
// leaves no request behind.
func TestQuitNothingRunning(t *testing.T) {
	ctx := context.Background()
	missing := desktop.Folders{Data: filepath.Join(t.TempDir(), "none")}
	if err := RequestQuit(ctx, missing); !errors.Is(err, ErrNotRunning) {
		t.Errorf("no data folder: %v", err)
	}
	folders := desktop.Folders{Data: t.TempDir()}
	if err := RequestQuit(ctx, folders); !errors.Is(err, ErrNotRunning) {
		t.Errorf("no lock file: %v", err)
	}
	// An app that ended left its lock file, and a crash its record.
	release, err := Lock(folders.Lock())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err := os.WriteFile(folders.Instance(), []byte("1 0123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequestQuit(ctx, folders); !errors.Is(err, ErrNotRunning) {
		t.Errorf("a lock no app holds: %v", err)
	}
	if _, err := os.Stat(folders.QuitRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a request is left: %v", err)
	}
}

// An app that does not answer still runs when the time is up.
func TestQuitTimeout(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	startFakeApp(t, "deaf", folders.Data)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := RequestQuit(ctx, folders); !errors.Is(err, ErrStillRunning) {
		t.Errorf("RequestQuit = %v", err)
	}
}

// A request a crash left, for the app that crashed, quits no later app:
// the next start removes it, and one written again with the old token is
// not the new app's.
func TestQuitStaleRequest(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	const old = "0123456789abcdef"
	if err := os.WriteFile(folders.Instance(), []byte("1 "+old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(folders.QuitRequest(), []byte(old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := Lock(folders.Lock())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	requested, stop, err := ListenQuit(folders)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folders.QuitRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the start left the old request: %v", err)
	}
	pid, token, ok := desktop.ReadInstance(folders.Instance())
	if !ok || pid != os.Getpid() || token == old {
		t.Fatalf("the record names %d %q", pid, token)
	}
	if err := os.WriteFile(folders.QuitRequest(), []byte(old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if quitRequested(folders, token) {
		t.Error("the old token's request quits the new app")
	}
	stop()
	select {
	case <-requested:
		t.Error("the app was asked to quit")
	default:
	}
	if _, err := os.Stat(folders.Instance()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the record is left: %v", err)
	}
	// The app's own token is a request, which is then removed.
	if err := os.WriteFile(folders.QuitRequest(), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !quitRequested(folders, token) {
		t.Error("the app's token is no request")
	}
	if _, err := os.Stat(folders.QuitRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the answered request is left: %v", err)
	}
}
