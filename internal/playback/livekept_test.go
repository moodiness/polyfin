package playback

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
)

// keptSource answers a live stream with data at once, then holds its
// connection open sending nothing more: sent is closed once all of data
// was read.
type keptSource struct {
	data []byte
	once sync.Once
	sent chan struct{}
}

func (k *keptSource) Open(ctx context.Context, _, _ string, _ http.Header, _ bool) (*http.Response, error) {
	paused := pausedBody{ctx: ctx, reached: func() { k.once.Do(func() { close(k.sent) }) }}
	return answer(http.StatusOK, "video/mp2t", io.MultiReader(bytes.NewReader(k.data), paused)), nil
}

// pausedBody tells it was reached, then sends nothing until its request
// ends.
type pausedBody struct {
	ctx     context.Context
	reached func()
}

func (p pausedBody) Read([]byte) (int, error) {
	p.reached()
	<-p.ctx.Done()
	return 0, p.ctx.Err()
}

// The first live playlist of a channel whose feed kept a few seconds of
// its stream, as an analysis or a play prepared ahead leaves it, takes
// its lead from them: FFmpeg starts at the oldest keyframe kept, keeps
// what it probed, and cuts the first segments at once, where starting
// feedJoin back and dropping the probed second and a half left too few,
// the next ones waiting for keyframes the source had not sent.
func TestTheFirstLivePlaylistTakesItsLeadFromTheKeptStream(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	stream := filepath.Join(t.TempDir(), "kept.ts")
	// Five seconds at 1 MB a second, a keyframe every second: more than
	// feedJoin, less than the feed keeps.
	if output, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=5",
		"-c:v", "libx264", "-g", "25", "-keyint_min", "25", "-sc_threshold", "0", "-bf", "0", "-pix_fmt", "yuv420p",
		"-muxrate", "8M", "-f", "mpegts", stream).CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v %s", err, output)
	}
	data, err := os.ReadFile(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= feedJoin || len(data) > feedBuffer-feedSlack {
		t.Fatalf("the stream is %d bytes, want more than feedJoin and less than the feed keeps", len(data))
	}
	source := &keptSource{data: data, sent: make(chan struct{})}
	s := newService(t, source, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
	segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	s.segments = segments
	// The source's pause outlasts the test: the feed never ends as stalled,
	// which would end FFmpeg's run, whose playlist then answers at once.
	times := defaultLiveTimes
	times.stall = time.Hour
	s.feeds.times = times
	version := channelVersion(1, "kept.ts", accounts.ID{})

	// The analysis opens the feed, as a PlaybackInfo or a play prepared
	// ahead does; the source sends the whole stream, then nothing more.
	if _, err := s.AnalyzeLive(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	select {
	case <-source.sent:
	case <-time.After(30 * time.Second):
		t.Fatal("the source's stream was not read")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	started := time.Now()
	playlist, err := s.LivePlaylist(ctx, Remux{Version: version, Audio: -1, Format: hls.TS, MinSegments: 1}, func(name string) string { return name })
	t.Logf("the first playlist after %s", time.Since(started).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("the first playlist, the source holding its next bytes: %v", err)
	}
	if listed := strings.Count(string(playlist), "#EXTINF:"); listed < 2 {
		t.Errorf("the first playlist lists %d segments, want 2:\n%s", listed, playlist)
	}
}
