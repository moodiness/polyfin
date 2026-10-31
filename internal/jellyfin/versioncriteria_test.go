package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

// criteriaAddon serves a two-hour movie with six versions, each named by
// the first word of its name, whose file names tell: Web, a web copy with
// 5.1 lossy audio, 6 GB; Disc, a disc's full copy with object-based
// lossless 7.1 audio, 60 GB; Encode, a Blu-ray encode with lossless 5.1
// audio, 12 GB; TV, the director's cut captured from TV with stereo
// audio, 2 GB; Extended, the extended cut as a web copy with stereo
// audio, 3 GB; Plain, nothing.
func criteriaAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3757", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	streams := []stremio.Stream{
		{Name: "Web", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.1080p.WEB-DL.DDP5.1.H.264.mkv", VideoSize: 6e9}},
		{Name: "Disc", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.2160p.BluRay.REMUX.HEVC.TrueHD.Atmos.7.1.mkv", VideoSize: 60e9}},
		{Name: "Encode", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.1080p.BluRay.DTS-HD.MA.5.1.x264.mkv", VideoSize: 12e9}},
		{Name: "TV", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.Directors.Cut.720p.HDTV.AAC2.0.x264.mkv", VideoSize: 2e9}},
		{Name: "Extended", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.Extended.1080p.WEB-DL.AAC2.0.H.264.mkv", VideoSize: 3e9}},
		{Name: "Plain", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv"}},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "criteria", Name: "Criteria", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3757.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3757.json":
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

// criteriaOn is the criteria addon's movie, with its versions in the
// addon's order, as a member sees it.
func criteriaOn(t *testing.T, s testServer) rulesSetup {
	t.Helper()
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), criteriaAddon(t), false); err != nil {
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

// The source tier, the audio format, the edition and the size a minute
// sort the versions as their labels tell them, those a criterion knows
// nothing of last; a version naming no edition is the theatrical cut.
func TestVersionCriteriaSortByTheRelease(t *testing.T) {
	t.Parallel()
	p := criteriaOn(t, newTestServer(t, 10))
	for i, tc := range []struct {
		name   string
		change func(*accounts.Settings)
		want   []string
	}{
		{"tier", func(s *accounts.Settings) { s.VersionOrder = []string{"tier"} },
			[]string{"Disc", "Encode", "Web", "Extended", "TV", "Plain"}},
		{"audio", func(s *accounts.Settings) { s.VersionOrder = []string{"audio"} },
			[]string{"Disc", "Encode", "Web", "TV", "Extended", "Plain"}},
		{"edition", func(s *accounts.Settings) { s.VersionOrder = []string{"edition"} },
			[]string{"Web", "Disc", "Encode", "Plain", "Extended", "TV"}},
		{"the administrator's editions", func(s *accounts.Settings) {
			s.VersionOrder = []string{"edition", "size-large"}
			s.VersionEditionOrder = []string{"directors", "extended", "imax", "theatrical"}
		}, []string{"TV", "Extended", "Disc", "Encode", "Web", "Plain"}},
		// 50, 500, 100, 17, 25 and unknown MB a minute.
		{"largest a minute", func(s *accounts.Settings) { s.VersionOrder = []string{"size-per-minute"} },
			[]string{"Disc", "Encode", "Web", "Extended", "TV", "Plain"}},
		{"closest to 50 MB a minute", func(s *accounts.Settings) {
			s.VersionOrder = []string{"size-per-minute"}
			s.VersionMBPerMinute = 50
		}, []string{"Web", "Extended", "TV", "Encode", "Disc", "Plain"}},
		{"tier then audio", func(s *accounts.Settings) { s.VersionOrder = []string{"tier", "audio"} },
			[]string{"Disc", "Encode", "Web", "Extended", "TV", "Plain"}},
	} {
		p.setting(t, func(s *accounts.Settings) {
			s.VersionEditionOrder = slices.Clone(accounts.VersionEditions)
			s.VersionMBPerMinute = 0
			tc.change(s)
		})
		user, token := p.another(t, "criteria"+strconv.Itoa(i), "")
		if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An analysis tells the audio and the runtime better than the labels:
// Plain's lossless object-based 7.1 track puts it first by audio, and the
// three hours Web runs for halve its size a minute.
func TestVersionCriteriaReadTheAnalysis(t *testing.T) {
	t.Parallel()
	p := criteriaOn(t, newTestServer(t, 10))
	p.storeAnalysis(t, p.versions[5], media.Analysis{Format: "matroska,webm", Duration: 2 * time.Hour, Size: 40e9, Remote: true,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160},
			{Index: 1, Type: "audio", Codec: "aac", Channels: 2},
			{Index: 2, Type: "audio", Codec: "truehd", Profile: "Dolby TrueHD + Dolby Atmos", Channels: 8}}})
	p.storeAnalysis(t, p.versions[0], media.Analysis{Format: "matroska,webm", Duration: 3 * time.Hour, Remote: true,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
			{Index: 1, Type: "audio", Codec: "eac3", Channels: 6}}})
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"audio", "size-small"} })
	user, token := p.another(t, "audio", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"Plain", "Disc", "Encode", "Web", "TV", "Extended"}) {
		t.Errorf("by audio: %v", got)
	}
	// Web: 6 GB in three hours, 33 MB a minute, the closest to 30; Plain:
	// the 40 GB its analysis tells in two hours.
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"size-per-minute"}
		s.VersionMBPerMinute = 30
	})
	user, token = p.another(t, "minute", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"Web", "Extended", "TV", "Encode", "Plain", "Disc"}) {
		t.Errorf("closest to 30 MB a minute: %v", got)
	}
}

// Label rules score versions by a word or a regular expression, in any
// letter case; the score orders them where the labels criterion stands,
// else after the criteria. A rule set to hide hides the versions it
// matches, after the other hide rules, and never every version.
func TestVersionLabelRules(t *testing.T) {
	t.Parallel()
	p := criteriaOn(t, newTestServer(t, 10))
	p.setting(t, func(s *accounts.Settings) {
		s.VersionLabelRules = []accounts.VersionLabelRule{{Text: "X264", Score: 10}, {Text: `\bweb-dl\b`, Regex: true, Score: 5},
			{Text: "aac2", Score: -20}, {Text: "hdtv", Hide: true}}
	})
	// Encode 10, Web 5, Disc and Plain 0, Extended -15; TV hidden.
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"Encode", "Web", "Disc", "Plain", "Extended"}) {
		t.Errorf("by score alone: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"tier"} })
	user, token := p.another(t, "tier", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"Disc", "Encode", "Web", "Extended", "Plain"}) {
		t.Errorf("by tier, then score: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"labels", "tier"} })
	user, token = p.another(t, "labels", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"Encode", "Web", "Disc", "Plain", "Extended"}) {
		t.Errorf("by score, then tier: %v", got)
	}
	// A hidden version is refused by its own identifier, though it plays.
	p.analyzed(t, p.versions[3], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[3].ID.String()}); !answer.refused() {
		t.Errorf("a hidden version asked for: %+v", answer)
	}
	// The word rules hide first: with the label rule, nothing would be
	// left, so none is hidden.
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = nil
		s.HiddenVersionWords = []string{"movie.2008"}
		s.VersionLabelRules = []accounts.VersionLabelRule{{Text: `^Plain`, Regex: true, Hide: true}}
	})
	user, token = p.another(t, "all", "")
	if got := firstWords(p.detailed(t, user, token)); len(got) != 6 {
		t.Errorf("all hidden: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) {
		s.HiddenVersionWords = []string{"directors"}
		s.VersionLabelRules = []accounts.VersionLabelRule{{Text: `(?:remux|^plain)`, Regex: true, Hide: true}}
	})
	user, token = p.another(t, "some", "")
	if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, []string{"Web", "Encode", "Extended"}) {
		t.Errorf("hidden by a word and a label rule: %v", got)
	}
}
