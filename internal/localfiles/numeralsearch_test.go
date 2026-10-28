package localfiles

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// literalTitles are the titles the literal addon knows.
var literalTitles = []stremio.Meta{
	{ID: "tt2000001", Type: "movie", Name: "Harbor Lights II", Year: "2011"},
	{ID: "tt2000002", Type: "movie", Name: "Tidewater Part One", Year: "2015"},
	{ID: "tt2000003", Type: "movie", Name: "Gull Point 2", Year: "2010"},
	{ID: "tt2000004", Type: "movie", Name: "Quay Street 3", Year: "1999"},
	{ID: "tt2000005", Type: "movie", Name: "Broken Reel II", Year: "2000"},
}

// literalAddon is a metadata addon whose movie search finds the
// literalTitles whose name holds the term as it is written, so that a
// search for "Harbor Lights 2" finds no "Harbor Lights II"; its search for
// "Broken Reel II" fails. It records the terms searched.
type literalAddon struct {
	mu    sync.Mutex
	terms []string
}

func (a *literalAddon) start(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta.literal", Name: "Meta", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
		case strings.HasPrefix(path, "/catalog/movie/top/search="):
			term, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/movie/top/search="), ".json"))
			a.mu.Lock()
			a.terms = append(a.terms, term)
			a.mu.Unlock()
			if term == "Broken Reel II" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			metas := []stremio.Meta{}
			for _, meta := range literalTitles {
				if strings.Contains(strings.ToLower(meta.Name), strings.ToLower(term)) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/"):
			id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
			for _, meta := range literalTitles {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// searched returns the terms searched since the last call, sorted.
func (a *literalAddon) searched() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	terms := slices.Sorted(slices.Values(a.terms))
	a.terms = nil
	return terms
}

// A file whose search finds no title of its name is searched once more
// with its numerals written the other way, Arabic for Roman, Roman for
// Arabic, figures for words after "Part" and back, and what that finds is
// matched by the looser rules, flagged. A file found at once, or without
// such a numeral, is searched once; a unit unlinked from a looser match is
// matched by the exact rule only, without the second search.
func TestNumeralsSearchedTheOtherWay(t *testing.T) {
	t.Parallel()
	addon := &literalAddon{}
	e := newEnvWith(t, addon.start(t))
	ctx := t.Context()
	movies, _ := e.folder(KindMovies, map[string]int{
		"Harbor Lights 2 (2011).mkv":  10,
		"Tidewater Part 1 (2015).mkv": 10,
		"Gull Point II (2009).mkv":    10,
		"Quay Street 3 (1999).mkv":    10,
		"Lost Reel 4 (1931).mkv":      10,
		"Unknown Picture (1950).mkv":  10,
		"Broken Reel 2 (2000).mkv":    10,
	})
	want := map[string]string{
		"Harbor Lights 2 (2011).mkv":  "tt2000001 by numerals",
		"Tidewater Part 1 (2015).mkv": "tt2000002 by numerals",
		"Gull Point II (2009).mkv":    "tt2000003 by year_numerals",
		"Quay Street 3 (1999).mkv":    "tt2000004 by search",
		"Lost Reel 4 (1931).mkv":      "unmatched: not_found",
		"Unknown Picture (1950).mkv":  "unmatched: not_found",
		"Broken Reel 2 (2000).mkv":    "unmatched: search_failed",
	}
	if got := e.matches(movies.ID); !maps.Equal(got, want) {
		t.Errorf("matched: %v", got)
	}
	// The failed search is tried twice more within the scan (see
	// serverWaits).
	if got, want := addon.searched(), []string{"Broken Reel 2", "Broken Reel II", "Broken Reel II", "Broken Reel II", "Gull Point 2", "Gull Point II", "Harbor Lights 2",
		"Harbor Lights II", "Lost Reel 4", "Lost Reel IV", "Quay Street 3", "Tidewater Part 1", "Tidewater Part One", "Unknown Picture"}; !slices.Equal(got, want) {
		t.Errorf("searched: %q", got)
	}
	if folder, err := e.service.Folder(ctx, movies.ID); err != nil || folder.MatchedBy != (Matches{Search: 1, Numerals: 2, YearNumerals: 1}) {
		t.Errorf("figures: %+v %v", folder.MatchedBy, err)
	}

	// The library may answer a search it made again from what it read: the
	// searches asked of it are counted.
	before := e.titles.searches.Load()
	if err := e.service.Unlink(ctx, movies.ID, "Harbor Lights 2 (2011).mkv"); err != nil {
		t.Fatal(err)
	}
	if got := e.matches(movies.ID)["Harbor Lights 2 (2011).mkv"]; got != "unmatched: not_found" {
		t.Errorf("unlinked: %s", got)
	}
	if searched := e.titles.searches.Load() - before; searched != 1 {
		t.Errorf("searches once unlinked: %d", searched)
	}
}
