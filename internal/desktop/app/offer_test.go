package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/desktop/update"
)

// The update the menu offers is told to the admin app while the server
// runs: with its progress while it downloads, and its failure once
// Polyfin runs again; nothing once Polyfin stops.
func TestTheOfferIsToldToTheAdminApp(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	a := New(Options{Folders: folders})
	read := func() *desktop.Offered {
		t.Helper()
		offered, err := desktop.ReadOffer(folders)
		if err != nil {
			t.Fatal(err)
		}
		return offered
	}
	offer := &update.Offer{Version: "0.47.0", Notes: "https://github.com/moodiness/polyfin/releases/tag/v0.47.0"}
	a.update(func(s *Status) { s.Phase, s.Offer = Starting, offer })
	if got := read(); got != nil {
		t.Errorf("offered while the server starts: %+v", got)
	}
	a.update(func(s *Status) { s.Phase = Running })
	if got := read(); got == nil || *got != (desktop.Offered{Version: "0.47.0", Notes: offer.Notes}) {
		t.Errorf("running: %+v", got)
	}
	a.update(func(s *Status) { s.Phase, s.Percent = Updating, 40 })
	if got := read(); got == nil || !got.Installing || got.Percent != 40 {
		t.Errorf("downloading: %+v", got)
	}
	a.problem(UpdateFailed, errors.New("the installer could not start"))
	a.update(func(s *Status) { s.Phase, s.Percent = Running, 0 })
	if got := read(); got == nil || got.Installing || !got.Failed {
		t.Errorf("failed: %+v", got)
	}
	a.update(func(s *Status) { s.Phase = Stopped })
	if got := read(); got != nil {
		t.Errorf("offered once stopped: %+v", got)
	}
}

// The app gets the version each request of the server asks to install,
// once; a request a crash left before it started installs nothing.
func TestListenInstall(t *testing.T) {
	folders := desktop.Folders{Data: t.TempDir()}
	release, err := Lock(folders.Lock())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := os.WriteFile(folders.InstallRequest(), []byte("old-token 0.46.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stopQuit, err := ListenQuit(folders)
	if err != nil {
		t.Fatal(err)
	}
	defer stopQuit()
	requested, stop, err := ListenInstall(folders)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, err := os.Stat(folders.InstallRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the start left the old request: %v", err)
	}
	if err := desktop.RequestInstall(folders, "0.47.0"); err != nil {
		t.Fatal(err)
	}
	select {
	case version := <-requested:
		if version != "0.47.0" {
			t.Errorf("version %q", version)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the request did not come")
	}
	if _, err := os.Stat(folders.InstallRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the request answered is left: %v", err)
	}
}
