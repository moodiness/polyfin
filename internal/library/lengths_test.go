package library

import (
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A movie whose description gives no length shows TMDB's runtime, which
// comes with its credits and details, with no request of its own; one
// whose description gives one keeps it.
func TestAMovieWithoutALengthShowsTMDBs(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	for _, id := range []string{"/movie/1001", "/movie/1002"} {
		e.tmdb.answers[id].(map[string]any)["runtime"] = 128
	}
	pier := e.addon.metas["movie/tt0882002"]
	pier.Runtime = "1h35min"
	e.addon.metas["movie/tt0882002"] = pier

	if got := e.open(e.admin, "Harbor Lights").Runtime; got != 128*time.Minute {
		t.Errorf("Harbor Lights' length: %v, want TMDB's 2h8m", got)
	}
	if got := e.open(e.admin, "Quiet Pier").Runtime; got != 95*time.Minute {
		t.Errorf("Quiet Pier's length: %v, want its description's 1h35m", got)
	}
	e.settle()
	if asked := e.tmdb.requests("/3/movie/1001?"); len(asked) != 1 {
		t.Errorf("TMDB was asked %v", asked)
	}
}

// A series' aired episode whose description gives no length shows the one
// TMDB's seasons give it, asked once as the series' page opens, in one
// request for its seasons, then kept; an episode whose description gives
// one keeps it, and one TMDB gives none of shows none.
func TestAnEpisodeWithoutALengthShowsTMDBs(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	tidewater := e.addon.metas["series/tt0882031"]
	tidewater.Videos = []stremio.Video{
		{ID: "tt0882031:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2018-01-01T00:00:00Z"},
		{ID: "tt0882031:1:2", Title: "Low Water", Season: 1, Episode: 2, Released: "2018-01-08T00:00:00Z", Runtime: "50 min"},
		{ID: "tt0882031:1:3", Title: "Spring Tide", Season: 1, Episode: 3, Released: "2018-01-15T00:00:00Z"},
	}
	e.addon.metas["series/tt0882031"] = tidewater
	answer := e.tmdb.answers["/tv/2031"].(map[string]any)
	answer["seasons"] = []any{map[string]any{"season_number": 1, "episode_count": 3, "air_date": "2018-01-01"}}
	answer["season/1"] = map[string]any{"episodes": []any{
		map[string]any{"season_number": 1, "episode_number": 1, "name": "Pilot", "air_date": "2018-01-01", "runtime": 44},
		map[string]any{"season_number": 1, "episode_number": 2, "name": "Low Water", "air_date": "2018-01-08", "runtime": 47},
		map[string]any{"season_number": 1, "episode_number": 3, "name": "Spring Tide", "air_date": "2018-01-15"},
	}}

	lengths := func() []time.Duration {
		t.Helper()
		series, err := e.service.Item(t.Context(), e.admin, e.titles["Tidewater"])
		if err != nil {
			t.Fatal(err)
		}
		e.service.AwaitEpisodeLengths(t.Context(), e.admin, series)
		episodes, err := e.service.Episodes(t.Context(), e.admin, series.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got []time.Duration
		for _, episode := range episodes {
			got = append(got, episode.Runtime)
		}
		return got
	}
	want := []time.Duration{44 * time.Minute, 50 * time.Minute, 0}
	if got := lengths(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("the episodes' lengths: %v, want %v", got, want)
	}
	seasonRequests := func() []string {
		var asked []string
		for _, request := range e.tmdb.requests("/3/tv/2031?") {
			if strings.Contains(request, "season%2F") {
				asked = append(asked, request)
			}
		}
		return asked
	}
	if asked := seasonRequests(); len(asked) != 1 {
		t.Errorf("TMDB's seasons were asked %v", asked)
	}

	// Opened again, also by a server that forgot what it remembered, the
	// series asks TMDB nothing more: the lengths are kept.
	e.service.episodeLengths.mu.Lock()
	e.service.episodeLengths.known = nil
	e.service.episodeLengths.mu.Unlock()
	if got := lengths(); len(got) != 3 || got[0] != want[0] {
		t.Errorf("the episodes' lengths again: %v", got)
	}
	e.settle()
	if asked := seasonRequests(); len(asked) != 1 {
		t.Errorf("TMDB's seasons were asked again: %v", asked)
	}
}
