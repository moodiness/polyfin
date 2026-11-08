package library

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// skipsByPlace stands in for PublicMetaDB: it gives every episode asked
// about an intro ending at its season times 100 plus its number, in
// seconds, so that the segments tell which episode was asked, and
// records the episodes asked, as "<tmdb id> S<season>E<number>".
type skipsByPlace struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string
}

func newSkipsByPlace(t *testing.T) *skipsByPlace {
	t.Helper()
	f := &skipsByPlace{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		season, _ := strconv.Atoi(q.Get("season"))
		number, _ := strconv.Atoi(q.Get("episode"))
		f.mu.Lock()
		f.asked = append(f.asked, fmt.Sprintf("%s S%dE%d", q.Get("tmdb_id"), season, number))
		f.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"items":[{"id":"r1","tmdb_id":%s,"media_type":"tv","season":%d,"episode":%d,"source":"streaming",
			"created":"2026-01-01T00:00:00Z","intro_start_ms":0,"intro_end_ms":%d,"credits_start_ms":null,"credits_end_ms":null,
			"recap_start_ms":null,"recap_end_ms":null,"preview_start_ms":null,"preview_end_ms":null}],"total":1,"page":1,"perPage":200,
			"totalPages":1}`, q.Get("tmdb_id"), season, number, (season*100+number)*1000)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *skipsByPlace) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// PublicMetaDB knows episodes by TMDB's numbers: the skip segments of the
// reporter's episodes are asked with them, as the series' numbering
// pairs them, and those of an episode no episode of TMDB is surely, not
// at all. Playing them again asks TMDB nothing more.
func TestSkipSegmentsAreAskedWithTMDBsNumbers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	series := []struct {
		tmdb    int
		imdb    string
		fixture []airedFixture
	}{{71446, "tt6468322", moneyHeist()}, {96677, "tt2531336", lupin()}, {agreeingTMDB, agreeingIMDb, weekly(1, 8, "2021-02-07")}}
	byTMDB := map[int][]airedFixture{}
	for _, s := range series {
		byTMDB[s.tmdb] = s.fixture
	}
	f := newAlignedTMDB(t, byTMDB)
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	for _, s := range series {
		listSeries(t, e, s.imdb, s.fixture)
		if _, err := e.service.db.Exec(t.Context(), `UPDATE items SET data = jsonb_set(data, '{meta,_tmdbId}', to_jsonb($2::text)) WHERE key = $1`,
			titleKey(KindSeries, s.imdb), strconv.Itoa(s.tmdb)); err != nil {
			t.Fatal(err)
		}
	}
	skips := newSkipsByPlace(t)
	segments := mediasegments.New(e.service.db, []mediasegments.Source{{Name: mediasegments.PublicMetaDB, URL: skips.URL}}, "test",
		slog.New(slog.NewTextHandler(io.Discard, nil)), func() accounts.Settings {
			return accounts.Settings{PublicMetaDBKey: "pm-Good4mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"}
		})
	cases := []struct {
		name                     string
		tmdb                     int
		imdb                     string
		addonSeason, addonNumber int
		// season and number are TMDB's, which PublicMetaDB is asked
		// with; 0 when it is not asked.
		season, number int
	}{
		{"Money Heist S3E3", 71446, "tt6468322", 3, 3, 2, 3},
		{"Money Heist S4E1", 71446, "tt6468322", 4, 1, 2, 9},
		{"Money Heist S1E5, of the recut", 71446, "tt6468322", 1, 5, 0, 0},
		{"Lupin S2E1", 96677, "tt2531336", 2, 1, 1, 6},
		{"Lupin S4E6", 96677, "tt2531336", 4, 6, 3, 6},
		{"a series that agrees", agreeingTMDB, agreeingIMDb, 1, 3, 1, 3},
	}
	// Each episode is played twice, as segments are asked at each
	// playback; the second time with TMDB's answers no longer in memory,
	// as after a restart.
	for playback := range 2 {
		if playback > 0 {
			e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
		}
		for _, c := range cases {
			before := len(skips.requests())
			numbering, err := e.service.TMDBNumbering(t.Context(), itemID(titleKey(KindSeries, c.imdb)))
			if err != nil {
				t.Fatal(err)
			}
			title := mediasegments.Title{Item: itemID(episodeKey(fmt.Sprintf("%s:%d:%d", c.imdb, c.addonSeason, c.addonNumber))),
				IMDb: c.imdb, TMDB: strconv.Itoa(c.tmdb), Season: c.addonSeason, Episode: c.addonNumber, Runtime: time.Hour}
			title.SetTMDBNumbers(numbering.ToTMDB(c.addonSeason, c.addonNumber))
			got := segments.Segments(t.Context(), title)
			var want []string
			if c.number > 0 && playback == 0 {
				want = []string{fmt.Sprintf("%d S%dE%d", c.tmdb, c.season, c.number)}
			}
			if asked := skips.requests()[before:]; !slices.Equal(asked, want) {
				t.Errorf("%s, playback %d: PublicMetaDB asked %v, want %v", c.name, playback, asked, want)
			}
			switch {
			case c.number == 0 && len(got) != 0:
				t.Errorf("%s: segments %+v, want none", c.name, got)
			case c.number > 0 && (len(got) != 1 || got[0].End != time.Duration(c.season*100+c.number)*time.Second):
				t.Errorf("%s: segments %+v, want those of TMDB's S%dE%d", c.name, got, c.season, c.number)
			}
		}
	}
	// TMDB was asked once for each series' seasons, and once more for the
	// episodes of those numbered otherwise, however many playbacks.
	for id, wanted := range map[int]int{71446: 2, 96677: 2, agreeingTMDB: 1} {
		if got := f.requests(id); len(got) != wanted {
			t.Errorf("series %d: TMDB asked %v, want %d requests", id, got, wanted)
		}
	}
}
