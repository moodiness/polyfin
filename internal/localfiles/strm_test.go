package localfiles

import (
	"bytes"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// badStrmMessage is what a scan logs of a .strm file naming no address
// Polyfin plays.
const badStrmMessage = "A .strm file names no http or https address: it is left out"

// The address of a .strm file is its first line neither blank nor a
// comment, http or https only.
func TestStrmAddresses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		content, address, reason string
	}{
		{"https://cdn.example/a.mp4", "https://cdn.example/a.mp4", ""},
		{"\uFEFF#EXTM3U\r\n#KODIPROP:inputstream=inputstream.adaptive\r\n\r\n  HTTP://cdn.example/a.m3u8?key=1  \r\nhttps://other.example/\r\n",
			"http://cdn.example/a.m3u8?key=1", ""},
		{"", "", strmEmpty},
		{" \n#KODIPROP:x=y\n\n", "", strmEmpty},
		{"movies/a.mp4", "", strmNotHTTP},
		{"/media/a.mp4", "", strmNotHTTP},
		{"file:///media/a.mp4", "", strmNotHTTP},
		{"rtsp://camera.example/live", "", strmNotHTTP},
		{"http:///a.mp4", "", strmNotHTTP},
		{"https://cdn.example/a\x01.mp4", "", strmNotHTTP},
	} {
		if address, reason := strmAddress([]byte(c.content)); address != c.address || reason != c.reason {
			t.Errorf("%q: %q %q", c.content, address, reason)
		}
	}
}

// writeStrm writes a .strm file holding content at rel under dir, its
// modification time at.
func writeStrm(t *testing.T, dir, rel, content string, at time.Time) {
	t.Helper()
	name := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(name, at, at); err != nil {
		t.Fatal(err)
	}
}

// scanAgain scans a folder and waits for the scan to end.
func (e env) scanAgain(id accounts.ID) {
	e.t.Helper()
	if err := e.service.Scan(e.t.Context(), id); err != nil {
		e.t.Fatal(err)
	}
}

// A .strm file of a movies or shows folder is a title named as a video
// is, whose one stream is the address it names, remote; a rescan plays
// the address the file names now. A file naming none, empty, relative or
// of another scheme, or larger than maxStrm, is left out, logged once
// until it changes; one whose address turns bad leaves the library, and
// one made good joins it.
func TestStrmFilesAreRemoteStreams(t *testing.T) {
	t.Parallel()
	logs := &keptLogs{}
	e := newEnvLogged(t, slog.New(logs), newMetaAddon(t))
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeStrm(t, dir, "Nosferatu (1922).strm", "#KODIPROP:inputstream=inputstream.adaptive\n\nhttps://cdn.example/v/nosferatu.mp4?token=abc\n", at)
	writeStrm(t, dir, "The General (1926).strm", "ftp://cdn.example/general.mp4", at)
	writeStrm(t, dir, "The Gold Rush (1925).strm", "movies/gold rush.mp4", at)
	writeStrm(t, dir, "Night of the Living Dead (1968).strm", "", at)
	writeStrm(t, dir, "Night of the Living Dead (1990).strm", "https://cdn.example/"+strings.Repeat("a", maxStrm), at)
	writeStrm(t, dir, "An Unknown Picture (1950).strm", "https://cdn.example/unknown", at)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).strm": "tt0013442",
		"An Unknown Picture (1950).strm": "unmatched: not_found"}) {
		t.Errorf("files: %v", got)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0013442")
	if err != nil || len(streams) != 1 || streams[0].URL != "https://cdn.example/v/nosferatu.mp4?token=abc" || !streams[0].Remote ||
		streams[0].Name != "Shelf" || streams[0].Description != "" || streams[0].BehaviorHints.Filename != "Nosferatu (1922).mp4" ||
		streams[0].BehaviorHints.VideoSize != 0 || !streams[0].Playable() {
		t.Fatalf("streams: %+v %v", streams, err)
	}
	reasons := map[string]string{}
	for _, attrs := range logs.with(badStrmMessage) {
		reasons[attrs["path"]] = attrs["reason"]
	}
	if !maps.Equal(reasons, map[string]string{"The General (1926).strm": strmNotHTTP, "The Gold Rush (1925).strm": strmNotHTTP,
		"Night of the Living Dead (1968).strm": strmEmpty, "Night of the Living Dead (1990).strm": strmTooLarge}) {
		t.Errorf("logged: %v", reasons)
	}
	// What the file holds is never served as a file of the folder.
	if _, _, err := e.service.Open(t.Context(), e.fileKey(addon.ID, "Nosferatu (1922).strm")); err != ErrGone {
		t.Errorf("the .strm file opened: %v", err)
	}

	// The file changes: the rescan plays its new address. The bad files,
	// unchanged, are not logged again; one made good joins the library,
	// and a good one turned bad leaves it.
	later := at.Add(time.Minute)
	writeStrm(t, dir, "Nosferatu (1922).strm", "https://cdn.example/v/nosferatu/master.m3u8", later)
	writeStrm(t, dir, "The General (1926).strm", "https://cdn.example/general.mkv", later)
	writeStrm(t, dir, "An Unknown Picture (1950).strm", "", later)
	e.scanAgain(addon.ID)
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).strm": "tt0013442", "The General (1926).strm": "tt0017925"}) {
		t.Errorf("files after the change: %v", got)
	}
	streams, err = e.service.Streams(t.Context(), addon.ID, "tt0013442")
	if err != nil || len(streams) != 1 || streams[0].URL != "https://cdn.example/v/nosferatu/master.m3u8" ||
		streams[0].BehaviorHints.Filename != "Nosferatu (1922).m3u8" {
		t.Errorf("streams after the change: %+v %v", streams, err)
	}
	if got := len(logs.with(badStrmMessage)); got != 5 {
		t.Errorf("logged %d times", got)
	}
	// A scan with nothing changed keeps every address and logs nothing.
	e.scanAgain(addon.ID)
	if streams, err := e.service.Streams(t.Context(), addon.ID, "tt0017925"); err != nil || len(streams) != 1 ||
		streams[0].URL != "https://cdn.example/general.mkv" {
		t.Errorf("streams of a file unchanged: %+v %v", streams, err)
	}
	if got := len(logs.with(badStrmMessage)); got != 5 {
		t.Errorf("logged %d times after a scan with nothing changed", got)
	}
}

// A shows folder's .strm file is an episode, named as an episode's video
// is; a home videos folder lists none.
func TestStrmFilesInShowsAndHomeVideos(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	at := time.Now().Add(-time.Hour)
	shows := t.TempDir()
	writeStrm(t, shows, "The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02 - 720p.strm", "https://cdn.example/ranger/1x02", at)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: shows, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0041038:1:2")
	if err != nil || len(streams) != 1 || streams[0].URL != "https://cdn.example/ranger/1x02" || !streams[0].Remote ||
		streams[0].Description != "720p" || streams[0].BehaviorHints.Filename != "The Lone Ranger - S01E02 - 720p" {
		t.Errorf("streams: %+v %v", streams, err)
	}
	home := t.TempDir()
	writeStrm(t, home, "Beach.strm", "https://cdn.example/beach.mp4", at)
	videos, err := e.service.Add(t.Context(), NewFolder{Name: "Family", Path: home, Kind: KindHomeVideos})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(videos.ID)
	if got := e.files(videos.ID); len(got) != 0 {
		t.Errorf("home videos: %v", got)
	}
}

// A WebDAV share's .strm file is read from the share as a local one is.
func TestStrmFilesOnShares(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dir := t.TempDir()
	writeStrm(t, dir, "Nosferatu (1922)/Nosferatu (1922).strm", "https://cdn.example/nosferatu.webm", time.Now().Add(-time.Hour))
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	streams, err := e.service.Streams(t.Context(), addon.ID, "tt0013442")
	if err != nil || len(streams) != 1 || streams[0].URL != "https://cdn.example/nosferatu.webm" || !streams[0].Remote {
		t.Errorf("streams: %+v %v", streams, err)
	}
}

// With POLYFIN_SECRET_KEY set, a .strm file's address is stored sealed and
// read back as it was. With another key, its title plays it for no one,
// until the next scan reads it again from its file and stores it sealed
// with that key.
func TestStrmAddressesAreStoredSealed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	key, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = key
	dir := t.TempDir()
	const address = "https://cdn.example/v/nosferatu.mp4?token=abc"
	writeStrm(t, dir, "Nosferatu (1922).strm", address, time.Now().Add(-time.Hour))
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	storedAddress := func() string {
		t.Helper()
		var value string
		if err := e.pool.QueryRow(t.Context(), "SELECT stream_url FROM local_files WHERE addon_id = $1", addon.ID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	sealed := storedAddress()
	if !secrets.Sealed(sealed) || strings.Contains(sealed, "cdn.example") {
		t.Fatalf("stored: %q", sealed)
	}
	if opened, err := key.Open(sealed); err != nil || opened != address {
		t.Errorf("opened: %q %v", opened, err)
	}
	if streams, err := e.service.Streams(t.Context(), addon.ID, "tt0013442"); err != nil || len(streams) != 1 || streams[0].URL != address {
		t.Errorf("streams: %+v %v", streams, err)
	}
	// A rescan of the file unchanged stores nothing again.
	e.scanAgain(addon.ID)
	if again := storedAddress(); again != sealed {
		t.Errorf("a rescan sealed the address again: %q", again)
	}

	other, err := secrets.New(bytes.Repeat([]byte{9}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = other
	if streams, err := e.service.Streams(t.Context(), addon.ID, "tt0013442"); err != nil || len(streams) != 0 {
		t.Errorf("streams of an address the key cannot open: %+v %v", streams, err)
	}
	e.scanAgain(addon.ID)
	if opened, err := other.Open(storedAddress()); err != nil || opened != address {
		t.Errorf("after the rescan with another key: %q %v", opened, err)
	}
	if streams, err := e.service.Streams(t.Context(), addon.ID, "tt0013442"); err != nil || len(streams) != 1 || streams[0].URL != address {
		t.Errorf("streams after the rescan: %+v %v", streams, err)
	}
	if got := e.files(addon.ID); !maps.Equal(got, map[string]string{"Nosferatu (1922).strm": "tt0013442"}) {
		t.Errorf("files: %v", got)
	}
}
