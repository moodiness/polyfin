package trackers

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/library"
)

func TestTitlesAreRenumberedAsTMDBNumbersThem(t *testing.T) {
	t.Parallel()
	shifted := episode
	shifted.SetTMDBNumbers(2, 5, true)
	same := pilot
	same.SetTMDBNumbers(1, 1, true)
	missing := finale
	missing.SetTMDBNumbers(0, 0, false)
	notEpisode := movie
	notEpisode.SetTMDBNumbers(3, 4, true)
	if shifted.TMDBSeason != 2 || shifted.TMDBNumber != 5 || same.TMDBNumber != 0 || missing.TMDBSeason != 0 ||
		missing.TMDBNumber != -1 || notEpisode != movie {
		t.Fatalf("set: %+v %+v %+v %+v", shifted, same, missing, notEpisode)
	}

	list := []Title{shifted, same, missing, notEpisode}
	before := slices.Clone(list)
	got := tmdbNumbered(list)
	renumbered := episode
	renumbered.Season, renumbered.Number = 2, 5
	if want := []Title{renumbered, pilot, movie}; !reflect.DeepEqual(got, want) {
		t.Errorf("renumbered:\n got %+v\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(list, before) {
		t.Errorf("the titles given were changed: %+v", list)
	}
}

func TestTMDBNumberedServicesGetTMDBsEpisodes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.connectAll(t, "hana")
	// TMDB numbers the episode 5 of season 2, and has none of the pilot.
	shifted := episode
	shifted.TMDBSeason, shifted.TMDBNumber = 2, 5
	missing := pilot
	missing.TMDBNumber = -1
	h.Mark(user, Mark{Played: true, Titles: titles(missing, shifted)})
	h.Playback(user, play(Stopped, shifted, 50, false, false))
	// Only TMDB's episode is unplayed on PublicMetaDB, by TMDB's season.
	h.Mark(user, Mark{Played: false, Scope: ScopeSeason, Titles: titles(missing, shifted)})

	show := `"ids":{"imdb":"tt0100","tmdb":30,"tvdb":20}`
	now := clockStart.Format(time.RFC3339)
	trakt := h.f.wait(t, 1, "trakt", "/sync/history")
	sameJSON(t, "Trakt", trakt[0].body, `{"shows":[{`+show+`,"seasons":[{"number":2,"episodes":[{"number":5,"watched_at":"`+now+`"}]}]}]}`)
	mdblist := h.f.wait(t, 1, "mdblist", "/sync/watched")
	sameJSON(t, "MDBList", mdblist[0].body, `{"shows":[{`+show+`,"seasons":[{"number":2,"episodes":[{"number":5,"watched_at":"`+now+`"}]}]}]}`)
	simkl := h.f.wait(t, 1, "simkl", "/sync/history")
	sameJSON(t, "Simkl", simkl[0].body,
		`{"shows":[{`+show+`,"seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"`+now+`"},{"number":2,"watched_at":"`+now+`"}]}]}]}`)

	traktStop := h.f.wait(t, 1, "trakt", "/scrobble/stop")
	sameJSON(t, "Trakt stop", traktStop[0].body, `{"show":{`+show+`},"episode":{"season":2,"number":5},"progress":50}`)
	simklStop := h.f.wait(t, 1, "simkl", "/scrobble/stop")
	sameJSON(t, "Simkl stop", simklStop[0].body, `{"show":{`+show+`},"episode":{"season":1,"number":2},"progress":50}`)
	resume := h.f.wait(t, 1, "publicmetadb", "/api/external/resume")
	sameJSON(t, "PublicMetaDB resume point", resume[0].body,
		`{"tmdb_id":30,"media_type":"tv","season":2,"episode":5,"position_ms":3000000,"runtime_ms":6000000}`)

	h.f.wait(t, 2, "publicmetadb", "/api/external/watched")
	h.idle(t)
	var plays, removals []string
	for _, r := range h.f.sent("publicmetadb", "/api/external/watched") {
		if r.method == http.MethodPost {
			plays = append(plays, mustJSON(r.body))
		} else {
			removals = append(removals, r.query.Encode())
		}
	}
	wantPlays := `{"episode":5,"media_type":"tv","season":2,"tmdb_id":30,"watched_at":"` + now + `"}`
	if strings.Join(plays, "\n") != wantPlays || strings.Join(removals, " ") != "media_type=tv&season=2&tmdb_id=30" {
		t.Errorf("PublicMetaDB:\n%s\n%s", strings.Join(plays, "\n"), strings.Join(removals, " "))
	}
	if n := len(h.f.sent("trakt", "/sync/history")); n != 1 {
		t.Errorf("%d marks sent to Trakt", n)
	}
}

// recordingTitles finds titles as titles does, and records the refs it
// was asked.
type recordingTitles struct {
	Titles
	mu   sync.Mutex
	refs []library.TitleRef
}

func (r *recordingTitles) Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error) {
	r.mu.Lock()
	r.refs = append(r.refs, refs...)
	r.mu.Unlock()
	return r.Titles.Resolve(ctx, refs)
}

// episodes lists the episode refs asked, with season and number.
func (r *recordingTitles) episodes() []library.TitleRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found []library.TitleRef
	for _, ref := range r.refs {
		if ref.Episode {
			found = append(found, ref)
		}
	}
	return found
}

func TestImportsFindEpisodesByTheNumbersOfTheirService(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	recorded := &recordingTitles{Titles: h.titles}
	h.titles = recorded
	user := h.user(t, "ivan")

	h.connected(t, user, Trakt, "trakt-ivan")
	h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK,
		`{"movies":{"watched_at":"2026-10-01T10:00:00.000Z"},"episodes":{"watched_at":"2026-10-02T10:00:00.000Z"}}`)
	h.f.reply("GET", "/trakt/sync/history/movies", http.StatusOK, `[]`)
	h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK,
		`[{"watched_at":"2026-09-03T21:00:00.000Z","type":"episode","episode":{"season":2,"number":5,"ids":{"trakt":9}},
			"show":{"ids":{"trakt":5,"imdb":"tt0100","tmdb":30,"tvdb":20}}}]`)
	h.f.reply("GET", "/trakt/sync/playback", http.StatusOK, `[]`)
	if result := h.turnOn(t, user, Trakt); result.Problem != "" {
		t.Fatalf("Trakt: %+v", result)
	}
	traktRefs := recorded.episodes()
	if len(traktRefs) == 0 {
		t.Fatal("Trakt: no episode found")
	}
	for _, ref := range traktRefs {
		if !ref.TMDBNumbered || ref.Season != 2 || ref.Number != 5 {
			t.Errorf("Trakt: %+v", ref)
		}
	}

	h.connected(t, user, Simkl, "simkl-ivan")
	h.f.reply("GET", "/simkl/sync/activities", http.StatusOK, `{"all":"2026-10-01T00:00:00Z"}`)
	h.f.reply("GET", "/simkl/sync/all-items/shows/watching", http.StatusOK, `{"shows":[{"show":{"ids":{"imdb":"tt0100","tvdb":"20"}},
		"seasons":[{"number":1,"episodes":[{"number":2,"watched_at":"2026-09-02T00:00:00Z"}]}]}]}`)
	h.f.reply("GET", "/simkl/sync/playback", http.StatusOK, `[]`)
	if result := h.turnOn(t, user, Simkl); result.Problem != "" {
		t.Fatalf("Simkl: %+v", result)
	}
	simklRefs := recorded.episodes()[len(traktRefs):]
	if len(simklRefs) == 0 {
		t.Fatal("Simkl: no episode found")
	}
	for _, ref := range simklRefs {
		if ref.TMDBNumbered || ref.Season != 1 || ref.Number != 2 {
			t.Errorf("Simkl: %+v", ref)
		}
	}
}
