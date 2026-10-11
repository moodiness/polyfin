package jellyfin

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A local music folder is a music library of Jellyfin apps: its albums,
// their artists and tracks, by disc and number, with their artwork, and
// its tracks play as they are or converted.
func TestLocalMusicFoldersPlayInJellyfinApps(t *testing.T) {
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
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders.Input))
		folders.UseTools(ffprobe, ffmpeg)
	})
	dir := t.TempDir()
	create := func(rel string, args ...string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		args = append([]string{"-v", "error", "-y"}, append(args, path)...)
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	create("Tone Quartet/Sine Studies/cover.png", "-f", "lavfi", "-i", "color=c=red:s=64x64", "-frames:v", "1")
	create("Tone Quartet/Sine Studies/01 First.flac", "-f", "lavfi", "-i", "sine=duration=3", "-ac", "2", "-metadata", "title=First Light",
		"-metadata", "artist=Tone Quartet", "-metadata", "album=Sine Studies", "-metadata", "track=1", "-metadata", "disc=1")
	create("Tone Quartet/Sine Studies/CD2/01 Second.flac", "-f", "lavfi", "-i", "sine=duration=3", "-ac", "2")
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Records", Path: dir, Kind: localfiles.KindMusic})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	s.user("listener", nil)
	token := s.signIn("listener", "web")

	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 || views.Items[0].Name != "Records" || views.Items[0].CollectionType != "music" {
		t.Fatalf("libraries: %+v", views.Items)
	}
	records := views.Items[0].Id
	var albums QueryResult
	s.get(t, "/Items?ParentId="+records+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	if len(albums.Items) != 1 || albums.Items[0].Name != "Sine Studies" || albums.Items[0].AlbumArtist != "Tone Quartet" {
		t.Fatalf("albums: %+v", albums.Items)
	}
	album := albums.Items[0].Id
	var songs QueryResult
	s.get(t, "/Items?ParentId="+album+"&SortBy=ParentIndexNumber,IndexNumber", token, &songs)
	if len(songs.Items) != 2 || songs.Items[0].Name != "First Light" || songs.Items[1].Name != "Second" || songs.Items[1].ParentIndexNumber == nil ||
		*songs.Items[1].ParentIndexNumber != 2 || songs.Items[1].IndexNumber == nil || *songs.Items[1].IndexNumber != 1 ||
		songs.Items[0].ArtistItems == nil || (*songs.Items[0].ArtistItems)[0].Name != "Tone Quartet" {
		t.Fatalf("songs: %+v", songs.Items)
	}
	var artists QueryResult
	s.get(t, "/Artists/AlbumArtists?ParentId="+records, token, &artists)
	if len(artists.Items) != 1 || artists.Items[0].Name != "Tone Quartet" {
		t.Errorf("artists: %+v", artists.Items)
	}
	var artistAlbums QueryResult
	s.get(t, "/Items?ArtistIds="+artists.Items[0].Id+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &artistAlbums)
	if len(artistAlbums.Items) != 1 || artistAlbums.Items[0].Id != album {
		t.Errorf("the artist's albums: %+v", artistAlbums.Items)
	}
	var found QueryResult
	s.get(t, "/Items?searchTerm=light&IncludeItemTypes=Audio&Recursive=true", token, &found)
	if len(found.Items) != 1 || found.Items[0].Name != "First Light" {
		t.Errorf("search: %+v", found.Items)
	}
	if response, body := s.fetch(t, "/Items/"+album+"/Images/Primary"); response.StatusCode != http.StatusOK || len(body) == 0 {
		t.Errorf("the album's artwork: %d", response.StatusCode)
	}

	song := songs.Items[0].Id
	universal := func(query url.Values) (*http.Response, []byte) {
		query.Set("ApiKey", token)
		return s.fetch(t, "/Audio/"+song+"/universal?"+query.Encode())
	}
	response, body := universal(url.Values{"Container": {"flac,mp3"}, "TranscodingContainer": {"mp3"}, "TranscodingProtocol": {"http"},
		"AudioCodec": {"mp3"}})
	if response.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("fLaC")) {
		t.Fatalf("as it is: %d %s %q", response.StatusCode, response.Header.Get("Content-Type"), body[:min(len(body), 8)])
	}
	response, body = universal(url.Values{"Container": {"opus"}, "TranscodingContainer": {"mp3"}, "TranscodingProtocol": {"http"},
		"AudioCodec": {"mp3"}})
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/mpeg" || len(body) < 1000 {
		t.Fatalf("converted: %d %s %d bytes", response.StatusCode, response.Header.Get("Content-Type"), len(body))
	}
}
