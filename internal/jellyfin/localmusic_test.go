package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/lyrics"
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
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
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

// A local music folder's playlists are listed with the user's, their
// tracks in order; a song lists under each of its artists, the picture of
// an artist's folder is theirs; a song's lyrics are those of its lyrics
// file, else of its lyrics tag, before LRCLIB's, which is never asked
// about it, and served with LRCLIB's lyrics turned off too.
func TestLocalMusicPlaylistsArtistsAndLyrics(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	var mu sync.Mutex
	asked := map[string]int{}
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("track_name")
		mu.Lock()
		asked[name]++
		mu.Unlock()
		if name != "Online" {
			http.Error(w, `{"code":404,"name":"TrackNotFound"}`, http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"instrumental": false, "plainLyrics": "From LRCLIB", "syncedLyrics": "[00:01.00]From LRCLIB"}`)
	}))
	t.Cleanup(lrclib.Close)
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.DiscardHandler), o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		folders.UseTools(ffprobe, ffmpeg)
		o.Lyrics = lyrics.New(pool, lrclib.URL, "test", slog.New(slog.DiscardHandler), o.Accounts.Settings)
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
	text := func(rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sine := []string{"-f", "lavfi", "-i", "sine=duration=3", "-metadata", "album=Sines", "-metadata", "album_artist=Tone Quartet"}
	create("Tone Quartet/Sines/01 Filed.flac", append(sine, "-metadata", "title=Filed", "-metadata", "artist=Tone Quartet feat. Square",
		"-metadata", "ARTISTS=Tone Quartet;Square", "-metadata", "track=1", "-metadata", "lyrics=[00:01.00]From the tag")...)
	text("Tone Quartet/Sines/01 Filed.lrc", "[ar:Tone Quartet]\n[00:01.00]From the file\n[00:02.00]\n")
	create("Tone Quartet/Sines/02 Tagged.flac", append(sine, "-metadata", "title=Tagged", "-metadata", "artist=Tone Quartet", "-metadata", "track=2",
		"-metadata", "lyrics=Plain words")...)
	create("Tone Quartet/Sines/03 Online.flac", append(sine, "-metadata", "title=Online", "-metadata", "artist=Tone Quartet", "-metadata", "track=3")...)
	create("Tone Quartet/artist.png", "-f", "lavfi", "-i", "color=c=blue:s=64x64", "-frames:v", "1")
	text("Sines.m3u8", "#EXTM3U\n#PLAYLIST:Sine Mix\nTone Quartet/Sines/03 Online.flac\nnowhere.flac\nTone Quartet/Sines/01 Filed.flac\n")
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Records", Path: dir, Kind: localfiles.KindMusic})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	s.user("listener", nil)
	token := s.signIn("listener", "web")

	// The playlist, listed as Jellyfin apps list playlists.
	var lists QueryResult
	s.get(t, "/Items?IncludeItemTypes=Playlist&Recursive=true&SortBy=SortName", token, &lists)
	if len(lists.Items) != 1 || lists.Items[0].Name != "Sine Mix" || lists.Items[0].Type != "Playlist" || lists.TotalRecordCount != 1 {
		t.Fatalf("playlists: %+v", lists.Items)
	}
	var entries QueryResult
	s.get(t, "/Playlists/"+lists.Items[0].Id+"/Items", token, &entries)
	if names := itemNames(entries.Items); !slices.Equal(names, []string{"Online", "Filed"}) {
		t.Errorf("the playlist's songs: %v", names)
	}

	// A song lists under each of its artists.
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var albums QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	if len(albums.Items) != 1 {
		t.Fatalf("albums: %+v", albums.Items)
	}
	var songs QueryResult
	s.get(t, "/Items?ParentId="+albums.Items[0].Id+"&SortBy=IndexNumber", token, &songs)
	if names := itemNames(songs.Items); !slices.Equal(names, []string{"Filed", "Tagged", "Online"}) {
		t.Fatalf("songs: %v", names)
	}
	filed := songs.Items[0]
	if filed.ArtistItems == nil || len(*filed.ArtistItems) != 2 || (*filed.ArtistItems)[1].Name != "Square" {
		t.Fatalf("Filed's artists: %+v", filed.ArtistItems)
	}
	has := map[string]bool{}
	for _, song := range songs.Items {
		has[song.Name] = song.HasLyrics != nil && *song.HasLyrics
	}
	if !has["Filed"] || !has["Tagged"] || has["Online"] {
		t.Errorf("songs with lyrics: %v", has)
	}
	square := (*filed.ArtistItems)[1].Id
	var squareSongs QueryResult
	s.get(t, "/Items?ArtistIds="+square+"&IncludeItemTypes=Audio&Recursive=true", token, &squareSongs)
	if names := itemNames(squareSongs.Items); !slices.Equal(names, []string{"Filed"}) {
		t.Errorf("Square's songs: %v", names)
	}
	var quartet BaseItemDto
	s.get(t, "/Items/"+(*filed.ArtistItems)[0].Id, token, &quartet)
	if quartet.Name != "Tone Quartet" || quartet.ImageTags["Primary"] == "" {
		t.Errorf("Tone Quartet: %s %v", quartet.Name, quartet.ImageTags)
	}
	if response, body := s.fetch(t, "/Items/"+quartet.Id+"/Images/Primary"); response.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("\x89PNG")) {
		t.Errorf("Tone Quartet's picture: %d", response.StatusCode)
	}

	// The lyrics file, then the tag, then LRCLIB.
	lines := func(song BaseItemDto) (int, []LyricLine) {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Audio/"+song.Id+"/Lyrics", app("web", token), nil)
		var dto LyricDto
		if status == http.StatusOK {
			if err := json.Unmarshal(body, &dto); err != nil {
				t.Fatal(err)
			}
		}
		return status, dto.Lyrics
	}
	if status, got := lines(filed); status != http.StatusOK || len(got) != 2 || got[0].Text != "From the file" || got[0].Start == nil ||
		*got[0].Start != 10_000_000 {
		t.Errorf("Filed: %d %+v", status, got)
	}
	if status, got := lines(songs.Items[1]); status != http.StatusOK || len(got) != 1 || got[0].Text != "Plain words" || got[0].Start != nil {
		t.Errorf("Tagged: %d %+v", status, got)
	}
	if status, got := lines(songs.Items[2]); status != http.StatusOK || len(got) != 1 || got[0].Text != "From LRCLIB" {
		t.Errorf("Online: %d %+v", status, got)
	}
	mu.Lock()
	if asked["Filed"]+asked["Tagged"] != 0 || asked["Online"] != 1 {
		t.Errorf("LRCLIB was asked: %v", asked)
	}
	mu.Unlock()
	s.setting(t, func(settings *accounts.Settings) { settings.Lyrics = false })
	if status, _ := lines(filed); status != http.StatusOK {
		t.Errorf("Filed with LRCLIB off: %d", status)
	}
	if status, _ := lines(songs.Items[2]); status != http.StatusNotFound {
		t.Errorf("Online with LRCLIB off: %d", status)
	}
}
