package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
)

// notAVideo is what the first stream's host of brokenFirstAddon answers
// in place of its file: binary, as a file is, but no video.
var notAVideo = strings.Repeat("\x00\x01\x00\xfe", 4096)

// brokenFirstAddon serves one movie with two streams on publicSource:
// the first's host answers bytes that are no video, the second is a real
// Matroska file. It returns its manifest's address and the host it
// listens at, which the server reaches publicSource at.
func brokenFirstAddon(t *testing.T) (manifest, host string) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		movie := stremio.Meta{ID: "tt5100", Type: "movie", Name: "Harbour", Year: "2011"}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "busy", Name: "Busy", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt5100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt5100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Busy host", URL: "http://" + publicSource + "/files/busy.mkv"},
				{Name: "Real file", URL: "http://" + publicSource + "/files/real.mkv"},
			}})
		case path == "/files/busy.mkv":
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(notAVideo))
		case path == "/files/real.mkv":
			http.ServeFile(w, r, filepath.Join("..", "container", "testdata", "forced.mkv"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", strings.TrimPrefix(server.URL, "http://")
}

// A stream request with no play session, from the start of a version
// never analyzed, as Infuse and external players send with no
// PlaybackInfo before, analyzes the version before the app is sent to
// it: a request for the title alone whose first version's host answers
// bytes that are no video is sent to the next version, and one naming
// that version is answered 502 rather than sent there. A request further
// into the file is sent there as before.
func TestAStreamFromItsStartIsAnalyzedFirst(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: the versions are analyzed with its ffprobe")
	}
	manifest, host := brokenFirstAddon(t)
	s := newServerReaching(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"),
		func(o source.Opener) source.Opener { return reachedAt{Opener: o, local: host} })
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	// A member is sent to a server addon's source only with Send apps to
	// the source at everyone.
	s.setting(t, func(settings *accounts.Settings) { settings.SendToSource = accounts.SendToSourceEveryone })
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	ask := func(query, ranged string) (int, string) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+"/Videos/"+movie+"/stream?static=true&api_key="+token+query, nil)
		if ranged != "" {
			request.Header.Set("Range", ranged)
		}
		response, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode, response.Header.Get("Location")
	}
	busy, real := "http://"+publicSource+"/files/busy.mkv", "http://"+publicSource+"/files/real.mkv"
	named := "&mediaSourceId=" + versions[0].ID.String()
	// Further into the file, the app is sent there as before: the
	// player's first request decides.
	if status, location := ask(named, "bytes=4-"); status != http.StatusFound || location != busy {
		t.Fatalf("further into the first version: %d to %q, want a 302 to %s", status, location, busy)
	}
	if status, location := ask("", "bytes=0-"); status != http.StatusFound || location != real {
		t.Errorf("the title alone: %d to %q, want a 302 to the second version, %s", status, location, real)
	}
	if status, location := ask(named, ""); status != http.StatusBadGateway {
		t.Errorf("the first version named: %d to %q, want 502", status, location)
	}
}
