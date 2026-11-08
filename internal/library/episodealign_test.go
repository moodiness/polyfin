package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// airedFixture is an episode of a fixture: as TMDB numbers it (season,
// number), and as the addon does (addonSeason, addonNumber, 0 for an
// episode the addon leaves out), with its name and the day TMDB tells it
// aired. The addon dates it a day later, at evening's end in UTC, as the
// most common metadata addon dates evening episodes aired in the US, and
// may spell its name otherwise (addonName).
type airedFixture struct {
	season, number           int
	addonSeason, addonNumber int
	name, addonName, day     string
}

// addonDate is the date and time the addon gives an episode TMDB tells
// aired day: the next day, at 04:00 UTC.
func addonDate(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return t.AddDate(0, 0, 1).Add(4 * time.Hour).Format("2006-01-02T15:04:05.000Z")
}

// weekly are count episodes of season, numbered alike, aired a week apart
// from first.
func weekly(season, count int, first string) []airedFixture {
	start, err := time.Parse(time.DateOnly, first)
	if err != nil {
		panic(err)
	}
	episodes := make([]airedFixture, count)
	for i := range episodes {
		n := i + 1
		episodes[i] = airedFixture{season: season, number: n, addonSeason: season, addonNumber: n,
			name: fmt.Sprintf("Chapter %d.%d", season, n), day: start.AddDate(0, 0, 7*i).Format(time.DateOnly)}
	}
	return episodes
}

// survivorSecondSeason is the report's Survivor S2: 17 episodes on TMDB,
// 16 for the addon, which leaves out "Back from the Outback", TMDB's
// eighth; its first season agrees.
func survivorSecondSeason() []airedFixture {
	episodes := weekly(1, 13, "2000-05-31")
	second := weekly(2, 17, "2001-01-28")
	for i := range second {
		switch e := &second[i]; {
		case e.number == 8:
			e.name, e.addonSeason, e.addonNumber = "Back from the Outback", 0, 0
		case e.number > 8:
			e.addonNumber--
		}
	}
	second[2].name, second[2].addonName = "Trial by Fire", "Trial By Fire!"
	return append(episodes, second...)
}

// familyGuyEighthSeason is the report's Family Guy S8: 21 episodes for the
// addon, 20 on TMDB, which counts the last among the specials, aired a
// year later.
func familyGuyEighthSeason() []airedFixture {
	episodes := weekly(8, 20, "2009-09-27")
	return append(episodes, airedFixture{season: 0, number: 28, addonSeason: 8, addonNumber: 21, name: "Partial Terms of Endearment",
		day: "2010-06-20"})
}

// splitSeason is a series whose addon counts TMDB's first two seasons, of
// 10 episodes each, as one of 20; TMDB's third is the addon's second.
func splitSeason() []airedFixture {
	episodes := slices.Concat(weekly(1, 10, "2015-01-04"), weekly(2, 10, "2015-09-06"), weekly(3, 8, "2016-09-04"))
	for i := range episodes {
		switch e := &episodes[i]; e.season {
		case 2:
			e.addonSeason, e.addonNumber = 1, 10+e.number
		case 3:
			e.addonSeason = 2
		}
	}
	return episodes
}

// releasedTogether is count episodes of season, numbered alike, released together
// on day, as a streaming service releases a part, from the number first.
func releasedTogether(season, first, count int, day string) []airedFixture {
	episodes := make([]airedFixture, count)
	for i := range episodes {
		n := first + i
		episodes[i] = airedFixture{season: season, number: n, addonSeason: season, addonNumber: n,
			name: fmt.Sprintf("Episodio %d", n), addonName: fmt.Sprintf("Part %d.%d", season, n), day: day}
	}
	return episodes
}

// addonOnly are episodes the addon numbers season and first on, which TMDB
// numbers none of.
func addonOnly(season, first, count int, day string) []airedFixture {
	episodes := releasedTogether(season, first, count, day)
	for i := range episodes {
		episodes[i].season, episodes[i].number = 0, 0
	}
	return episodes
}

// tmdbOnly are episodes TMDB numbers that the addon numbers none of.
func tmdbOnly(episodes []airedFixture) []airedFixture {
	for i := range episodes {
		episodes[i].addonSeason, episodes[i].addonNumber = 0, 0
	}
	return episodes
}

// renumbered gives the addon's season of episodes, numbered from first.
func renumbered(episodes []airedFixture, season, first int) []airedFixture {
	for i := range episodes {
		episodes[i].addonSeason, episodes[i].addonNumber = season, first+i
	}
	return episodes
}

// moneyHeist is the reporter's Money Heist (TMDB 71446, tt6468322). TMDB
// numbers the TV cut: S1, 15 weekly episodes in two runs, 2017-05-02 to
// 2017-11-23; S2, 16 released 8 on 2019-07-19 and 8 on 2020-04-03; S3, 10
// released 5 on 2021-09-03 and 5 on 2021-12-03. The addon numbers the
// streaming recut: S1, 13 on 2017-12-20; S2, 9 on 2018-04-06; S3, 8 on
// 2019-07-19; S4, 8 on 2020-04-03; S5, 10, on TMDB's S3 days.
func moneyHeist() []airedFixture {
	first := slices.Concat(weekly(1, 9, "2017-05-02"), weekly(1, 6, "2017-10-19"))
	for i := range first {
		first[i].number = i + 1
	}
	return slices.Concat(tmdbOnly(first), addonOnly(1, 1, 13, "2017-12-20"), addonOnly(2, 1, 9, "2018-04-06"),
		renumbered(releasedTogether(2, 1, 8, "2019-07-19"), 3, 1), renumbered(releasedTogether(2, 9, 8, "2020-04-03"), 4, 1),
		renumbered(releasedTogether(3, 1, 5, "2021-09-03"), 5, 1), renumbered(releasedTogether(3, 6, 5, "2021-12-03"), 5, 6))
}

// lupin is the reporter's Lupin (TMDB 96677, tt2531336). TMDB numbers S1,
// 10, released 5 on 2021-01-08 and 5 on 2021-06-11; S2, 7 on 2023-10-05;
// S3, 8 on 2026-10-23. The addon numbers S1, 5; S2, 5; S3, 7; S4, 8, on
// those days.
func lupin() []airedFixture {
	return slices.Concat(releasedTogether(1, 1, 5, "2021-01-08"), renumbered(releasedTogether(1, 6, 5, "2021-06-11"), 2, 1),
		renumbered(releasedTogether(2, 1, 7, "2023-10-05"), 3, 1), renumbered(releasedTogether(3, 1, 8, "2026-10-23"), 4, 1))
}

// addonSide and tmdbSide are the fixture's episodes as each side gives
// them.
func addonSide(fixture []airedFixture) []airedEpisode {
	var episodes []airedEpisode
	for _, e := range fixture {
		if e.addonNumber > 0 {
			episodes = append(episodes, airedEpisode{place: episodePlace{e.addonSeason, e.addonNumber}, day: episodeDay(addonDate(e.day)),
				name: airedName(cmpOrString(e.addonName, e.name))})
		}
	}
	return episodes
}

func tmdbSide(fixture []airedFixture) []airedEpisode {
	episodes := make([]airedEpisode, 0, len(fixture))
	for _, e := range fixture {
		if e.number > 0 {
			episodes = append(episodes, airedEpisode{place: episodePlace{e.season, e.number}, day: episodeDay(e.day), name: airedName(e.name)})
		}
	}
	return episodes
}

func cmpOrString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// checkNumbering checks that n pairs the fixture's episodes, both ways,
// and that those one side leaves out have none on it.
func checkNumbering(t *testing.T, name string, n EpisodeNumbering, fixture []airedFixture) {
	t.Helper()
	for _, e := range fixture {
		if e.number > 0 {
			season, number, ok := n.FromTMDB(e.season, e.number)
			switch {
			case e.addonNumber == 0 && ok:
				t.Errorf("%s: TMDB's S%dE%d, which the addon leaves out, is its S%dE%d", name, e.season, e.number, season, number)
			case e.addonNumber > 0 && (!ok || season != e.addonSeason || number != e.addonNumber):
				t.Errorf("%s: TMDB's S%dE%d is the addon's S%dE%d %v, want S%dE%d", name, e.season, e.number, season, number, ok,
					e.addonSeason, e.addonNumber)
			}
		}
		if e.addonNumber == 0 {
			continue
		}
		season, number, ok := n.ToTMDB(e.addonSeason, e.addonNumber)
		switch {
		case e.number == 0 && ok:
			t.Errorf("%s: the addon's S%dE%d, which TMDB leaves out, is its S%dE%d", name, e.addonSeason, e.addonNumber, season, number)
		case e.number > 0 && (!ok || season != e.season || number != e.number):
			t.Errorf("%s: the addon's S%dE%d is TMDB's S%dE%d %v, want S%dE%d", name, e.addonSeason, e.addonNumber, season, number, ok,
				e.season, e.number)
		}
	}
}

func TestEpisodesArePairedByTheirDayAndName(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string][]airedFixture{
		"a TMDB-only episode":           survivorSecondSeason(),
		"an episode among the specials": familyGuyEighthSeason(),
		"a split season":                splitSeason(),
		"Money Heist":                   moneyHeist(),
		"Lupin":                         lupin(),
	} {
		checkNumbering(t, name, numberingOf(alignEpisodes(addonSide(fixture), tmdbSide(fixture))), fixture)
	}
	// The reporter's cases, as TMDB numbers them, and the addon's
	// episodes, none for Money Heist's S1E14, which no episode of the
	// addon's recut aired with.
	for _, c := range []struct {
		name                     string
		fixture                  []airedFixture
		season, number           int
		addonSeason, addonNumber int
	}{
		{"Money Heist", moneyHeist(), 2, 3, 3, 3}, {"Money Heist", moneyHeist(), 2, 9, 4, 1}, {"Money Heist", moneyHeist(), 1, 14, 0, 0},
		{"Lupin", lupin(), 1, 6, 2, 1}, {"Lupin", lupin(), 2, 7, 3, 7}, {"Lupin", lupin(), 3, 6, 4, 6},
	} {
		n := numberingOf(alignEpisodes(addonSide(c.fixture), tmdbSide(c.fixture)))
		season, number, ok := n.FromTMDB(c.season, c.number)
		if want := c.addonNumber > 0; ok != want || season != c.addonSeason || number != c.addonNumber {
			t.Errorf("%s S%dE%d: the addon's S%dE%d %v, want S%dE%d", c.name, c.season, c.number, season, number, ok, c.addonSeason,
				c.addonNumber)
		}
	}
	// A series whose seasons agree keeps its numbers.
	agreeing := weekly(1, 10, "2020-01-05")
	if places := alignEpisodes(addonSide(agreeing), tmdbSide(agreeing)); places != nil {
		t.Errorf("agreeing seasons paired %v", places)
	}
}

// An addon dates an evening episode by the next day in UTC: a day apart,
// it is the same episode, two days apart it is not.
func TestEpisodeDaysAreReadInUTC(t *testing.T) {
	t.Parallel()
	tmdbDay := airedEpisode{day: episodeDay("2006-01-08")}
	for text, near := range map[string]bool{"2006-01-09T06:00:00.000Z": true, "2006-01-08T23:00:00-05:00": true, "2006-01-07": true,
		"2006-01-10T04:00:00Z": false, "2006": false, "": false} {
		if got := nearDays(airedEpisode{day: episodeDay(text)}, tmdbDay); got != near {
			t.Errorf("%q: near %v, want %v", text, got, near)
		}
	}
	if got := airedName("Trial By Fire!"); got != airedName("trial by fire") {
		t.Errorf("names: %q", got)
	}
}

// Two of TMDB's episodes aired the same day, as a double episode, pair
// with as many of the addon's in order; one the addon does not have, aired
// the same day as two of its own, pairs with none, and keeps nothing it
// cannot be sure of.
func TestEpisodesOfTheSameDayPairInOrder(t *testing.T) {
	t.Parallel()
	fixture := weekly(1, 6, "2018-03-04")
	// TMDB's fourth and fifth aired together; the addon numbers them as
	// TMDB does, but TMDB renamed them, and the second too.
	fixture[4].day = fixture[3].day
	fixture[3].addonName, fixture[4].addonName, fixture[1].addonName = "Part One", "Part Two", "Second chapter"
	// The addon counts a recap TMDB leaves out, aired the day of the
	// second: TMDB's numbers from the second on are one less.
	addon := addonSide(fixture)
	for i := range addon {
		if addon[i].place.number >= 2 {
			addon[i].place.number++
		}
	}
	addon = append(addon, airedEpisode{place: episodePlace{1, 2}, day: addon[1].day, name: airedName("Previously")})
	n := numberingOf(alignEpisodes(addon, tmdbSide(fixture)))
	for tmdbNumber, addonNumber := range map[int]int{1: 1, 4: 5, 5: 6, 6: 7} {
		if _, number, ok := n.FromTMDB(1, tmdbNumber); !ok || number != addonNumber {
			t.Errorf("TMDB's E%d: the addon's E%d %v, want E%d", tmdbNumber, number, ok, addonNumber)
		}
	}
	// TMDB's second aired the day of the addon's second and third: neither
	// is sure, and in a series numbered otherwise, none is guessed.
	if _, number, ok := n.FromTMDB(1, 2); ok {
		t.Errorf("TMDB's E2: the addon's E%d", number)
	}
	for _, number := range []int{2, 3} {
		if _, _, ok := n.ToTMDB(1, number); ok {
			t.Errorf("the addon's E%d, TMDB's E%d is not, has TMDB's numbers", number, number)
		}
	}
}

// alignedTMDB is TMDB with the episodes of the fixtures of series, by
// their TMDB identifiers: the series' details list their seasons, and
// append their episodes as TMDB does; their external identifiers give
// their IMDb ones (imdb). It records the paths and appended seasons
// asked.
type alignedTMDB struct {
	*httptest.Server
	mu     sync.Mutex
	asked  []string
	series map[int][]airedFixture
	imdb   map[int]string
}

func newAlignedTMDB(t *testing.T, series map[int][]airedFixture) *alignedTMDB {
	t.Helper()
	f := &alignedTMDB{series: series, imdb: map[int]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appended := r.URL.Query().Get("append_to_response")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked = append(f.asked, r.URL.Path+"?"+appended)
		path, external := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/3/tv/"), "/external_ids")
		id, err := strconv.Atoi(path)
		fixture, ok := f.series[id]
		switch {
		case err != nil || !ok:
			w.WriteHeader(http.StatusNotFound)
			return
		case external:
			_ = json.NewEncoder(w).Encode(map[string]any{"imdb_id": f.imdb[id]})
			return
		}
		var seasons []tmdb.SeasonSummary
		episodes := map[string][]tmdb.SeasonEpisode{}
		for _, e := range fixture {
			if e.number == 0 {
				continue
			}
			at := slices.IndexFunc(seasons, func(s tmdb.SeasonSummary) bool { return s.Season == e.season })
			if at < 0 {
				at, seasons = len(seasons), append(seasons, tmdb.SeasonSummary{Season: e.season, AirDate: e.day})
			}
			seasons[at].Episodes++
			key := "season/" + strconv.Itoa(e.season)
			episodes[key] = append(episodes[key], tmdb.SeasonEpisode{Season: e.season, Episode: e.number, Name: e.name, AirDate: e.day})
		}
		answer := map[string]any{"id": id, "seasons": seasons}
		for name := range strings.SplitSeq(appended, ",") {
			if list, ok := episodes[name]; ok {
				answer[name] = map[string]any{"episodes": list}
			}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(f.Close)
	return f
}

// requests returns the requests asked of the series of TMDB identifier id.
func (f *alignedTMDB) requests(id int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := "/3/tv/" + strconv.Itoa(id) + "?"
	return slices.DeleteFunc(slices.Clone(f.asked), func(asked string) bool { return !strings.HasPrefix(asked, prefix) })
}

// listSeries lists the series of IMDb identifier imdb, as an addon not
// numbering as TMDB does gives it, with no TMDB identifier, with the
// fixture's episodes it has.
func listSeries(t *testing.T, e env, imdb string, fixture []airedFixture) {
	t.Helper()
	meta := stremio.Meta{ID: imdb, Type: "series", Name: "Series " + imdb}
	records := []record{{ID: itemID(titleKey(KindSeries, imdb)), Key: titleKey(KindSeries, imdb), Kind: KindSeries, Meta: &meta}}
	for _, f := range fixture {
		if f.addonNumber == 0 {
			continue
		}
		video := stremio.Video{ID: fmt.Sprintf("%s:%d:%d", imdb, f.addonSeason, f.addonNumber), Season: stremio.Number(f.addonSeason),
			Episode: stremio.Number(f.addonNumber), Name: cmpOrString(f.addonName, f.name), Released: addonDate(f.day)}
		records = append(records, record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode, SeriesID: imdb,
			Season: f.addonSeason, Video: &video})
	}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}
}

// tmdbEpisodeRef names the episode TMDB numbers season and number of the
// series of TMDB identifier id, as a tracking service numbering as TMDB
// does gives it.
func tmdbEpisodeRef(id, season, number int) TitleRef {
	return TitleRef{Episode: true, TMDB: id, Season: season, Number: number, TMDBNumbered: true}
}

// The fixtures' series, by their TMDB identifiers and IMDb ones.
const (
	survivorTMDB, familyGuyTMDB, agreeingTMDB = 14658, 1434, 900
	survivorIMDb, familyGuyIMDb, agreeingIMDb = "tt0239195", "tt0182576", "tt0000900"
)

func TestResolveFindsEpisodesTMDBNumbers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	survivor, familyGuy, agreeing := survivorSecondSeason(), familyGuyEighthSeason(), weekly(1, 8, "2021-02-07")
	f := newAlignedTMDB(t, map[int][]airedFixture{survivorTMDB: survivor, familyGuyTMDB: familyGuy, agreeingTMDB: agreeing})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e.setting(func(s *accounts.Settings) { s.TMDBKey = "k3y" })
	// The series are listed by their IMDb identifiers only, which TMDB's
	// external identifiers give.
	for imdb, fixture := range map[string][]airedFixture{survivorIMDb: survivor, familyGuyIMDb: familyGuy, agreeingIMDb: agreeing} {
		listSeries(t, e, imdb, fixture)
	}
	f.mu.Lock()
	f.imdb = map[int]string{survivorTMDB: survivorIMDb, familyGuyTMDB: familyGuyIMDb, agreeingTMDB: agreeingIMDb}
	f.mu.Unlock()
	refs := []TitleRef{
		{Episode: true, IMDb: survivorIMDb, TMDB: survivorTMDB, Season: 2, Number: 9, TMDBNumbered: true},
		tmdbEpisodeRef(survivorTMDB, 2, 8),
		tmdbEpisodeRef(survivorTMDB, 2, 17),
		tmdbEpisodeRef(survivorTMDB, 1, 13),
		tmdbEpisodeRef(familyGuyTMDB, 0, 28),
		tmdbEpisodeRef(familyGuyTMDB, 8, 20),
		tmdbEpisodeRef(agreeingTMDB, 1, 3),
		// As the addon numbers it, as other services do.
		{Episode: true, IMDb: survivorIMDb, Season: 2, Number: 9},
	}
	episode := func(imdb string, season, number int) []TitleTarget {
		videoID := fmt.Sprintf("%s:%d:%d", imdb, season, number)
		return []TitleTarget{{ID: itemID(episodeKey(videoID)), Series: itemID(titleKey(KindSeries, imdb)),
			Season: itemID(seasonKey(imdb, season)), SeasonNumber: season, Number: number}}
	}
	want := [][]TitleTarget{
		episode(survivorIMDb, 2, 8),
		nil,
		episode(survivorIMDb, 2, 16),
		episode(survivorIMDb, 1, 13),
		episode(familyGuyIMDb, 8, 21),
		episode(familyGuyIMDb, 8, 20),
		episode(agreeingIMDb, 1, 3),
		episode(survivorIMDb, 2, 9),
	}
	check := func(when string) {
		t.Helper()
		got, err := e.service.Resolve(t.Context(), refs)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if !slices.Equal(got[i], want[i]) {
				t.Errorf("%s, %+v: %+v, want %+v", when, refs[i], got[i], want[i])
			}
		}
	}
	check("first")
	// The series that agree is asked its seasons, not their episodes; the
	// others, their episodes in one request.
	for id, wanted := range map[int][]string{
		agreeingTMDB:  {"/3/tv/900?"},
		survivorTMDB:  {"/3/tv/14658?", "/3/tv/14658?season/1,season/2"},
		familyGuyTMDB: {"/3/tv/1434?", "/3/tv/1434?season/8,season/0"},
	} {
		if got := f.requests(id); !slices.Equal(got, wanted) {
			t.Errorf("series %d: asked %v, want %v", id, got, wanted)
		}
	}
	// After a restart, the pairs kept answer: TMDB is not asked again.
	restarted := e.restarted()
	restarted.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	e = restarted
	check("after a restart")
	if got := len(f.requests(survivorTMDB)) + len(f.requests(familyGuyTMDB)) + len(f.requests(agreeingTMDB)); got != 5 {
		t.Errorf("TMDB asked %d times, want 5", got)
	}
	// A week later, or once the addon lists another episode, they are
	// made again, with TMDB's answers of the day.
	reunion := airedFixture{season: 2, number: 18, addonSeason: 2, addonNumber: 17, name: "Reunion", day: "2001-05-24"}
	f.mu.Lock()
	f.series[survivorTMDB] = append(slices.Clone(survivor), reunion)
	f.mu.Unlock()
	listSeries(t, e, survivorIMDb, append(slices.Clone(survivor), reunion))
	e.service.SetClock(func() time.Time { return time.Now().Add(alignmentFresh + time.Hour) })
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	refs = append(refs, tmdbEpisodeRef(survivorTMDB, 2, 18))
	want = append(want, episode(survivorIMDb, 2, 17))
	check("a week later")
	if got := len(f.requests(survivorTMDB)); got != 4 {
		t.Errorf("a week later, Survivor asked %v, want 4 requests", f.requests(survivorTMDB))
	}
}

// What a tracking service numbering as TMDB does is sent of an episode,
// then imported, marks that very episode, whichever source tells TMDB's
// numbering: a server addon describing the series as "tmdb:<id>", with no
// key, here. An episode TMDB leaves out is not sent, and one the addon
// leaves out marks none.
func TestEpisodesSentAndImportedAsTMDBNumbersThemAreTheSame(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	series := []struct {
		tmdb    int
		imdb    string
		fixture []airedFixture
	}{{1200, "tt0001200", splitSeason()}, {71446, "tt6468322", moneyHeist()}, {96677, "tt2531336", lupin()}}
	byTMDB := &fakeAddon{
		manifest: stremio.Manifest{ID: "tmdb", Name: "By TMDB", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tmdb:"}},
		metas: map[string]stremio.Meta{},
	}
	for _, s := range series {
		listSeries(t, e, s.imdb, s.fixture)
		if _, err := e.service.db.Exec(t.Context(), `UPDATE items SET data = jsonb_set(data, '{meta,_tmdbId}', to_jsonb($2::text)) WHERE key = $1`,
			titleKey(KindSeries, s.imdb), strconv.Itoa(s.tmdb)); err != nil {
			t.Fatal(err)
		}
		var videos []stremio.Video
		for _, f := range s.fixture {
			if f.number > 0 {
				videos = append(videos, stremio.Video{ID: fmt.Sprintf("tmdb:%d:%d:%d", s.tmdb, f.season, f.number), Season: stremio.Number(f.season),
					Episode: stremio.Number(f.number), Title: f.name, Released: f.day + "T00:00:00.000Z"})
			}
		}
		id := "tmdb:" + strconv.Itoa(s.tmdb)
		byTMDB.metas["series/"+id] = stremio.Meta{ID: id, Type: "series", Name: s.imdb, Videos: videos}
	}
	e.install(addons.Shared(), byTMDB)

	for _, s := range series {
		numbering, err := e.service.TMDBNumbering(t.Context(), itemID(titleKey(KindSeries, s.imdb)))
		if err != nil {
			t.Fatal(err)
		}
		checkNumbering(t, s.imdb, numbering, s.fixture)
		for _, f := range s.fixture {
			if f.number == 0 || f.addonNumber == 0 {
				continue
			}
			season, number, ok := numbering.ToTMDB(f.addonSeason, f.addonNumber)
			if !ok {
				continue // reported by checkNumbering
			}
			got, err := e.service.Resolve(t.Context(), []TitleRef{tmdbEpisodeRef(s.tmdb, season, number)})
			videoID := fmt.Sprintf("%s:%d:%d", s.imdb, f.addonSeason, f.addonNumber)
			if err != nil || len(got[0]) != 1 || got[0][0].ID != itemID(episodeKey(videoID)) {
				t.Errorf("%s: the addon's S%dE%d, sent as S%dE%d, comes back as %+v %v", s.imdb, f.addonSeason, f.addonNumber, season, number,
					got[0], err)
			}
		}
		if asked := metaAsked(byTMDB, "series", "tmdb:"+strconv.Itoa(s.tmdb)); asked != 1 {
			t.Errorf("%s: the description asked %d times", s.imdb, asked)
		}
	}
	// Money Heist's S1E14, of the TV cut, marks none of the recut.
	if got, err := e.service.Resolve(t.Context(), []TitleRef{tmdbEpisodeRef(71446, 1, 14)}); err != nil || len(got[0]) != 0 {
		t.Errorf("Money Heist's S1E14: %+v %v", got, err)
	}
}

// Without a key, an addon describing "tmdb:<id>", or TMDB's video
// identifiers, the numbers are kept, and nothing is asked or kept.
func TestResolveKeepsTMDBNumbersWithoutASource(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	fixture := survivorSecondSeason()
	f := newAlignedTMDB(t, map[int][]airedFixture{survivorTMDB: fixture})
	e.service.UseTMDB(tmdb.New(f.URL+"/3", f.URL+"/img/", "test", e.users.Settings))
	listSeries(t, e, survivorIMDb, fixture)
	imdbOnly := &fakeAddon{
		manifest: stremio.Manifest{ID: "imdb", Name: "By IMDb", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt"}},
	}
	e.install(addons.Shared(), imdbOnly)
	refs := []TitleRef{{Episode: true, IMDb: survivorIMDb, TMDB: survivorTMDB, Season: 2, Number: 9}}
	flagged := slices.Clone(refs)
	flagged[0].TMDBNumbered = true
	without, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	with, err := e.service.Resolve(t.Context(), flagged)
	if err != nil || !slices.Equal(with[0], without[0]) || len(with[0]) != 1 || with[0][0].Number != 9 {
		t.Errorf("TMDB's numbers without a source: %+v %v, want %+v", with, err, without)
	}
	if len(f.requests(survivorTMDB)) != 0 || requestsTo(imdbOnly, "meta/") != 0 {
		t.Errorf("asked TMDB %v, the addon %d times", f.requests(survivorTMDB), requestsTo(imdbOnly, "meta/"))
	}
	var kept int
	if err := e.service.db.QueryRow(t.Context(), `SELECT count(*) FROM episode_alignments`).Scan(&kept); err != nil || kept != 0 {
		t.Errorf("%d alignments kept %v", kept, err)
	}
	if numbering, err := e.service.TMDBNumbering(t.Context(), itemID(titleKey(KindSeries, survivorIMDb))); err != nil ||
		numbering.toAddon != nil {
		t.Errorf("numbering without a source: %+v %v", numbering, err)
	}
}

// An addon naming its videos by TMDB's identifiers numbers them as TMDB
// does, exactly: nothing is asked.
func TestTMDBVideoIdentifiersTellTMDBsNumbers(t *testing.T) {
	t.Parallel()
	listed := episodeList{{videoID: "tmdb:30:2:1", season: 1, number: 11}, {videoID: "tmdb:30:1:1", season: 1, number: 1}}
	places, ok := tmdbVideoPlaces(listed)
	n := numberingOf(places)
	if season, number, found := n.FromTMDB(2, 1); !ok || !found || season != 1 || number != 11 {
		t.Errorf("TMDB's S2E1: S%dE%d %v %v", season, number, found, ok)
	}
	if _, ok := tmdbVideoPlaces(episodeList{{videoID: "tt1:1:1", season: 1, number: 1}}); ok {
		t.Error("an IMDb video identifier told TMDB's numbers")
	}
}
