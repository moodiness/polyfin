package stremio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/metrics"
)

func TestHealthFollowsTheRequestsMadeToAnAddon(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusOK)
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"metas":[]}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/secret-config/manifest.json"
	client := NewClient("test")
	if _, known := client.Health(manifest); known {
		t.Fatal("health of an addon never asked")
	}

	if _, err := client.Catalog(t.Context(), manifest, "movie", "top", nil, false); err != nil {
		t.Fatal(err)
	}
	health, known := client.Health(manifest)
	if !known || health.LastSuccess.IsZero() || health.Failure != "" || health.Requests != 1 || health.ResponseTime <= 0 {
		t.Fatalf("after a success: %+v", health)
	}

	// "Not found" is an answer: the addon is up.
	status.Store(http.StatusNotFound)
	_, _ = client.Meta(t.Context(), manifest, "movie", "tt1", false)
	if health, _ := client.Health(manifest); health.Failure != "" || health.Failures != 0 {
		t.Errorf("after a 404: %+v", health)
	}

	status.Store(http.StatusTooManyRequests)
	_, _ = client.Streams(t.Context(), manifest, "movie", "tt1", false)
	health, _ = client.Health(manifest)
	if health.Failure != FailureRateLimited || health.Failures != 1 || health.LastFailure.IsZero() || health.Requests != 3 {
		t.Errorf("after a 429: %+v", health)
	}

	// A request the caller gave up on says nothing about the addon.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _ = client.Catalog(ctx, manifest, "movie", "top", nil, false)
	if after, _ := client.Health(manifest); after.Requests != 3 {
		t.Errorf("a cancelled request was counted: %+v", after)
	}

	addon.Close()
	_, _ = client.Catalog(t.Context(), manifest, "movie", "top", nil, false)
	if health, _ := client.Health(manifest); health.Failure != FailureUnreachable {
		t.Errorf("after the addon went away: %+v", health)
	}
}

// The metrics count an addon's requests by resource and outcome, under
// the name the addon is listed with: a failing request moves the failures.
func TestAddonRequestsAreCountedInTheMetrics(t *testing.T) {
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stream/") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"metas":[]}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/secret-config/manifest.json"
	metrics.AddonNames(func(context.Context) (map[string]string, error) {
		return map[string]string{BaseURL(manifest): "Failing addon"}, nil
	})
	t.Cleanup(func() { metrics.AddonNames(nil) })
	client := NewClient("test")
	if _, err := client.Catalog(t.Context(), manifest, "movie", "top", nil, false); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := client.Streams(t.Context(), manifest, "movie", "tt1", false); err == nil {
			t.Fatal("a stream request answered 502 succeeded")
		}
	}
	for _, c := range []struct {
		resource, outcome string
		want              float64
	}{
		{"catalog", metrics.Success, 1},
		{"stream", metrics.Failure, 2},
		{"stream", metrics.Success, 0},
	} {
		labels := map[string]string{"addon": "Failing addon", "resource": c.resource, "outcome": c.outcome}
		if got := metrics.Value("polyfin_addon_request_duration_seconds", labels); got != c.want {
			t.Errorf("%s %s: %v, want %v", c.resource, c.outcome, got, c.want)
		}
	}
}
