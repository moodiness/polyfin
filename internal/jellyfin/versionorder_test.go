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
	"github.com/moodiness/polyfin/internal/stremio"
)

// threeVersionsAddon serves a movie with three streams, named Version 1 to
// Version 3, and their bytes.
func threeVersionsAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3000", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "versions", Name: "Versions", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3000.json":
			var streams []stremio.Stream
			for _, n := range []string{"1", "2", "3"} {
				streams = append(streams, stremio.Stream{Name: "Version " + n, URL: server.URL + "/files/" + n + ".mp4",
					BehaviorHints: stremio.StreamBehavior{Filename: "Movie." + n + ".mp4"}})
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

// Picking a version, playing it, a version failing and asking PlaybackInfo
// again never move a version: item details and PlaybackInfo keep the
// addon's order. jellyfin-web's version menu opens the version picked as an
// item, which Jellyfin 12.1 would answer with that version first.
func TestVersionsKeepTheirOrder(t *testing.T) {
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), threeVersionsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := s.library.Versions(t.Context(), user, movie)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	// The first version cannot be analyzed, ffprobe being missing in
	// tests; the others were analyzed before.
	p.analyzed(t, versions[1], "h264-aac-mp4")
	p.analyzed(t, versions[2], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	third := versions[2].ID.String()

	names := func(sources []MediaSourceInfo) []string {
		result := make([]string, len(sources))
		for i, source := range sources {
			result[i] = source.Name
		}
		return result
	}
	details := func(what, item string, want ...string) []MediaSourceInfo {
		t.Helper()
		var dto BaseItemDto
		if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+item, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("%s: %d", what, status)
		}
		if got := names(*dto.MediaSources); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", what, got, want)
		}
		return *dto.MediaSources
	}
	playbackInfo := func(what, item string, more map[string]any, want ...string) playbackAnswer {
		t.Helper()
		answer := s.ask(t, token, item, chrome, more)
		if got := names(answer.MediaSources); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", what, got, want)
		}
		return answer
	}
	// chosen is the version an answer's play session plays.
	chosen := func(answer playbackAnswer) accounts.ID {
		t.Helper()
		grant, err := s.handler.Playback.Signer().Verify(answer.PlaySessionId)
		if err != nil {
			t.Fatalf("play session %q: %v", answer.PlaySessionId, err)
		}
		return grant.Version
	}
	all := []string{"Version 1", "Version 2", "Version 3"}

	sources := details("details", p.movie, all...)
	if sources[0].Id != p.movie || sources[2].Id != third {
		t.Errorf("identifiers: %s %s", sources[0].Id, sources[2].Id)
	}
	// jellyfin-web opens the version picked as an item: its source carries
	// its identifier, in its place.
	if sources := details("version 3 picked", third, all...); sources[2].Id != third || sources[0].Id != versions[0].ID.String() {
		t.Errorf("identifiers of the version picked: %s %s", sources[0].Id, sources[2].Id)
	}
	// It plays the version picked, by its identifier, briefly.
	if answer := playbackInfo("version 3 played", third, map[string]any{"MediaSourceId": third}, "Version 3"); answer.MediaSources[0].Id != third {
		t.Errorf("version played: %+v", answer.MediaSources[0])
	}
	for _, report := range []string{"/Sessions/Playing", "/Sessions/Playing/Stopped"} {
		body := map[string]any{"ItemId": third, "MediaSourceId": third, "PositionTicks": 600_000_000, "PlayMethod": "DirectPlay"}
		if status, data := s.call(http.MethodPost, report, app("tv", token), body); status != http.StatusNoContent {
			t.Fatalf("%s: %d %s", report, status, data)
		}
	}
	details("details after the play", p.movie, all...)
	details("version 3 opened again", third, all...)

	// An app that picks no version plays the first that can: the first
	// cannot be read, and is left out of the answer and, having failed,
	// of what follows. The others keep their order.
	if answer := playbackInfo("no version picked", p.movie, nil, "Version 2", "Version 3"); chosen(answer) != versions[1].ID {
		t.Errorf("version chosen: %+v", answer.MediaSources[0])
	}
	sources = details("details after the failure", p.movie, "Version 2", "Version 3")
	// The title's identifier names its first version still listed.
	if sources[0].Id != p.movie {
		t.Errorf("first source after the failure: %s", sources[0].Id)
	}
	if answer := playbackInfo("no version picked again", p.movie, nil, "Version 2", "Version 3"); chosen(answer) != versions[1].ID {
		t.Errorf("version chosen again: %+v", answer.MediaSources[0])
	}
	// The version opened as an item is the one tried first, decided in its
	// place.
	answer := playbackInfo("no version picked, version 3 opened", third, nil, "Version 2", "Version 3")
	if chosen(answer) != versions[2].ID || answer.MediaSources[1].Id != third {
		t.Errorf("version 3 opened: %+v", answer.MediaSources[1])
	}
	details("version 3 opened after the failure", third, "Version 2", "Version 3")
}
