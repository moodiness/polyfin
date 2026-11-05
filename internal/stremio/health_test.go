package stremio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// A 503 that says how long to wait asks to wait, as a 429 does; one that
// does not is a server error. While the addons of a host wait for their
// catalogs, their health tells it, whatever answered since.
func TestA503WithRetryAfterAsksToWait(t *testing.T) {
	var retryAfter atomic.Value
	retryAfter.Store("")
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/meta/") {
			_, _ = w.Write([]byte(`{"meta":{"id":"tt1","type":"movie","name":"One"}}`))
			return
		}
		if after := retryAfter.Load().(string); after != "" {
			w.Header().Set("Retry-After", after)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer addon.Close()
	manifest := addon.URL + "/config/manifest.json"
	client := NewClient("test")
	_, err := client.Catalog(t.Context(), manifest, "movie", "top", nil, false)
	if _, waits := RetryAfter(err); waits || FailureOf(err) != FailureServerError {
		t.Errorf("a 503 without Retry-After: %v, %s", err, FailureOf(err))
	}
	retryAfter.Store("7")
	_, err = client.Catalog(t.Context(), manifest, "movie", "top", nil, false)
	if wait, waits := RetryAfter(err); !waits || wait != 7*time.Second || FailureOf(err) != FailureRateLimited || StatusOf(err) != 503 {
		t.Errorf("a 503 with Retry-After: %v, %s", err, FailureOf(err))
	}
	if health, _ := client.Health(manifest); health.Failure != FailureRateLimited {
		t.Errorf("health after a 503 with Retry-After: %+v", health)
	}

	// Another addon of the same server waits too; a description answered
	// meanwhile leaves the wait told.
	until := time.Now().Add(time.Minute)
	client.CatalogsWait(manifest, until)
	if _, err := client.Meta(t.Context(), manifest, "movie", "tt1", false); err != nil {
		t.Fatal(err)
	}
	other := addon.URL + "/other/manifest.json"
	_, _ = client.Meta(t.Context(), other, "movie", "tt1", false)
	for _, m := range []string{manifest, other} {
		if health, _ := client.Health(m); health.Failure != FailureRateLimited || !health.CatalogsWaitUntil.Equal(until) {
			t.Errorf("health of %s during the wait: %+v", m, health)
		}
	}
	if !errors.Is(AskedToWait(time.Second), ErrAskedToWait) || FailureOf(AskedToWait(time.Second)) != FailureRateLimited {
		t.Error("a request not sent during a wait is not told as one")
	}
}

// An addon that answers with an error status was reached: its error tells
// what it answered, a refusal as such, and is still ErrUnreachable to the
// callers, with its status.
func TestAnErrorAnswerTellsWhatTheAddonAnswered(t *testing.T) {
	var status atomic.Int32
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer addon.Close()
	manifest := addon.URL + "/secret-config/manifest.json"
	client := NewClient("test")
	for code, want := range map[int]string{
		http.StatusForbidden:            "addon refused the request: HTTP 403",
		http.StatusUnauthorized:         "addon refused the request: HTTP 401",
		http.StatusBadGateway:           "addon failed: HTTP 502",
		http.StatusTooManyRequests:      "addon asked to slow down: HTTP 429",
		http.StatusServiceUnavailable:   "addon failed: HTTP 503",
		http.StatusNonAuthoritativeInfo: "addon answered HTTP 203",
	} {
		status.Store(int32(code))
		_, err := client.Fetch(t.Context(), manifest, addon.URL+"/secret-config/search?q=a", false)
		if err == nil || err.Error() != want || !errors.Is(err, ErrUnreachable) || StatusOf(err) != code ||
			strings.Contains(err.Error(), "secret-config") {
			t.Errorf("HTTP %d: %v", code, err)
		}
	}
}

// A refusal tells the reason the addon gave, from the error or message of
// its JSON answer or from a short text, on one line, cut short, with what
// looks like an address, a token or a key redacted.
func TestARefusalTellsTheAddonsReason(t *testing.T) {
	type answer struct {
		status            int
		contentType, body string
	}
	answers := map[string]answer{
		"error":   {http.StatusForbidden, "application/json", `{"error":"Unavailable"}`},
		"nested":  {http.StatusUnauthorized, "application/json", `{"error":{"code":7,"message":"Token expired"}}`},
		"token":   {http.StatusUnauthorized, "application/json", `{"message":"invalid key eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln"}`},
		"text":    {http.StatusForbidden, "text/plain; charset=utf-8", "Blocked:\n see https://addon.example/u/secret-token/manifest.json\tor ask token=abc"},
		"long":    {http.StatusBadRequest, "application/json", `{"message":"` + strings.Repeat("word ", 40) + `"}`},
		"page":    {http.StatusForbidden, "text/html", "<html><body>Forbidden</body></html>"},
		"other":   {http.StatusForbidden, "application/json", `{"metas":[]}`},
		"empty":   {http.StatusForbidden, "", ""},
		"missing": {http.StatusNotFound, "application/json", `{"error":"No such track"}`},
	}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := answers[r.URL.Query().Get("case")]
		w.Header().Set("Content-Type", a.contentType)
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(a.body))
	}))
	defer addon.Close()
	manifest := addon.URL + "/secret-config/manifest.json"
	client := NewClient("test")
	for name, want := range map[string]string{
		"error":   "addon refused the request: HTTP 403 (Unavailable)",
		"nested":  "addon refused the request: HTTP 401 (Token expired)",
		"token":   "addon refused the request: HTTP 401 (invalid key <redacted>)",
		"text":    "addon refused the request: HTTP 403 (Blocked: see <redacted> or ask token=<redacted>)",
		"long":    "addon refused the request: HTTP 400 (" + strings.TrimSpace(strings.Repeat("word ", 24)) + "…)",
		"page":    "addon refused the request: HTTP 403",
		"other":   "addon refused the request: HTTP 403",
		"empty":   "addon refused the request: HTTP 403",
		"missing": "not found by the addon: HTTP 404",
	} {
		_, err := client.Fetch(t.Context(), manifest, addon.URL+"/secret-config/search?case="+name, false)
		if err == nil || err.Error() != want || !errors.Is(err, ErrUnreachable) && name != "missing" {
			t.Errorf("%s: %v", name, err)
		}
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
