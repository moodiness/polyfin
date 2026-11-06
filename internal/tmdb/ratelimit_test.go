package tmdb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"
)

// TMDB's last 429 is kept, with the wait it asked, for Health; forgetting
// a person has their details and the searches of their name asked again.
func TestTheLastRateLimitIsKeptAndAPersonForgotten(t *testing.T) {
	t.Parallel()
	var limited atomic.Bool
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if limited.Load() {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		switch r.URL.Path {
		case "/3/person/81":
			_, _ = w.Write([]byte(`{"id":81,"name":"Hana Actor","biography":"Her life."}`))
		case "/3/search/person":
			_, _ = w.Write([]byte(`{"results":[{"id":81,"name":"Hana Actor"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	now := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	c := newClient(server.URL+"/3/", "https://images.example/t/p/", "test", keyed("k3y"), func() time.Time { return now })
	if at, wait := c.LastRateLimit(); !at.IsZero() || wait != 0 {
		t.Errorf("before any 429: %v %v", at, wait)
	}
	limited.Store(true)
	if _, err := c.PersonDetails(t.Context(), 81, "en"); !errors.As(err, new(*RateLimited)) {
		t.Fatalf("a 429: %v", err)
	}
	if at, wait := c.LastRateLimit(); !at.Equal(now) || wait != 7*time.Second {
		t.Errorf("after a 429: %v %v", at, wait)
	}

	limited.Store(false)
	for range 2 {
		if _, err := c.PersonDetails(t.Context(), 81, "en"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.SearchPeople(t.Context(), "Hana Actor"); err != nil {
			t.Fatal(err)
		}
	}
	before := asked.Load()
	c.ForgetPerson(81, "Hana Actor")
	_, _ = c.PersonDetails(t.Context(), 81, "en")
	_, _ = c.SearchPeople(t.Context(), "Hana Actor")
	if got := asked.Load() - before; got != 2 {
		t.Errorf("after forgetting the person: %d requests, want 2", got)
	}
}

// A 429 is told as TMDB's rate even when another test closes its server
// right as the 429's connection, its answer without a body, goes back
// among the idle ones, before the answer reaches the request (#958):
// httptest.Server.Close closes the idle connections of
// http.DefaultTransport, and the client's transport is its own.
func TestARateLimitIsToldWhileAnotherServerCloses(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)
	c := newClient(server.URL+"/3/", "https://images.example/t/p/", "test", keyed("k3y"), time.Now)
	// PutIdleConn runs in that window: a server closes there, and the
	// request is given the time to see a closed connection.
	ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{PutIdleConn: func(err error) {
		if err == nil {
			httptest.NewServer(http.NotFoundHandler()).Close()
			time.Sleep(2 * time.Millisecond)
		}
	}})
	for id := range 10 {
		if _, err := c.PersonDetails(ctx, id, "en"); !errors.As(err, new(*RateLimited)) {
			t.Fatalf("a 429 while another server closes: %v", err)
		}
	}
}
