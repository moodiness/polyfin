package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// subtitlesAddon is a subtitles addon named name, offering subtitles for the
// first movie of streamingAddon, given the server's address, and serving
// each file as one cue showing the file's name.
func subtitlesAddon(t *testing.T, name string, subtitles func(base string) []stremio.Subtitle) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: strings.ToLower(name), Name: name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "subtitles"}}})
		case path == "/subtitles/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": subtitles(server.URL)})
		case strings.HasPrefix(path, "/subtitles/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{}})
		case strings.HasPrefix(path, "/files/"):
			_, _ = io.WriteString(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\n"+strings.TrimPrefix(path, "/files/")+"\r\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

type remoteSubtitlesSetup struct {
	playbackSetup
	// addonURLs are the manifest addresses of every addon involved.
	addonURLs []string
	// adminToken signs in an administrator with a subtitles addon of
	// their own.
	adminToken string
}

// remoteSubtitles adds to the playing setup a shared subtitles addon, and
// an administrator with a subtitles addon of their own.
func remoteSubtitles(t *testing.T) remoteSubtitlesSetup {
	t.Helper()
	p := playing(t)
	shared := subtitlesAddon(t, "Subtitles", func(base string) []stremio.Subtitle {
		return []stremio.Subtitle{
			{ID: "en-1", URL: base + "/files/Hello", Lang: "eng"},
			{ID: "fr-2", URL: base + "/files/Salut", Lang: "fr"},
			{ID: "de-1", URL: base + "/files/Hallo", Lang: "ger"},
			{ID: "pb-1", URL: base + "/files/Oi", Lang: "pob"},
			{ID: "en-2", URL: "magnet:?xt=unusable", Lang: "eng"},
		}
	})
	if _, err := p.addons.Install(t.Context(), addons.Shared(), shared, false); err != nil {
		t.Fatal(err)
	}
	admin := p.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	own := subtitlesAddon(t, "Own Subtitles", func(base string) []stremio.Subtitle {
		return []stremio.Subtitle{{ID: "en-9", URL: base + "/files/Howdy", Lang: "en"}}
	})
	if _, err := p.addons.Install(t.Context(), addons.Personal(admin.ID), own, false); err != nil {
		t.Fatal(err)
	}
	return remoteSubtitlesSetup{playbackSetup: p, addonURLs: []string{shared, own}, adminToken: p.signIn("admin", "phone")}
}

func (p remoteSubtitlesSetup) search(t *testing.T, token, item, language string) []RemoteSubtitleInfo {
	t.Helper()
	var results []RemoteSubtitleInfo
	if status := p.get(t, "/Items/"+item+"/RemoteSearch/Subtitles/"+language, token, &results); status != http.StatusOK {
		t.Fatalf("search %s: %d", language, status)
	}
	return results
}

func TestRemoteSubtitleSearchMatchesLanguageCodes(t *testing.T) {
	p := remoteSubtitles(t)
	describe := func(results []RemoteSubtitleInfo) []string {
		described := make([]string, len(results))
		for i, result := range results {
			described[i] = result.ProviderName + " " + result.ThreeLetterISOLanguageName + " " + result.Format
		}
		return described
	}
	// ISO 639-2/B, 639-2/T and 639-1 codes name one language, whatever
	// code each addon gave it; the addons come in order.
	french := []string{"Streams fra srt", "Subtitles fra srt"}
	for language, want := range map[string][]string{
		"fre": french, "fra": french, "fr": french, "FRE": french,
		"eng": {"Subtitles eng srt"}, "en": {"Subtitles eng srt"},
		"ger": {"Subtitles deu srt"}, "deu": {"Subtitles deu srt"}, "de": {"Subtitles deu srt"},
		// A code missing from the language list matches only itself.
		"pob": {"Subtitles pob srt"}, "por": {},
		"spa": {},
	} {
		if got := describe(p.search(t, p.token, p.movie, language)); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", language, got, want)
		}
	}
	// Another user's addon is not the member's; the administrator sees it
	// after the shared ones.
	if got := describe(p.search(t, p.adminToken, p.movie, "eng")); !slices.Equal(got, []string{"Subtitles eng srt", "Own Subtitles eng srt"}) {
		t.Errorf("administrator: %q", got)
	}
	// Addons ask by title: no subtitle is made for the very file.
	if got := p.search(t, p.token, p.movie, "fre?isPerfectMatch=true"); len(got) != 0 {
		t.Errorf("perfect matches: %+v", got)
	}
	if got := p.search(t, p.token, p.remote, "fre"); len(got) != 0 {
		t.Errorf("title without subtitles: %+v", got)
	}
	// A version opened as an item searches its title.
	if got := describe(p.search(t, p.token, p.versions[1].ID.String(), "fr")); !slices.Equal(got, french) {
		t.Errorf("by version: %q", got)
	}
}

func TestRemoteSubtitleIDsRevealNoAddress(t *testing.T) {
	p := remoteSubtitles(t)
	status, body := p.call(http.MethodGet, "/Items/"+p.movie+"/RemoteSearch/Subtitles/fre", app("tv", p.token), nil)
	if status != http.StatusOK {
		t.Fatalf("search: %d", status)
	}
	var results []RemoteSubtitleInfo
	_ = json.Unmarshal(body, &results)
	if len(results) != 2 || results[0].Id == results[1].Id || results[0].Id == "" {
		t.Fatalf("results: %+v", results)
	}
	// Neither the addons' addresses nor the files' or the addons' own
	// subtitle identifiers show.
	for _, url := range p.addonURLs {
		host := strings.TrimSuffix(strings.TrimPrefix(url, "http://"), "/manifest.json")
		if strings.Contains(string(body), host) {
			t.Errorf("addon address %s in %s", host, body)
		}
	}
	for _, leak := range []string{"http", "/files/", "Salut", "fr-1", "fr-2"} {
		if strings.Contains(string(body), leak) {
			t.Errorf("%q in %s", leak, body)
		}
	}
}

func TestDownloadingARemoteSubtitleKeepsItAmongTheTitles(t *testing.T) {
	p := remoteSubtitles(t)
	english := p.search(t, p.token, p.movie, "en")[0]
	if status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/RemoteSearch/Subtitles/"+english.Id, app("tv", p.token), nil); status != http.StatusNoContent {
		t.Fatalf("download: %d %s", status, body)
	}
	// The subtitle is one of each version's streams, shown under the name
	// the search gave, and serves the addon's file.
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, source := range *movie.MediaSources {
		i := slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Language == "eng" && s.IsExternal })
		if i < 0 || source.MediaStreams[i].DisplayTitle != english.Name {
			t.Fatalf("source %s streams: %+v", source.Id, source.MediaStreams)
		}
		status, body := p.call(http.MethodGet, fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/Stream.srt", p.movie, source.Id, source.MediaStreams[i].Index), "", nil)
		if status != http.StatusOK || !strings.Contains(string(body), "Hello") {
			t.Errorf("source %s subtitle: %d %s", source.Id, status, body)
		}
	}
	// The search's file itself is served too.
	status, body := p.call(http.MethodGet, "/Providers/Subtitles/Subtitles/"+english.Id, app("tv", p.token), nil)
	if status != http.StatusOK || !strings.HasPrefix(string(body), "1\n00:00:01,000 --> 00:00:04,000\nHello") {
		t.Errorf("file: %d %s", status, body)
	}

	// Only a subtitle the user's addons offer for this title downloads.
	own := p.search(t, p.adminToken, p.movie, "en")[1]
	for name, path := range map[string]string{
		"another title":        "/Items/" + p.remote + "/RemoteSearch/Subtitles/" + english.Id,
		"another user's addon": "/Items/" + p.movie + "/RemoteSearch/Subtitles/" + own.Id,
		"an unknown subtitle":  "/Items/" + p.movie + "/RemoteSearch/Subtitles/" + p.movie + strings.Repeat("0", 32),
		"a malformed id":       "/Items/" + p.movie + "/RemoteSearch/Subtitles/fr-1",
	} {
		if status, _ := p.call(http.MethodPost, path, app("tv", p.token), nil); status != http.StatusNotFound {
			t.Errorf("%s: %d", name, status)
		}
	}
	if status, _ := p.call(http.MethodGet, "/Providers/Subtitles/Subtitles/"+own.Id, app("tv", p.token), nil); status != http.StatusNotFound {
		t.Errorf("another user's file: %d", status)
	}
	if status, _ := p.call(http.MethodGet, "/Providers/Subtitles/Subtitles/"+own.Id, app("phone", p.adminToken), nil); status != http.StatusOK {
		t.Errorf("own file: %d", status)
	}
}

func TestRemoteSubtitlesOfUnknownItems(t *testing.T) {
	p := remoteSubtitles(t)
	english := p.search(t, p.token, p.movie, "eng")[0]
	unknown := strings.Repeat("ab", 16)
	for name, c := range map[string]struct {
		method, path string
		token        string
		status       int
	}{
		"search an unknown item":   {http.MethodGet, "/Items/" + unknown + "/RemoteSearch/Subtitles/eng", p.token, http.StatusNotFound},
		"download for an unknown":  {http.MethodPost, "/Items/" + unknown + "/RemoteSearch/Subtitles/" + english.Id, p.token, http.StatusNotFound},
		"search a malformed item":  {http.MethodGet, "/Items/movie/RemoteSearch/Subtitles/eng", p.token, http.StatusBadRequest},
		"search a malformed flag":  {http.MethodGet, "/Items/" + p.movie + "/RemoteSearch/Subtitles/eng?isPerfectMatch=maybe", p.token, http.StatusBadRequest},
		"search signed out":        {http.MethodGet, "/Items/" + p.movie + "/RemoteSearch/Subtitles/eng", "", http.StatusUnauthorized},
		"download signed out":      {http.MethodPost, "/Items/" + p.movie + "/RemoteSearch/Subtitles/" + english.Id, "", http.StatusUnauthorized},
		"file signed out":          {http.MethodGet, "/Providers/Subtitles/Subtitles/" + english.Id, "", http.StatusUnauthorized},
		"file of an unknown title": {http.MethodGet, "/Providers/Subtitles/Subtitles/" + unknown + strings.TrimPrefix(english.Id, p.movie), p.token, http.StatusNotFound},
	} {
		status, body := p.call(c.method, c.path, app("tv", c.token), nil)
		if status != c.status {
			t.Errorf("%s: %d %s, want %d", name, status, body, c.status)
		}
		var problem problemDetails
		if c.status == http.StatusNotFound && (json.Unmarshal(body, &problem) != nil || problem.Status != http.StatusNotFound) {
			t.Errorf("%s: %s", name, body)
		}
	}
}

// remoteSubtitleInfoTypes are the properties of Jellyfin 12.2's
// RemoteSubtitleInfo schema, with the JSON type of each: the recorded
// search found nothing to show them.
var remoteSubtitleInfoTypes = map[string]string{
	"ThreeLetterISOLanguageName": "string", "Id": "string", "ProviderName": "string", "Name": "string",
	"Format": "string", "Author": "string", "Comment": "string", "DateCreated": "string",
	"CommunityRating": "float64", "FrameRate": "float64", "DownloadCount": "float64",
	"IsHashMatch": "bool", "AiTranslated": "bool", "MachineTranslated": "bool", "Forced": "bool", "HearingImpaired": "bool",
}

func TestRemoteSubtitleSearchMatchesJellyfin(t *testing.T) {
	p := remoteSubtitles(t)
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "remote-subtitles.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{p.remote, p.movie} {
		_, body := p.call(http.MethodGet, "/Items/"+item+"/RemoteSearch/Subtitles/fre", app("tv", p.token), nil)
		var got any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%v in %s", err, body)
		}
		for _, difference := range compareShapes("remote-subtitles", want, got, shapeRules{}) {
			t.Error(difference)
		}
		results, _ := got.([]any)
		for _, result := range results {
			for key, value := range result.(map[string]any) {
				if kind, known := remoteSubtitleInfoTypes[key]; !known || fmt.Sprintf("%T", value) != kind {
					t.Errorf("%s: %T, want %q", key, value, kind)
				}
			}
		}
		if item == p.movie && len(results) != 2 {
			t.Errorf("movie: %s", body)
		}
	}
}
