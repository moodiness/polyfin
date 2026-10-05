package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/stremio"
)

// skipAddon serves a movie with two streams and a series of two episodes,
// with the identifiers segment databases know titles by.
func skipAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt1234567", Type: "movie", Name: "Movie", Runtime: "2h", TmdbID: "550"}
	show := stremio.Meta{ID: "tt7654321", Type: "series", Name: "Show", Runtime: "45min",
		Videos: []stremio.Video{
			{ID: "tt7654321:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2010-06-16T00:00:00.000Z", Runtime: "45min"},
			{ID: "tt7654321:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2010-06-23T00:00:00.000Z", Runtime: "45min"},
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
	token, movie, series, episode string
	user                          accounts.User
	databases                     *segmentDatabases
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
		if item.Name == "Second" {
			p.episode = item.Id
		}
	}
	if p.movie == "" || p.episode == "" {
		t.Fatalf("movie %q, episode %q", p.movie, p.episode)
	}
	return p
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
// recorded from Jellyfin 12.1, which had no segments, and the segments
// with the fields of Jellyfin's MediaSegmentDto.
func TestMediaSegmentsMatchJellyfin(t *testing.T) {
	p := skipping(t, mediasegments.TheIntroDB, mediasegments.IntroDB)
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "media-segments.json"))
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
