package library

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A series' poster tells a new season when its latest season, past the
// first, started within the window, a new episode when any other episode
// came out within it, and nothing for episodes before it, to come, marked
// unavailable, or specials, which are often extras.
func TestReleaseStatusTellsNewEpisodesAndSeasons(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -newReleaseDays)
	day := func(days int) string { return now.AddDate(0, 0, -days).Format(time.RFC3339) }
	unavailable := false
	for _, c := range []struct {
		name   string
		recent []stremio.Video
		older  []stremio.Number
		want   ReleaseStatus
	}{
		{"nothing recent", nil, []stremio.Number{1, 2}, NoRelease},
		{"an episode of an ongoing season", []stremio.Video{{Season: 2, Released: day(3)}}, []stremio.Number{1, 2}, NewEpisode},
		{"a season's first episode", []stremio.Video{{Season: 3, Released: day(3)}}, []stremio.Number{1, 2}, NewSeason},
		{"a whole season at once", []stremio.Video{{Season: 3, Released: day(10)}, {Season: 3, Released: day(10)}}, []stremio.Number{1, 2}, NewSeason},
		{"a new series", []stremio.Video{{Season: 1, Released: day(3)}}, nil, NewEpisode},
		{"specials", []stremio.Video{{Season: 0, Released: day(3)}}, []stremio.Number{1}, NoRelease},
		{"a special beside an episode", []stremio.Video{{Season: 0, Released: day(3)}, {Season: 1, Released: day(9)}}, []stremio.Number{1}, NewEpisode},
		{"an episode to come", []stremio.Video{{Season: 3, Released: day(-3)}}, []stremio.Number{1, 2}, NoRelease},
		{"an episode before the window", []stremio.Video{{Season: 3, Released: day(newReleaseDays + 1)}}, []stremio.Number{1, 2}, NoRelease},
		{"an episode not available", []stremio.Video{{Season: 3, Released: day(3), Available: &unavailable}}, []stremio.Number{1, 2}, NoRelease},
		{"an episode without a date", []stremio.Video{{Season: 3}}, []stremio.Number{1, 2}, NoRelease},
	} {
		if got := releaseStatus(c.recent, c.older, since, now); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// The poster address template fills in a title's identifiers and type,
// and leaves a title lacking one it names, or that is no movie or series,
// to its addon's poster.
func TestPosterAddressFillsTheTemplate(t *testing.T) {
	ids := map[string]string{"Imdb": "tt0000", "Tmdb": "10"}
	for _, c := range []struct {
		template string
		kind     Kind
		ids      map[string]string
		want     string
		ok       bool
	}{
		{"https://posters.example/{type}/{imdb_id}.jpg", KindMovie, ids, "https://posters.example/movie/tt0000.jpg", true},
		{"https://posters.example/{type}/{tmdb_id}?imdb={imdb_id}", KindSeries, ids, "https://posters.example/series/10?imdb=tt0000", true},
		{"https://posters.example/{tmdb_type}/{tmdb_id}.jpg", KindSeries, ids, "https://posters.example/tv/10.jpg", true},
		{"https://posters.example/{tmdb_type}/{tmdb_id}-{type}.jpg", KindMovie, ids, "https://posters.example/movie/10-movie.jpg", true},
		{"https://posters.example/{tmdb_id}.jpg", KindMovie, map[string]string{"Imdb": "tt0000"}, "", false},
		{"https://posters.example/{imdb_id}.jpg", KindEpisode, ids, "", false},
		{"", KindMovie, ids, "", false},
	} {
		if got, ok := PosterAddress(c.template, c.kind, c.ids); got != c.want || ok != c.ok {
			t.Errorf("%s for a %s: %q %v, want %q %v", c.template, c.kind, got, ok, c.want, c.ok)
		}
	}
}
