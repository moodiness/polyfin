package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/update"
)

// updated runs Polyfin 0.42.0 from an AppImage, and installs 0.43.0 over
// it, as Install Polyfin 0.43.0 does: it returns the options of the new
// version's app, and the AppImage's path.
func updated(t *testing.T) (Options, string) {
	t.Helper()
	inst := install(t)
	data := t.TempDir()
	t.Setenv("POLYFIN_LISTEN", "127.0.0.1:"+listenPort(t))
	t.Setenv("FAKE_POLYFIN_SERVER", "1")
	manifest, _ := readyTools(t, data)
	image := filepath.Join(t.TempDir(), "Polyfin.AppImage")
	if err := os.WriteFile(image, []byte("Polyfin 0.42.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", image)
	body := []byte("Polyfin 0.43.0")
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(github.Close)
	sum := sha256.Sum256(body)
	offer := &update.Offer{Version: "0.43.0", Asset: update.Asset{Name: "Polyfin-Desktop-0.43.0-linux-x86_64.AppImage",
		URL: github.URL + "/download", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}, Expires: time.Now().Add(time.Hour)}
	o := Options{Version: "0.42.0", Install: inst, Folders: desktop.Folders{Data: data}, Keys: &fakeKeys{secrets: map[string]string{}},
		Tools: manifest, DatabasePort: databasePort(t),
		Updates:    &update.Checker{Version: "0.42.0", Kind: update.AppImage, Client: github.Client()},
		Executable: filepath.Join(inst.Dir, "polyfin-desktop")}
	a := New(o)
	a.Start()
	t.Cleanup(func() { a.Stop(context.Background()) })
	waitFor(t, a, "0.42.0 running", func(s Status) bool { return s.Phase == Running })
	a.update(func(s *Status) { s.Offer = offer })
	cmd, err := a.InstallUpdate(context.Background())
	if err != nil || cmd == nil || cmd.Path != image {
		t.Fatalf("InstallUpdate = %v, %v", cmd, err)
	}
	if got, _ := os.ReadFile(image); string(got) != "Polyfin 0.43.0" {
		t.Fatalf("the AppImage holds %q", got)
	}
	if _, err := os.Stat(filepath.Join(o.Folders.PreviousDatabase(), "PG_VERSION")); err != nil {
		t.Fatalf("no copy of the database: %v", err)
	}
	o.Version = "0.43.0"
	o.Updates = &update.Checker{Version: "0.43.0", Kind: update.AppImage, Client: github.Client()}
	return o, image
}

// A deliberately broken build, whose server ends before it answers, is
// rolled back once it started update.MaxStarts times: the previous
// AppImage and the database as the previous version left it are put
// back, the app gives what starts the previous version, and the return
// is recorded for System › Health. The previous version then starts.
func TestABrokenUpdateIsRolledBack(t *testing.T) {
	o, image := updated(t)
	runs := filepath.Join(o.Folders.Data, "fake-runs")
	before, _ := os.ReadFile(runs)

	t.Setenv("FAKE_POLYFIN_BROKEN", "1")
	broken := New(o)
	broken.Start()
	t.Cleanup(func() { broken.Stop(context.Background()) })
	var previous string
	select {
	case cmd := <-broken.RolledBack():
		previous = cmd.Path
	case <-time.After(2 * time.Minute):
		t.Fatalf("the broken build was not rolled back: %+v", broken.Status())
	}
	broken.Stop(context.Background())
	if previous != image {
		t.Errorf("what starts the previous version: %q", previous)
	}
	if got, _ := os.ReadFile(image); string(got) != "Polyfin 0.42.0" {
		t.Errorf("the AppImage holds %q", got)
	}
	after, _ := os.ReadFile(runs)
	if starts := len(after) - len(before); starts != update.MaxStarts {
		t.Errorf("the broken build's server started %d times, want %d", starts, update.MaxStarts)
	}
	for _, path := range []string{image + ".old", o.Folders.PreviousDatabase(), o.Folders.PendingUpdate(),
		filepath.Join(o.Folders.Cluster(), "migrated-by-the-broken-build")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is left: %v", path, err)
		}
	}
	back, err := desktop.ReadRolledBack(o.Folders)
	if err != nil || back == nil || back.Version != "0.43.0" || back.Previous != "0.42.0" || back.Reason == "" {
		t.Errorf("the return is recorded as %+v, %v", back, err)
	}

	t.Setenv("FAKE_POLYFIN_BROKEN", "")
	o.Version = "0.42.0"
	again := New(o)
	again.Start()
	t.Cleanup(func() { again.Stop(context.Background()) })
	waitFor(t, again, "0.42.0 running again", func(s Status) bool { return s.Phase == Running })
}

// A new version whose server answers /healthz keeps no previous app nor
// copy of the database, and clears the record of an earlier return.
func TestAGoodUpdateIsConfirmed(t *testing.T) {
	o, image := updated(t)
	if err := desktop.WriteRolledBack(o.Folders, desktop.RolledBack{Version: "0.42.1", Previous: "0.42.0"}); err != nil {
		t.Fatal(err)
	}
	a := New(o)
	a.Start()
	t.Cleanup(func() { a.Stop(context.Background()) })
	waitFor(t, a, "0.43.0 running", func(s Status) bool { return s.Phase == Running })
	waitFor(t, a, "the previous version removed", func(Status) bool {
		_, err := os.Stat(o.Folders.PendingUpdate())
		return os.IsNotExist(err)
	})
	for _, path := range []string{image + ".old", o.Folders.PreviousDatabase(), o.Folders.RolledBack()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is left: %v", path, err)
		}
	}
	if got, _ := os.ReadFile(image); string(got) != "Polyfin 0.43.0" {
		t.Errorf("the AppImage holds %q", got)
	}
}
