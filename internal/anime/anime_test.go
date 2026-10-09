package anime

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// idsFixture is an identifier list: a series of two entries (the second
// season's first half, then its second half as AniDB splits it), a long
// series numbered absolutely, one whose episodes the mapping-list names,
// a movie, and an entry as older lists spelled them.
const idsFixture = `[
	{"type":"TV","anidb_id":100,"kitsu_id":200,"mal_id":300,"anilist_id":400,"imdb_id":["tt0000100"],"tvdb_id":5000,
		"themoviedb_id":{"tv":6000},"season":{"tvdb":1,"tmdb":1}},
	{"type":"TV","anidb_id":101,"kitsu_id":201,"mal_id":301,"anilist_id":401,"imdb_id":["tt0000100"],"tvdb_id":5000,
		"themoviedb_id":{"tv":6000}},
	{"type":"TV","anidb_id":102,"kitsu_id":202,"mal_id":302,"imdb_id":["tt0000200"],"tvdb_id":5100},
	{"type":"TV","anidb_id":103,"kitsu_id":203,"imdb_id":["tt0000300"],"tvdb_id":5200},
	{"type":"MOVIE","anidb_id":104,"kitsu_id":204,"mal_id":304,"imdb_id":["tt0000400"],"themoviedb_id":{"movie":[7000]}},
	{"type":"OVA","anidb_id":105,"kitsu_id":"205","thetvdb_id":"5300","imdb_id":"tt0000500","themoviedb_id":8000}
]`

// episodesFixture maps those entries' episodes to TVDB: 101's first
// episode is the 13th of TVDB's first season, 102 follows TVDB's absolute
// numbering, 103's are partly named one by one and by range, 104 is a
// movie TVDB has no series of, 105 one of a series' specials.
const episodesFixture = `<?xml version="1.0" encoding="UTF-8"?>
<anime-list>
  <anime anidbid="100" tvdbid="5000" defaulttvdbseason="1" episodeoffset="" tmdbid="" imdbid="">
    <name>First</name>
  </anime>
  <anime anidbid="101" tvdbid="5000" defaulttvdbseason="1" episodeoffset="12">
    <name>First part 2</name>
  </anime>
  <anime anidbid="102" tvdbid="5100" defaulttvdbseason="a" episodeoffset="">
    <name>Long</name>
  </anime>
  <anime anidbid="103" tvdbid="5200" defaulttvdbseason="2">
    <name>Mapped</name>
    <mapping-list>
      <mapping anidbseason="0" tvdbseason="0">;1-3;2-0;</mapping>
      <mapping anidbseason="1" tvdbseason="2">;5-6+7;</mapping>
      <mapping anidbseason="1" tvdbseason="3" start="11" end="20" offset="-10"/>
      <mapping anidbseason="0" tmdbseason="0">;3-9;</mapping>
    </mapping-list>
    <supplemental-info><studio>A studio</studio></supplemental-info>
  </anime>
  <anime anidbid="104" tvdbid="movie" defaulttvdbseason="1" imdbid="tt0000400">
    <name>The movie</name>
  </anime>
  <anime anidbid="105" tvdbid="5300" defaulttvdbseason="0" episodeoffset="4">
    <name>A special</name>
  </anime>
</anime-list>`

// lists serves the two lists, with an ETag, answering 304 to a request
// that names it, or fails with status while failing is set.
type lists struct {
	mu       sync.Mutex
	failing  int
	garbage  bool
	requests []*http.Request
	server   *httptest.Server
}

func serveLists(t *testing.T) *lists {
	t.Helper()
	l := &lists{}
	l.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.requests = append(l.requests, r)
		failing, garbage := l.failing, l.garbage
		l.mu.Unlock()
		if failing != 0 {
			w.WriteHeader(failing)
			return
		}
		if garbage {
			w.Header().Set("ETag", `"other"`)
			_, _ = w.Write([]byte("<html>Not here</html>"))
			return
		}
		body, etag := idsFixture, `"ids-1"`
		if r.URL.Path == "/episodes.xml" {
			body, etag = episodesFixture, `"episodes-1"`
		}
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(l.server.Close)
	return l
}

func (l *lists) fail(status int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failing = status
}

func (l *lists) sent() []*http.Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*http.Request(nil), l.requests...)
}

// start runs a service keeping its copies in dir and reading the lists
// from l, checking them every refreshEvery.
func (l *lists) start(t *testing.T, dir string, refreshEvery time.Duration) *Service {
	t.Helper()
	s := newService(Options{Dir: dir, Version: "1.2.3", IDsURL: l.server.URL + "/ids.json", EpisodesURL: l.server.URL + "/episodes.xml"},
		timing{refreshEvery: refreshEvery, retryFirst: 5 * time.Millisecond, retryMax: 20 * time.Millisecond, download: 5 * time.Second})
	s.start()
	t.Cleanup(s.Close)
	return s
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("%s never happened", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ready is a service that read both lists from fixtures.
func ready(t *testing.T) *Service {
	t.Helper()
	s := serveLists(t).start(t, t.TempDir(), time.Hour)
	eventually(t, "the lists being read", s.Ready)
	return s
}

func anidbOf(entries []IDs) []int {
	var ids []int
	for _, e := range entries {
		ids = append(ids, e.AniDB)
	}
	return ids
}

func TestIdentifiersMapBetweenSources(t *testing.T) {
	s := ready(t)
	for _, c := range []struct {
		source Source
		id     string
		want   []int
	}{
		{Kitsu, "201", []int{101}},
		{MyAnimeList, "300", []int{100}},
		{AniList, "401", []int{101}},
		{AniDB, "103", []int{103}},
		// A series AniDB splits in two entries.
		{TVDB, "5000", []int{100, 101}},
		{IMDb, "tt0000100", []int{100, 101}},
		{TMDBShow, "6000", []int{100, 101}},
		{TMDBMovie, "7000", []int{104}},
		{TMDBMovie, "6000", nil},
		// Spelled as older lists did.
		{Kitsu, "205", []int{105}},
		{TVDB, "5300", []int{105}},
		{IMDb, "tt0000500", []int{105}},
		{TMDBShow, "8000", []int{105}},
		{Kitsu, "999", nil},
		{Kitsu, "abc", nil},
	} {
		if got := anidbOf(s.IDs(c.source, c.id)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s: %v, want %v", c.source, c.id, got, c.want)
		}
	}
	movie := s.IDs(Kitsu, "204")
	if len(movie) != 1 || !movie[0].Movie || !reflect.DeepEqual(movie[0].TMDBMovies, []int{7000}) || !reflect.DeepEqual(movie[0].IMDb, []string{"tt0000400"}) {
		t.Errorf("movie: %+v", movie)
	}
	if old := s.IDs(AniDB, "105"); len(old) != 1 || old[0].Movie || old[0].TMDBShow != 8000 || old[0].Kitsu != 205 {
		t.Errorf("older entry: %+v", old)
	}
}

func TestAniDBEpisodesMapToTVDB(t *testing.T) {
	s := ready(t)
	for _, c := range []struct {
		anidb, number int
		special       bool
		want          Place
		ok            bool
	}{
		{anidb: 100, number: 3, want: Place{TVDB: 5000, Season: 1, Number: 3}, ok: true},
		// The default season, after the offset.
		{anidb: 101, number: 1, want: Place{TVDB: 5000, Season: 1, Number: 13}, ok: true},
		// Absolute numbering.
		{anidb: 102, number: 400, want: Place{TVDB: 5100, Number: 400, Absolute: true}, ok: true},
		// Named one by one, the first of two TVDB episodes winning.
		{anidb: 103, number: 5, want: Place{TVDB: 5200, Season: 2, Number: 6}, ok: true},
		// A range of the mapping-list, over the default.
		{anidb: 103, number: 12, want: Place{TVDB: 5200, Season: 3, Number: 2}, ok: true},
		{anidb: 103, number: 4, want: Place{TVDB: 5200, Season: 2, Number: 4}, ok: true},
		// Specials: as the mapping-list names them, none mapped to 0, nor
		// those only TMDB's seasons name, nor those it does not name.
		{anidb: 103, number: 1, special: true, want: Place{TVDB: 5200, Season: 0, Number: 3}, ok: true},
		{anidb: 103, number: 2, special: true},
		{anidb: 103, number: 3, special: true},
		{anidb: 100, number: 1, special: true},
		// An entry that is one of a series' specials.
		{anidb: 105, number: 1, want: Place{TVDB: 5300, Season: 0, Number: 5}, ok: true},
		// A movie, and an entry the list does not know.
		{anidb: 104, number: 1},
		{anidb: 999, number: 1},
	} {
		got, ok := s.Episode(c.anidb, c.special, c.number)
		if ok != c.ok || got != c.want {
			t.Errorf("AniDB %d episode %d (special %v): %+v %v, want %+v %v", c.anidb, c.number, c.special, got, ok, c.want, c.ok)
		}
	}
}

func TestTitlesAreNamedAsPolyfinNamesThem(t *testing.T) {
	s := ready(t)
	for _, c := range []struct {
		source  Source
		id      int
		special bool
		number  int
		movie   bool
		want    Title
		ok      bool
	}{
		// An episode of the second half, by Kitsu: the series' IMDb and
		// TVDB identifiers, in TVDB's numbering.
		{source: Kitsu, id: 201, number: 2, want: Title{IMDb: "tt0000100", TVDB: 5000, Season: 1, Number: 14}, ok: true},
		{source: MyAnimeList, id: 302, number: 7, want: Title{IMDb: "tt0000200", TVDB: 5100, Number: 7, Absolute: true}, ok: true},
		// A movie, whether or not the caller knows it is one.
		{source: Kitsu, id: 204, number: 1, movie: true, want: Title{Movie: true, IMDb: "tt0000400", TMDB: 7000}, ok: true},
		{source: AniDB, id: 104, number: 1, want: Title{Movie: true, IMDb: "tt0000400", TMDB: 7000}, ok: true},
		// One of a series' specials, with the series' IMDb identifier.
		{source: AniDB, id: 105, number: 1, want: Title{IMDb: "tt0000500", TVDB: 5300, Season: 0, Number: 5}, ok: true},
		{source: Kitsu, id: 999, number: 1},
	} {
		got, ok := s.Title(c.source, c.id, c.special, c.number, c.movie)
		if ok != c.ok || got != c.want {
			t.Errorf("%s %d episode %d: %+v %v, want %+v %v", c.source, c.id, c.number, got, ok, c.want, c.ok)
		}
	}
	for _, c := range []struct {
		id             string
		source         Source
		entry, episode int
		ok             bool
	}{
		{"kitsu:201:2", Kitsu, 201, 2, true},
		{"mal:302", MyAnimeList, 302, 0, true},
		{"anidb:104", AniDB, 104, 0, true},
		{"tt0000100:1:2", "", 0, 0, false},
		{"kitsu:x:1", "", 0, 0, false},
		{"kitsu:1:2:3", "", 0, 0, false},
	} {
		source, entry, episode, ok := StremioID(c.id)
		if source != c.source || entry != c.entry || episode != c.episode || ok != c.ok {
			t.Errorf("%s: %s %d %d %v", c.id, source, entry, episode, ok)
		}
	}
}

func TestRefreshKeepsTheLastCopyAndAsksConditionally(t *testing.T) {
	l := serveLists(t)
	dir := t.TempDir()
	first := l.start(t, dir, time.Hour)
	eventually(t, "the lists being read", first.Ready)
	first.Close()
	downloads := len(l.sent())
	if downloads != 2 || l.sent()[0].Header.Get("User-Agent") != "Polyfin/1.2.3" {
		t.Fatalf("%d downloads", downloads)
	}

	// A restart reads the copies kept, fresh, without asking again.
	l.fail(http.StatusInternalServerError)
	restarted := l.start(t, dir, time.Hour)
	eventually(t, "the copies being read", restarted.Ready)
	time.Sleep(50 * time.Millisecond)
	restarted.Close()
	if n := len(l.sent()); n != downloads {
		t.Errorf("a restart asked %d times", n-downloads)
	}

	// Due again, the lists are asked for conditionally: unchanged, they
	// stay.
	l.fail(0)
	again := l.start(t, dir, 30*time.Millisecond)
	eventually(t, "conditional requests", func() bool {
		conditional := 0
		for _, r := range l.sent()[downloads:] {
			if r.Header.Get("If-None-Match") != "" {
				conditional++
			}
		}
		return conditional >= 2
	})
	for _, r := range l.sent()[downloads:] {
		if want := map[string]string{"/ids.json": `"ids-1"`, "/episodes.xml": `"episodes-1"`}[r.URL.Path]; r.Header.Get("If-None-Match") != want {
			t.Errorf("%s asked with %q", r.URL.Path, r.Header.Get("If-None-Match"))
		}
	}
	// Then failing, or answering what is not the list, the last copy stays
	// in use, and kept.
	for _, broken := range []struct {
		status  int
		garbage bool
	}{{status: http.StatusInternalServerError}, {garbage: true}} {
		l.mu.Lock()
		l.requests = nil
		l.failing, l.garbage = broken.status, broken.garbage
		l.mu.Unlock()
		eventually(t, "failed refreshes", func() bool { return len(l.sent()) >= 4 })
		if got := anidbOf(again.IDs(Kitsu, "201")); !reflect.DeepEqual(got, []int{101}) {
			t.Errorf("after %+v: %v", broken, got)
		}
		if place, ok := again.Episode(101, false, 1); !ok || place.Number != 13 {
			t.Errorf("after %+v: %+v %v", broken, place, ok)
		}
	}
	again.Close()
	if kept, err := os.ReadFile(filepath.Join(dir, "anime-list-master.xml")); err != nil || string(kept) != episodesFixture {
		t.Errorf("copy kept: %d bytes, %v", len(kept), err)
	}
}
