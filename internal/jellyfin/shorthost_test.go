package jellyfin

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// shortHost serves data with ranges, each answer telling the whole file,
// but cut before the byte at cut: every answer when cuts is negative, a
// host whose file is shorter than it tells, else the first cuts answers
// reaching it, a host closing a connection early now and then. It counts
// the requests, and the answers it cut.
func shortHost(t *testing.T, data []byte, cut int, cuts int32) (server *httptest.Server, requests, made *atomic.Int32) {
	t.Helper()
	requests, made = new(atomic.Int32), new(atomic.Int32)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		start := 0
		if value, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
			start, _ = strconv.Atoi(strings.Split(value, "-")[0])
		}
		writer := &shortWriter{ResponseWriter: w, left: len(data), made: made}
		if start <= cut || cuts < 0 {
			if cuts < 0 || made.Load() < cuts {
				writer.left = max(cut-start, 0)
			}
		}
		http.ServeContent(writer, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return server, requests, made
}

// shortWriter writes left bytes of an answer, then closes its connection,
// counting it in made.
type shortWriter struct {
	http.ResponseWriter
	left int
	made *atomic.Int32
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) <= w.left {
		w.left -= len(p)
		return w.ResponseWriter.Write(p)
	}
	_, _ = w.ResponseWriter.Write(p[:w.left])
	w.ResponseWriter.(http.Flusher).Flush()
	w.made.Add(1)
	panic(http.ErrAbortHandler)
}

// middleOfMedia is the offset halfway through the media data of an MP4
// file.
func middleOfMedia(t *testing.T, clip []byte) int {
	t.Helper()
	for at := 0; at+8 <= len(clip); at += int(binary.BigEndian.Uint32(clip[at:])) {
		if string(clip[at+4:at+8]) == "mdat" {
			return at + 8 + (len(clip)-at-8)/2
		}
	}
	t.Fatal("the clip has no media data")
	return 0
}

// shortHostPlay is a server listing one movie, its stream on host, data
// of the size its addon tells, with a member signed in, and a count of
// the FFmpeg runs it started for that member's plays.
func shortHostPlay(t *testing.T, host *httptest.Server, data []byte) (failoverSetup, func() int) {
	t.Helper()
	_, ffprobe := failoverFFmpeg(t)
	p := failoverServer(t, ffprobe, failoverAddon(t, "catalog", []stremio.Stream{{Name: "Short", URL: host.URL + "/short.mp4",
		BehaviorHints: stremio.StreamBehavior{Filename: "Failover.2026.1080p.Short.mp4", VideoSize: stremio.Number64(len(data))}}}))
	p.warnings.all.Store(true)
	return p, func() int {
		n := 0
		for _, message := range p.warnings.logged() {
			if message == "Started a remux" {
				n++
			}
		}
		return n
	}
}

// playInOrder asks for each segment once, in order, as a player does,
// until one is not served, and returns the statuses.
func playInOrder(t *testing.T, uris []string) []int {
	t.Helper()
	var statuses []int
	for _, uri := range uris {
		status, _, _ := fetchText(t, uri)
		statuses = append(statuses, status)
		if status != http.StatusOK {
			break
		}
	}
	return statuses
}

// A movie whose host tells the whole file's size, then closes every
// connection at the same byte halfway through, plays up to there, then
// ends: no segment answers that the server is busy, the segment past the
// end answers that it does not exist, and the play jellyfin-web asks for
// next lists the segments up to there only. The host is asked a few times
// at the cut, and nothing more once it is known.
func TestAMovieWhoseHostSendsLessThanItTellsPlaysUpToThereThenEnds(t *testing.T) {
	t.Parallel()
	ffmpeg, _ := failoverFFmpeg(t)
	clip := failoverClip(t, ffmpeg, false, 48)
	host, requests, _ := shortHost(t, clip, middleOfMedia(t, clip), -1)
	p, runs := shortHostPlay(t, host, clip)

	started := time.Now()
	uris, seconds, _ := openPlay(t, p.testServer, p.token, p.movie, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
	statuses := playInOrder(t, uris)
	t.Logf("%d segments: %v, %d requests of the host, in %v", len(uris), statuses, requests.Load(), time.Since(started).Round(time.Millisecond))
	end := len(statuses) - 1
	if statuses[end] != http.StatusNotFound || end < 1 || end+1 >= len(uris) {
		t.Fatalf("the play answered %v of %d segments", statuses, len(uris))
	}
	// 4 tries of the connection reaching the cut, as many of one more
	// reading ahead past it, if any, and the analysis's.
	asked := requests.Load()
	if asked > 12 {
		t.Errorf("the host was asked %d times", asked)
	}
	before := runs()
	if status, _, _ := fetchText(t, uris[end+1]); status != http.StatusNotFound {
		t.Errorf("a segment past the end: %d", status)
	}
	if got := runs(); got != before {
		t.Errorf("FFmpeg ran %d times for a segment past the end", got-before)
	}
	if found := p.warnings.attrsOf("A version ends early: it plays up to a segment before its last"); len(found) != 1 ||
		found[0]["segment"] != strconv.Itoa(end) {
		t.Errorf("the end told: %v", found)
	}
	// FFmpeg ended cleanly where the source's bytes did, once.
	if found := p.warnings.attrsOf("A remux failed"); len(found) != 0 || runs() != 1 {
		t.Errorf("FFmpeg ran %d times, failing %v", runs(), found)
	}
	if found := p.warnings.attrsOf("A source's host sends less than it tells: the file is read up to where it stops"); len(found) != 1 {
		t.Errorf("the cut told: %v", found)
	}

	// jellyfin-web plays on, after the error, from a little before where
	// the play stopped, with the video converted.
	at := 0.0
	for _, d := range seconds[:end] {
		at += d
	}
	retry, _, ends := openPlay(t, p.testServer, p.token, p.movie, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false,
		"EnableDirectStream": false, "AllowVideoStreamCopy": false, "StartTimeTicks": int64((at - 3) * 10_000_000)})
	if len(retry) != end || !ends {
		t.Fatalf("the playlist after the end lists %d segments, ending %v, want %d", len(retry), ends, end)
	}
	for n, uri := range retry {
		if status, _, _ := fetchText(t, uri); status != http.StatusOK {
			t.Errorf("segment %d of the playlist ending early: %d", n, status)
		}
	}
	if got := requests.Load(); got != asked {
		t.Errorf("the host was asked %d times more once the end was known", got-asked)
	}
}

// A movie whose host closes a connection early once, then serves the
// rest, plays whole: a passing cut is no end.
func TestAMovieWhoseHostCutsOnceIsPlayedWhole(t *testing.T) {
	t.Parallel()
	ffmpeg, _ := failoverFFmpeg(t)
	clip := failoverClip(t, ffmpeg, false, 48)
	host, requests, made := shortHost(t, clip, middleOfMedia(t, clip), 1)
	p, _ := shortHostPlay(t, host, clip)
	uris, _, ends := openPlay(t, p.testServer, p.token, p.movie, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
	statuses := playInOrder(t, uris)
	t.Logf("%d segments: %v, %d requests of the host, %d cut", len(uris), statuses, requests.Load(), made.Load())
	if made.Load() != 1 {
		t.Errorf("the host cut %d answers, want 1", made.Load())
	}
	if len(statuses) != len(uris) || statuses[len(statuses)-1] != http.StatusOK || !ends {
		t.Fatalf("the play answered %v of %d segments", statuses, len(uris))
	}
	if found := p.warnings.attrsOf("A version ends early: it plays up to a segment before its last"); len(found) != 0 {
		t.Errorf("an end was told: %v", found)
	}
	if found := p.warnings.attrsOf("A source's host sends less than it tells: the file is read up to where it stops"); len(found) != 0 {
		t.Errorf("a cut was taken for the end: %v", found)
	}
}
