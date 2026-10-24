package desktop

import (
	"errors"
	"os"
	"testing"
)

// The offer is read as written, and gone once the app offers nothing.
func TestOffer(t *testing.T) {
	folders := Folders{Data: t.TempDir()}
	if offered, err := ReadOffer(folders); offered != nil || err != nil {
		t.Fatalf("before any offer: %+v, %v", offered, err)
	}
	want := Offered{Version: "0.47.0", Notes: "https://github.com/moodiness/polyfin/releases/tag/v0.47.0", Installing: true, Percent: 40}
	if err := WriteOffer(folders, &want); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadOffer(folders); err != nil || got == nil || *got != want {
		t.Errorf("offer: %+v, %v", got, err)
	}
	if err := WriteOffer(folders, nil); err != nil {
		t.Fatal(err)
	}
	if offered, err := ReadOffer(folders); offered != nil || err != nil {
		t.Errorf("after none: %+v, %v", offered, err)
	}
	if err := WriteOffer(folders, nil); err != nil {
		t.Errorf("none twice: %v", err)
	}
}

// A request to install carries the running app's token: without an app,
// none is written, and one of another app, left by a crash, installs
// nothing. Each is read once.
func TestInstallRequest(t *testing.T) {
	folders := Folders{Data: t.TempDir()}
	if err := RequestInstall(folders, "0.47.0"); !errors.Is(err, ErrNoApp) {
		t.Errorf("without an app: %v", err)
	}
	if _, err := os.Stat(folders.InstallRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a request was written without an app: %v", err)
	}
	if err := os.WriteFile(folders.Instance(), []byte("4242 old-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequestInstall(folders, "0.47.0"); err != nil {
		t.Fatal(err)
	}
	if version, ok := InstallRequested(folders, "new-token"); ok {
		t.Errorf("another app's request installs %s", version)
	}
	if _, err := os.Stat(folders.InstallRequest()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the stale request is left: %v", err)
	}
	if err := os.WriteFile(folders.Instance(), []byte("4243 new-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequestInstall(folders, "0.47.0"); err != nil {
		t.Fatal(err)
	}
	if version, ok := InstallRequested(folders, "new-token"); !ok || version != "0.47.0" {
		t.Errorf("request: %q %v", version, ok)
	}
	if version, ok := InstallRequested(folders, "new-token"); ok {
		t.Errorf("the request was read twice: %s", version)
	}
}
