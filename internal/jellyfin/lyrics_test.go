package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/lyrics"
)

// lyricsAnswers are what a fake LRCLIB knows of the fake music addon's
// songs: synced lyrics, plain lyrics, and an instrumental.
var lyricsAnswers = map[string]string{
	"First Light": `{"instrumental": false, "plainLyrics": "Hello\nWorld", "syncedLyrics": "[00:01.50] Hello\n[00:02.25]World\n"}`,
	"Second Wind": `{"instrumental": false, "plainLyrics": "Plain words\nMore words", "syncedLyrics": null}`,
	"Rude Words":  `{"instrumental": true, "plainLyrics": null, "syncedLyrics": null}`,
}

// lyricsServer is a music addon's songs served with lyrics from a fake
// LRCLIB, which counts the requests for each song.
type lyricsServer struct {
	testServer
	token string
	mu    sync.Mutex
	asked map[string]int
	// songs are the songs of the addon's albums, by name.
	songs map[string]string
}

func newLyricsServer(t *testing.T) *lyricsServer {
	t.Helper()
	l := &lyricsServer{asked: map[string]int{}, songs: map[string]string{}}
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("track_name")
		l.mu.Lock()
		l.asked[name]++
		l.mu.Unlock()
		answer, ok := lyricsAnswers[name]
		if r.URL.Path != "/api/get" || r.URL.Query().Get("artist_name") != "Tone Quartet" || !ok {
			http.Error(w, `{"code":404,"name":"TrackNotFound"}`, http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(lrclib.Close)
	l.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		o.Lyrics = lyrics.New(pool, lrclib.URL, "test", slog.New(slog.DiscardHandler), o.Accounts.Settings)
	})
	if _, err := l.addons.Install(t.Context(), addons.Shared(), newFakeEclipse(t, "").url, false); err != nil {
		t.Fatal(err)
	}
	l.user("listener", nil)
	l.token = l.signIn("listener", "web")
	var albums QueryResult
	l.get(t, "/Items?IncludeItemTypes=MusicAlbum&Recursive=true", l.token, &albums)
	for _, album := range albums.Items {
		for _, song := range l.album(t, album.Id) {
			l.songs[song.Name] = song.Id
		}
	}
	if len(l.songs) != 3 {
		t.Fatalf("songs: %v", l.songs)
	}
	return l
}

// album lists an album's songs.
func (l *lyricsServer) album(t *testing.T, id string) []BaseItemDto {
	t.Helper()
	var songs QueryResult
	l.get(t, "/Items?ParentId="+id, l.token, &songs)
	return songs.Items
}

func (l *lyricsServer) count(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.asked[name]
}

func (l *lyricsServer) total() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	total := 0
	for _, n := range l.asked {
		total += n
	}
	return total
}

// lyricLines answers a song's lyrics as raw lines, and their metadata.
func (l *lyricsServer) lyricLines(t *testing.T, name string) (int, map[string]any, []map[string]any) {
	t.Helper()
	status, body := l.call(http.MethodGet, "/Audio/"+l.songs[name]+"/Lyrics", app("web", l.token), nil)
	if status != http.StatusOK {
		if !jsonHas(body, "title", "Not Found") {
			t.Errorf("%s: %d %s", name, status, body)
		}
		return status, nil, nil
	}
	var dto struct {
		Metadata map[string]any
		Lyrics   []map[string]any
	}
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatal(err)
	}
	return status, dto.Metadata, dto.Lyrics
}

// hasLyrics tells which songs of the album holding name have lyrics, as
// its listing says.
func (l *lyricsServer) hasLyrics(t *testing.T, name string) map[string]bool {
	t.Helper()
	var details BaseItemDto
	l.get(t, "/Items/"+l.songs[name], l.token, &details)
	result := map[string]bool{}
	for _, song := range l.album(t, details.AlbumId) {
		result[song.Name] = song.HasLyrics != nil && *song.HasLyrics
	}
	return result
}

func TestSongLyricsFromLRCLIB(t *testing.T) {
	l := newLyricsServer(t)

	// Listings never look songs up.
	var songs QueryResult
	l.get(t, "/Items?IncludeItemTypes=Audio&Recursive=true", l.token, &songs)
	for _, song := range songs.Items {
		if song.HasLyrics == nil || *song.HasLyrics {
			t.Errorf("%s listed with lyrics %v before they were looked up", song.Name, song.HasLyrics)
		}
	}
	if n := l.total(); n != 0 {
		t.Fatalf("listings asked LRCLIB %d times", n)
	}

	// Synced lyrics: each line starts at its time, in ticks.
	status, metadata, lines := l.lyricLines(t, "First Light")
	if status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Hello" || lines[0]["Start"] != float64(15_000_000) ||
		lines[1]["Text"] != "World" || lines[1]["Start"] != float64(22_500_000) {
		t.Errorf("synced lyrics: %d %v", status, lines)
	}
	if metadata["IsSynced"] != true || metadata["Title"] != "First Light" || metadata["Length"] != float64(100_000_000) {
		t.Errorf("synced metadata: %v", metadata)
	}

	// A song's details look it up, and tell it has lyrics; its plain lyrics
	// are lines without starts.
	var details BaseItemDto
	l.get(t, "/Items/"+l.songs["Second Wind"], l.token, &details)
	if details.HasLyrics == nil || !*details.HasLyrics {
		t.Errorf("details of a song with lyrics: HasLyrics %v", details.HasLyrics)
	}
	status, metadata, lines = l.lyricLines(t, "Second Wind")
	if status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Plain words" || metadata["IsSynced"] != false {
		t.Errorf("plain lyrics: %d %v %v", status, metadata, lines)
	}
	for _, line := range lines {
		if _, synced := line["Start"]; synced {
			t.Errorf("a plain line has a start: %v", line)
		}
	}
	if has := l.hasLyrics(t, "First Light"); !has["First Light"] || !has["Second Wind"] {
		t.Errorf("listed once known: %v", has)
	}

	// A song that starts playing is looked up in the background: an
	// instrumental has no lyrics.
	if status, body := l.call(http.MethodPost, "/Sessions/Playing", app("web", l.token),
		map[string]any{"ItemId": l.songs["Rude Words"], "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("playback start: %d %s", status, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for l.count("Rude Words") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("a song played was not looked up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status, _, _ := l.lyricLines(t, "Rude Words"); status != http.StatusNotFound {
		t.Errorf("an instrumental: %d", status)
	}
	if has := l.hasLyrics(t, "Rude Words"); has["Rude Words"] {
		t.Error("an instrumental is listed with lyrics")
	}

	// Each song was asked about once, however often it was asked for.
	for name := range lyricsAnswers {
		if n := l.count(name); n != 1 {
			t.Errorf("LRCLIB was asked %d times about %s", n, name)
		}
	}

	// Turned off, no song has lyrics, and LRCLIB is not asked.
	l.setting(t, func(s *accounts.Settings) { s.Lyrics = false })
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound {
		t.Errorf("lyrics turned off: %d", status)
	}
	if has := l.hasLyrics(t, "First Light"); has["First Light"] || has["Second Wind"] {
		t.Errorf("listed with lyrics turned off: %v", has)
	}
	if n := l.total(); n != 3 {
		t.Errorf("LRCLIB was asked %d times in all", n)
	}
}
