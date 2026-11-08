package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// While the background requests to an addon's server pause, as it did not
// answer requests for versions in time, System › Health warns of it: the
// addon "did not answer in time", whatever it answered since but versions,
// with when the pause ends, linking to its page, and the notifications
// tell what to do.
func TestHealthTellsAnAddonThatKeepsTimingOut(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": "slow", "name": "Slow", "version": "1.0.0", "resources": ["stream"], "types": ["movie"]}`)
	}))
	t.Cleanup(server.Close)
	until := time.Date(2026, 11, 8, 21, 30, 0, 0, time.Local)
	var pausing atomic.Bool
	var options *Options
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Health = HealthSources{Addons: stremio.NewClient("test"), StreamsTimingOut: func(addons.Addon) (time.Time, bool) {
			return until, pausing.Load()
		}}
		options, store = o, deps.addons
	})
	addon, err := store.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	administrator := api.signedIn("administrator", true)
	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		return found
	}
	// Its manifest answered: nothing to tell.
	if found := problems(); len(found) != 0 {
		t.Fatalf("problems while the addon answers: %v", found)
	}

	pausing.Store(true)
	found := problems()
	if len(found) != 1 {
		t.Fatalf("problems: %v", found)
	}
	p, _ := found[0].(map[string]any)
	at, _ := time.Parse(time.RFC3339, p["until"].(string))
	if p["key"] != "addon:"+addon.ID.String() || p["code"] != "addon" || p["failure"] != "timeout" || p["name"] != "Slow" ||
		p["to"] != "/sources/shared/"+addon.ID.String() || p["transient"] != false || !at.Equal(until) {
		t.Errorf("the problem: %v", p)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 {
		t.Fatalf("notifications: %+v %v", told, err)
	}
	if text := told[0].Text; !strings.HasPrefix(text, "Slow does not answer in time when asked for versions") ||
		!strings.Contains(text, "pause until 21:30") || !strings.Contains(text, "turn it off") {
		t.Errorf("text: %s", text)
	}
}
