package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/update"
)

// An update that fails once Polyfin has stopped, as an installer that
// cannot start or an AppImage that cannot be replaced, leaves Polyfin
// running again, with the menu telling that the update could not be
// installed, and offering it again.
func TestAFailedUpdateIsTold(t *testing.T) {
	inst := install(t)
	data := t.TempDir()
	t.Setenv("POLYFIN_LISTEN", "127.0.0.1:"+listenPort(t))
	t.Setenv("FAKE_POLYFIN_SERVER", "1")
	manifest, _ := readyTools(t, data)
	// The package is no program: the system refuses to start it.
	body := []byte("not a program")
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(github.Close)
	sum := sha256.Sum256(body)
	offer := &update.Offer{Version: "0.43.0", Asset: update.Asset{Name: "Polyfin-Desktop-0.43.0-windows-x64-setup.exe",
		URL: github.URL + "/download", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}, Expires: time.Now().Add(time.Hour)}
	checker := &update.Checker{Version: "0.42.0", Client: github.Client()}
	a := New(Options{Version: "0.42.0", Install: inst, Folders: desktop.Folders{Data: data}, Keys: &fakeKeys{secrets: map[string]string{}},
		Tools: manifest, DatabasePort: databasePort(t), Updates: checker, Executable: filepath.Join(inst.Dir, "polyfin-desktop")})
	a.Start()
	t.Cleanup(func() { a.Stop(context.Background()) })

	for _, c := range []struct {
		kind     update.Kind
		appImage string
	}{
		{update.AppImage, filepath.Join(data, "missing", "Polyfin.AppImage")},
		{update.Installer, ""},
	} {
		t.Setenv("APPIMAGE", c.appImage)
		checker.Kind = c.kind
		waitFor(t, a, "running", func(s Status) bool { return s.Phase == Running })
		a.update(func(s *Status) { s.Offer = offer })
		cmd, err := a.InstallUpdate(context.Background())
		if err == nil || cmd != nil {
			t.Fatalf("%s: InstallUpdate = %v, %v", c.kind, cmd, err)
		}
		if s := a.Status(); !s.Has(UpdateFailed) {
			t.Errorf("%s: the failure is not told: %+v", c.kind, s)
		}
		s := waitFor(t, a, "running again", func(s Status) bool { return s.Phase == Running })
		if !s.Has(UpdateFailed) || s.Offer == nil {
			t.Errorf("%s: once Polyfin runs again, problems %v, offer %v", c.kind, s.Problems, s.Offer)
		}
	}
}
