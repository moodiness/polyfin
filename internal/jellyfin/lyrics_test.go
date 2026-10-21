package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
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

// lyricsResults are what the fake LRCLIB's search lists for "First Light",
// a song of 10 seconds, and answers by identifier: tracks of several
// lengths, with synced and plain lyrics or either. Its search answers a
// truncated list for "Second Wind", and an empty one otherwise.
var lyricsResults = map[string]string{
	"21": `{"id": 21, "trackName": "First Light", "artistName": "Tone Quartet", "albumName": "Live", "duration": 31, "instrumental": false, "plainLyrics": "Far", "syncedLyrics": "[00:01.00] Far"}`,
	"22": `{"id": 22, "trackName": "First Light", "artistName": "Tone Quartet", "albumName": "Sine Studies", "duration": 10.4, "instrumental": false, "plainLyrics": "Closest words\nAgain", "syncedLyrics": null}`,
	"23": `{"id": 23, "trackName": "First Light (Edit)", "artistName": "Tone Quartet", "albumName": "Sine Studies", "duration": 12, "instrumental": false, "plainLyrics": null, "syncedLyrics": "[00:00.50] Near\n[00:01.00]Nearer"}`,
}

// lyricsServer is a music addon's songs served with lyrics from a fake
// LRCLIB, which counts the requests for each song: "search:" and "id:"
// count its searches and its answers by identifier.
type lyricsServer struct {
	testServer
	token string
	mu    sync.Mutex
	asked map[string]int
	query url.Values
	// songs are the songs of the addon's albums, by name.
	songs map[string]string
}

func newLyricsServer(t *testing.T) *lyricsServer {
	t.Helper()
	l := &lyricsServer{asked: map[string]int{}, songs: map[string]string{}}
	lrclib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("track_name")
		switch {
		case r.URL.Path == "/api/search":
			l.mu.Lock()
			l.asked["search:"+name]++
			l.query = r.URL.Query()
			l.mu.Unlock()
			switch name {
			case "First Light":
				_, _ = io.WriteString(w, "["+lyricsResults["21"]+","+lyricsResults["22"]+","+lyricsResults["23"]+"]")
			case "Second Wind":
				_, _ = io.WriteString(w, "["+lyricsResults["21"][:40])
			default:
				_, _ = io.WriteString(w, "[]")
			}
			return
		case strings.HasPrefix(r.URL.Path, "/api/get/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/get/")
			l.mu.Lock()
			l.asked["id:"+id]++
			l.mu.Unlock()
			if answer, ok := lyricsResults[id]; ok {
				_, _ = io.WriteString(w, answer)
				return
			}
			http.Error(w, `{"code":404,"name":"TrackNotFound"}`, http.StatusNotFound)
			return
		}
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
	t.Parallel()
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

// searchLyrics runs the lyrics search of jellyfin-web's lyrics editor for
// a song, as the user of token.
func (l *lyricsServer) searchLyrics(t *testing.T, token, name string) (int, []RemoteLyricInfoDto, []byte) {
	t.Helper()
	status, body := l.call(http.MethodGet, "/Audio/"+l.songs[name]+"/RemoteSearch/Lyrics", app("web", token), nil)
	var results []RemoteLyricInfoDto
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &results); err != nil {
			t.Fatalf("%v in %s", err, body)
		}
	}
	return status, results, body
}

// chooseLyrics downloads a search result for a song, as jellyfin-web's
// lyrics editor does, as the user of token.
func (l *lyricsServer) chooseLyrics(t *testing.T, token, name, id string) (int, LyricDto) {
	t.Helper()
	status, body := l.call(http.MethodPost, "/Audio/"+l.songs[name]+"/RemoteSearch/Lyrics/"+id, app("web", token), nil)
	var dto LyricDto
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("%v in %s", err, body)
		}
	}
	return status, dto
}

// texts are the texts of lines of lyrics, with their starts in ticks when
// synced.
func texts(lines []LyricLine) []string {
	var list []string
	for _, line := range lines {
		text := line.Text
		if line.Start != nil {
			text = fmt.Sprintf("%d %s", *line.Start, text)
		}
		list = append(list, text)
	}
	return list
}

func TestLyricsSearch(t *testing.T) {
	t.Parallel()
	l := newLyricsServer(t)
	l.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := l.signIn("admin", "web")
	providerID := "5ef86bcecd71e4b91c950b94ebc1556e"

	// Only administrators search and choose lyrics, as on Jellyfin
	// without the permission to manage lyrics; and they are told so.
	if status, _, body := l.searchLyrics(t, l.token, "First Light"); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a listener's search: %d %s", status, body)
	}
	if status, _ := l.chooseLyrics(t, l.token, "First Light", providerID+"_22_plain"); status != http.StatusForbidden {
		t.Errorf("a listener's choice: %d", status)
	}
	if n := l.total(); n != 0 {
		t.Fatalf("LRCLIB was asked %d times for a listener", n)
	}
	var users []UserDto
	l.get(t, "/Users", admin, &users)
	for _, user := range users {
		if user.Policy.EnableLyricManagement != user.Policy.IsAdministrator {
			t.Errorf("%s manages lyrics: %v", user.Name, user.Policy.EnableLyricManagement)
		}
	}

	// The search lists LRCLIB's lyrics for the song's title, artist and
	// album, the closest length first, synced before plain, with their
	// lines for the editor's preview.
	status, results, body := l.searchLyrics(t, admin, "First Light")
	want := []string{providerID + "_22_plain", providerID + "_23_synced", providerID + "_21_synced", providerID + "_21_plain"}
	var got []string
	for _, result := range results {
		got = append(got, result.Id)
	}
	if status != http.StatusOK || !slices.Equal(got, want) {
		t.Fatalf("search: %d %s", status, body)
	}
	l.mu.Lock()
	query := l.query
	l.mu.Unlock()
	if wantQuery := (url.Values{"track_name": {"First Light"}, "artist_name": {"Tone Quartet"}, "album_name": {"Sine Studies"}}); query.Encode() != wantQuery.Encode() {
		t.Errorf("query %v, want %v", query, wantQuery)
	}
	closest, edit := results[0], results[1]
	if closest.ProviderName != "LRCLIB" || closest.Lyrics.Metadata.Title != "First Light" || closest.Lyrics.Metadata.Album != "Sine Studies" ||
		closest.Lyrics.Metadata.Artist != "Tone Quartet" || closest.Lyrics.Metadata.Length == nil || *closest.Lyrics.Metadata.Length != 104_000_000 ||
		closest.Lyrics.Metadata.IsSynced == nil || *closest.Lyrics.Metadata.IsSynced || !slices.Equal(texts(closest.Lyrics.Lyrics), []string{"Closest words", "Again"}) {
		t.Errorf("plain result: %+v", closest)
	}
	if edit.Lyrics.Metadata.Title != "First Light (Edit)" || edit.Lyrics.Metadata.IsSynced == nil || !*edit.Lyrics.Metadata.IsSynced ||
		!slices.Equal(texts(edit.Lyrics.Lyrics), []string{"5000000 Near", "10000000 Nearer"}) {
		t.Errorf("synced result: %+v", edit)
	}
	for _, field := range []string{`"Id":`, `"ProviderName":"LRCLIB"`, `"Lyrics":{"Metadata":`, `"Cues":[]`} {
		if !strings.Contains(string(body), field) {
			t.Errorf("no %s in %s", field, body)
		}
	}

	// Choosing synced lyrics replaces those LRCLIB gave the song: they are
	// the song's for every user, and LRCLIB is not asked about it again.
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusOK || l.count("First Light") != 1 {
		t.Fatalf("lyrics before the choice: %d, asked %d times", status, l.count("First Light"))
	}
	status, chosen := l.chooseLyrics(t, admin, "First Light", edit.Id)
	if status != http.StatusOK || chosen.Metadata.Title != "First Light" || chosen.Metadata.IsSynced == nil || !*chosen.Metadata.IsSynced ||
		!slices.Equal(texts(chosen.Lyrics), []string{"5000000 Near", "10000000 Nearer"}) {
		t.Errorf("choice: %d %+v", status, chosen)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Near" || lines[0]["Start"] != float64(5_000_000) {
		t.Errorf("lyrics chosen: %d %v", status, lines)
	}

	// Plain lyrics too, for a song never looked up: the look-up its
	// lyrics would have started keeps the choice.
	if status, chosen := l.chooseLyrics(t, admin, "Second Wind", closest.Id); status != http.StatusOK || !slices.Equal(texts(chosen.Lyrics), []string{"Closest words", "Again"}) {
		t.Errorf("plain choice: %d %+v", status, chosen)
	}
	if status, metadata, lines := l.lyricLines(t, "Second Wind"); status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Closest words" || metadata["IsSynced"] != false {
		t.Errorf("plain lyrics chosen: %d %v %v", status, metadata, lines)
	}
	if has := l.hasLyrics(t, "Second Wind"); !has["Second Wind"] || !has["First Light"] {
		t.Errorf("listed with lyrics: %v", has)
	}
	if l.count("First Light") != 1 || l.count("Second Wind") != 0 || l.count("id:23") != 1 || l.count("id:22") != 1 {
		t.Errorf("asked %v", l.asked)
	}

	// What is no result answers as on Jellyfin, without asking LRCLIB.
	album := BaseItemDto{}
	l.get(t, "/Items/"+l.songs["First Light"], l.token, &album)
	for name, path := range map[string]string{
		"an unknown identifier":   "/Audio/" + l.songs["First Light"] + "/RemoteSearch/Lyrics/" + providerID + "_x_plain",
		"another provider":        "/Audio/" + l.songs["First Light"] + "/RemoteSearch/Lyrics/0123_22_plain",
		"lyrics LRCLIB lacks":     "/Audio/" + l.songs["First Light"] + "/RemoteSearch/Lyrics/" + providerID + "_99_plain",
		"an album":                "/Audio/" + album.AlbumId + "/RemoteSearch/Lyrics/" + providerID + "_22_plain",
		"an unknown song":         "/Audio/" + strings.Repeat("0", 32) + "/RemoteSearch/Lyrics/" + providerID + "_22_plain",
		"the search of an album":  "/Audio/" + album.AlbumId + "/RemoteSearch/Lyrics",
		"the search of a nothing": "/Audio/" + strings.Repeat("0", 32) + "/RemoteSearch/Lyrics",
	} {
		method := http.MethodPost
		if strings.HasSuffix(path, "/Lyrics") {
			method = http.MethodGet
		}
		if status, body := l.call(method, path, app("web", admin), nil); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}

	// Turned off, the search lists nothing and a choice is not found, as
	// on a server without a lyrics provider, and LRCLIB is not asked.
	asked := l.total()
	l.setting(t, func(s *accounts.Settings) { s.Lyrics = false })
	if status, results, body := l.searchLyrics(t, admin, "First Light"); status != http.StatusOK || len(results) != 0 || strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("search turned off: %d %s", status, body)
	}
	if status, _ := l.chooseLyrics(t, admin, "First Light", closest.Id); status != http.StatusNotFound {
		t.Errorf("choice turned off: %d", status)
	}
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound {
		t.Errorf("chosen lyrics turned off: %d", status)
	}
	if n := l.total(); n != asked {
		t.Errorf("LRCLIB was asked %d times turned off", n-asked)
	}

	// Back on, the choice is back; a malformed answer lists nothing.
	l.setting(t, func(s *accounts.Settings) { s.Lyrics = true })
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Near" {
		t.Errorf("lyrics chosen, back on: %d %v", status, lines)
	}
	if status, results, body := l.searchLyrics(t, admin, "Second Wind"); status != http.StatusOK || len(results) != 0 || l.count("search:Second Wind") != 1 {
		t.Errorf("a malformed answer: %d %s", status, body)
	}
}

func TestLyricsRemoval(t *testing.T) {
	t.Parallel()
	l := newLyricsServer(t)
	l.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := l.signIn("admin", "web")
	providerID := "5ef86bcecd71e4b91c950b94ebc1556e"
	remove := func(token, id string) (int, []byte) {
		t.Helper()
		return l.call(http.MethodDelete, "/Audio/"+id+"/Lyrics", app("web", token), nil)
	}

	// Lyrics chosen, then removed, are none, as on Jellyfin once a lyrics
	// file is deleted; listings say so.
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusOK {
		t.Fatalf("lyrics found: %d", status)
	}
	if status, _ := l.chooseLyrics(t, admin, "First Light", providerID+"_23_synced"); status != http.StatusOK {
		t.Fatalf("choice: %d", status)
	}
	// Only administrators remove lyrics.
	if status, body := remove(l.token, l.songs["First Light"]); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a listener's removal: %d %s", status, body)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Near" {
		t.Errorf("lyrics after a listener's removal: %d %v", status, lines)
	}
	if status, body := remove(admin, l.songs["First Light"]); status != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("removal: %d %s", status, body)
	}
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound {
		t.Errorf("lyrics after the removal: %d", status)
	}
	if has := l.hasLyrics(t, "First Light"); has["First Light"] {
		t.Error("listed with lyrics after the removal")
	}

	// A month later, when LRCLIB would be asked again about a song it did
	// not know, the removal stays: LRCLIB is not asked.
	if _, err := l.pool.Exec(t.Context(), "UPDATE track_lyrics SET looked_up_at = now() - interval '31 days' WHERE item_id = $1",
		l.songs["First Light"]); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound || l.count("First Light") != 1 {
		t.Errorf("a month later: %d, asked %d times", status, l.count("First Light"))
	}

	// Removing lyrics never found answers as Jellyfin does without a
	// lyrics file, and keeps the song from being looked up.
	if status, _ := remove(admin, l.songs["Second Wind"]); status != http.StatusNoContent {
		t.Errorf("removal of lyrics never found: %d", status)
	}
	if status, _, _ := l.lyricLines(t, "Second Wind"); status != http.StatusNotFound || l.count("Second Wind") != 0 {
		t.Errorf("a song whose lyrics were removed: %d, asked %d times", status, l.count("Second Wind"))
	}

	// Lyrics chosen after a removal are the song's.
	if status, _ := l.chooseLyrics(t, admin, "First Light", providerID+"_22_plain"); status != http.StatusOK {
		t.Fatalf("choice after the removal: %d", status)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Closest words" {
		t.Errorf("lyrics chosen after the removal: %d %v", status, lines)
	}

	// What is no song answers 404.
	var details BaseItemDto
	l.get(t, "/Items/"+l.songs["First Light"], l.token, &details)
	for name, id := range map[string]string{"an album": details.AlbumId, "an unknown song": strings.Repeat("0", 32)} {
		if status, body := remove(admin, id); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
}

// uploadLyrics uploads a lyrics file for a song as jellyfin-web's lyrics
// editor does: its text as the body, named by fileName.
func (l *lyricsServer) uploadLyrics(t *testing.T, token, id, fileName string, body []byte) (int, []byte) {
	t.Helper()
	path := "/Audio/" + id + "/Lyrics"
	if fileName != "" {
		path += "?" + url.Values{"fileName": {fileName}}.Encode()
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, l.url+path, bytes.NewReader(body))
	request.Header.Set("Authorization", app("web", token))
	request.Header.Set("Content-Type", "text/plain")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(response.Body)
	return response.StatusCode, answer
}

func TestLyricsUpload(t *testing.T) {
	t.Parallel()
	l := newLyricsServer(t)
	l.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := l.signIn("admin", "web")
	first, second := l.songs["First Light"], l.songs["Second Wind"]
	lrc := []byte("[ti:First Light]\n[00:01.00] Uploaded line\n[00:02.50]Second uploaded line\n")

	// Only administrators upload, refused before anything is read.
	if status, body := l.uploadLyrics(t, l.token, first, "first.lrc", lrc); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a listener's upload: %d %s", status, body)
	}

	// An LRC file is the song's lyrics, synced, answered as Jellyfin
	// answers an upload, and served to every user.
	status, body := l.uploadLyrics(t, admin, first, "first.lrc", lrc)
	var dto LyricDto
	if status != http.StatusOK || json.Unmarshal(body, &dto) != nil || dto.Metadata.IsSynced == nil || !*dto.Metadata.IsSynced ||
		!slices.Equal(texts(dto.Lyrics), []string{"10000000 Uploaded line", "25000000 Second uploaded line"}) || dto.Metadata.Title != "First Light" {
		t.Fatalf("LRC upload: %d %s", status, body)
	}
	if status, metadata, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || len(lines) != 2 || lines[0]["Text"] != "Uploaded line" ||
		lines[0]["Start"] != float64(10_000_000) || metadata["IsSynced"] != true {
		t.Errorf("lyrics uploaded: %d %v %v", status, metadata, lines)
	}
	// A look-up a month later keeps them: LRCLIB is not asked.
	if _, err := l.pool.Exec(t.Context(), "UPDATE track_lyrics SET looked_up_at = now() - interval '31 days' WHERE item_id = $1", first); err != nil {
		t.Fatal(err)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Uploaded line" || l.count("First Light") != 0 {
		t.Errorf("a month later: %d %v, asked %d times", status, lines, l.count("First Light"))
	}

	// Turned off, uploads still work and are served, plain text plain,
	// listed with lyrics; LRCLIB is never asked.
	l.setting(t, func(s *accounts.Settings) { s.Lyrics = false })
	if status, body := l.uploadLyrics(t, admin, second, "second.txt", []byte("\xef\xbb\xbfTyped words\r\n\r\nMore words")); status != http.StatusOK {
		t.Fatalf("text upload turned off: %d %s", status, body)
	}
	if status, metadata, lines := l.lyricLines(t, "Second Wind"); status != http.StatusOK || len(lines) != 3 || lines[0]["Text"] != "Typed words" ||
		metadata["IsSynced"] != false || lines[0]["Start"] != nil {
		t.Errorf("plain lyrics uploaded turned off: %d %v %v", status, metadata, lines)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Uploaded line" {
		t.Errorf("synced lyrics uploaded turned off: %d %v", status, lines)
	}
	if has := l.hasLyrics(t, "First Light"); !has["First Light"] || !has["Second Wind"] {
		t.Errorf("listed turned off: %v", has)
	}
	if n := l.total(); n != 0 {
		t.Errorf("LRCLIB was asked %d times", n)
	}

	// What is not a lyrics file is refused, and keeps the lyrics there.
	for name, tc := range map[string]struct {
		fileName string
		body     []byte
		status   int
	}{
		"no file name":      {"", lrc, http.StatusBadRequest},
		"no body":           {"first.lrc", nil, http.StatusBadRequest},
		"no extension":      {"first", lrc, http.StatusBadRequest},
		"another extension": {"first.mp3", lrc, http.StatusBadRequest},
		"binary":            {"first.lrc", []byte("[00:01.00]\x00\x01\x02"), http.StatusBadRequest},
		"not UTF-8":         {"first.txt", []byte("caf\xe9"), http.StatusBadRequest},
		"blank":             {"first.txt", []byte(" \n \n"), http.StatusBadRequest},
		"too large":         {"first.txt", bytes.Repeat([]byte("la "), lyrics.MaxUpload/3+1), http.StatusRequestEntityTooLarge},
	} {
		if status, body := l.uploadLyrics(t, admin, first, tc.fileName, tc.body); status != tc.status {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Uploaded line" {
		t.Errorf("lyrics after refused uploads: %d %v", status, lines)
	}
	var details BaseItemDto
	l.get(t, "/Items/"+first, l.token, &details)
	for name, id := range map[string]string{"an album": details.AlbumId, "an unknown song": strings.Repeat("0", 32)} {
		if status, body := l.uploadLyrics(t, admin, id, "first.lrc", lrc); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}

	// Delete removes an upload; another upload brings lyrics back.
	if status, _ := l.call(http.MethodDelete, "/Audio/"+first+"/Lyrics", app("web", admin), nil); status != http.StatusNoContent {
		t.Fatalf("removal: %d", status)
	}
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound {
		t.Errorf("lyrics after the removal: %d", status)
	}
	l.setting(t, func(s *accounts.Settings) { s.Lyrics = true })
	if status, _, _ := l.lyricLines(t, "First Light"); status != http.StatusNotFound || l.count("First Light") != 0 {
		t.Errorf("removed, turned on: %d, asked %d times", status, l.count("First Light"))
	}
	if status, _ := l.uploadLyrics(t, admin, first, "first.lrc", lrc); status != http.StatusOK {
		t.Fatalf("upload after the removal: %d", status)
	}
	if status, _, lines := l.lyricLines(t, "First Light"); status != http.StatusOK || lines[0]["Text"] != "Uploaded line" {
		t.Errorf("lyrics uploaded after the removal: %d %v", status, lines)
	}
}
