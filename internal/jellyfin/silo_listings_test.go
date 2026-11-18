package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/silo"
)

// countedSilo serves a test server's Silo API as servingSilo does, telling
// seen each request it makes of the Jellyfin API. It returns the API too,
// for the requests a test sends it directly.
func countedSilo(t *testing.T, s testServer, seen func(r *http.Request)) (siloApps, http.Handler) {
	t.Helper()
	counting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen(r)
		s.handler.ServeHTTP(w, r)
	})
	api := silo.New(silo.Options{ServerID: testServerID, Version: "test", Accounts: s.store, Jellyfin: counting, Genres: s.handler.CatalogGenres, DB: s.pool,
		QuickConnect: s.handler.QuickConnect, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ContainerTracks: s.handler.ContainerTracks})
	site := httptest.NewServer(api)
	t.Cleanup(site.Close)
	return siloApps{testServer: s, site: site.URL}, api
}

// listedByGenre reports whether a request of the Jellyfin API lists the
// titles of a genre.
func listedByGenre(r *http.Request) bool {
	return r.URL.Path == "/Items" && r.URL.Query().Get("Genres") != ""
}

// siloGenres is a filter any of whose genres a title may have, each
// named as many times, in turn, as makes count rules.
func siloGenres(count int, genres ...string) []any {
	rules := make([]any, count)
	for n := range rules {
		rules[n] = map[string]any{"field": "genre", "op": "is", "value": genres[n%len(genres)]}
	}
	return []any{map[string]any{"match": "any", "rules": rules}}
}

// Silo's apps get the titles of the genres a filter names from one
// listing of the Jellyfin API a genre, however many of its rules name it,
// in any letter case: the same titles, in the same order, as when each is
// named once.
func TestSiloFiltersListEachGenreNamedOnce(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	var listings atomic.Int64
	counted, _ := countedSilo(t, f.testServer, func(r *http.Request) {
		if listedByGenre(r) {
			listings.Add(1)
		}
	})
	titles := func(a siloApps, groups []any) []string {
		t.Helper()
		status, data := a.do(t, http.MethodPost, "/api/v2/catalog/query", f.token, f.profile,
			map[string]any{"source": "query", "library_id": f.numbers["Films"], "limit": 10, "groups": groups})
		var page siloPage
		if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
			t.Fatalf("the query: %d %s", status, data)
		}
		return cardTitles(page.Items)
	}
	if got := titles(counted, siloGenres(512, "Action", "action", "ACTION")); !slices.Equal(got, []string{"Alpha", "Gamma"}) {
		t.Errorf("512 rules of one genre: %q", got)
	}
	if asked := listings.Swap(0); asked != 1 {
		t.Errorf("512 rules of one genre: %d listings by genre, want 1", asked)
	}
	once := titles(f.siloApps, siloGenres(3, "Action", "Drama", "Horror"))
	if got := titles(counted, siloGenres(510, "Action", "Drama", "Horror", "action", "DRAMA", "horror")); len(once) != 5 || !slices.Equal(got, once) {
		t.Errorf("510 rules of three genres: %q, each named once: %q", got, once)
	}
	if asked := listings.Swap(0); asked != 3 {
		t.Errorf("510 rules of three genres: %d listings by genre, want 3", asked)
	}
}

// Once the request's context is done, as when the app closed its
// connection, a filtered page asks the Jellyfin API for no further
// listing.
func TestSiloFilterListingsEndWithTheRequest(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	var listings, after atomic.Int64
	_, api := countedSilo(t, f.testServer, func(r *http.Request) {
		if listedByGenre(r) {
			listings.Add(1)
			if r.Context().Err() != nil {
				after.Add(1)
			}
			leave()
		}
	})
	body, _ := json.Marshal(map[string]any{"source": "query", "library_id": f.numbers["Films"], "limit": 10,
		"groups": siloGenres(3, "Action", "Drama", "Horror")})
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v2/catalog/query", bytes.NewReader(body))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Authorization", "Bearer "+f.token)
	r.Header.Set("X-Profile-Id", f.profile)
	r.Header.Set("Content-Type", "application/json")
	api.ServeHTTP(httptest.NewRecorder(), r)
	if listings.Load() != 1 || after.Load() != 0 {
		t.Errorf("%d listings by genre, %d of them after the request ended, want 1 and 0", listings.Load(), after.Load())
	}
}
