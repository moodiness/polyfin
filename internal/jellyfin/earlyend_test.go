package jellyfin

import (
	"encoding/binary"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A movie whose file ends early, cut short in the middle of its media
// data, plays up to where it ends: the segment past it is answered that it
// does not exist, without FFmpeg. The play jellyfin-web asks for next,
// from a little before there with its video converted, lists the segments
// up to there only, and each of them plays.
func TestAMovieWhoseFileEndsEarlyPlaysUpToThereThenEnds(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := failoverFFmpeg(t)
	clip := failoverClip(t, ffmpeg, false, 48)
	mdat := 0
	for at := 0; at+8 <= len(clip); at += int(binary.BigEndian.Uint32(clip[at:])) {
		if string(clip[at+4:at+8]) == "mdat" {
			mdat = at + 8
			break
		}
	}
	if mdat == 0 {
		t.Fatal("the clip has no media data")
	}
	cut := clip[:mdat+(len(clip)-mdat)/2]

	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, o.Logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	s.warnings.all.Store(true)
	s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), failoverAddon(t, "catalog", nil), false); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Failover (2026)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Failover (2026)", "Failover (2026).mp4"), cut, 0o644); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Failover (2026)/Failover (2026).mp4", "tt7160"); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	runs := func() int {
		n := 0
		for _, message := range s.warnings.logged() {
			if message == "Started a remux" {
				n++
			}
		}
		return n
	}

	open := func(more map[string]any) ([]string, []float64, bool) {
		t.Helper()
		return openPlay(t, s, token, movie, more)
	}

	// The first play copies the video, and goes as far as the file does.
	uris, seconds, _ := open(map[string]any{"EnableDirectPlay": false})
	last := -1
	for n, uri := range uris {
		status, _, _ := fetchText(t, uri)
		if status != http.StatusOK {
			if status != http.StatusNotFound {
				t.Fatalf("segment %d of %d: %d", n, len(uris), status)
			}
			break
		}
		last = n
	}
	if last < 1 || last+1 >= len(uris) {
		t.Fatalf("the play ended after segment %d of %d", last, len(uris))
	}
	end := last + 1
	before := runs()
	if status, _, _ := fetchText(t, uris[end+1]); status != http.StatusNotFound {
		t.Errorf("a segment past the end: %d", status)
	}
	if got := runs(); got != before {
		t.Errorf("FFmpeg ran %d times for a segment past the end", got-before)
	}
	if found := s.warnings.attrsOf("A version ends early: it plays up to a segment before its last"); len(found) != 1 ||
		found[0]["segment"] != strconv.Itoa(end) || found[0]["of"] != strconv.Itoa(len(uris)) {
		t.Errorf("the end told: %v", found)
	}

	// jellyfin-web plays on, after the error, from a little before where
	// the play stopped, with the video converted.
	at := 0.0
	for _, d := range seconds[:end] {
		at += d
	}
	retry, _, ends := open(map[string]any{"EnableDirectPlay": false, "EnableDirectStream": false, "AllowVideoStreamCopy": false,
		"StartTimeTicks": int64((at - 3) * 10_000_000)})
	if len(retry) != end || !ends {
		t.Fatalf("the playlist after the end lists %d segments, ending %v, want %d", len(retry), ends, end)
	}
	for n, uri := range retry {
		if status, _, _ := fetchText(t, uri); status != http.StatusOK {
			t.Errorf("segment %d of the playlist ending early: %d", n, status)
		}
	}
	before = runs()
	past := strings.Replace(retry[end-1], "/"+strconv.Itoa(end-1)+".", "/"+strconv.Itoa(end)+".", 1)
	if status, _, _ := fetchText(t, past); status != http.StatusNotFound {
		t.Errorf("the converted segment past the end: %d", status)
	}
	if got := runs(); got != before {
		t.Errorf("FFmpeg ran %d times for a converted segment past the end", got-before)
	}
}

// openPlay asks PlaybackInfo for movie with more, as jellyfin-web in
// Chrome does, then the master and media playlists and the initialization
// segment, and returns the segments' addresses, how long each is, and
// whether the playlist ends.
func openPlay(t *testing.T, s testServer, token, movie string, more map[string]any) (uris []string, seconds []float64, ends bool) {
	t.Helper()
	answer := s.ask(t, token, movie, recordedProfile(t, "jellyfin-web-chrome"), more)
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	base, query, _ := strings.Cut(s.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, master := fetchText(t, base+"master.m3u8?"+query)
	var media string
	for line := range strings.Lines(master) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			media = line
			break
		}
	}
	_, _, playlist := fetchText(t, base+media)
	for line := range strings.Lines(playlist) {
		line = strings.TrimSpace(line)
		if duration, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			value, err := strconv.ParseFloat(strings.Split(duration, ",")[0], 64)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			seconds = append(seconds, value)
		}
		if init, ok := strings.CutPrefix(line, `#EXT-X-MAP:URI="`); ok {
			if status, _, _ := fetchText(t, base+strings.TrimSuffix(init, `"`)); status != http.StatusOK {
				t.Fatalf("the initialization segment: %d", status)
			}
		}
	}
	relative, _ := segmentsOf(playlist)
	for _, uri := range relative {
		uris = append(uris, base+uri)
	}
	return uris, seconds, strings.Contains(playlist, "#EXT-X-ENDLIST")
}
