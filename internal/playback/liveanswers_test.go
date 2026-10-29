package playback

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
)

// liveStream encodes n MPEG-TS segments of a second, H.264 and AAC, with
// FFmpeg.
func liveStream(t *testing.T, n int) (ffmpeg string, segments [][]byte) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	command := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25:duration="+strconv.Itoa(n),
		"-f", "lavfi", "-i", "sine=duration="+strconv.Itoa(n), "-c:v", "libx264", "-preset", "ultrafast", "-g", "25", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-f", "hls", "-hls_time", "1", "-hls_list_size", "0", "-hls_segment_filename", filepath.Join(dir, "s%d.ts"),
		filepath.Join(dir, "all.m3u8"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v %s", err, output)
	}
	for i := range n {
		data, err := os.ReadFile(filepath.Join(dir, "s"+strconv.Itoa(i)+".ts"))
		if err != nil {
			t.Fatal(err)
		}
		segments = append(segments, data)
	}
	return ffmpeg, segments
}

// slidingChannel is a provider's live HLS channel: its playlist lists the
// last segments of a second each as they come, from when it was made, of
// a stream encoded once, each named with the provider's token. odd, when
// set, answers a segment in place of its media, which odds counts;
// frozen, once set, keeps the playlist listing the segments it lists
// then.
type slidingChannel struct {
	start    time.Time
	segments [][]byte

	mu     sync.Mutex
	odd    func(n int) *http.Response
	odds   int
	frozen int
}

func (c *slidingChannel) set(change func(c *slidingChannel)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	change(c)
}

func (c *slidingChannel) Open(_ context.Context, _, target string, _ http.Header, _ bool) (*http.Response, error) {
	address, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	frozen := c.frozen
	c.mu.Unlock()
	name := path.Base(address.Path)
	if name == "index.m3u8" {
		now := min(int(time.Since(c.start)/time.Second), len(c.segments)-4)
		if frozen >= 0 {
			now = frozen
		}
		var playlist strings.Builder
		fmt.Fprintf(&playlist, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n", now)
		for n := now; n < now+4; n++ {
			fmt.Fprintf(&playlist, "#EXTINF:1.000,\ns%d.ts?token=0123-token\n", n)
		}
		return sized(http.StatusOK, "application/vnd.apple.mpegurl", []byte(playlist.String())), nil
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "s"), ".ts"))
	if err != nil || n < 0 || n >= len(c.segments) {
		return sized(http.StatusNotFound, "text/plain", []byte("no such file")), nil
	}
	c.mu.Lock()
	var answer *http.Response
	if c.odd != nil {
		if answer = c.odd(n); answer != nil {
			c.odds++
		}
	}
	c.mu.Unlock()
	if answer != nil {
		return answer, nil
	}
	return sized(http.StatusOK, "video/mp2t", c.segments[n]), nil
}

// logText keeps every record a logger handles, as text.
type logText struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (l *logText) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

func (l *logText) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

// line is the first line of the log whose message is message, "" for
// none.
func (l *logText) line(message string) string {
	for line := range strings.Lines(l.String()) {
		if strings.Contains(line, "msg=\""+message+"\"") {
			return line
		}
	}
	return ""
}

// A live conversion whose source stops answering media during a play, as
// when its segments answer an error or a page, makes no segment more:
// FFmpeg skips what it cannot read and waits on. The encoding tells it as
// a stall, which says which file answered what, by its file name and
// never its address nor its body, and names the channel; one whose source
// merely stops sending, after a page it answered and media since, tells
// of no page. The log holds no address nor token.
func TestALiveConversionTellsWhatItsSourceAnswered(t *testing.T) {
	ffmpeg, segments := liveStream(t, 60)
	page := []byte("<!DOCTYPE html><html><body>Access denied for secret-token</body></html>")
	for _, tc := range []struct {
		name string
		// before is what the channel answers once the play started, after
		// the segments that answered when it started; breakdown what it
		// answers next.
		before, breakdown func(c *slidingChannel)
		tells             string
		tellsNot          string
	}{
		{"segments gone", nil, func(c *slidingChannel) {
			c.odd = func(int) *http.Response { return sized(http.StatusNotFound, "text/plain", []byte("gone")) }
		}, ".ts answered text (HTTP 404, text/plain, 4 bytes)", ""},
		{"pages for segments", nil, func(c *slidingChannel) {
			c.odd = func(int) *http.Response { return sized(http.StatusOK, "text/html", page) }
		}, ".ts answered a web page (HTTP 200, text/html", ""},
		{"a page long before", func(c *slidingChannel) {
			first := -1
			c.odd = func(n int) *http.Response {
				if first < 0 {
					first = n
				}
				if n == first {
					return sized(http.StatusOK, "text/html", page)
				}
				return nil
			}
		}, func(c *slidingChannel) {
			c.frozen = min(int(time.Since(c.start)/time.Second), len(c.segments)-4)
		}, "", "answered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channel := &slidingChannel{start: time.Now(), segments: segments, frozen: -1}
			s := newService(t, channel, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			log := &logText{}
			logger := slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
			s.logger = logger
			manager, err := hls.NewManager(ffmpeg, t.TempDir(), logger)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Close)
			s.segments = manager
			remux := Remux{Session: "session", User: accounts.ID{1}, Format: hls.TS, Audio: 1, Channel: "Zeb Sport",
				Version: library.Version{ID: accounts.ID{7}, Name: "Zeb Sport · Live", URL: "http://10.0.0.1/live/0123-token/index.m3u8"}}
			uri := func(name string) string { return name }
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			if _, err := s.LivePlaylist(ctx, remux, uri); err != nil {
				t.Fatalf("the first playlist: %v", err)
			}
			if tc.before != nil {
				channel.set(tc.before)
				// FFmpeg makes two segments more past the odd one, read
				// from the media that came after it, as the player reloads.
				seen, made := "", -1
				for made < 2 {
					data, err := s.LivePlaylist(ctx, remux, uri)
					if err != nil {
						t.Fatalf("a playlist before the breakdown: %v", err)
					}
					if last := lastListed(data); last != seen {
						seen, made = last, made+1
					}
					time.Sleep(time.Second)
				}
				channel.mu.Lock()
				odds := channel.odds
				channel.mu.Unlock()
				if odds == 0 {
					t.Fatal("no segment answered a page before the breakdown")
				}
			}
			channel.set(tc.breakdown)
			// The player reloads the playlist every second, as players do.
			for log.line("A live encoding stalled") == "" {
				if ctx.Err() != nil {
					t.Fatalf("no stall told:\n%s", log)
				}
				_, _ = s.LivePlaylist(ctx, remux, uri)
				time.Sleep(time.Second)
			}
			stalled := log.line("A live encoding stalled")
			t.Log(stalled)
			if !strings.Contains(stalled, `channel="Zeb Sport"`) || !strings.Contains(stalled, hls.ErrLiveStalled.Error()) {
				t.Errorf("the stall does not name the channel: %s", stalled)
			}
			if tc.tells != "" && !strings.Contains(stalled, tc.tells) {
				t.Errorf("the stall does not tell %q: %s", tc.tells, stalled)
			}
			if tc.tellsNot != "" && strings.Contains(stalled, tc.tellsNot) {
				t.Errorf("the stall tells %q: %s", tc.tellsNot, stalled)
			}
			// It is read again, saying why.
			for log.line("A live stream is read again") == "" && ctx.Err() == nil {
				_, _ = s.LivePlaylist(ctx, remux, uri)
				time.Sleep(time.Second)
			}
			if again := log.line("A live stream is read again"); !strings.Contains(again, `channel="Zeb Sport"`) ||
				!strings.Contains(again, hls.ErrLiveStalled.Error()) {
				t.Errorf("read again: %s", again)
			}
			for _, secret := range []string{"10.0.0.1", "0123-token", "token=", "127.0.0.1", "Access denied", "secret-token"} {
				if strings.Contains(log.String(), secret) {
					t.Errorf("the log tells %q", secret)
				}
			}
		})
	}
}

// lastListed is the last segment a live playlist lists, "" for none.
func lastListed(playlist []byte) string {
	last := ""
	for line := range strings.Lines(string(playlist)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			last = line
		}
	}
	return last
}
