package localfiles

import (
	"bytes"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/secrets"
)

// musicTools returns the FFmpeg and ffprobe of POLYFIN_TEST_FFMPEG, which
// make and read the tests' tracks, skipping the test without them.
func musicTools(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe = filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(ffprobe); err != nil {
		t.Skip("no ffprobe next to POLYFIN_TEST_FFMPEG")
	}
	return ffmpeg, ffprobe
}

// image makes a small picture at path, PNG or JPEG by its extension.
func image(t *testing.T, ffmpeg, path, color string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "color=c="+color+":s=64x64", "-frames:v", "1", path).
		CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// tone makes a second of sine at path, in the format its extension tells,
// with tags, and cover embedded when not empty.
func tone(t *testing.T, ffmpeg, path string, tags map[string]string, cover string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1"}
	if cover != "" {
		args = append(args, "-i", cover, "-map", "0:a", "-map", "1:v", "-c:v", "copy", "-disposition:v", "attached_pic")
	}
	for key, value := range tags {
		args = append(args, "-metadata", key+"="+value)
	}
	switch filepath.Ext(path) {
	case ".m4a":
		args = append(args, "-c:a", "aac")
	case ".mp3":
		args = append(args, "-c:a", "libmp3lame", "-id3v2_version", "3")
	}
	if out, err := exec.Command(ffmpeg, append(args, path)...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// musicFolder adds a music folder at dir and waits for its first scan.
func (e env) musicFolder(dir string) accounts.ID {
	e.t.Helper()
	addon, err := e.service.Add(e.t.Context(), NewFolder{Name: "Records", Path: dir, Kind: KindMusic})
	if err != nil {
		e.t.Fatal(err)
	}
	e.idle(addon.ID)
	return addon.ID
}

// albumTitles lists a music folder's row of albums as "title by artist".
func (e env) albumTitles(id accounts.ID) map[string]eclipse.Album {
	e.t.Helper()
	items, length, err := e.service.MusicCatalog(e.t.Context(), id, eclipse.Catalog{ID: catalogID, Type: eclipse.TypeAlbum}, 0)
	if err != nil || length != len(items) {
		e.t.Fatalf("catalog: %d %v", length, err)
	}
	albums := map[string]eclipse.Album{}
	for _, item := range items {
		albums[item.Album.Title+" by "+item.Album.Artist] = *item.Album
	}
	return albums
}

func TestNamesCompleteTags(t *testing.T) {
	for _, tc := range []struct {
		path   string
		tags   trackInfo
		want   trackInfo
		reason string
	}{
		{"Artist/Album/01 Title.flac", trackInfo{},
			trackInfo{title: "Title", artist: "Artist", album: "Album", albumArtist: "Artist", track: 1}, ""},
		{"Artist/Album/CD2/1-07 - Title.mp3", trackInfo{},
			trackInfo{title: "Title", artist: "Artist", album: "Album", albumArtist: "Artist", track: 7, disc: 2}, ""},
		{"Album/Song.ogg", trackInfo{}, trackInfo{title: "Song", album: "Album"}, ""},
		{"Song.ogg", trackInfo{}, trackInfo{title: "Song"}, "no_album"},
		{"2001.flac", trackInfo{album: "Numbers"}, trackInfo{title: "2001", album: "Numbers"}, ""},
		{"Folder/x.mp3", trackInfo{title: "Tagged", artist: "Singer", album: "Hits", compilation: true},
			trackInfo{title: "Tagged", artist: "Singer", album: "Hits", albumArtist: variousArtists, compilation: true}, ""},
		{"Folder/x.mp3", trackInfo{title: "Tagged", artist: "Singer", album: "Hits", albumArtist: "Band"},
			trackInfo{title: "Tagged", artist: "Singer", album: "Hits", albumArtist: "Band"}, ""},
	} {
		got, reason := named(tc.path, tc.tags)
		if !slices.Equal(got.genres, tc.want.genres) || reason != tc.reason {
			t.Errorf("%s: %+v %q", tc.path, got, reason)
		}
		got.genres, tc.want.genres = nil, nil
		if got.title != tc.want.title || got.artist != tc.want.artist || got.album != tc.want.album || got.albumArtist != tc.want.albumArtist ||
			got.track != tc.want.track || got.disc != tc.want.disc {
			t.Errorf("%s: %+v, want %+v", tc.path, got, tc.want)
		}
	}
	if nameID("ar", " The Band ") != nameID("ar", "the band") || nameID("al", "A", "B") == nameID("al", "AB", "") {
		t.Error("identifiers by name")
	}
}

// A music folder's tracks are read with their tags, else their names, and
// grouped into albums by album artist and album, a compilation under
// Various Artists; an album's artwork is a cover its tracks embed, else an
// image beside them. It answers as an Eclipse addon: its row of albums,
// album and artist pages, searches and streams.
func TestMusicFoldersAreReadAndGrouped(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	dir := t.TempDir()
	art := filepath.Join(t.TempDir(), "art.png")
	image(t, ffmpeg, art, "red")
	tone(t, ffmpeg, filepath.Join(dir, "Artist A", "Album One", "01 First.mp3"), map[string]string{"title": "First Song", "artist": "Artist A",
		"album": "Album One", "track": "1/2", "disc": "1/2", "date": "2001-05-01", "genre": "Rock;Pop"}, art)
	tone(t, ffmpeg, filepath.Join(dir, "Artist A", "Album One", "02 Second.flac"), map[string]string{"title": "Second Song",
		"artist": "Artist A", "album": "Album One", "track": "2", "disc": "1"}, "")
	tone(t, ffmpeg, filepath.Join(dir, "Artist A", "Album One", "CD2", "01 Third.wav"), nil, "")
	tone(t, ffmpeg, filepath.Join(dir, "Hits", "01.m4a"), map[string]string{"title": "Up", "artist": "Singer B", "album": "Hits",
		"compilation": "1"}, "")
	tone(t, ffmpeg, filepath.Join(dir, "Hits", "02.m4a"), map[string]string{"title": "Down", "artist": "Singer C", "album": "Hits",
		"compilation": "1"}, "")
	tone(t, ffmpeg, filepath.Join(dir, "Artist D", "Quiet", "01 Calm.ogg"), map[string]string{"title": "Calm", "artist": "Artist D",
		"album": "Quiet"}, "")
	image(t, ffmpeg, filepath.Join(dir, "Artist D", "Quiet", "Folder.jpg"), "blue")
	tone(t, ffmpeg, filepath.Join(dir, "Untagged Band", "Untagged Album", "03 Lonely Track.wav"), nil, "")
	tone(t, ffmpeg, filepath.Join(dir, "stray.wav"), nil, "")
	id := e.musicFolder(dir)

	albums := e.albumTitles(id)
	if got := slices.Sorted(maps.Keys(albums)); !slices.Equal(got, []string{"Album One by Artist A", "Hits by Various Artists",
		"Quiet by Artist D", "Untagged Album by Untagged Band"}) {
		t.Fatalf("albums: %v", got)
	}
	one := albums["Album One by Artist A"]
	if one.Year != 2001 || one.TrackCount != 3 || !slices.Equal(one.Genres, []string{"Rock", "Pop"}) || one.ArtworkURL == "" {
		t.Errorf("Album One: %+v", one)
	}
	if albums["Quiet by Artist D"].ArtworkURL == "" || albums["Hits by Various Artists"].ArtworkURL != "" {
		t.Errorf("artwork: %+v", albums)
	}
	page, err := e.service.MusicAlbum(t.Context(), id, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	type numbered struct {
		title, artist string
		disc, number  int
	}
	var tracks []numbered
	for _, track := range page.Tracks {
		tracks = append(tracks, numbered{track.Title, track.Artist, track.Disc, track.Number})
	}
	if !slices.Equal(tracks, []numbered{{"First Song", "Artist A", 1, 1}, {"Second Song", "Artist A", 1, 2}, {"Third", "Artist A", 2, 1}}) {
		t.Errorf("Album One's tracks: %+v", tracks)
	}
	hits, err := e.service.MusicAlbum(t.Context(), id, albums["Hits by Various Artists"].ID)
	if err != nil || len(hits.Tracks) != 2 || hits.Tracks[0].Artist != "Singer B" || hits.Tracks[0].AlbumArtist != variousArtists {
		t.Errorf("a compilation: %+v %v", hits, err)
	}

	// The covers are served: the one a track embeds, and the image.
	for _, album := range []eclipse.Album{one, albums["Quiet by Artist D"]} {
		file, _, err := e.service.Open(t.Context(), filepath.Base(album.ArtworkURL[:bytes.IndexByte([]byte(album.ArtworkURL), '?')]))
		if err != nil {
			t.Fatalf("%s's artwork: %v", album.Title, err)
		}
		data, _ := io.ReadAll(file)
		file.Close()
		if !bytes.HasPrefix(data, []byte("\x89PNG")) && !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
			t.Errorf("%s's artwork: % x", album.Title, data[:min(len(data), 8)])
		}
	}

	artist, err := e.service.MusicArtist(t.Context(), id, nameID("ar", "artist a"))
	if err != nil || artist.Name != "Artist A" || len(artist.Albums) != 1 || len(artist.TopTracks) != 3 || artist.ArtworkURL == "" {
		t.Errorf("an artist: %+v %v", artist, err)
	}
	singer, err := e.service.MusicArtist(t.Context(), id, nameID("ar", "Singer C"))
	if err != nil || len(singer.Albums) != 0 || len(singer.TopTracks) != 1 {
		t.Errorf("an artist of a compilation: %+v %v", singer, err)
	}
	results, err := e.service.MusicSearch(t.Context(), id, "song first")
	if err != nil || len(results.Tracks) != 1 || results.Tracks[0].Title != "First Song" {
		t.Errorf("a search for a track: %+v %v", results, err)
	}
	if results, _ := e.service.MusicSearch(t.Context(), id, "singer"); len(results.Artists) != 2 || len(results.Albums) != 0 {
		t.Errorf("a search for artists: %+v", results)
	}
	if results, _ := e.service.MusicSearch(t.Context(), id, "HITS various"); len(results.Albums) != 1 {
		t.Errorf("a search for an album: %+v", results)
	}
	stream, err := e.service.MusicStream(t.Context(), id, page.Tracks[1].ID)
	if err != nil || stream.Codec != "flac" || stream.Container != "flac" || stream.SampleRate == 0 || filepath.Ext(stream.URL) != ".flac" {
		t.Errorf("a stream: %+v %v", stream, err)
	}

	folder, err := e.service.Folder(t.Context(), id)
	if err != nil || folder.Files != 8 || folder.Matched != 7 || folder.Unmatched != 1 || folder.Albums != 4 || folder.Artists != 4 {
		t.Errorf("figures: %+v %v", folder, err)
	}
	unmatched, total, err := e.service.Unmatched(t.Context(), id)
	if err != nil || total != 1 || unmatched[0].Path != "stray.wav" || unmatched[0].Reason != "no_album" || unmatched[0].Title != "stray" {
		t.Errorf("unmatched: %+v %v", unmatched, err)
	}
}

// A rescan reads again the tracks whose files changed, forgets those gone
// and the artwork of albums gone, and reads no other.
func TestMusicRescans(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	dir := t.TempDir()
	first := filepath.Join(dir, "Band", "Early", "01 One.flac")
	tone(t, ffmpeg, first, map[string]string{"title": "One", "artist": "Band", "album": "Early"}, "")
	image(t, ffmpeg, filepath.Join(dir, "Band", "Early", "cover.png"), "green")
	tone(t, ffmpeg, filepath.Join(dir, "Band", "Late", "01 Two.flac"), map[string]string{"title": "Two", "artist": "Band", "album": "Late"}, "")
	id := e.musicFolder(dir)
	if got := slices.Sorted(maps.Keys(e.albumTitles(id))); !slices.Equal(got, []string{"Early by Band", "Late by Band"}) {
		t.Fatalf("albums: %v", got)
	}
	tone(t, ffmpeg, first, map[string]string{"title": "One (Remastered)", "artist": "Band", "album": "Early", "date": "1999"}, "")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(first, later, later); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "Band", "Late")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	albums := e.albumTitles(id)
	early, ok := albums["Early by Band"]
	if len(albums) != 1 || !ok || early.Year != 1999 || early.ArtworkURL == "" {
		t.Fatalf("albums after the rescan: %+v", albums)
	}
	page, err := e.service.MusicAlbum(t.Context(), id, early.ID)
	if err != nil || len(page.Tracks) != 1 || page.Tracks[0].Title != "One (Remastered)" {
		t.Errorf("the changed track: %+v %v", page, err)
	}
	var covers int
	if err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM local_covers WHERE addon_id = $1", id).Scan(&covers); err != nil || covers != 1 {
		t.Errorf("covers: %d %v", covers, err)
	}
}

// A share's tracks are read from the share, their tags and covers through
// the loopback server.
func TestShareMusic(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	art := filepath.Join(t.TempDir(), "art.jpg")
	image(t, ffmpeg, art, "yellow")
	tone(t, ffmpeg, filepath.Join(dir, "music", "Band", "Live", "01 Opening.mp3"), map[string]string{"title": "Opening", "artist": "Band",
		"album": "Live", "track": "1"}, art)
	tone(t, ffmpeg, filepath.Join(dir, "music", "Band", "Live", "02 Closing.mp3"), nil, "")
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/music/", Kind: KindMusic, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	albums := e.albumTitles(addon.ID)
	live, ok := albums["Live by Band"]
	if len(albums) != 1 || !ok || live.TrackCount != 2 || live.ArtworkURL == "" {
		t.Fatalf("albums: %+v", albums)
	}
	page, err := e.service.MusicAlbum(t.Context(), addon.ID, live.ID)
	if err != nil || len(page.Tracks) != 2 || page.Tracks[0].Title != "Opening" || page.Tracks[1].Title != "Closing" || page.Tracks[1].Number != 2 {
		t.Errorf("tracks: %+v %v", page.Tracks, err)
	}
}
