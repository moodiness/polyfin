package playback

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testspeech"
)

// An addon's subtitle shifted by 4 seconds, and one timed for 25 fps on a
// 23.976 fps version, come back in time, against a text track inside the
// file and against the audio; one already in time and one of another
// title are left as they are.
func TestAddonSubtitlesAreSyncedToTheVersionPlayed(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	t.Parallel()
	const length = 12 * time.Minute
	spoken := testspeech.Bursts(1, length)
	same := func(d time.Duration) time.Duration { return d }
	for _, reference := range []string{speechTrack, speechAudio} {
		t.Run(reference, func(t *testing.T) {
			t.Parallel()
			origin := &fileSource{data: testspeech.Version(t, ffmpeg, spoken, length, reference == speechTrack)}
			s := newService(t, origin, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(segments.Close)
			s.segments = segments
			version := library.Version{ID: accounts.ID{1}, URL: "http://files.example/version.mkv", Addon: "Files"}
			s.findSpeech(Background(t.Context()), version)
			if stored, ok := s.storedSpeech(t.Context(), version.ID); !ok || stored.kind != reference {
				t.Fatalf("the reference is %q (%v), want %q", stored.kind, ok, reference)
			}
			for i, tc := range []struct {
				name   string
				cues   []subtitles.Cue
				synced bool
			}{
				{"shifted by 4 seconds", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second }), true},
				{"timed for 25 fps", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return time.Duration(float64(d) * 24000 / 1001 / 25) }), true},
				{"in time", testspeech.Cues(spoken, same), false},
				{"of another title", testspeech.Cues(testspeech.Bursts(2, length), same), false},
			} {
				subtitle := accounts.ID{byte(10 + i)}
				timing, ok := s.SyncSubtitle(t.Context(), version.ID, subtitle, tc.cues)
				if !ok || timing.Changes() != tc.synced {
					t.Errorf("%s: timed %+v (%v)", tc.name, timing, ok)
					continue
				}
				for j, cue := range timing.Apply(tc.cues) {
					if gap := cue.Start - spoken[j].Start; tc.synced && gap.Abs() > 200*time.Millisecond {
						t.Errorf("%s: cue %d starts at %s, the speech at %s", tc.name, j, cue.Start, spoken[j].Start)
						break
					}
				}
				// The timing is kept: the next request gets the same.
				if again, _ := s.SyncSubtitle(t.Context(), version.ID, subtitle, nil); again != timing {
					t.Errorf("%s: timed %+v, then %+v", tc.name, timing, again)
				}
			}
		})
	}
}

// Within the read budget, an addon's file is looked at in stretches as
// long as the budget reads at its bitrate: shorter than 3 minutes, its
// late subtitle is still synced; under 30 seconds each, the version has
// no speech reference, and its subtitles are served as they are.
func TestStretchesShortenToTheReadBudget(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	t.Parallel()
	const length = 12 * time.Minute
	spoken := testspeech.Bursts(1, length)
	data := testspeech.Version(t, ffmpeg, spoken, length, false)
	late := testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second })
	for _, tc := range []struct {
		name    string
		stretch time.Duration
	}{{"shorter stretches", time.Minute}, {"under the floor", 25 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, &fileSource{data: data}, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(segments.Close)
			s.segments = segments
			// The budget reads three stretches of tc.stretch, and half a
			// second more, at the file's bitrate (see stretchWithin).
			perSecond := float64(len(data)) / length.Seconds()
			s.speech.budget = int64(3 * readOverhead * (readAhead + (tc.stretch+keyframeLead+time.Second/2).Seconds()*perSecond))
			version := library.Version{ID: accounts.ID{1}, URL: "http://files.example/version.mkv", Addon: "Files"}
			s.findSpeech(Background(t.Context()), version)
			stored, ok := s.storedSpeech(t.Context(), version.ID)
			timing, synced := s.SyncSubtitle(t.Context(), version.ID, accounts.ID{2}, late)
			if tc.stretch < minStretch {
				if !ok || stored.kind != speechNone || synced || timing.Changes() {
					t.Errorf("the reference is %q (%v), the late subtitle timed %+v (%v)", stored.kind, ok, timing, synced)
				}
				return
			}
			if !ok || stored.kind != speechAudio || len(stored.reference.Windows) != 3 {
				t.Fatalf("the reference is %q (%v), in %d windows", stored.kind, ok, len(stored.reference.Windows))
			}
			for _, window := range stored.reference.Windows {
				if got := window.End - window.Start; got != tc.stretch {
					t.Errorf("a window lasts %s, want %s", got, tc.stretch)
				}
			}
			for j, cue := range timing.Apply(late) {
				if gap := cue.Start - spoken[j].Start; !synced || gap.Abs() > 200*time.Millisecond {
					t.Fatalf("cue %d starts at %s, the speech at %s (timed %+v, %v)", j, cue.Start, spoken[j].Start, timing, synced)
				}
			}
		})
	}
}

// patterned is a file of size bytes whose bytes are their offset's last.
type patterned struct{ size int64 }

func (f patterned) ReadAt(p []byte, off int64) (int, error) {
	n := int(min(int64(len(p)), max(f.size-off, 0)))
	for i := range n {
		p[i] = byte(off + int64(i))
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// sentCounter counts what a server sends.
type sentCounter struct {
	http.ResponseWriter
	sent *atomic.Int64
}

func (w sentCounter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.sent.Add(int64(n))
	return n, err
}

// socketBuffer bounds the socket buffers of a fake addon's connections,
// so that what one holds unread when it closes is a little.
const socketBuffer = 64 << 10

// smallBuffers fetches from a local server over connections whose
// receive buffer is socketBuffer.
type smallBuffers struct{ client *http.Client }

func newSmallBuffers(t *testing.T) *smallBuffers {
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			err = conn.(*net.TCPConn).SetReadBuffer(socketBuffer)
		}
		return conn, err
	}}
	t.Cleanup(transport.CloseIdleConnections)
	return &smallBuffers{client: &http.Client{Transport: transport}}
}

func (o *smallBuffers) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header = header.Clone()
	return o.client.Do(request)
}

// What is read of an addon's file within a budget is what is fetched of
// it: FFmpeg's requests at its start, near its end and in its middle have
// the addon send no more than the budget, and allowance. A playback's
// reads still fetch ahead of what they read.
func TestReadsWithinABudgetFetchNoMoreThanIt(t *testing.T) {
	t.Parallel()
	const (
		size   = 256 << 20
		budget = 32 << 20
		// allowance is a block for each of the three requests, read past
		// what is served, and what each connection's buffers hold.
		allowance = 4 << 20
	)
	for _, tc := range []struct {
		name     string
		budgeted bool
	}{{"within a budget", true}, {"playback", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sent atomic.Int64
			addon := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(sentCounter{w, &sent}, r, "", time.Time{}, io.NewSectionReader(patterned{size}, 0, size))
			}))
			addon.Config.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
				_ = conn.(*net.TCPConn).SetWriteBuffer(socketBuffer)
				return ctx
			}
			addon.Start()
			t.Cleanup(addon.Close)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			sources, err := source.New(t.TempDir(), func() int64 { return 1 << 30 }, newSmallBuffers(t), logger)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sources.Close() })
			loopback, err := newLoopback()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loopback.Close() })
			src := sources.Open(accounts.ID{1}, source.Location{URL: addon.URL + "/version.mkv"}, nil)
			defer src.Release()
			register := loopback.register
			if tc.budgeted {
				register = func(src *source.Source) (string, func()) {
					return loopback.registerWithin(src, &readBudget{limit: budget})
				}
			}
			input, release := register(src)
			defer release()
			ffmpeg := newSmallBuffers(t).client
			// read reads n bytes from off, all that is served for n < 0,
			// and returns what the addon sent, the request still open,
			// once that reached enough or stopped growing.
			read := func(off, n, enough int64) int64 {
				t.Helper()
				request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, input, nil)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Range", fmt.Sprintf("bytes=%d-", off))
				response, err := ffmpeg.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusPartialContent {
					t.Fatalf("from %d: HTTP %d", off, response.StatusCode)
				}
				if n < 0 {
					_, _ = io.Copy(io.Discard, response.Body)
				} else if _, err := io.CopyN(io.Discard, response.Body, n); err != nil {
					t.Fatalf("from %d: %v", off, err)
				}
				return settled(&sent, enough)
			}
			if !tc.budgeted {
				// The window read ahead of a playback is 32 blocks.
				if got := read(0, 1<<20, 32<<20); got < 32<<20 {
					t.Errorf("the addon sent %d MiB for 1 MiB read: nothing ahead", got>>20)
				}
				return
			}
			read(0, 1<<20, math.MaxInt64)
			read(size-(2<<20), 1<<20, math.MaxInt64)
			if got := read(size/2, -1, math.MaxInt64); got > budget+allowance {
				t.Errorf("the addon sent %d MiB, over the budget of %d MiB", got>>20, budget>>20)
			}
		})
	}
}

// settled is what sent counts once it reached enough, or stopped growing
// for a second.
func settled(sent *atomic.Int64, enough int64) int64 {
	last := sent.Load()
	for still, deadline := time.Now(), time.Now().Add(10*time.Second); last < enough && time.Since(still) < time.Second && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		if now := sent.Load(); now != last {
			last, still = now, time.Now()
		}
	}
	return last
}
