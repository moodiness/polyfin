package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// regionsAddon serves a movie with four versions, each named by the first
// word of its name: Subbed, its original English audio with French
// subtitles; VFF, dubbed in French in France; FRENCH, in French of no
// region told; VFQ, dubbed in French in Quebec.
func regionsAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3800", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	streams := []stremio.Stream{
		{Name: "Subbed", Description: "1080p | Audio: English | Subtitles: French", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.1080p.mkv"}},
		{Name: "VFF", Description: "1080p", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.VFF.1080p.mkv"}},
		{Name: "FRENCH", Description: "1080p", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.FRENCH.1080p.mkv"}},
		{Name: "VFQ", Description: "1080p", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.2008.VFQ.1080p.mkv"}},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "regions", Name: "Regions", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3800.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3800.json":
			for i := range streams {
				streams[i].URL = server.URL + "/files/" + streams[i].BehaviorHints.Filename
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// With the language criterion, a user preferring Quebec French gets the
// version dubbed in Quebec first, then the French one of no region told,
// then the one dubbed in France, and the Quebec audio track by default; a
// user preferring French without a region keeps the addon's order among
// the French versions. The subtitled version is in English, not in French.
// A region of another language than the user's own is left aside.
func TestVersionsAndTracksFollowThePreferredRegion(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), regionsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.VersionOrder = []string{"language"} })
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := rulesSetup{playbackSetup: playbackSetup{testServer: s, token: token, user: member, movie: page.Items[0].Id}}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := s.library.Versions(t.Context(), member, movie)
	if err != nil || len(versions) != 4 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	signIn := func(name, region string, configuration map[string]any) (accounts.User, string) {
		t.Helper()
		user := s.user(name, func(c *accounts.UserChanges) { c.AudioRegion = new(region) })
		token := s.signIn(name, "tv")
		if configuration != nil {
			if status := s.configure(t, "/Users/Configuration", token, configuration); status != http.StatusNoContent {
				t.Fatalf("configuration of %s: %d", name, status)
			}
		}
		return user, token
	}
	quebec, quebecToken := signIn("quebec", "fr-CA", nil)
	french, frenchToken := signIn("french", "", map[string]any{"AudioLanguagePreference": "fre"})
	english, englishToken := signIn("english", "fr-CA", map[string]any{"AudioLanguagePreference": "eng"})
	for _, tc := range []struct {
		name  string
		user  accounts.User
		token string
		want  []string
	}{
		{"preferring Quebec French", quebec, quebecToken, []string{"VFQ", "FRENCH", "VFF", "Subbed"}},
		{"preferring French", french, frenchToken, []string{"VFF", "FRENCH", "VFQ", "Subbed"}},
		{"preferring English", english, englishToken, []string{"Subbed", "VFF", "FRENCH", "VFQ"}},
	} {
		if got := firstWords(p.detailed(t, tc.user, tc.token)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}

	// The Quebec version has the French dub of France, flagged default,
	// one tagged as Quebec's, and the English original.
	s.storeAnalysis(t, versions[3], media.Analysis{Format: "matroska,webm", Duration: time.Hour, Remote: true, Streams: []media.Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Language: "fre", Title: "VFF", Default: true, Channels: 2},
		{Index: 2, Type: "audio", Codec: "aac", Language: "fr-CA", Channels: 2},
		{Index: 3, Type: "audio", Codec: "aac", Language: "eng", Channels: 2}}})
	canadian, canadianToken := signIn("canadian", "fr-CA", map[string]any{"AudioLanguagePreference": "fre", "PlayDefaultAudioTrack": false})
	for _, tc := range []struct {
		name  string
		user  accounts.User
		token string
		want  int
	}{
		{"preferring Quebec French alone", quebec, quebecToken, 2},
		{"preferring French and its Quebec region", canadian, canadianToken, 2},
		{"preferring French", french, frenchToken, 1},
	} {
		sources := p.detailed(t, tc.user, tc.token)
		i := slices.IndexFunc(sources, func(source MediaSourceInfo) bool { return strings.HasPrefix(source.Name, "VFQ") })
		if i < 0 {
			t.Fatalf("%s: no Quebec version in %v", tc.name, firstWords(sources))
		}
		if !sameIndex(sources[i].DefaultAudioStreamIndex, new(tc.want)) {
			t.Errorf("%s: default audio %v, want %d", tc.name, deref(sources[i].DefaultAudioStreamIndex), tc.want)
		}
	}
}

// Among the tracks in the preferred language, those of the preferred
// region come first, then those of no known region, then those of
// another; the flagged track still comes first for a user playing it
// whatever its language.
func TestTheDefaultAudioTrackFollowsTheRegion(t *testing.T) {
	t.Parallel()
	audio := func(index int, language, title string, flagged bool) playback.MediaStream {
		return playback.MediaStream{Type: "Audio", Index: index, Language: language, Title: title, IsDefault: flagged}
	}
	video := playback.MediaStream{Type: "Video", Index: 0}
	france, quebec, plain := audio(1, "fra", "VFF", true), audio(2, "fra", "Français (Québec)", false), audio(3, "fra", "", false)
	for name, test := range map[string]struct {
		preferences trackPreferences
		streams     []playback.MediaStream
		want        int
	}{
		"the region":               {trackPreferences{audioLanguage: "fre", audioRegion: "fr-CA"}, []playback.MediaStream{video, france, quebec, plain}, 2},
		"no known region next":     {trackPreferences{audioLanguage: "fre", audioRegion: "fr-CA"}, []playback.MediaStream{video, france, plain}, 3},
		"another region last":      {trackPreferences{audioLanguage: "fre", audioRegion: "fr-FR"}, []playback.MediaStream{video, plain, quebec}, 3},
		"tagged region":            {trackPreferences{audioLanguage: "spa", audioRegion: "es-419"}, []playback.MediaStream{video, audio(1, "spa", "", true), audio(2, "es-MX", "", false)}, 2},
		"no region preferred":      {trackPreferences{audioLanguage: "fre"}, []playback.MediaStream{video, france, quebec}, 1},
		"the flagged track first":  {trackPreferences{audioLanguage: "fre", audioRegion: "fr-CA", playDefaultAudio: true}, []playback.MediaStream{video, france, quebec}, 1},
		"the region among flagged": {trackPreferences{audioLanguage: "fre", audioRegion: "fr-CA", playDefaultAudio: true}, []playback.MediaStream{video, audio(1, "eng", "", false), quebec, plain}, 2},
	} {
		if got := test.preferences.audio(test.streams); !sameIndex(got, new(test.want)) {
			t.Errorf("%s: %v, want %d", name, deref(got), test.want)
		}
	}
}
