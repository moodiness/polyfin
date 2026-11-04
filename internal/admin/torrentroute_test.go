package admin

import (
	"context"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent"
)

// routeTorrents tells torrents as a Status of the engine, without one.
type routeTorrents struct{ status torrent.Status }

func (r routeTorrents) Status(context.Context) torrent.Status { return r.status }

func (routeTorrents) Usage() int64 { return 0 }

func (routeTorrents) Uploaded() int64 { return 0 }

// routeHealth is the API with torrents over a SOCKS5 proxy as status tells
// them, at the time now holds.
func routeHealth(t *testing.T, status torrent.Status, now *atomic.Pointer[time.Time]) testAPI {
	t.Helper()
	return newTestAPI(t, 10, func(o *Options, _ testDeps) {
		settings := o.Accounts.Settings()
		settings.TorrentStreams, settings.TorrentProxy = true, "192.0.2.1:1080"
		if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
		o.Now = func() time.Time { return *now.Load() }
		o.Health = HealthSources{Torrents: routeTorrents{status}}
	})
}

// A torrent its route stopped while it played is a warning of System ›
// Health for 15 minutes, telling when and why, apart from a torrent that
// did not start; while the route still does not answer, the route's own
// error tells it alone.
func TestHealthTellsATorrentItsRouteStopped(t *testing.T) {
	t.Parallel()
	stoppedAt := time.Date(2026, 11, 4, 20, 4, 0, 0, time.UTC)
	down := &torrent.RouteError{Kind: torrent.ProxyUnreachable, Route: "192.0.2.1:1080"}
	status := torrent.Status{Enabled: true, Route: "socks5://192.0.2.1:1080", Failure: down, FailedAt: stoppedAt,
		Stopped: down, StoppedAt: stoppedAt}
	var now atomic.Pointer[time.Time]
	later := stoppedAt.Add(14 * time.Minute)
	now.Store(&later)
	administrator := routeHealth(t, status, &now).signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	want := map[string]any{"key": "torrents:stopped", "code": "torrent_stopped", "tone": "warning",
		"to": "/settings/torrents#torrent-route", "transient": false, "failure": "proxy_unreachable", "name": "192.0.2.1:1080",
		"at": stoppedAt.Format(time.RFC3339)}
	if len(found) != 1 || !reflect.DeepEqual(found[0], want) {
		t.Fatalf("problems 14 minutes after: %v", found)
	}

	past := stoppedAt.Add(16 * time.Minute)
	now.Store(&past)
	_, body, _ = administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	if found, _ := body["problems"].([]any); len(found) != 0 {
		t.Fatalf("problems 16 minutes after: %v", found)
	}

	now.Store(&later)
	status.RouteError = down
	administrator = routeHealth(t, status, &now).signedIn("administrator", true)
	_, body, _ = administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ = body["problems"].([]any)
	if len(found) != 1 || found[0].(map[string]any)["code"] != "torrent_route" {
		t.Fatalf("problems while the route does not answer: %v", found)
	}
}

// The outages of the route that plays went on through show in the
// torrents of System › Health for 15 minutes after they ended, those of the
// route set alone.
func TestHealthTellsTheOutagesPlaysWentOnThrough(t *testing.T) {
	t.Parallel()
	route := "socks5://192.0.2.1:1080"
	at := time.Date(2026, 11, 4, 20, 4, 0, 0, time.UTC)
	status := torrent.Status{Enabled: true, Route: route, Outages: []torrent.Outage{
		{Route: route, At: at.Add(-20 * time.Minute), For: 30 * time.Second},
		{Route: "socks5://192.0.2.9:1080", At: at.Add(-time.Minute), For: 5 * time.Second},
		{Route: route, At: at, For: 16200 * time.Millisecond},
	}}
	var now atomic.Pointer[time.Time]
	later := at.Add(10 * time.Minute)
	now.Store(&later)
	administrator := routeHealth(t, status, &now).signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health", nil)
	torrents, _ := body["torrents"].(map[string]any)
	want := []any{map[string]any{"at": at.Format(time.RFC3339), "seconds": float64(16)}}
	if !reflect.DeepEqual(torrents["outages"], want) {
		t.Fatalf("outages: %v", torrents["outages"])
	}
	// The route's problems tell none of them: the plays went on.
	_, body, _ = administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	if found, _ := body["problems"].([]any); len(found) != 0 {
		t.Fatalf("problems: %v", found)
	}
}

// While torrents play through an outage of their route, the route's error
// tells they go on for their grace and new ones cannot start; with none
// playing, it tells torrents do not play.
func TestHealthTellsTorrentsPlayingRideARouteOutage(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 4, 20, 4, 0, 0, time.UTC)
	down := &torrent.RouteError{Kind: torrent.ProxyUnreachable, Route: "192.0.2.1:1080"}
	var now atomic.Pointer[time.Time]
	later := at.Add(20 * time.Second)
	now.Store(&later)
	for _, test := range []struct {
		name   string
		status torrent.Status
		grace  any
	}{
		{"playing", torrent.Status{Enabled: true, RouteError: down, RouteDownSince: at, Playing: 1}, float64(torrent.RouteGrace / time.Second)},
		{"none playing", torrent.Status{Enabled: true, RouteError: down}, nil},
	} {
		administrator := routeHealth(t, test.status, &now).signedIn("administrator", true)
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		if len(found) != 1 {
			t.Fatalf("%s: problems %v", test.name, found)
		}
		problem := found[0].(map[string]any)
		if problem["code"] != "torrent_route" || problem["failure"] != "proxy_unreachable" || problem["grace"] != test.grace {
			t.Errorf("%s: %v", test.name, problem)
		}
	}
}
