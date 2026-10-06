package jellyfin

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/stremio"
)

// skipAddon serves a movie with two streams and a series of four episodes,
// each with four streams, with the identifiers segment databases know
// titles by. The third episode is listed as 53 minutes long, the fourth as
// 59.
func skipAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt1234567", Type: "movie", Name: "Movie", Runtime: "2h", TmdbID: "550"}
	show := stremio.Meta{ID: "tt7654321", Type: "series", Name: "Show", Runtime: "45min",
		Videos: []stremio.Video{
			{ID: "tt7654321:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2010-06-16T00:00:00.000Z", Runtime: "45min"},
			{ID: "tt7654321:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2010-06-23T00:00:00.000Z", Runtime: "45min"},
			{ID: "tt7654321:1:3", Title: "Third", Season: 1, Episode: 3, Released: "2010-06-30T00:00:00.000Z", Runtime: "53min"},
			{ID: "tt7654321:1:4", Title: "Fourth", Season: 1, Episode: 4, Released: "2010-07-07T00:00:00.000Z", Runtime: "59min"},
		}}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "skips", Name: "Skips", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case "/catalog/movie/top.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case "/catalog/series/shows.json":
			preview := show
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case "/meta/movie/tt1234567.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case "/meta/series/tt7654321.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		case "/stream/movie/tt1234567.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source 2160p", URL: server.URL + "/files/remux.mkv"},
				{Name: "Source 1080p", URL: server.URL + "/files/web.mp4"},
			}})
		default:
			if strings.HasPrefix(path, "/stream/series/tt7654321") {
				var streams []stremio.Stream
				for _, cut := range []string{"a", "b", "c", "d"} {
					streams = append(streams, stremio.Stream{Name: "Cut " + cut, URL: server.URL + "/files/" + cut + ".mkv"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
				return
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// segmentDatabases stands in for TheIntroDB (/media) and IntroDB
// (/segments), and records what they were asked.
type segmentDatabases struct {
	url   string
	mu    sync.Mutex
	asked []string
}

func newSegmentDatabases(t *testing.T) *segmentDatabases {
	t.Helper()
	d := &segmentDatabases{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.asked = append(d.asked, r.URL.Path+"?"+r.URL.RawQuery)
		d.mu.Unlock()
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/media" && q.Get("tmdb_id") == "550":
			_, _ = io.WriteString(w, `{"tmdb_id":550,"type":"movie","intro":[{"start_ms":null,"end_ms":90000}],
				"credits":[{"start_ms":6900000,"end_ms":null}]}`)
		case r.URL.Path == "/media" && q.Get("imdb_id") == "tt7654321" && q.Get("season") == "1" && q.Get("episode") == "2":
			_, _ = io.WriteString(w, `{"tmdb_id":1,"type":"tv","season":1,"episode":2,"intro":[{"start_ms":60000,"end_ms":95000}]}`)
		case r.URL.Path == "/media" && q.Get("imdb_id") == "tt7654321" && q.Get("season") == "1" && q.Get("episode") == "3":
			_, _ = io.WriteString(w, `{"tmdb_id":1,"type":"tv","season":1,"episode":3,"intro":[{"start_ms":321400,"end_ms":400400}],
				"credits":[{"start_ms":3072000,"end_ms":3304000}]}`)
		case r.URL.Path == "/media" && q.Get("imdb_id") == "tt7654321" && q.Get("season") == "1" && q.Get("episode") == "4":
			_, _ = io.WriteString(w, `{"tmdb_id":1,"type":"tv","season":1,"episode":4,"credits":[{"start_ms":3552300,"end_ms":3600400}]}`)
		case r.URL.Path == "/segments" && q.Get("imdb_id") == "tt1234567":
			_, _ = io.WriteString(w, `{"imdb_id":"tt1234567","media_type":"movie","is_movie":true,"season":0,"episode":0,
				"intro":{"start_ms":10000,"end_ms":50000},"recap":{"start_ms":5000,"end_ms":45000},"outro":null,
				"post_credits":{"start_ms":7100000,"end_ms":7150000}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"media not found"}`)
		}
	}))
	t.Cleanup(server.Close)
	d.url = server.URL
	return d
}

func (d *segmentDatabases) requests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.asked)
}

type skipSetup struct {
	testServer
	token, movie, series, episode, third, fourth string
	user                                         accounts.User
	databases                                    *segmentDatabases
}

// skipping installs the skip addon's catalogs as libraries, asks the fake
// segment databases named, in that order, and signs a member in.
func skipping(t *testing.T, names ...string) skipSetup {
	t.Helper()
	s := newTestServer(t, 10)
	d := newSegmentDatabases(t)
	var sources []mediasegments.Source
	for _, name := range names {
		sources = append(sources, mediasegments.Source{Name: name, URL: d.url})
	}
	s.handler.Segments = mediasegments.New(s.pool, sources, "test", s.handler.Logger, s.store.Settings)
	user := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), skipAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	p := skipSetup{testServer: s, token: s.signIn("member", "tv"), user: user, databases: d}
	var views QueryResult
	s.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, p.token, &page)
		for _, item := range page.Items {
			switch item.Name {
			case "Movie":
				p.movie = item.Id
			case "Show":
				p.series = item.Id
			}
		}
	}
	var episodes QueryResult
	s.get(t, "/Shows/"+p.series+"/Episodes", p.token, &episodes)
	for _, item := range episodes.Items {
		switch item.Name {
		case "Second":
			p.episode = item.Id
		case "Third":
			p.third = item.Id
		case "Fourth":
			p.fourth = item.Id
		}
	}
	if p.movie == "" || p.episode == "" || p.third == "" || p.fourth == "" {
		t.Fatalf("movie %q, episodes %q %q %q", p.movie, p.episode, p.third, p.fourth)
	}
	return p
}

// versions lists the versions of a title, asking the addon.
func (p skipSetup) versions(t *testing.T, item string) []library.Version {
	t.Helper()
	id, err := accounts.ParseID(item)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := p.library.Versions(t.Context(), p.user, id)
	if err != nil {
		t.Fatal(err)
	}
	return versions
}

// chapter is a chapter from and to times in milliseconds.
func chapter(title string, from, to int64) media.Chapter {
	return media.Chapter{Title: title, Start: time.Duration(from) * time.Millisecond, End: time.Duration(to) * time.Millisecond}
}

// lasting is the analysis of a version lasting length milliseconds, with
// chapters.
func lasting(length int64, chapters ...media.Chapter) media.Analysis {
	return media.Analysis{Format: "matroska,webm", Duration: time.Duration(length) * time.Millisecond, Remote: true, Chapters: chapters}
}

// spanMS is a segment from and to times in milliseconds.
func spanMS(kind string, from, to int64) span {
	return span{kind, from * 10_000, to * 10_000}
}

func (p skipSetup) segments(t *testing.T, path string) MediaSegmentDtoQueryResult {
	t.Helper()
	var result MediaSegmentDtoQueryResult
	if status := p.get(t, path, p.token, &result); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	if result.TotalRecordCount != len(result.Items) || result.StartIndex != 0 {
		t.Errorf("%s: %d of %d from %d", path, len(result.Items), result.TotalRecordCount, result.StartIndex)
	}
	return result
}

// span is a segment's type and ticks, for comparison.
type span struct {
	Type       string
	Start, End int64
}

func spans(result MediaSegmentDtoQueryResult) []span {
	var got []span
	for _, segment := range result.Items {
		got = append(got, span{segment.Type, segment.StartTicks, segment.EndTicks})
	}
	return got
}

func TestMediaSegmentsComeFromTheDatabases(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)

	// TheIntroDB's intro and credits win over IntroDB's intro; IntroDB
	// adds its recap. Its post-credits scene has no Jellyfin type. The
	// credits last until the end of the two-hour movie.
	movie := p.segments(t, "/MediaSegments/"+p.movie)
	want := []span{
		{"Intro", 0, 900_000_000},
		{"Recap", 50_000_000, 450_000_000},
		{"Outro", 69_000_000_000, 72_000_000_000},
	}
	if got := spans(movie); !slices.Equal(got, want) {
		t.Errorf("movie: got %v, want %v", got, want)
	}
	for _, segment := range movie.Items {
		if segment.ItemId != p.movie || len(segment.Id) != 32 {
			t.Errorf("segment %+v of %s", segment, p.movie)
		}
	}
	asked := p.databases.requests()
	if len(asked) != 2 {
		t.Fatalf("asked %v", asked)
	}

	// Apps may ask for some types only, by name in any letter case,
	// repeated or comma-separated.
	for path, want := range map[string][]string{
		"?includeSegmentTypes=Outro":                                {"Outro"},
		"?includeSegmentTypes=intro,RECAP":                          {"Intro", "Recap"},
		"?includeSegmentTypes=Intro&includeSegmentTypes=Commercial": {"Intro"},
	} {
		var got []string
		for _, segment := range p.segments(t, "/MediaSegments/"+p.movie+path).Items {
			got = append(got, segment.Type)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}

	// A version is opened as an item: it has its title's segments, under
	// its own identifier, from the answers kept.
	movieID, _ := accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), p.user, movieID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions %+v, %v", versions, err)
	}
	version := p.segments(t, "/MediaSegments/"+versions[1].ID.String())
	if got := spans(version); !slices.Equal(got, want) || version.Items[0].ItemId != versions[1].ID.String() {
		t.Errorf("version: %+v", version)
	}
	if again := p.databases.requests(); len(again) != len(asked) {
		t.Errorf("the databases were asked again: %v", again[len(asked):])
	}

	// An episode is asked for by its series' identifier and its numbers.
	episode := p.segments(t, "/MediaSegments/"+p.episode)
	if got := spans(episode); !slices.Equal(got, []span{{"Intro", 600_000_000, 950_000_000}}) {
		t.Errorf("episode: %v", got)
	}
	asked = p.databases.requests()[2:]
	if !slices.Contains(asked, "/media?duration_ms=2700000&episode=2&imdb_id=tt7654321&season=1") ||
		!slices.Contains(asked, "/segments?episode=2&imdb_id=tt7654321&season=1") {
		t.Errorf("for the episode, asked %v", asked)
	}

	// A series has no segments; an unknown item is not found.
	if series := p.segments(t, "/MediaSegments/"+p.series); len(series.Items) != 0 {
		t.Errorf("series: %+v", series)
	}
	if status := p.get(t, "/MediaSegments/0123456789abcdef0123456789abcdef", p.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
	if status := p.get(t, "/MediaSegments/not-an-id", p.token, nil); status != http.StatusBadRequest {
		t.Errorf("malformed identifier: %d", status)
	}
	if status := p.get(t, "/MediaSegments/"+p.movie, "", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}

func TestMediaSegmentsCanBeTurnedOff(t *testing.T) {
	p := skipping(t)
	if result := p.segments(t, "/MediaSegments/"+p.movie); len(result.Items) != 0 {
		t.Errorf("segments %+v", result)
	}
	if asked := p.databases.requests(); len(asked) != 0 {
		t.Errorf("asked %v", asked)
	}
}

func TestSkipButtonsSettingTurnsSegmentsOff(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)
	if !p.store.Settings().SkipButtons {
		t.Fatal("skip buttons are off by default")
	}
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = false })
	for _, id := range []string{p.movie, p.episode} {
		if result := p.segments(t, "/MediaSegments/"+id); len(result.Items) != 0 {
			t.Errorf("switched off, %s has segments %+v", id, result)
		}
	}
	if asked := p.databases.requests(); len(asked) != 0 {
		t.Errorf("switched off, the databases were asked %v", asked)
	}
	// Statuses stay those of Jellyfin.
	if status := p.get(t, "/MediaSegments/0123456789abcdef0123456789abcdef", p.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
	// Turned back on, the buttons come back at once.
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = true })
	if got := spans(p.segments(t, "/MediaSegments/"+p.movie)); len(got) != 3 {
		t.Errorf("switched on again: %v", got)
	}
	if asked := p.databases.requests(); len(asked) == 0 {
		t.Error("switched on again, no database was asked")
	}
}

// jellyfin-web asks for the segments of the version it plays, by the
// version's identifier, only when the version says it has some.
func TestVersionsSayTheyHaveSegments(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)
	p.listed(t, p.user, p.movie)
	sources := func() []MediaSourceInfo {
		t.Helper()
		var movie BaseItemDto
		if status := p.get(t, "/Items/"+p.movie, p.token, &movie); status != http.StatusOK || movie.MediaSources == nil || len(*movie.MediaSources) != 2 {
			t.Fatalf("movie: %d %+v", status, movie.MediaSources)
		}
		return *movie.MediaSources
	}
	for _, source := range sources() {
		if !source.HasSegments {
			t.Errorf("version %s says it has no segments", source.Id)
		}
	}
	if got := spans(p.segments(t, "/MediaSegments/"+sources()[1].Id)); len(got) != 3 {
		t.Errorf("by a version: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = false })
	for _, source := range sources() {
		if source.HasSegments {
			t.Errorf("switched off, version %s says it has segments", source.Id)
		}
	}
}

// Without a segment database asked, no version says it has segments.
func TestVersionsWithoutSegmentDatabases(t *testing.T) {
	p := skipping(t)
	var movie BaseItemDto
	if status := p.get(t, "/Items/"+p.movie, p.token, &movie); status != http.StatusOK || movie.MediaSources == nil {
		t.Fatalf("movie: %d", status)
	}
	for _, source := range *movie.MediaSources {
		if source.HasSegments {
			t.Errorf("version %s says it has segments", source.Id)
		}
	}
}

// TestMediaSegmentsMatchJellyfin compares the answers with the one
// recorded from Jellyfin 12.2, which had no segments, and the segments
// with the fields of Jellyfin's MediaSegmentDto.
func TestMediaSegmentsMatchJellyfin(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "media-segments.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{p.series, p.movie} {
		status, body := p.call(http.MethodGet, "/MediaSegments/"+id, app("tv", p.token), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", id, status, body)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		for _, difference := range compareShapes("media-segments", want, got, shapeRules{}) {
			t.Error(difference)
		}
		for _, item := range got["Items"].([]any) {
			var keys []string
			for key := range item.(map[string]any) {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			if !slices.Equal(keys, []string{"EndTicks", "Id", "ItemId", "StartTicks", "Type"}) {
				t.Errorf("segment fields %v", keys)
			}
		}
	}
}

// The chapters of two versions of the third episode, as their analyses
// saved them: the second is cut differently from the one the databases
// timed, its intro almost 3 minutes earlier.
var (
	firstCut = lasting(3_152_800,
		chapter("Chapter 1", 0, 321_500), chapter("Intro", 321_500, 388_750),
		chapter("Chapter 2", 388_750, 3_078_200), chapter("End Credits", 3_078_200, 3_152_800))
	secondCut = lasting(3_163_000,
		chapter("Chapter 1", 0, 156_000), chapter("Intro", 156_000, 223_250),
		chapter("Chapter 2", 223_250, 3_091_400), chapter("End Credits", 3_091_400, 3_163_000))
)

// A version's own chapters named as an intro or as credits give its intro
// and credits in place of the databases', type by type and for that version
// only. The third episode is listed as 53 minutes, longer than its
// versions: credits running to their end stop a second before it.
func TestChaptersGiveTheirVersionsSegments(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB)
	versions := p.versions(t, p.third)
	if len(versions) != 4 {
		t.Fatalf("versions %+v", versions)
	}
	p.storeAnalysis(t, versions[0], firstCut)
	p.storeAnalysis(t, versions[1], secondCut)
	// The third version names its credits only, which end 1.5 s before
	// its end.
	p.storeAnalysis(t, versions[2], lasting(3_150_000,
		chapter("Chapter 1", 0, 3_080_000), chapter("Credits", 3_080_000, 3_148_500), chapter("Chapter 3", 3_148_500, 3_150_000)))
	first := []span{spanMS("Intro", 321_500, 388_750), spanMS("Outro", 3_078_200, 3_151_800)}
	databases := spanMS("Intro", 321_400, 400_400)
	for _, test := range []struct {
		name, path string
		want       []span
	}{
		{"the title, which plays its first version", p.third, first},
		{"the first version", versions[0].ID.String(), first},
		{"the second version", versions[1].ID.String(), []span{spanMS("Intro", 156_000, 223_250), spanMS("Outro", 3_091_400, 3_162_000)}},
		{"as jellyfin-web asks", versions[1].ID.String() + "?includeSegmentTypes=Outro&includeSegmentTypes=Intro",
			[]span{spanMS("Intro", 156_000, 223_250), spanMS("Outro", 3_091_400, 3_162_000)}},
		{"credits only", versions[2].ID.String(), []span{databases, spanMS("Outro", 3_080_000, 3_149_000)}},
		{"not analyzed", versions[3].ID.String(), []span{databases, spanMS("Outro", 3_072_000, 3_304_000)}},
	} {
		if got := spans(p.segments(t, "/MediaSegments/"+test.path)); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

// Credits running to the end of a version end a second before it when the
// title is listed as at least as long, so that jellyfin-web offers to skip
// them; otherwise exactly at its end, where jellyfin-web shows its "Up Next"
// card. The third episode is listed as 53 minutes, the fourth as 59, and
// the databases time the fourth's credits from 59:12.3 to 1:00:00.4.
func TestCreditsRunningToTheEnd(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB)
	third, fourth := p.versions(t, p.third), p.versions(t, p.fourth)
	p.storeAnalysis(t, third[0], lasting(3_152_800))
	// Credits a second shorter would last under 3 s, which jellyfin-web
	// ignores: they keep their end.
	p.storeAnalysis(t, third[1], lasting(3_075_500))
	p.storeAnalysis(t, fourth[0], lasting(3_558_100))
	p.storeAnalysis(t, fourth[1], lasting(3_598_100))
	intro := spanMS("Intro", 321_400, 400_400)
	for _, test := range []struct {
		name, path string
		want       []span
	}{
		{"listed longer", third[0].ID.String(), []span{intro, spanMS("Outro", 3_072_000, 3_151_800)}},
		{"listed longer, short credits", third[1].ID.String(), []span{intro, spanMS("Outro", 3_072_000, 3_075_500)}},
		{"listed shorter", fourth[0].ID.String(), []span{spanMS("Outro", 3_552_300, 3_558_100)}},
		{"listed shorter, another version", fourth[1].ID.String(), []span{spanMS("Outro", 3_552_300, 3_598_100)}},
	} {
		if got := spans(p.segments(t, "/MediaSegments/"+test.path)); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
	// jellyfin-web compares the credits' end with the runtime the title is
	// listed with, and with the length of the version it plays.
	var episode BaseItemDto
	p.get(t, "/Items/"+p.third, p.token, &episode)
	if episode.RunTimeTicks == nil || *episode.RunTimeTicks != 3_180_000*10_000 || episode.MediaSources == nil ||
		(*episode.MediaSources)[0].RunTimeTicks == nil || *(*episode.MediaSources)[0].RunTimeTicks != 3_152_800*10_000 {
		t.Errorf("the episode is listed as %v, its first version as %+v", episode.RunTimeTicks, episode.MediaSources)
	}
}

// No segment passes a version's end; credits ending well before it, and
// the segments of a version not analyzed, stay as the databases give them.
func TestSegmentsStopAtTheVersionsEnd(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)
	movie, fourth := p.versions(t, p.movie), p.versions(t, p.fourth)
	p.storeAnalysis(t, movie[0], lasting(80_000))
	p.storeAnalysis(t, movie[1], lasting(7_300_000))
	p.storeAnalysis(t, fourth[0], lasting(3_500_000))
	for _, test := range []struct {
		name, path string
		want       []span
	}{
		{"the intro ends at the end, the credits start after it", movie[0].ID.String(),
			[]span{spanMS("Intro", 0, 80_000), spanMS("Recap", 5_000, 45_000)}},
		{"the credits end 100 s before the end", movie[1].ID.String(),
			[]span{spanMS("Intro", 0, 90_000), spanMS("Recap", 5_000, 45_000), spanMS("Outro", 6_900_000, 7_200_000)}},
		{"the credits start after the end", fourth[0].ID.String(), nil},
		{"not analyzed", fourth[1].ID.String(), []span{spanMS("Outro", 3_552_300, 3_600_400)}},
	} {
		if got := spans(p.segments(t, "/MediaSegments/"+test.path)); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

// A version whose own chapters name an intro or credits says it has
// segments, and gives them, with no segment database asked.
func TestChaptersSayVersionsHaveSegments(t *testing.T) {
	p := skipping(t)
	versions := p.versions(t, p.third)
	p.storeAnalysis(t, versions[0], lasting(3_150_000, chapter("Chapter 1", 0, 1_500_000), chapter("Chapter 2", 1_500_000, 3_150_000)))
	p.storeAnalysis(t, versions[1], secondCut)
	p.listed(t, p.user, p.third)
	has := func() map[string]bool {
		t.Helper()
		var episode BaseItemDto
		if status := p.get(t, "/Items/"+p.third, p.token, &episode); status != http.StatusOK || episode.MediaSources == nil {
			t.Fatalf("episode: %d", status)
		}
		has := map[string]bool{}
		for _, source := range *episode.MediaSources {
			has[source.Id] = source.HasSegments
		}
		return has
	}
	// The first version, named after the title, has generic chapters; the
	// third is not analyzed.
	want := map[string]bool{p.third: false, versions[1].ID.String(): true, versions[2].ID.String(): false, versions[3].ID.String(): false}
	if got := has(); !maps.Equal(got, want) {
		t.Errorf("has segments %v, want %v", got, want)
	}
	second := "/MediaSegments/" + versions[1].ID.String()
	if got, want := spans(p.segments(t, second)), []span{spanMS("Intro", 156_000, 223_250), spanMS("Outro", 3_091_400, 3_162_000)}; !slices.Equal(got, want) {
		t.Errorf("second version: got %v, want %v", got, want)
	}
	for _, id := range []string{p.third, versions[2].ID.String()} {
		if got := spans(p.segments(t, "/MediaSegments/"+id)); len(got) != 0 {
			t.Errorf("%s has segments %v", id, got)
		}
	}
	if asked := p.databases.requests(); len(asked) != 0 {
		t.Errorf("asked %v", asked)
	}
	p.setting(t, func(s *accounts.Settings) { s.SkipButtons = false })
	for id, has := range has() {
		if has {
			t.Errorf("switched off, version %s says it has segments", id)
		}
	}
	if got := spans(p.segments(t, second)); len(got) != 0 {
		t.Errorf("switched off, the second version has segments %v", got)
	}
}

func TestChapterKinds(t *testing.T) {
	for name, want := range map[string]mediasegments.Type{
		"Intro": mediasegments.Intro, "INTRODUCTION": mediasegments.Intro, "Opening": mediasegments.Intro,
		"OP": mediasegments.Intro, "op 1": mediasegments.Intro, "Générique de début": mediasegments.Intro,
		"GÉNÉRIQUE DE DÉBUT": mediasegments.Intro, "Opening Credits": mediasegments.Intro, "Intro (Part 2)": mediasegments.Intro,
		"Credits": mediasegments.Outro, "End Credits": mediasegments.Outro, "closing credits": mediasegments.Outro,
		"Ending": mediasegments.Outro, "ED": mediasegments.Outro, "Outro": mediasegments.Outro,
		"Générique de fin": mediasegments.Outro, "End-Credits": mediasegments.Outro,
		"Chapter 2": "", "": "", "Introspection": "", "Opera": "", "Edge": "", "Credited": "",
		"Generic": "", "Générique": "", "The End": "", "Fin": "", "Openings": "",
		// The short forms count only as the whole name, maybe numbered.
		"OP2": mediasegments.Intro, "Op - 1": mediasegments.Intro, "ED2": mediasegments.Outro,
		"ed 1": mediasegments.Outro, "ED - 2": mediasegments.Outro, "(ED)": mediasegments.Outro,
		"Ed's Story": "", "Ed Wood": "", "OP Center": "", "The ED Report": "", "ED 2 Part": "",
		"OPA": "", "EDx2": "", "Edited": "",
	} {
		if got := chapterKind(name); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}
