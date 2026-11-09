package tmdb

import (
	"errors"
	"testing"
)

// The IMDb identifier of a title tells a title TMDB does not know
// (ErrNotFound) from one it knows without an IMDb identifier (empty, no
// error), and gives the identifier when TMDB has one.
func TestIMDbIDTellsAnUnknownTitleFromOneWithoutAnIMDbID(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{
		"/3/tv/19614/external_ids":  map[string]any{"imdb_id": "tt0099864"},
		"/3/tv/113981/external_ids": map[string]any{"imdb_id": nil},
	})
	c := New(fake.URL+"/3", Images, "test", keyed("k3y"))
	ctx := t.Context()
	if id, err := c.IMDbID(ctx, TV, 19614); err != nil || id != "tt0099864" {
		t.Errorf("19614: %q, %v; want tt0099864", id, err)
	}
	if id, err := c.IMDbID(ctx, TV, 113981); err != nil || id != "" {
		t.Errorf("113981: %q, %v; want no identifier and no error", id, err)
	}
	if id, err := c.IMDbID(ctx, TV, 502119); !errors.Is(err, ErrNotFound) {
		t.Errorf("502119: %q, %v; want ErrNotFound", id, err)
	}
}

// The year of a title is that of its release date for a movie and of its
// first air date for a series, 0 without a date, ErrNotFound for a title
// TMDB does not know; the studios read after it come from the same answer,
// with no second request.
func TestTitleYearReadsTheDetailsStudiosShare(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{
		"/3/tv/19614":    map[string]any{"first_air_date": "1990-11-18", "release_date": "2001-01-01", "networks": []any{map[string]any{"name": "ABC"}}},
		"/3/movie/603":   map[string]any{"release_date": "1999-03-30", "first_air_date": "2005-01-01"},
		"/3/tv/100088":   map[string]any{"first_air_date": ""},
		"/3/movie/12345": map[string]any{},
	})
	c := New(fake.URL+"/3", Images, "test", keyed("k3y"))
	ctx := t.Context()
	for _, tc := range []struct {
		kind Kind
		id   int
		want int
	}{{TV, 19614, 1990}, {Movie, 603, 1999}, {TV, 100088, 0}, {Movie, 12345, 0}} {
		if year, err := c.TitleYear(ctx, tc.kind, tc.id); err != nil || year != tc.want {
			t.Errorf("%s %d: %d, %v; want %d", tc.kind, tc.id, year, err, tc.want)
		}
	}
	if _, err := c.TitleYear(ctx, TV, 502119); !errors.Is(err, ErrNotFound) {
		t.Errorf("502119: %v, want ErrNotFound", err)
	}
	studios, err := c.Studios(ctx, TV, "19614")
	if err != nil || len(studios) != 1 || studios[0] != "ABC" {
		t.Errorf("studios: %v, %v; want [ABC]", studios, err)
	}
	if _, err := c.MovieCollection(ctx, "603"); err != nil {
		t.Errorf("collection: %v", err)
	}
	for _, path := range []string{"/3/tv/19614", "/3/movie/603"} {
		if n := fake.asked(path); n != 1 {
			t.Errorf("%s asked %d times, want once", path, n)
		}
	}
}

// One answer of TMDB's find by IMDb identifier gives both the movie and
// the series, and finding either kind afterwards asks TMDB nothing more.
func TestFoundByIMDbGivesBothKindsFromOneAnswer(t *testing.T) {
	t.Parallel()
	fake := newFakeTMDB(t, map[string]any{
		"/3/find/tt0099864": map[string]any{
			"movie_results": []any{map[string]any{"id": 9000}},
			"tv_results":    []any{map[string]any{"id": 19614}},
		},
	})
	c := New(fake.URL+"/3", Images, "test", keyed("k3y"))
	ctx := t.Context()
	found, err := c.FoundByIMDb(ctx, "tt0099864")
	if err != nil || found != (Found{Movie: 9000, TV: 19614}) {
		t.Fatalf("found %+v, %v; want movie 9000 and series 19614", found, err)
	}
	if id, err := c.FindByIMDb(ctx, TV, "tt0099864"); err != nil || id != "19614" {
		t.Errorf("series: %q, %v; want 19614", id, err)
	}
	if id, err := c.FindByIMDb(ctx, Movie, "tt0099864"); err != nil || id != "9000" {
		t.Errorf("movie: %q, %v; want 9000", id, err)
	}
	if n := fake.asked("/3/find/tt0099864"); n != 1 {
		t.Errorf("find asked %d times, want once", n)
	}
}
