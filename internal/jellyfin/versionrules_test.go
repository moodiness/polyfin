package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/remuxdb"
	"github.com/moodiness/polyfin/internal/stremio"
)

// rulesAddon serves a movie with six versions, each named by the first
// word of its name, whose labels tell: English, 1080p, H.264, 4 GB;
// Source, nothing; MULTI, 2160p, HEVC, HDR, 60 GB; FRENCH, 720p, H.264,
// 2 GB; Downloading, 1080p, 3 GB; ITA, 1080p, AV1, 5 GB.
func rulesAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3700", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	streams := []stremio.Stream{
		{Name: "English", Description: "1080p x264", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.English.1080p.mkv", VideoSize: 4e9}},
		{Name: "Source", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv"}},
		{Name: "MULTI", Description: "2160p HEVC HDR10", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2160p.mkv", VideoSize: 60e9}},
		{Name: "FRENCH", Description: "720p x264", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.720p.mkv", VideoSize: 2e9}},
		{Name: "Downloading", Description: "1080p", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.1080p.mkv", VideoSize: 3e9}},
		{Name: "ITA", Description: "1080p AV1", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.AV1.mkv", VideoSize: 5e9}},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "rules", Name: "Rules", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3700.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3700.json":
			for i := range streams {
				streams[i].URL = server.URL + "/files/" + streams[i].BehaviorHints.Filename
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// rulesSetup is the rules addon's movie, with its versions in the addon's
// order, as a member sees it.
type rulesSetup struct {
	playbackSetup
	versions []library.Version
}

func versionRulesOn(t *testing.T, s testServer) rulesSetup {
	t.Helper()
	user := s.user("member", func(c *accounts.UserChanges) { c.ContentDownloading = new(true) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), rulesAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := rulesSetup{playbackSetup: playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := s.library.Versions(t.Context(), user, movie)
	if err != nil || len(versions) != 6 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	p.versions = versions
	return p
}

// another signs in one more user, with an audio language preference
// unless empty.
func (p rulesSetup) another(t *testing.T, name, language string) (accounts.User, string) {
	t.Helper()
	user := p.testServer.user(name, nil)
	token := p.signIn(name, "tv")
	if language != "" {
		if status := p.configure(t, "/Users/Configuration", token, map[string]any{"AudioLanguagePreference": language}); status != http.StatusNoContent {
			t.Fatalf("configuration of %s: %d", name, status)
		}
	}
	return user, token
}

// firstWords names media sources by the first word of their names.
func firstWords(sources []MediaSourceInfo) []string {
	words := make([]string, len(sources))
	for i, source := range sources {
		words[i], _, _ = strings.Cut(source.Name, " ")
	}
	return words
}

// detailed is the media sources item details list for a user.
func (p rulesSetup) detailed(t *testing.T, user accounts.User, token string) []MediaSourceInfo {
	t.Helper()
	var dto BaseItemDto
	if status := p.get(t, "/Users/"+user.ID.String()+"/Items/"+p.movie, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
		t.Fatalf("details: %d", status)
	}
	return *dto.MediaSources
}

// Without rules, versions keep the addons' order. The language criterion
// reads each user's own preference: the versions known to carry it first,
// then those that say they have several languages, then those of unknown
// language, then the others; a user without one keeps the addons' order.
// The first source carries the title's identifier.
func TestVersionRulesOrderByTheUsersLanguage(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	addonOrder := []string{"English", "Source", "MULTI", "FRENCH", "Downloading", "ITA"}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, addonOrder) {
		t.Errorf("without rules: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"language"} })
	french, frenchToken := p.another(t, "french", "fre")
	english, englishToken := p.another(t, "english", "eng")
	sources := p.detailed(t, french, frenchToken)
	if got := firstWords(sources); !slices.Equal(got, []string{"FRENCH", "MULTI", "Source", "Downloading", "English", "ITA"}) {
		t.Errorf("preferring French: %v", got)
	}
	if sources[0].Id != p.movie || sources[0].ETag != p.versions[3].ID.String() || sources[1].Id != p.versions[2].ID.String() {
		t.Errorf("identifiers: %s %s, %s", sources[0].Id, sources[0].ETag, sources[1].Id)
	}
	if got := firstWords(p.detailed(t, english, englishToken)); !slices.Equal(got, []string{"English", "MULTI", "Source", "Downloading", "FRENCH", "ITA"}) {
		t.Errorf("preferring English: %v", got)
	}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, addonOrder) {
		t.Errorf("without a preference: %v", got)
	}

	// An analysis tells more than the labels: the English version is in
	// French, and comes first for the user who opens the title now; the
	// page open keeps it in its place.
	p.storeAnalysis(t, p.versions[0], media.Analysis{Format: "matroska,webm", Duration: time.Hour, Remote: true, Streams: []media.Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Language: "fre", Channels: 2}}})
	other, otherToken := p.another(t, "other", "fra")
	if got := firstWords(p.detailed(t, other, otherToken)); !slices.Equal(got, []string{"English", "FRENCH", "MULTI", "Source", "Downloading", "ITA"}) {
		t.Errorf("after the analysis: %v", got)
	}
	if got := firstWords(p.detailed(t, french, frenchToken)); !slices.Equal(got, []string{"FRENCH", "MULTI", "Source", "Downloading", "English", "ITA"}) {
		t.Errorf("the page open after the analysis: %v", got)
	}
}

// Resolution, size, codec and HDR sort the versions, the next criterion
// within the first, those a criterion knows nothing of last.
func TestVersionRulesSortByWhatTheLabelsTell(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	for i, tc := range []struct {
		order []string
		want  []string
	}{
		{[]string{"resolution-high", "size-small"}, []string{"MULTI", "Downloading", "English", "ITA", "FRENCH", "Source"}},
		{[]string{"resolution-low"}, []string{"FRENCH", "English", "Downloading", "ITA", "MULTI", "Source"}},
		{[]string{"size-large"}, []string{"MULTI", "ITA", "English", "Downloading", "FRENCH", "Source"}},
		{[]string{"codec"}, []string{"ITA", "MULTI", "English", "FRENCH", "Source", "Downloading"}},
		{[]string{"hdr-first"}, []string{"MULTI", "English", "FRENCH", "Downloading", "ITA", "Source"}},
		{[]string{"hdr-last", "size-small"}, []string{"FRENCH", "Downloading", "English", "ITA", "MULTI", "Source"}},
	} {
		p.setting(t, func(s *accounts.Settings) { s.VersionOrder = tc.order })
		user, token := p.another(t, "sort"+strconv.Itoa(i), "")
		if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, tc.want) {
			t.Errorf("%v: %v, want %v", tc.order, got, tc.want)
		}
	}
	// The codec order is the administrator's.
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"codec"}
		s.VersionCodecOrder = []string{"h264", "hevc", "av1", "vp9", "vc1", "mpeg4", "mpeg2video"}
	})
	user, token := p.another(t, "h264", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"English", "FRENCH", "MULTI", "ITA", "Source", "Downloading"}) {
		t.Errorf("H.264 first: %v", got)
	}
}

// RemuxDB's description tells more than the labels: the streaming addon's
// first version says nothing of HDR, which RemuxDB finds in its file.
func TestVersionRulesReadRemuxDB(t *testing.T) {
	t.Parallel()
	remux := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/media/tt1000/versions" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, remuxDBAnswer)
	}))
	t.Cleanup(remux.Close)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.RemuxDB = remuxdb.New(testServerID, "test", slog.New(slog.DiscardHandler), o.Accounts.Settings)
	})
	s.setting(t, func(settings *accounts.Settings) {
		settings.RemuxDB, settings.RemuxDBURL = true, remux.URL
		settings.VersionOrder = []string{"hdr-last"}
	})
	p := playingOn(t, s)
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if got := firstWords(*movie.MediaSources); !slices.Equal(got, []string{"Source", "Source"}) || (*movie.MediaSources)[0].ETag != p.versions[1].ID.String() {
		t.Errorf("HDR last: %v %+v", got, *movie.MediaSources)
	}
}

// The hide rules leave versions out of item details, PlaybackInfo and
// downloads; when they would hide them all, they hide none.
func TestVersionRulesHideVersions(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	// Every file name holds the title's name.
	p.setting(t, func(s *accounts.Settings) {
		s.HideVersionsLargerGB = 50
		s.HiddenVersionCodecs = []string{"av1"}
		s.HiddenVersionWords = []string{"movie"}
	})
	if got := firstWords(p.detailed(t, p.user, p.token)); len(got) != 6 {
		t.Errorf("all hidden: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.HiddenVersionWords = []string{"DOWNLOADING", "english"} })
	visible := []string{"Source", "FRENCH"}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, visible) {
		t.Errorf("details: %v", got)
	}
	// The title downloads as its first version shown.
	response, _ := fetchURL(t, p.url+"/Items/"+p.movie+"/Download?ApiKey="+p.token, nil)
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Disposition"), "filename=Movie.mkv") {
		t.Errorf("download of the title: %d %v", response.StatusCode, response.Header)
	}
	if response, _ := fetchURL(t, p.url+"/Items/"+p.versions[4].ID.String()+"/Download?ApiKey="+p.token, nil); response.StatusCode != http.StatusBadRequest {
		t.Errorf("download of a hidden version: %d", response.StatusCode)
	}
	if response, _ := fetchURL(t, p.url+"/Items/"+p.versions[3].ID.String()+"/Download?ApiKey="+p.token, nil); response.StatusCode != http.StatusOK {
		t.Errorf("download of a version shown: %d", response.StatusCode)
	}
	p.analyzed(t, p.versions[3], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[2].ID.String()}); !answer.refused() {
		t.Errorf("a hidden version asked for: %+v", answer)
	}
	// Source cannot be read, ffprobe being missing in tests: FRENCH plays,
	// alone.
	if answer := p.ask(t, p.token, p.movie, chrome, nil); !slices.Equal(firstWords(answer.MediaSources), []string{"FRENCH"}) {
		t.Errorf("PlaybackInfo: %v", firstWords(answer.MediaSources))
	}
}

// releasesAddon serves three movies whose versions are named One, Two and
// Three, in that order, by their file names: Some Movie, a camera copy, a
// telesync and an MPEG-TS file of a web release; Cam, a web release and a
// camera copy; Early Movie, a camera copy and a screener.
func releasesAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movies := []stremio.Meta{
		{ID: "tt9001", Type: "movie", Name: "Some Movie", Runtime: "2h", Year: "2026"},
		{ID: "tt9002", Type: "movie", Name: "Cam", Runtime: "2h", Year: "2018"},
		{ID: "tt9003", Type: "movie", Name: "Early Movie", Runtime: "2h", Year: "2026"},
	}
	files := map[string][]string{
		"tt9001": {"Some.Movie.2026.HDCAM.x264.mkv", "Some.Movie.2026.HDTS.mkv", "Some.Movie.2026.1080p.WEB.ts"},
		"tt9002": {"Cam.2018.1080p.WEB-DL.mkv", "Cam.2018.HDCAM.mkv"},
		"tt9003": {"Early.Movie.2026.CAM.mkv", "Early.Movie.2026.DVDSCR.mkv"},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "releases", Name: "Releases", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
			for _, movie := range movies {
				if movie.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
					return
				}
			}
			http.NotFound(w, r)
		case strings.HasPrefix(path, "/stream/movie/"):
			var streams []stremio.Stream
			for i, file := range files[strings.TrimSuffix(strings.TrimPrefix(path, "/stream/movie/"), ".json")] {
				streams = append(streams, stremio.Stream{Name: []string{"One", "Two", "Three"}[i] + " 1080p", URL: server.URL + "/files/" + file,
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: 2e9}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Hiding low-quality releases leaves out of item details, PlaybackInfo and
// downloads the versions their file names tag as of the kinds ticked, the
// title's own words aside: Cam's web release stays. Nothing ticked hides
// nothing, and when every version would be hidden, none is.
func TestVersionRulesHideLowQualityReleases(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", func(c *accounts.UserChanges) { c.ContentDownloading = new(true) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), releasesAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movies := map[string]string{}
	for _, item := range page.Items {
		movies[item.Name] = item.Id
	}
	if len(movies) != 3 {
		t.Fatalf("titles: %v", movies)
	}
	s.listed(t, user, movies["Some Movie"], movies["Cam"], movies["Early Movie"])
	detailed := func(name string) []string {
		t.Helper()
		var dto BaseItemDto
		if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+movies[name], token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("details of %s: %d", name, status)
		}
		return firstWords(*dto.MediaSources)
	}
	check := func(when string, want map[string][]string) {
		t.Helper()
		for name, versions := range want {
			if got := detailed(name); !slices.Equal(got, versions) {
				t.Errorf("%s, %s: %v, want %v", when, name, got, versions)
			}
		}
	}
	check("nothing ticked", map[string][]string{"Some Movie": {"One", "Two", "Three"}, "Cam": {"One", "Two"}, "Early Movie": {"One", "Two"}})
	s.setting(t, func(s *accounts.Settings) { s.HiddenReleases = []string{"cam"} })
	check("camera copies", map[string][]string{"Some Movie": {"Two", "Three"}, "Cam": {"One"}, "Early Movie": {"Two"}})
	s.setting(t, func(s *accounts.Settings) { s.HiddenReleases = slices.Clone(accounts.ReleaseKinds) })
	check("every kind", map[string][]string{"Some Movie": {"Three"}, "Cam": {"One"}, "Early Movie": {"One", "Two"}})
	id, _ := accounts.ParseID(movies["Some Movie"])
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	// Every version plays: only the rules leave some out.
	p := playbackSetup{testServer: s, token: token, user: user, movie: movies["Some Movie"]}
	for _, version := range versions {
		p.analyzed(t, version, "h264-aac-mp4")
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, token, p.movie, chrome, nil); !slices.Equal(firstWords(answer.MediaSources), []string{"Three"}) {
		t.Errorf("PlaybackInfo: %v", firstWords(answer.MediaSources))
	}
	if answer := p.ask(t, token, p.movie, chrome, map[string]any{"MediaSourceId": versions[0].ID.String()}); !answer.refused() {
		t.Errorf("a camera copy asked for: %+v", answer)
	}
	if response, _ := fetchURL(t, s.url+"/Items/"+versions[1].ID.String()+"/Download?ApiKey="+token, nil); response.StatusCode != http.StatusBadRequest {
		t.Errorf("download of a telesync: %d", response.StatusCode)
	}
}

// The quality group still leaves taller versions out, the rules ordering
// those that fit; when none fits, the closest height comes first, the
// rules ordering versions of the same height.
func TestVersionRulesWithQualityGroups(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"size-large"}
		s.HiddenVersionWords = []string{"source"}
	})
	p.group(t, p.user, 1080)
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"ITA", "English", "Downloading", "FRENCH"}) {
		t.Errorf("1080p: %v", got)
	}
	member, token := p.another(t, "small", "")
	p.group(t, member, 480)
	if got := firstWords(p.detailed(t, member, token)); !slices.Equal(got, []string{"FRENCH", "ITA", "English", "Downloading", "MULTI"}) {
		t.Errorf("480p: %v", got)
	}
}

// Prefer versions the app plays without conversion looks among the first
// versions in the rules' order; without it, the first that plays is
// taken, in that order.
func TestVersionRulesBeforePreferDirectPlay(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	_, token := p.another(t, "french", "fre")
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"language"} })
	// FRENCH plays with its AC3 audio converted for Chrome; MULTI, of no
	// language its analysis tells, as it is.
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-ac3-srt-mkv.json"))
	if err != nil {
		t.Fatal(err)
	}
	converted, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	converted.Remote = true
	for i := range converted.Streams {
		if converted.Streams[i].Type == "audio" {
			converted.Streams[i].Language = "fre"
		}
	}
	p.storeAnalysis(t, p.versions[3], converted)
	p.keyframed(t, p.versions[3])
	p.remuxableVersion(t, p.versions[2], false)
	chrome := p.profile(t, "jellyfin-web-chrome")
	chosen := func(answer playbackAnswer) accounts.ID {
		t.Helper()
		grant, err := p.handler.Playback.Signer().Verify(answer.PlaySessionId)
		if err != nil {
			t.Fatalf("play session %q: %v", answer.PlaySessionId, err)
		}
		return grant.Version
	}
	if answer := p.ask(t, token, p.movie, chrome, nil); chosen(answer) != p.versions[3].ID || answer.MediaSources[0].Id != p.movie {
		t.Errorf("first that plays: %v %+v", chosen(answer), firstWords(answer.MediaSources))
	}
	p.setting(t, func(s *accounts.Settings) { s.PreferDirectPlay = true })
	if answer := p.ask(t, token, p.movie, chrome, nil); chosen(answer) != p.versions[2].ID {
		t.Errorf("preferring no conversion: %v %+v", chosen(answer), firstWords(answer.MediaSources))
	}
}

// A page open keeps the versions it lists in place; versions that come
// take their places by the rules.
func TestKeptInPlace(t *testing.T) {
	t.Parallel()
	version := func(name string) library.Version {
		return library.Version{ID: accounts.ID{name[0]}, Name: name}
	}
	a, b, c, d, e := version("a"), version("b"), version("c"), version("d"), version("e")
	rank := map[string]int{"a": 1, "b": 2, "c": 3, "d": 0, "e": 4}
	compare := func(x, y library.Version) int { return rank[x.Name] - rank[y.Name] }
	// The page lists c, a, b; d ranks before them all, e after.
	got := keptInPlace([]library.Version{d, a, b, c, e}, []accounts.ID{c.ID, a.ID, b.ID}, compare)
	var names []string
	for _, v := range got {
		names = append(names, v.Name)
	}
	if !slices.Equal(names, []string{"d", "c", "a", "b", "e"}) {
		t.Errorf("kept in place: %v", names)
	}
}

// A page opened again once it is no longer watched lists the versions in
// the rules' order: the versions it listed before keep no place, so that
// what analyses told since moves them.
func TestPageOpenedAgainKeepsNoPlaces(t *testing.T) {
	t.Parallel()
	h := &Handler{watched: newWatchedPages()}
	user, title := accounts.User{ID: accounts.ID{1}}, accounts.ID{2}
	versions := []library.Version{{ID: accounts.ID{3}}, {ID: accounts.ID{4}}}
	h.watch(user, title, title)
	h.watched.show(user.ID, title, versions, "language")
	h.watch(user, title, title)
	if shown := h.watched.shown(user.ID, title, "language"); len(shown) != 2 {
		t.Fatalf("a page watched lists %v", shown)
	}
	h.watched.mu.Lock()
	h.watched.titles[title][user.ID].until = time.Now().Add(-time.Second)
	h.watched.mu.Unlock()
	h.watch(user, title, title)
	if shown := h.watched.shown(user.ID, title, "language"); shown != nil {
		t.Errorf("a page opened again keeps %v in place", shown)
	}
}
