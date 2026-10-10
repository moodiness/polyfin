package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// musicRig is an Eclipse addon whose catalog lists albums of four tracks,
// each answered after latency. The streams of most albums are described;
// those of album "bare" are only a URL, the tracks of album "listed" come
// with a streamURL and their format, and the links of album "short"
// expire 30 seconds after they are given. It counts the stream requests
// of each track and the album pages read at once; once holdAlbums is
// called, album pages are held until as many are read at once as it asks.
type musicRig struct {
	url     string
	latency time.Duration

	mu           sync.Mutex
	streams      map[string]int
	albumsNow    int
	albumsMost   int
	albumsAtOnce int
	albumsHeld   chan struct{}
	albumsGiveUp <-chan struct{}
}

func newMusicRig(t *testing.T, mp3 string, albums int, latency time.Duration) *musicRig {
	t.Helper()
	rig := &musicRig{latency: latency, streams: map[string]int{}}
	var server *httptest.Server
	names := []string{"bare", "listed", "short"}
	for i := range albums {
		names = append(names, fmt.Sprintf("a%02d", i))
	}
	album := func(id string) map[string]any {
		var tracks []any
		for n := 1; n <= 4; n++ {
			track := map[string]any{"id": fmt.Sprintf("%s-%d", id, n), "title": fmt.Sprintf("Song %d of %s", n, id), "artist": "Rig", "duration": 10}
			switch id {
			case "listed":
				track["format"], track["streamURL"] = "mp3", server.URL+"/tone.mp3"
			case "bare":
			default:
				track["format"] = "mp3"
			}
			tracks = append(tracks, track)
		}
		return map[string]any{"id": id, "title": "Album " + id, "artist": "Rig", "tracks": tracks}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		path := r.URL.Path
		if path == "/tone.mp3" {
			http.ServeFile(w, r, mp3)
			return
		}
		if path != "/manifest.json" {
			time.Sleep(rig.latency)
		}
		switch {
		case path == "/manifest.json":
			answer(map[string]any{"id": "org.example.rig", "name": "Rig", "version": "1.0.0", "resources": []string{"search", "stream", "catalog"},
				"types": []string{"track", "album"}, "catalogs": []any{map[string]string{"id": "albums", "type": "album", "name": "Rig Albums"}}})
		case path == "/catalog/albums":
			var items []any
			for _, name := range names {
				items = append(items, map[string]any{"id": name, "title": "Album " + name, "artist": "Rig"})
			}
			answer(map[string]any{"items": items})
		case strings.HasPrefix(path, "/album/"):
			rig.mu.Lock()
			rig.albumsNow++
			rig.albumsMost = max(rig.albumsMost, rig.albumsNow)
			held, giveUp := rig.albumsHeld, rig.albumsGiveUp
			if held != nil && rig.albumsNow == rig.albumsAtOnce {
				close(held)
				rig.albumsHeld = nil
			}
			rig.mu.Unlock()
			if held != nil {
				select {
				case <-held:
				case <-giveUp:
				}
			}
			// Held a little longer, so that a page read past the bound
			// would show.
			time.Sleep(rig.latency)
			rig.mu.Lock()
			rig.albumsNow--
			rig.mu.Unlock()
			answer(album(strings.TrimPrefix(path, "/album/")))
		case path == "/search":
			answer(map[string]any{})
		case strings.HasPrefix(path, "/stream/"):
			id := strings.TrimPrefix(path, "/stream/")
			rig.mu.Lock()
			rig.streams[id]++
			rig.mu.Unlock()
			reply := map[string]any{"url": server.URL + "/tone.mp3?track=" + id}
			switch {
			case strings.HasPrefix(id, "bare-"):
			case strings.HasPrefix(id, "short-"):
				reply["codec"], reply["container"], reply["expiresAt"] = "mp3", "mp3", time.Now().Add(30*time.Second).Unix()
			default:
				reply["codec"], reply["container"] = "mp3", "mp3"
			}
			answer(reply)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	rig.url = server.URL + "/manifest.json"
	return rig
}

func (rig *musicRig) streamRequests(track string) int {
	rig.mu.Lock()
	defer rig.mu.Unlock()
	return rig.streams[track]
}

// holdAlbums holds the album pages read from now on until n of them are
// read at once, whatever the machine's speed, or until the test gives up.
func (rig *musicRig) holdAlbums(t *testing.T, n int) {
	held, giveUp := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(giveUp)
	rig.mu.Lock()
	defer rig.mu.Unlock()
	rig.albumsAtOnce, rig.albumsHeld, rig.albumsGiveUp = n, make(chan struct{}), held.Done()
}

// mp3Tone generates a ten-second MP3 tone, with the ffprobe beside the
// test's FFmpeg.
func mp3Tone(t *testing.T) (ffprobe, path string) {
	t.Helper()
	ffmpeg, _ := tone(t)
	path = filepath.Join(t.TempDir(), "tone.mp3")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=10",
		"-ac", "2", "-c:a", "libmp3lame", "-b:a", "128k", path).CombinedOutput(); err != nil {
		t.Fatalf("tone: %v %s", err, out)
	}
	return filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), path
}

// albumSongs lists an album's songs by name, from its library.
func albumSongs(t *testing.T, s testServer, token, libraryID, album string) []BaseItemDto {
	t.Helper()
	var albums QueryResult
	s.get(t, "/Items?ParentId="+libraryID+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	for _, a := range albums.Items {
		if a.Name == "Album "+album {
			var songs QueryResult
			s.get(t, "/Items?ParentId="+a.Id, token, &songs)
			return songs.Items
		}
	}
	t.Fatalf("no album %s", album)
	return nil
}

// mp3Profile plays MP3 as it is, and nothing else.
var mp3Profile = map[string]any{"MaxStreamingBitrate": 140000000,
	"DirectPlayProfiles": []any{map[string]string{"Type": "Audio", "Container": "mp3"}}}

func playbackInfo(t *testing.T, s testServer, token, item string) (playbackInfoResponse, string) {
	t.Helper()
	var info playbackInfoResponse
	status, body := s.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("web", token), map[string]any{"DeviceProfile": mp3Profile})
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil {
		t.Fatalf("playback info: %d %s", status, body)
	}
	return info, string(body)
}

func TestTracksTheAddonDoesNotDescribePlay(t *testing.T) {
	ffprobe, mp3 := mp3Tone(t)
	s := newProbingServer(t, 10, ffprobe)
	rig := newMusicRig(t, mp3, 1, 0)
	token, _, views := listening(t, s, rig.url)
	listener := mustUser(t, s, "listener")
	for _, album := range []string{"bare", "listed"} {
		song := albumSongs(t, s, token, views["Rig Albums"], album)[0].Id
		info, body := playbackInfo(t, s, token, song)
		if len(info.MediaSources) != 1 || !info.MediaSources[0].SupportsDirectPlay || info.MediaSources[0].Container != "mp3" {
			t.Fatalf("%s: %s", album, body)
		}
		if response, _ := s.fetch(t, info.MediaSources[0].Path); response.StatusCode != http.StatusOK && response.StatusCode != http.StatusFound {
			t.Errorf("%s: the track's bytes: %d", album, response.StatusCode)
		}
		version, err := s.library.Version(t.Context(), listener, mustID(t, song), mustID(t, song))
		if err != nil {
			t.Fatal(err)
		}
		_, analyzed := s.handler.Playback.Analyzed(t.Context(), version.ID)
		switch album {
		case "bare":
			// Analyzed as an audio file; the same file standing in for a
			// video is still refused.
			if !analyzed {
				t.Error("an undescribed track was not analyzed")
			}
			video := version
			video.ID = accounts.ID{9}
			if _, err := s.handler.Playback.Analyze(t.Context(), video); !errors.Is(err, media.ErrNotMedia) {
				t.Errorf("an audio file analyzed as a video: %v", err)
			}
		case "listed":
			// Its format describes it: no probe.
			if analyzed {
				t.Error("a track of a known format was probed")
			}
		}
	}
}

func TestTheNextTrackIsResolvedAhead(t *testing.T) {
	ffprobe, mp3 := mp3Tone(t)
	s := newProbingServer(t, 10, ffprobe)
	s.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	rig := newMusicRig(t, mp3, 1, 0)
	token, _, views := listening(t, s, rig.url)
	songs := albumSongs(t, s, token, views["Rig Albums"], "a00")
	playbackInfo(t, s, token, songs[0].Id)
	waitFor := func(track string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); rig.streamRequests(track) == 0; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s was not resolved ahead", track)
			}
		}
	}
	// The queue the player reports plays the third track next, not the
	// second.
	queue := []map[string]string{{"Id": songs[0].Id, "PlaylistItemId": "q0"}, {"Id": songs[2].Id, "PlaylistItemId": "q1"}}
	s.call(http.MethodPost, "/Sessions/Playing", app("web", token), map[string]any{"ItemId": songs[0].Id, "PositionTicks": 0,
		"PlaylistItemId": "q0", "NowPlayingQueue": queue})
	waitFor("a00-3")
	if rig.streamRequests("a00-2") != 0 {
		t.Error("a track out of the queue was resolved")
	}
	playbackInfo(t, s, token, songs[2].Id)
	if n := rig.streamRequests("a00-3"); n != 1 {
		t.Errorf("the next track was resolved %d times", n)
	}
	// Without a queue, the next track is the album's.
	s.call(http.MethodPost, "/Sessions/Playing", app("web", token), map[string]any{"ItemId": songs[2].Id, "PositionTicks": 0})
	waitFor("a00-4")
}

func TestReportedQueuesNameTheNextTrack(t *testing.T) {
	a, b, c := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	queue := []queueItem{{a.String(), "q0"}, {b.String(), "q1"}, {a.String(), "q2"}, {c.String(), "q3"}}
	for _, test := range []struct {
		name   string
		report playbackReport
		want   accounts.ID
	}{
		{"by entry", playbackReport{ItemId: a.String(), PlaylistItemId: "q2", NowPlayingQueue: queue}, c},
		{"by item", playbackReport{ItemId: a.String(), NowPlayingQueue: queue}, b},
		{"at the end", playbackReport{ItemId: c.String(), NowPlayingQueue: queue}, accounts.ID{}},
		{"repeating the queue", playbackReport{ItemId: c.String(), RepeatMode: "RepeatAll", NowPlayingQueue: queue}, a},
		{"repeating the track", playbackReport{ItemId: a.String(), RepeatMode: "RepeatOne", NowPlayingQueue: queue}, accounts.ID{}},
		{"not queued", playbackReport{ItemId: accounts.ID{4}.String(), NowPlayingQueue: queue}, accounts.ID{}},
		{"no queue", playbackReport{ItemId: a.String()}, accounts.ID{}},
	} {
		if got := test.report.next(); got != test.want {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
	if state := (playbackReport{ItemId: a.String()}).state(); state.Queued {
		t.Error("a report without a queue counts as queued")
	}
	if state := (playbackReport{ItemId: c.String(), NowPlayingQueue: queue}).state(); !state.Queued || state.Next != (accounts.ID{}) {
		t.Errorf("the last track of a queue: %+v", state)
	}
}

func TestShortLivedLinksAreAskedOncePerPlayAndRelayed(t *testing.T) {
	ffprobe, mp3 := mp3Tone(t)
	s := newProbingServer(t, 10, ffprobe)
	rig := newMusicRig(t, mp3, 1, 0)
	token, _, views := listening(t, s, rig.url)
	song := albumSongs(t, s, token, views["Rig Albums"], "short")[0].Id
	info, body := playbackInfo(t, s, token, song)
	if len(info.MediaSources) != 1 {
		t.Fatalf("playback info: %s", body)
	}
	if response, _ := s.fetch(t, info.MediaSources[0].Path); response.StatusCode != http.StatusOK {
		t.Errorf("the track's bytes: %d", response.StatusCode)
	}
	if n := rig.streamRequests("short-1"); n != 1 {
		t.Errorf("a link expiring in 30 s was asked for %d times in one play", n)
	}
	// A link that expires before the track ends goes through Polyfin,
	// which renews it; one that lasts longer may be redirected to.
	request := httptest.NewRequest(http.MethodGet, "/Audio/x/stream", nil)
	if !relayTrack(request, library.Version{Runtime: 4 * time.Minute, Expires: time.Now().Add(time.Minute)}) {
		t.Error("a link expiring before the track ends is not relayed")
	}
	if relayTrack(request, library.Version{Runtime: 4 * time.Minute, Expires: time.Now().Add(time.Hour)}) ||
		relayTrack(request, library.Version{Runtime: 4 * time.Minute}) {
		t.Error("a lasting link is relayed")
	}
}

func TestTheSongsTabReadsEightAlbumsAtOnce(t *testing.T) {
	s := newTestServer(t, 10)
	rig := newMusicRig(t, "", 30, 20*time.Millisecond)
	token, _, views := listening(t, s, rig.url)
	rig.holdAlbums(t, 8)
	var songs QueryResult
	s.get(t, "/Items?ParentId="+views["Rig Albums"]+"&IncludeItemTypes=Audio&Recursive=true&Limit=500", token, &songs)
	if songs.TotalRecordCount != 33*4 {
		t.Errorf("%d songs, want %d", songs.TotalRecordCount, 33*4)
	}
	rig.mu.Lock()
	defer rig.mu.Unlock()
	if rig.albumsMost != 8 {
		t.Errorf("%d album pages read at once, want 8", rig.albumsMost)
	}
}
