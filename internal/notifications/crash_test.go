package notifications

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/crashes"
)

// crashDir names the data folder of TestCrashingProcess, which only
// TestServerCrashIsToldAtTheRestart runs.
const crashDir = "POLYFIN_TEST_CRASH_DIR"

// The secrets the crashing process holds: one only in its memory, as a
// request's token is, one in the message of its panic.
const (
	heldSecret    = "held-0f9e8d7c6b5a"
	messageSecret = "said-1a2b3c4d5e6f"
)

// TestCrashingProcess is the server that crashes: it watches for crashes
// as the server does, then a goroutine holding a token panics.
func TestCrashingProcess(t *testing.T) {
	dir := os.Getenv(crashDir)
	if dir == "" {
		t.Skip("run by TestServerCrashIsToldAtTheRestart")
	}
	if _, err := crashes.Watch(dir); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		token := strings.Clone(heldSecret)
		serve(&token)
		close(done)
	}()
	<-done
}

// serve panics while it holds token, with an address that carries
// another.
//
//go:noinline
func serve(token *string) {
	runtime.KeepAlive(token)
	panic("the stream at https://media.example.org/Videos/1/stream?api_key=" + messageSecret + " failed")
}

// A panic writes the crash file in the data folder, without the secrets
// the process held; the next start tells the server's targets and
// administrators' own of it once, with the start of its trace and none of
// the secrets in it, and System › Health reads it.
func TestServerCrashIsToldAtTheRestart(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestCrashingProcess$", "-test.count=1")
	command.Env = append(os.Environ(), crashDir+"="+dir)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("the crashing process: %v\n%s", err, output)
	}
	written, err := os.ReadFile(filepath.Join(dir, "crash.log"))
	if err != nil {
		t.Fatal(err)
	}
	file := string(written)
	if !strings.Contains(file, "panic: the stream at") || !strings.Contains(file, "goroutine ") || !strings.Contains(file, "notifications.serve") {
		t.Fatalf("crash file:\n%s", file)
	}
	if strings.Contains(file, heldSecret) {
		t.Errorf("the crash file holds the token the process held:\n%s", file)
	}

	h.add(t, nil, h.webhook("/server", ServerCrashed))
	h.add(t, &h.admin, h.webhook("/admin", ServerCrashed))
	if err := h.reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	close(h.loaded)
	// The restart finds it.
	report, err := crashes.Found(dir)
	if err != nil || report == nil {
		t.Fatalf("found: %v, %v", report, err)
	}
	if err := h.ServerCrashed(t.Context(), *report); err != nil {
		t.Fatal(err)
	}
	if err := crashes.Forget(dir); err != nil {
		t.Fatal(err)
	}
	body := h.targets.wait(t, "/server", 1)[0].body
	h.targets.wait(t, "/admin", 1)
	h.idle(t)
	crash, _ := body["crash"].(map[string]any)
	trace, _ := crash["trace"].(string)
	message, _ := body["message"].(string)
	if body["type"] != ServerCrashed || body["title"] != "Polyfin crashed" || !strings.HasPrefix(trace, "panic: the stream at https://media.example.org/…") ||
		!strings.Contains(trace, "goroutine ") || !strings.Contains(message, "panic: the stream at") {
		t.Errorf("message: %v", body)
	}
	for _, secret := range []string{heldSecret, messageSecret} {
		if strings.Contains(trace, secret) || strings.Contains(message, secret) {
			t.Errorf("the message tells a secret: %v", body)
		}
	}
	last, err := h.LastCrash(t.Context())
	// The database keeps microseconds.
	if err != nil || last == nil || last.Trace != trace || !last.At.Equal(report.At.Truncate(time.Microsecond)) {
		t.Errorf("last crash: %+v, %v", last, err)
	}
	// Once kept, the crash is not found again.
	if again, err := crashes.Found(dir); err != nil || again != nil {
		t.Errorf("found again: %+v, %v", again, err)
	}
}
