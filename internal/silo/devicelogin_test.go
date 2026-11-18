package silo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

// codeAccounts are the harness's accounts while Quick Connect is on.
type codeAccounts struct{ *fakeAccounts }

func (codeAccounts) Settings() accounts.Settings {
	return accounts.Settings{SiloApps: true, QuickConnectEnabled: true, Language: "en"}
}

// One client opens TV requests without a session, more than the store
// keeps, each as a new device: none is refused, each takes the place of
// that client's oldest, and the TVs elsewhere, Silo's and Jellyfin apps',
// keep their code and still get one.
func TestAnonymousStartsLeaveOtherTVsTheirCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	store := quickconnect.New()
	h.server.options.QuickConnect = store
	h.server.options.Accounts = codeAccounts{h.accounts}
	// start opens a request from the address remote, with no token and no
	// device identifier, and returns the answer's status and the secret.
	start := func(remote string) (int, string) {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v2/auth/device/start", strings.NewReader(`{"device_name":"x"}`))
		r.RemoteAddr = remote
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.server.ServeHTTP(w, r)
		var started struct {
			DeviceCode string `json:"device_code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &started)
		return w.Code, started.DeviceCode
	}
	waits := func(secret string) bool {
		_, err := store.BySecret(secret)
		return err == nil
	}
	status, tv := start("192.168.1.20:50000")
	if status != http.StatusCreated || !waits(tv) {
		t.Fatalf("a TV's start: %d", status)
	}
	var opened []string
	for n := range 1200 {
		status, secret := start("203.0.113.9:50000")
		if status != http.StatusCreated {
			t.Fatalf("start %d of one client: %d, want 201", n+1, status)
		}
		opened = append(opened, secret)
	}
	if waits(opened[0]) || !waits(opened[len(opened)-1]) {
		t.Errorf("one client's requests: the first waits %v, the last %v, want the last alone", waits(opened[0]), waits(opened[len(opened)-1]))
	}
	if !waits(tv) {
		t.Error("the TV that waited before them gave way")
	}
	if _, err := store.Initiate(quickconnect.Origin{Address: "198.51.100.7"}, "tv-1", "Living room", "Jellyfin for Android TV", "1.0"); err != nil {
		t.Errorf("a Jellyfin app's request after them: %v", err)
	}
	if status, secret := start("198.51.100.8:50000"); status != http.StatusCreated || !waits(secret) {
		t.Errorf("another TV's start after them: %d, want 201", status)
	}
}
