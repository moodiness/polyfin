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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// jellyfin-web opens the genre a title's page links to by asking for it as
// an item, before it lists the genre's titles.
func TestGenreLinksOpenTheirPage(t *testing.T) {
	t.Parallel()
	s, token, user, views := pagesServer(t)
	var movies QueryResult
	s.get(t, "/Items?parentId="+views["Popular"], token, &movies)
	var movie BaseItemDto
	s.get(t, "/Users/"+user+"/Items/"+movies.Items[0].Id, token, &movie)
	if movie.GenreItems == nil || len(*movie.GenreItems) != 1 || (*movie.GenreItems)[0].Name != "Action" {
		t.Fatalf("genres of %s: %+v", movie.Name, movie.GenreItems)
	}
	linked := (*movie.GenreItems)[0].Id
	for _, path := range []string{"/Users/" + user + "/Items/" + linked, "/Items/" + linked + "?userId=" + user} {
		genre := namedPageOf(t, s, token, path)
		if genre.Type != "Genre" || genre.Name != "Action" || genre.Id != linked || genre.MovieCount != 2 || genre.SeriesCount != 1 ||
			genre.UserData == nil || genre.UserData.ItemId != linked {
			t.Errorf("%s: %+v", path, genre)
		}
	}
	if names, _ := listed(t, s, token, titlesOf+"genreIds="+linked); !slices.Equal(names, []string{"Movie 1", "Movie 3", "Show 1"}) {
		t.Errorf("titles of the genre linked: %v", names)
	}

	// A genre no page gave yet, as after a restart, is found among the
	// catalogs' genre options.
	drama := nameID("genre", "Drama")
	if genre := namedPageOf(t, s, token, "/Users/"+user+"/Items/"+drama); genre.Type != "Genre" || genre.Name != "Drama" || genre.MovieCount != 1 {
		t.Errorf("a genre of the catalogs: %+v", genre)
	}
	// A name no title or catalog gave is no item.
	if status, body := s.call(http.MethodGet, "/Users/"+user+"/Items/"+nameID("genre", "Western"), app("web", token), nil); status != http.StatusNotFound {
		t.Errorf("an unknown genre: %d %s", status, body)
	}
}

// shelfAddon serves a movie and a series catalog without genre filter, of
// titles whose genres differ in letter case, most spelled "Mystery".
func shelfAddon(t *testing.T) string {
	t.Helper()
	title := func(id, metaType, name string, genres ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: metaType, Name: name, Genres: genres, ReleaseInfo: "2020", Released: "2020-04-10T00:00:00.000Z",
			Extras: &stremio.Extras{Certification: "PG"}}
	}
	listed := map[string][]stremio.Meta{
		"shelf": {title("tt3001", "movie", "Ash Road", "Mystery"), title("tt3002", "movie", "Cold Night", "mystery", "Crime"),
			title("tt3003", "movie", "Dawn Light", "Comedy")},
		"shows": {title("tt4001", "series", "Harbor Tales", "Mystery")},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shelf", Name: "Shelf", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "shelf", Name: "Shelf"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed[strings.Split(strings.TrimPrefix(path, "/catalog/"), "/")[1]]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// forget forgets the names given identifiers and those read of the
// genres and studios kept, as a restart does.
func (n *namedItems) forget() {
	n.mu.Lock()
	n.byID, n.ring, n.next = nil, nil, 0
	n.mu.Unlock()
	for _, kept := range []*keptNames{&n.genres, &n.studios} {
		kept.mu.Lock()
		kept.byID, kept.read = nil, time.Time{}
		kept.mu.Unlock()
	}
}

// A genre no catalog filters by lists the titles kept of it, whatever the
// letter case of their genres, and opens by the identifier a title links
// to, also after a restart.
func TestKeptGenresOpenTheirPageAfterARestart(t *testing.T) {
	t.Parallel()
	s, token, user, views := pagesServerWith(t, shelfAddon(t), "")
	var shows QueryResult
	s.get(t, "/Items?parentId="+views["Shows"], token, &shows)
	var movies QueryResult
	s.get(t, "/Items?parentId="+views["Shelf"], token, &movies)
	var movie BaseItemDto
	s.get(t, "/Users/"+user+"/Items/"+movies.Items[0].Id, token, &movie)
	if movie.Name != "Ash Road" || movie.GenreItems == nil || len(*movie.GenreItems) != 1 || (*movie.GenreItems)[0].Name != "Mystery" {
		t.Fatalf("genres of %s: %+v", movie.Name, movie.GenreItems)
	}
	linked := (*movie.GenreItems)[0].Id
	opens := func(when string) {
		t.Helper()
		genre := namedPageOf(t, s, token, "/Users/"+user+"/Items/"+linked)
		if genre.Type != "Genre" || genre.Name != "Mystery" || genre.Id != linked || genre.MovieCount != 2 || genre.SeriesCount != 1 {
			t.Errorf("%s, the genre's page: %+v", when, genre)
		}
		for _, filter := range []string{"genreIds=" + linked, "genres=MYSTERY"} {
			if names, total := listed(t, s, token, titlesOf+filter); !slices.Equal(names, []string{"Ash Road", "Cold Night", "Harbor Tales"}) || total != 3 {
				t.Errorf("%s, the titles of %s: %v of %d", when, filter, names, total)
			}
		}
	}
	opens("linked")
	s.handler.names.forget()
	opens("after a restart")
	// A genre no title has is still no item.
	if status, body := s.call(http.MethodGet, "/Users/"+user+"/Items/"+nameID("genre", "Western"), app("web", token), nil); status != http.StatusNotFound {
		t.Errorf("an unknown genre: %d %s", status, body)
	}
}

// The genre an album's page links to is a genre of music, whose page lists
// the albums of that genre. Addons give no genres of albums: a local music
// folder's come from its tracks' tags.
func TestMusicGenreLinksOpenTheirPage(t *testing.T) {
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
	for _, album := range []struct{ name, genre string }{{"Sine Studies", "Electronic"}, {"Slow Hours", "Jazz"}} {
		path := filepath.Join(dir, "Tone Quartet", album.name, "01 Track.flac")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=duration=1", "-metadata", "title=Track",
			"-metadata", "artist=Tone Quartet", "-metadata", "album="+album.name, "-metadata", "genre="+album.genre, path).CombinedOutput(); err != nil {
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
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var albums QueryResult
	s.get(t, "/Items?ParentId="+libraryViews(views.Items)[0].Id+"&IncludeItemTypes=MusicAlbum&Recursive=true&SortBy=SortName", token, &albums)
	if len(albums.Items) != 2 || albums.Items[0].Name != "Sine Studies" {
		t.Fatalf("albums: %+v", albums.Items)
	}
	var album BaseItemDto
	s.get(t, "/Users/"+user+"/Items/"+albums.Items[0].Id, token, &album)
	if album.GenreItems == nil || len(*album.GenreItems) != 1 || (*album.GenreItems)[0].Name != "Electronic" {
		t.Fatalf("genres of %s: %+v", album.Name, album.GenreItems)
	}
	linked := (*album.GenreItems)[0].Id
	if linked == nameID("genre", "Electronic") {
		t.Errorf("a genre of music has the identifier of the genre of titles")
	}
	genre := namedPageOf(t, s, token, "/Users/"+user+"/Items/"+linked)
	if genre.Type != "MusicGenre" || genre.Name != "Electronic" || genre.Id != linked {
		t.Errorf("genre of music: %+v", genre)
	}
	// jellyfin-web lists the albums of the genre.
	listing := "/Users/" + user + "/Items?Recursive=true&SortBy=SortName&IncludeItemTypes=MusicAlbum&GenreIds="
	if names, total := listed(t, s, token, listing+linked); !slices.Equal(names, []string{"Sine Studies"}) || total != 1 {
		t.Errorf("albums of the genre: %v (%d)", names, total)
	}
	if names, _ := listed(t, s, token, listing+nameID("musicgenre", "Ambient")); len(names) != 0 {
		t.Errorf("albums of another genre: %v", names)
	}
}
