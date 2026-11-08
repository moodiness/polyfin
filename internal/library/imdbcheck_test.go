package library

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestPolyfinsDataContradictsIMDbIdentifiersOfAnotherTitle(t *testing.T) {
	t.Parallel()
	e, f := tmdbFallbackEnv(t, "k3y")
	// TMDB finds tt0702000, listed without a TMDB identifier, as movie
	// 702, and tt0704000 as movie 704; it knows nothing of tt0704001.
	// Polyfin learnt tt0704002 is movie 705, and that series 804 is
	// tt0804000.
	f.finds["tt0702000"] = map[string]any{"movie_results": []any{map[string]any{"id": 702}}}
	f.finds["tt0704000"] = map[string]any{"movie_results": []any{map[string]any{"id": 704}}}
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO title_credits (kind, provider, id, tmdb_id, asked_at)
		VALUES ('movie', 'imdb', 'tt0704002', 705, now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO title_imdb_ids (kind, tmdb_id, imdb_id, asked_at)
		VALUES ('tv', 804, 'tt0804000', now())`); err != nil {
		t.Fatal(err)
	}

	got, err := e.service.ContradictedIMDb(t.Context(), TitleRef{TMDB: 704}, []string{"tt0702000", "tt0704000", "tt0704001", "tt0704002"})
	if want := map[string]bool{"tt0702000": true, "tt0704002": true}; err != nil || !maps.Equal(got, want) {
		t.Errorf("movie 704: %v %v, want %v", got, err, want)
	}
	// Series 804 is another IMDb identifier than tt0804001.
	got, err = e.service.ContradictedIMDb(t.Context(), TitleRef{Episode: true, TMDB: 804, Season: 1, Number: 1}, []string{"tt0804000", "tt0804001"})
	if want := map[string]bool{"tt0804001": true}; err != nil || !maps.Equal(got, want) {
		t.Errorf("series 804: %v %v, want %v", got, err, want)
	}

	// TMDB failing decides nothing.
	f.mu.Lock()
	f.failing["/find/tt0704009"] = true
	f.mu.Unlock()
	if got, err := e.service.ContradictedIMDb(t.Context(), TitleRef{TMDB: 704}, []string{"tt0704009"}); err == nil {
		t.Errorf("with TMDB failing: %v", got)
	}

	// Without a key, TMDB is not asked.
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "" })
	asked := len(f.requests("/"))
	got, err = e.service.ContradictedIMDb(t.Context(), TitleRef{TMDB: 704}, []string{"tt0702000", "tt0704002"})
	if want := map[string]bool{"tt0704002": true}; err != nil || !maps.Equal(got, want) {
		t.Errorf("without a key: %v %v, want %v", got, err, want)
	}
	if len(f.requests("/")) != asked {
		t.Errorf("TMDB was asked without a key: %v", f.requests("/"))
	}
}

// TestAMovieIdentifierContradictsASeries checks tt2967286, a movie of
// series 42509 (TMDB's 225745), against the series: TMDB finding it a
// movie, or a movie listed under it, contradicts it, whatever its TMDB
// identifier; with neither, nothing does.
func TestAMovieIdentifierContradictsASeries(t *testing.T) {
	t.Parallel()
	series := TitleRef{Episode: true, TMDB: 42509, Season: 1, Number: 1}
	candidates := []string{"tt1910272", "tt2967286"}
	for _, variant := range []struct {
		name          string
		found, listed bool
		want          map[string]bool
	}{
		{name: "found a movie", found: true, want: map[string]bool{"tt2967286": true}},
		{name: "listed as a movie", listed: true, want: map[string]bool{"tt2967286": true}},
		{name: "known of nothing", want: map[string]bool{}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			t.Parallel()
			key := ""
			if variant.found {
				key = "k3y"
			}
			e, f := tmdbFallbackEnv(t, key)
			f.finds["tt1910272"] = map[string]any{"tv_results": []any{map[string]any{"id": 42509}}}
			f.finds["tt2967286"] = map[string]any{"movie_results": []any{map[string]any{"id": 225745}}}
			if variant.listed {
				meta := stremio.Meta{ID: "tt2967286", Type: "movie", Name: "The movie", TmdbID: "225745"}
				if err := e.service.save(t.Context(), []record{{ID: itemID(titleKey(KindMovie, meta.ID)), Key: titleKey(KindMovie, meta.ID), Kind: KindMovie,
					Meta: &meta}}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := e.service.ContradictedIMDb(t.Context(), series, candidates)
			if err != nil || !maps.Equal(got, variant.want) {
				t.Errorf("%v %v, want %v", got, err, variant.want)
			}
		})
	}
}
