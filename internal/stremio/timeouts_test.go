package stremio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A request for streams is given StreamTimeout, twice what the other
// requests are given: an addon gathering other addons' streams answers
// once its own limit for them ran out, often what the other requests are
// given. An answer slower than that limit, but within StreamTimeout, is
// read; a description as slow is not.
func TestARequestForStreamsWaitsLongerThanTheOthers(t *testing.T) {
	t.Parallel()
	if client := NewClient("test"); client.timeout != 15*time.Second || client.streamTimeout != 30*time.Second || StreamTimeout != 2*requestTimeout {
		t.Fatalf("limits: %s and %s for streams", client.timeout, client.streamTimeout)
	}
	// Limits scaled down, with room on both sides for a loaded machine:
	// 100 ms for the other requests and 2 s for streams, and an addon
	// answering after 500 ms, as one gathering addons that give up after
	// the other requests' limit does.
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		switch r.URL.Path {
		case "/stream/movie/tt1.json":
			_, _ = w.Write([]byte(`{"streams":[{"name":"Source","url":"https://cdn.example/movie.mkv"}]}`))
		default:
			_, _ = w.Write([]byte(`{"meta":{"id":"tt1","type":"movie","name":"Movie"}}`))
		}
	}))
	t.Cleanup(addon.Close)
	manifest := addon.URL + "/manifest.json"
	client := NewClient("test")
	client.SetTimeouts(100*time.Millisecond, 2*time.Second)

	if streams, err := client.Streams(t.Context(), manifest, "movie", "tt1", false); err != nil || len(streams) != 1 {
		t.Fatalf("streams: %v, %v; want the answer", streams, err)
	}
	_, err := client.Meta(t.Context(), manifest, "movie", "tt1", false)
	if FailureOf(err) != FailureTimeout {
		t.Fatalf("the description: %v, want a timeout", err)
	}
	if health, _ := client.Health(manifest); health.Failure != FailureTimeout || health.Failures != 1 || health.Requests != 2 {
		t.Errorf("health: %+v", health)
	}
}

// A request the addon does not answer within its limit says so, naming
// the limit, whether no answer came or its body stopped coming. It is
// still ErrUnreachable and context.DeadlineExceeded, which Health and the
// pause of a server timing out read. A request its caller ended first
// keeps its own error: the addon's limit did not end it.
func TestATimeoutNamesTheLimitTheAddonDidNotAnswerWithin(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream/movie/tt1.json" {
			// The headers come, the body does not.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"streams":[`))
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(addon.Close)
	manifest := addon.URL + "/manifest.json"
	client := NewClient("test")
	client.SetTimeouts(100*time.Millisecond, 200*time.Millisecond)

	for _, c := range []struct {
		name string
		ask  func() error
		want string
	}{
		{"a description", func() error {
			_, err := client.Meta(t.Context(), manifest, "movie", "tt1", false)
			return err
		}, "addon did not answer within 100ms"},
		{"streams", func() error {
			_, err := client.Streams(t.Context(), manifest, "movie", "tt1", false)
			return err
		}, "addon did not answer within 200ms"},
	} {
		err := c.ask()
		if err == nil || err.Error() != c.want || !errors.Is(err, ErrUnreachable) || !errors.Is(err, context.DeadlineExceeded) ||
			FailureOf(err) != FailureTimeout {
			t.Errorf("%s: %v, want %q, ErrUnreachable and a timeout", c.name, err, c.want)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := client.Meta(ctx, manifest, "movie", "tt1", false)
	if err == nil || strings.Contains(err.Error(), "did not answer within") || !errors.Is(err, ErrUnreachable) {
		t.Errorf("a request its caller ended: %v, want its own error", err)
	}
}
