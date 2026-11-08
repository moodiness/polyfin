package library

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
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
