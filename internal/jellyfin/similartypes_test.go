package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// newMixedListsAddon serves a series catalog and a movie catalog of one
// genre whose lists hold titles of every type, as list addons give them:
// series, movies, a Live TV channel and a channel of videos.
func newMixedListsAddon(t *testing.T) string {
	t.Helper()
	drama := stremio.Names{"Drama"}
	meta := func(id, kind, name string) stremio.Meta {
		return stremio.Meta{ID: id, Type: kind, Name: name, Genres: drama}
	}
	lists := map[string][]stremio.Meta{
		"series": {meta("tt9100001", "series", "Show A"), meta("tt9100002", "movie", "Movie A"), meta("live:drama", "tv", "Drama TV"),
			meta("yt:drama", "channel", "Drama Clips"), meta("tt9100003", "series", "Show B")},
		"movie": {meta("tt9100002", "movie", "Movie A"), meta("tt9100004", "series", "Show C"), meta("tt9100005", "movie", "Movie B"),
			meta("live:drama", "tv", "Drama TV")},
	}
	genre := []stremio.Extra{{Name: "genre", Options: []string{"Drama"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "mixed", Name: "Mixed lists", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "mixed", Name: "Mixed Shows", Extra: genre},
					{Type: "movie", ID: "mixed", Name: "Mixed Movies", Extra: genre}}})
		case strings.HasPrefix(path, "/catalog/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": lists[strings.Split(path, "/")[2]]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A series' similar titles are series, and a movie's movies, as Jellyfin's,
// even when the catalogs they come from list titles of other types:
// neither a movie, a series, a Live TV channel nor a channel of videos
// slips in.
func TestSimilarTitlesKeepTheTitlesType(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), newMixedListsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	s.user("viewer", nil)
	token := s.signIn("viewer", "web")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range libraryViews(views.Items) {
		var listed QueryResult
		s.get(t, "/Items?ParentId="+view.Id, token, &listed)
		for _, item := range listed.Items {
			ids[item.Name] = item.Id
		}
	}
	if ids["Show A"] == "" || ids["Movie A"] == "" {
		t.Fatalf("the libraries: %v", ids)
	}
	for _, c := range []struct {
		path, kind string
		want       []string
	}{
		{"/Shows/" + ids["Show A"] + "/Similar", "Series", []string{"Show B"}},
		{"/Items/" + ids["Show A"] + "/Similar", "Series", []string{"Show B"}},
		{"/Movies/" + ids["Movie A"] + "/Similar", "Movie", []string{"Movie B"}},
		{"/Items/" + ids["Movie A"] + "/Similar", "Movie", []string{"Movie B"}},
	} {
		var page QueryResult
		if status := s.get(t, c.path+"?limit=12&fields=PrimaryImageAspectRatio,CanDelete", token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", c.path, status)
		}
		if got := itemNames(page.Items); !slices.Equal(got, c.want) || slices.ContainsFunc(page.Items, func(item BaseItemDto) bool { return item.Type != c.kind }) {
			types := make([]string, len(page.Items))
			for i, item := range page.Items {
				types[i] = item.Type
			}
			t.Errorf("%s: %v of types %v, want %v", c.path, got, types, c.want)
		}
	}
}

// An album's similar titles are the albums sharing one of its genres, as
// Jellyfin's; jellyfin-web leaves out the album's own artist's through
// excludeArtistIds.
func TestSimilarAlbumsShareTheAlbumsGenre(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseTools(ffprobe, ffmpeg)
	})
	dir := t.TempDir()
	for _, album := range []struct{ artist, name, genre string }{
		{"Tone Quartet", "Sine Studies", "Electronic"}, {"Tone Quartet", "Pulse Works", "Electronic"},
		{"Other Band", "Night Lines", "Electronic"}, {"Tone Quartet", "Slow Hours", "Jazz"},
	} {
		path := filepath.Join(dir, album.artist, album.name, "01 Track.flac")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=duration=1", "-metadata", "title=Track",
			"-metadata", "artist="+album.artist, "-metadata", "album="+album.name, "-metadata", "genre="+album.genre, path).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Records", Path: dir, Kind: localfiles.KindMusic})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	user := s.user("listener", nil).ID.String()
	token := s.signIn("listener", "web")
	var views, albums QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+libraryViews(views.Items)[0].Id+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	ids := map[string]string{}
	for _, album := range albums.Items {
		ids[album.Name] = album.Id
	}
	var sine BaseItemDto
	s.get(t, "/Users/"+user+"/Items/"+ids["Sine Studies"], token, &sine)
	if sine.AlbumArtists == nil || len(*sine.AlbumArtists) != 1 {
		t.Fatalf("Sine Studies: %+v", sine)
	}
	similar := func(path string) []string {
		t.Helper()
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		if slices.ContainsFunc(page.Items, func(item BaseItemDto) bool { return item.Type != "MusicAlbum" }) {
			t.Errorf("%s: %+v", path, page.Items)
		}
		names := itemNames(page.Items)
		slices.Sort(names)
		return names
	}
	for _, route := range []string{"/Items/", "/Albums/"} {
		if got := similar(route + ids["Sine Studies"] + "/Similar?userId=" + user + "&limit=12"); !slices.Equal(got, []string{"Night Lines", "Pulse Works"}) {
			t.Errorf("%s: similar albums %v", route, got)
		}
	}
	// jellyfin-web's album page.
	path := "/Items/" + ids["Sine Studies"] + "/Similar?userId=" + user + "&limit=12&fields=PrimaryImageAspectRatio,CanDelete&ExcludeArtistIds=" +
		(*sine.AlbumArtists)[0].Id
	if got := similar(path); !slices.Equal(got, []string{"Night Lines"}) {
		t.Errorf("without the album's artist: %v", got)
	}
}
