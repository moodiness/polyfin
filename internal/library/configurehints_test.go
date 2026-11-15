package library

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// hintAddon serves a manifest that declares a configure page or not, and
// fails while failing is set; it counts the manifests asked.
type hintAddon struct {
	mu           sync.Mutex
	configurable bool
	failing      bool
	asked        int
}

func (a *hintAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !strings.HasSuffix(r.URL.Path, "/manifest.json") {
		http.NotFound(w, r)
		return
	}
	a.asked++
	if a.failing {
		http.Error(w, "failing", http.StatusInternalServerError)
		return
	}
	hints := ""
	if a.configurable {
		hints = `, "behaviorHints": {"configurable": true}`
	}
	_, _ = w.Write([]byte(`{"id": "hint", "name": "Hint", "version": "1.0.0", "resources": ["catalog"],
		"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]` + hints + `}`))
}

func (a *hintAddon) set(configurable, failing bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configurable, a.failing, a.asked = configurable, failing, 0
}

func (a *hintAddon) manifests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked
}

// hintEnv is an env whose service logs into log, reads hints without
// spacing, and installs hint addons as stored before Polyfin kept the
// configure hint.
type hintEnv struct {
	env
	log *bytes.Buffer
}

func newHintEnv(t *testing.T) hintEnv {
	e := newEnv(t)
	log := new(bytes.Buffer)
	e.service.logger = slog.New(slog.NewTextHandler(log, nil))
	e.service.hintSpacing = 0
	return hintEnv{env: e, log: log}
}

// installOld serves addon at an address holding a key, installs it in
// scope, then stores it as before the hint was kept: without it, unread.
func (e hintEnv) installOld(scope addons.Scope, addon *hintAddon) (addons.Addon, string) {
	e.t.Helper()
	server := httptest.NewServer(addon)
	e.t.Cleanup(server.Close)
	manifestURL := server.URL + "/secret-key/manifest.json?token=hidden"
	installed, err := e.addons.Install(e.t.Context(), scope, manifestURL, false)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.service.db.Exec(e.t.Context(),
		"UPDATE addons SET hints_read = false, manifest = manifest - 'behaviorHints' WHERE id = $1", installed.ID); err != nil {
		e.t.Fatal(err)
	}
	return installed, manifestURL
}

// stored returns whether the addon's hint is read, whether it is
// configurable, its stored manifest but the hints, and when it was
// refreshed.
func (e hintEnv) stored(addon addons.Addon) (read, configurable bool, rest string, refreshed time.Time) {
	e.t.Helper()
	if err := e.service.db.QueryRow(e.t.Context(), `SELECT hints_read, coalesce((manifest->'behaviorHints'->>'configurable')::boolean, false),
		(manifest - 'behaviorHints')::text, refreshed_at FROM addons WHERE id = $1`, addon.ID).Scan(&read, &configurable, &rest, &refreshed); err != nil {
		e.t.Fatal(err)
	}
	return read, configurable, rest, refreshed
}

// The addons stored before the configure hint have it read in the
// background, the server's and users' own, and only it changes; an
// addon read is not asked again, and the log never names an address.
func TestConfigureHintsAreReadOnceAndAloneInTheBackground(t *testing.T) {
	t.Parallel()
	e := newHintEnv(t)
	configurable, plain := &hintAddon{configurable: true}, &hintAddon{}
	shared, sharedURL := e.installOld(addons.Shared(), configurable)
	own, ownURL := e.installOld(addons.Personal(e.admin.ID), plain)
	_, _, sharedBefore, sharedRefreshed := e.stored(shared)
	_, _, ownBefore, _ := e.stored(own)
	if found, err := e.addons.Find(t.Context(), shared.ID); err != nil || found.Configurable() {
		t.Fatalf("before the read: configurable %v, %v", found.Configurable(), err)
	}
	// The manifest the addon serves now differs: only the hint is kept.
	configurable.set(true, false)
	plain.set(false, false)

	e.service.ReadConfigureHints(t.Context())

	read, flag, rest, refreshed := e.stored(shared)
	if !read || !flag || rest != sharedBefore || !refreshed.Equal(sharedRefreshed) {
		t.Errorf("server addon: read %v, configurable %v, refreshed %v (was %v), rest changed %v", read, flag, refreshed, sharedRefreshed,
			rest != sharedBefore)
	}
	if read, flag, rest, _ := e.stored(own); !read || flag || rest != ownBefore {
		t.Errorf("own addon: read %v, configurable %v, rest changed %v", read, flag, rest != ownBefore)
	}
	found, err := e.addons.Find(t.Context(), shared.ID)
	if err != nil || !found.Configurable() {
		t.Errorf("after the read: configurable %v, %v", found.Configurable(), err)
	}
	if configurable.manifests() != 1 || plain.manifests() != 1 {
		t.Errorf("manifests asked: %d and %d", configurable.manifests(), plain.manifests())
	}

	// A second pass asks nothing.
	configurable.set(true, false)
	plain.set(false, false)
	e.service.ReadConfigureHints(t.Context())
	if configurable.manifests() != 0 || plain.manifests() != 0 {
		t.Errorf("a second pass asked %d and %d manifests", configurable.manifests(), plain.manifests())
	}

	logged := e.log.String()
	if !strings.Contains(logged, "read=2 configurable=1 skipped=0 failed=0") {
		t.Errorf("the pass's line: %s", logged)
	}
	for _, address := range []string{sharedURL, ownURL} {
		host := stremio.HostOf(address)
		if strings.Contains(logged, host) || strings.Contains(logged, "secret-key") || strings.Contains(logged, "hidden") {
			t.Errorf("the log names an address: %s", logged)
		}
	}
}

// A read that fails leaves the addon unread, and the next pass reads it.
func TestAFailedConfigureHintReadIsTriedAgain(t *testing.T) {
	t.Parallel()
	e := newHintEnv(t)
	addon := &hintAddon{configurable: true}
	installed, manifestURL := e.installOld(addons.Shared(), addon)
	addon.set(true, true)

	e.service.ReadConfigureHints(t.Context())
	if read, flag, _, _ := e.stored(installed); read || flag || addon.manifests() != 1 {
		t.Errorf("after a failed read: read %v, configurable %v, %d manifests asked", read, flag, addon.manifests())
	}
	if !strings.Contains(e.log.String(), "read=0 configurable=0 skipped=0 failed=1") {
		t.Errorf("the pass's line: %s", e.log.String())
	}

	addon.set(true, false)
	e.service.ReadConfigureHints(t.Context())
	if read, flag, _, _ := e.stored(installed); !read || !flag {
		t.Errorf("at the next pass: read %v, configurable %v", read, flag)
	}
	if logged := e.log.String(); strings.Contains(logged, stremio.HostOf(manifestURL)) || strings.Contains(logged, "secret-key") {
		t.Errorf("the log names the address: %s", logged)
	}
}

// An addon whose server asked to wait, for its background requests or
// its catalog pages, is skipped this time, and read at a later pass.
func TestConfigureHintsSkipAddonsAskedToWait(t *testing.T) {
	t.Parallel()
	e := newHintEnv(t)
	paused, waiting := &hintAddon{configurable: true}, &hintAddon{configurable: true}
	pausedAddon, pausedURL := e.installOld(addons.Shared(), paused)
	waitingAddon, waitingURL := e.installOld(addons.Shared(), waiting)
	paused.set(true, false)
	waiting.set(true, false)
	e.service.pace.refused(stremio.HostOf(pausedURL), time.Minute)
	e.service.catalogWaits.mu.Lock()
	e.service.catalogWaits.hosts = map[string]catalogWait{stremio.HostOf(waitingURL): {until: e.service.now().Add(time.Minute)}}
	e.service.catalogWaits.mu.Unlock()

	e.service.ReadConfigureHints(t.Context())
	for _, addon := range []addons.Addon{pausedAddon, waitingAddon} {
		if read, _, _, _ := e.stored(addon); read {
			t.Errorf("%s was read while its server asked to wait", addon.Name())
		}
	}
	if paused.manifests() != 0 || waiting.manifests() != 0 {
		t.Errorf("manifests asked of servers that asked to wait: %d and %d", paused.manifests(), waiting.manifests())
	}
	if !strings.Contains(e.log.String(), "read=0 configurable=0 skipped=2 failed=0") {
		t.Errorf("the pass's line: %s", e.log.String())
	}
}

// A refresh stores the hint at once, as an install does: the pass then
// has nothing to read.
func TestARefreshReadsTheConfigureHintAtOnce(t *testing.T) {
	t.Parallel()
	e := newHintEnv(t)
	addon := &hintAddon{configurable: true}
	installed, _ := e.installOld(addons.Shared(), addon)
	if _, _, err := e.addons.Refresh(t.Context(), addons.Shared(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if read, flag, _, _ := e.stored(installed); !read || !flag {
		t.Errorf("after a refresh: read %v, configurable %v", read, flag)
	}
	addon.set(true, false)
	e.service.ReadConfigureHints(t.Context())
	if addon.manifests() != 0 {
		t.Errorf("the pass asked %d manifests", addon.manifests())
	}
}
