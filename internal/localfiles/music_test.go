package localfiles

import (
	"bytes"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
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

// A track's artists are those of its artists tag, else of its artist tag,
// split at ";"; its artist stays as written. Its album artists split the
// same way only when its album artist tag names several; without one, its
// album artist is its artist as written. Its lyrics are its lyrics tag's,
// an ID3 tag's too.
func TestArtistsAndLyricsTags(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		tags                  string
		artist, albumArtist   string
		artists, albumArtists []string
		lyrics                string
	}{
		{"one artist", `{"artist": "Ann", "album": "A", "title": "T"}`, "Ann", "Ann", []string{"Ann"}, []string{"Ann"}, ""},
		{"an artists tag", `{"ARTIST": "Ann feat. Ben", "ARTISTS": "Ann;Ben", "album": "A", "title": "T", "LYRICS": "[00:01.00]Hi"}`,
			"Ann feat. Ben", "Ann feat. Ben", []string{"Ann", "Ben"}, []string{"Ann feat. Ben"}, "[00:01.00]Hi"},
		{"names separated", `{"artist": "Ann; Ben ;ann", "album": "A", "title": "T", "lyrics-eng": "Words"}`,
			"Ann; Ben ;ann", "Ann; Ben ;ann", []string{"Ann", "Ben"}, []string{"Ann; Ben ;ann"}, "Words"},
		{"several album artists", `{"artist": "Ann", "album_artist": "Cat; Dan", "album": "A", "title": "T"}`,
			"Ann", "Cat; Dan", []string{"Ann"}, []string{"Cat", "Dan"}, ""},
		{"an artists tag alone", `{"artists": "Ann;Ben", "album": "A", "title": "T", "unsyncedlyrics": "Plain"}`,
			"Ann; Ben", "Ann; Ben", []string{"Ann", "Ben"}, []string{"Ann; Ben"}, "Plain"},
	} {
		info, err := readProbe([]byte(`{"streams": [{"codec_type": "audio", "codec_name": "flac"}], "format": {"tags": ` + tc.tags + `}}`))
		if err != nil {
			t.Fatal(err)
		}
		info, _ = named("A/T.flac", info)
		if info.artist != tc.artist || info.albumArtist != tc.albumArtist || !slices.Equal(info.artists, tc.artists) ||
			!slices.Equal(info.albumArtists, tc.albumArtists) || info.lyrics != tc.lyrics {
			t.Errorf("%s: %q %q %q %q %q", tc.name, info.artist, info.albumArtist, info.artists, info.albumArtists, info.lyrics)
		}
	}
	if info, _ := named("Various/Hits/01 Up.flac", trackInfo{title: "Up", artist: "Ann;Ben", album: "Hits", compilation: true}); info.albumArtist != variousArtists ||
		!slices.Equal(info.albumArtists, []string{variousArtists}) || !slices.Equal(info.artists, []string{"Ann", "Ben"}) {
		t.Errorf("a compilation: %+v", info)
	}
}

// A playlist's lines name tracks by paths relative to its folder, or
// absolute within the folder, as paths or file: addresses, with slashes
// or backslashes; those leading out of it are left out. Its title is its
// #PLAYLIST line's, else its name.
func TestM3UPlaylistsAreRead(t *testing.T) {
	text := "#EXTM3U\n#PLAYLIST:Evening Mix\n#EXTINF:123,Ann - Evening\nEvening.flac\n\n  ../Other/Night.mp3  \n/media/music/Ann/Debut/Morning.flac\n" +
		"/srv/real/Ann/Debut/Noon.flac\nfile:///media/music/Ann/Debut/Dusk%20Song.flac\nAnn\\Live\\Encore.flac\n/elsewhere/Song.flac\n" +
		"../../../outside.flac\nhttps://radio.example/stream.mp3\nC:\\Music\\Song.flac\n"
	title, entries := readM3U("Ann/Debut/Mix.m3u8", text, []string{"/media/music", "/srv/real"})
	want := []string{"Ann/Debut/Evening.flac", "Ann/Other/Night.mp3", "Ann/Debut/Morning.flac", "Ann/Debut/Noon.flac", "Ann/Debut/Dusk Song.flac",
		"Ann/Debut/Ann/Live/Encore.flac"}
	if title != "Evening Mix" || !slices.Equal(entries, want) {
		t.Errorf("%q %q", title, entries)
	}
	if title, entries := readM3U("Road Trip.m3u", "Ann/Debut/Evening.flac\n/media/music/Ann/x.flac", nil); title != "Road Trip" ||
		!slices.Equal(entries, []string{"Ann/Debut/Evening.flac"}) {
		t.Errorf("a share's playlist: %q %q", title, entries)
	}
}

// An artist's folder holds the folders of their albums, and of no album
// of another artist or of several.
func TestArtistFolders(t *testing.T) {
	got := artistFolders(map[string][]string{
		"Ann/Debut/01.flac":       {"ann"},
		"Ann/Live/CD2/01.flac":    {"ann"},
		"Mixed/One/01.flac":       {"ben"},
		"Mixed/Two/01.flac":       {"cat"},
		"Duo/Joint/01.flac":       {"cat", "dan"},
		"Loose.flac":              {"eve"},
		"Single/01.flac":          {"fay"},
		"Deep/Fay/Album/01.flac":  {"fay"},
		"Deep/Fay/Album2/02.flac": {"fay"},
	})
	if want := map[string]string{"Ann": "ann", "Deep/Fay": "fay"}; !maps.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

// A music folder's playlists list the tracks they name in their order,
// leaving out lines naming none; a track lists under each of its artists,
// an album under each of its album artists; a track's lyrics are its
// lyrics file's, else its lyrics tag's; an artist's picture is the image
// in their folder. Rescans read them again when they change.
func TestMusicPlaylistsArtistsLyricsAndPictures(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	dir := t.TempDir()
	debut := filepath.Join(dir, "Ann", "Debut")
	tone(t, ffmpeg, filepath.Join(debut, "01 Morning.flac"), map[string]string{"title": "Morning", "artist": "Ann feat. Ben", "ARTISTS": "Ann;Ben",
		"album": "Debut", "album_artist": "Ann", "track": "1", "lyrics": "Tag words"}, "")
	tone(t, ffmpeg, filepath.Join(debut, "02 Evening.flac"), map[string]string{"title": "Evening", "artist": "Ann", "album": "Debut", "track": "2",
		"lyrics": "[00:00.50]Evening line"}, "")
	tone(t, ffmpeg, filepath.Join(debut, "03 Night.flac"), map[string]string{"title": "Night", "artist": "Ann", "album": "Debut", "track": "3"}, "")
	tone(t, ffmpeg, filepath.Join(dir, "Duo", "Joint", "01 Shared.flac"), map[string]string{"title": "Shared", "artist": "Cat; Dan",
		"album_artist": "Cat; Dan", "album": "Joint"}, "")
	image(t, ffmpeg, filepath.Join(dir, "Ann", "artist.jpg"), "purple")
	lrc := filepath.Join(debut, "01 Morning.LRC")
	now := time.Now()
	writeText(t, lrc, "[ti:Morning]\n[00:00.20]First line\n[00:00.70]Second line\n", now)
	mix := filepath.Join(dir, "Mix.m3u8")
	writeText(t, mix, "#EXTM3U\n#PLAYLIST:Evening Mix\nAnn/Debut/02 Evening.flac\n"+filepath.ToSlash(filepath.Join(dir, "Ann", "Debut", "01 Morning.flac"))+
		"\nAnn/Debut/missing.flac\n../outside.flac\nann/debut/03 NIGHT.flac\n", now)
	writeText(t, filepath.Join(debut, "Best.m3u"), "01 Morning.flac\n", now)
	writeText(t, filepath.Join(dir, "Empty.m3u"), "nothing.flac\n", now)
	id := e.musicFolder(dir)

	albums := e.albumTitles(id)
	debutAlbum, joint := albums["Debut by Ann"], albums["Joint by Cat; Dan"]
	if len(albums) != 2 || debutAlbum.ID == "" || !slices.Equal(joint.Artists, []string{"Cat", "Dan"}) || debutAlbum.ArtworkURL != "" {
		t.Fatalf("albums: %+v", albums)
	}
	page, err := e.service.MusicAlbum(t.Context(), id, debutAlbum.ID)
	if err != nil || len(page.Tracks) != 3 {
		t.Fatalf("Debut: %+v %v", page, err)
	}
	morning, evening, night := page.Tracks[0], page.Tracks[1], page.Tracks[2]
	if morning.Artist != "Ann feat. Ben" || !slices.Equal(morning.Artists, []string{"Ann", "Ben"}) || !morning.Lyrics || !evening.Lyrics || night.Lyrics {
		t.Errorf("tracks: %+v", page.Tracks)
	}

	// Each artist has their page.
	for _, tc := range []struct {
		name           string
		albums, tracks int
	}{{"Ann", 1, 3}, {"Ben", 0, 1}, {"Cat", 1, 1}, {"Dan", 1, 1}} {
		artist, err := e.service.MusicArtist(t.Context(), id, nameID("ar", tc.name))
		if err != nil || artist.Name != tc.name || len(artist.Albums) != tc.albums || len(artist.TopTracks) != tc.tracks {
			t.Errorf("%s: %+v %v", tc.name, artist, err)
		}
	}
	if _, err := e.service.MusicArtist(t.Context(), id, nameID("ar", "Cat; Dan")); err == nil {
		t.Error("an album artist tag naming several is an artist")
	}
	results, err := e.service.MusicSearch(t.Context(), id, "ben")
	if err != nil || len(results.Artists) != 1 || results.Artists[0].Name != "Ben" || len(results.Tracks) != 1 {
		t.Errorf("a search for an artist: %+v %v", results, err)
	}
	if folder, err := e.service.Folder(t.Context(), id); err != nil || folder.Artists != 3 {
		t.Errorf("figures: %+v %v", folder, err)
	}

	// Ann's picture is the image of her folder; Cat and Dan have none.
	ann, _ := e.service.MusicArtist(t.Context(), id, nameID("ar", "Ann"))
	if ann.ArtworkURL == "" || filepath.Ext(strings.Split(ann.ArtworkURL, "?")[0]) != ".jpg" {
		t.Errorf("Ann's picture: %q", ann.ArtworkURL)
	}
	if cat, _ := e.service.MusicArtist(t.Context(), id, nameID("ar", "Cat")); cat.ArtworkURL != "" {
		t.Errorf("Cat's picture: %q", cat.ArtworkURL)
	}
	file, _, err := e.service.Open(t.Context(), path.Base(strings.Split(ann.ArtworkURL, "?")[0]))
	if err != nil {
		t.Fatalf("Ann's picture: %v", err)
	}
	data, _ := io.ReadAll(file)
	file.Close()
	if !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		t.Errorf("Ann's picture: % x", data[:min(len(data), 4)])
	}

	// The playlists, by title, keep the tracks they name in order.
	playlists, err := e.service.MusicPlaylists(t.Context(), id)
	if err != nil || len(playlists) != 2 || playlists[0].Title != "Best" || playlists[0].TrackCount != 1 || playlists[1].Title != "Evening Mix" ||
		playlists[1].TrackCount != 3 {
		t.Fatalf("playlists: %+v %v", playlists, err)
	}
	titles := func(id accounts.ID, playlist string) []string {
		t.Helper()
		page, err := e.service.MusicPlaylist(t.Context(), id, playlist)
		if err != nil {
			t.Fatalf("a playlist: %v", err)
		}
		var titles []string
		for _, track := range page.Tracks {
			titles = append(titles, track.Title)
		}
		return titles
	}
	if got := titles(id, playlists[1].ID); !slices.Equal(got, []string{"Evening", "Morning", "Night"}) {
		t.Errorf("Evening Mix: %v", got)
	}
	if results, _ := e.service.MusicSearch(t.Context(), id, "mix"); len(results.Playlists) != 1 || results.Playlists[0].Title != "Evening Mix" {
		t.Errorf("a search for a playlist: %+v", results.Playlists)
	}

	// The lyrics file comes before the tag.
	lyricsOf := func(track eclipse.Track) string {
		t.Helper()
		text, err := e.service.MusicLyrics(t.Context(), id, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	if got := lyricsOf(morning); !strings.Contains(got, "[00:00.20]First line") {
		t.Errorf("Morning's lyrics: %q", got)
	}
	if got := lyricsOf(evening); got != "[00:00.50]Evening line" {
		t.Errorf("Evening's lyrics: %q", got)
	}
	if got := lyricsOf(night); got != "" {
		t.Errorf("Night's lyrics: %q", got)
	}

	// Without its lyrics file, a track has its tag's; a playlist changed
	// is read again; an artist's picture gone is forgotten.
	if err := os.Remove(lrc); err != nil {
		t.Fatal(err)
	}
	writeText(t, mix, "#PLAYLIST:Evening Mix\nAnn/Debut/03 Night.flac\nAnn/Debut/02 Evening.flac\n", now.Add(time.Minute))
	if err := os.Remove(filepath.Join(dir, "Ann", "artist.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if got := lyricsOf(morning); got != "Tag words" {
		t.Errorf("Morning's lyrics without its file: %q", got)
	}
	if got := titles(id, playlists[1].ID); !slices.Equal(got, []string{"Night", "Evening"}) {
		t.Errorf("Evening Mix changed: %v", got)
	}
	if ann, _ := e.service.MusicArtist(t.Context(), id, nameID("ar", "Ann")); ann.ArtworkURL != "" {
		t.Errorf("Ann's picture gone: %q", ann.ArtworkURL)
	}
}

// The tracks a scan first finds count among the files added, by album.
func TestAddedTracksAreCountedByAlbum(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	since := time.Now().Add(-time.Second)
	dir := t.TempDir()
	for _, name := range []string{"01 One.flac", "02 Two.flac"} {
		tone(t, ffmpeg, filepath.Join(dir, "Band", "Early", name), map[string]string{"artist": "Band", "album": "Early"}, "")
	}
	tone(t, ffmpeg, filepath.Join(dir, "stray.flac"), nil, "")
	e.folder(KindMovies, map[string]int{"The General (1926).mkv": 10})
	e.musicFolder(dir)
	added, total, err := e.service.Added(t.Context(), since, time.Now().Add(time.Minute), 10)
	if err != nil || total != 4 || len(added) != 2 || added[0] != (Added{Name: "Early", Kind: KindMusic, Files: 2}) || added[1].Kind != KindMovies {
		t.Errorf("added: %+v, %d files, %v", added, total, err)
	}
}
