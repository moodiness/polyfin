package playback

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// tsChunk is 16 MPEG-TS packets.
var tsChunk = func() []byte {
	packet := make([]byte, tsPacket)
	packet[0] = 0x47
	return bytes.Repeat(packet, 16)
}()

// liveSource answers like an IPTV provider: each address behaves as its
// path says, and it counts the connections open.
type liveSource struct {
	// interval paces the bytes of TS streams; zero sends them at once.
	interval time.Duration
	// refuse answers 429 to so many opens first.
	refuse int

	mu     sync.Mutex
	open   int
	opened int
	most   int
}

func (l *liveSource) Open(ctx context.Context, _, target string, _ http.Header, _ bool) (*http.Response, error) {
	l.mu.Lock()
	l.opened++
	if l.refuse > 0 {
		l.refuse--
		l.mu.Unlock()
		return answer(http.StatusTooManyRequests, "text/plain", strings.NewReader("max connections")), nil
	}
	l.mu.Unlock()
	name := target[strings.LastIndexByte(target, '/')+1:]
	switch {
	case strings.HasPrefix(name, "html"):
		return answer(http.StatusOK, "text/html", strings.NewReader("<html>"+strings.Repeat("x", 5000)+"</html>")), nil
	case strings.HasPrefix(name, "missing"):
		return answer(http.StatusNotFound, "application/json", strings.NewReader(`{"error":"no such stream"}`)), nil
	case strings.HasPrefix(name, "empty"):
		return answer(http.StatusOK, "video/mp2t", strings.NewReader("")), nil
	case strings.HasPrefix(name, "forbidden"):
		return answer(http.StatusForbidden, "text/plain", strings.NewReader("max connections reached")), nil
	case strings.HasPrefix(name, "garbage"):
		return answer(http.StatusOK, "video/mp2t", bytes.NewReader(bytes.Repeat([]byte{1, 2, 3, 4}, 512))), nil
	case strings.HasPrefix(name, "playlist"):
		return answer(http.StatusOK, "application/vnd.apple.mpegurl", strings.NewReader("#EXTM3U\n#EXTINF:2,\nsegment.ts\n")), nil
	case strings.HasPrefix(name, "silent"):
		return l.stream(ctx, 0), nil
	case strings.HasPrefix(name, "stall"):
		return l.stream(ctx, 64), nil
	case strings.HasPrefix(name, "reset"):
		return answer(http.StatusOK, "video/mp2t", io.MultiReader(bytes.NewReader(bytes.Repeat(tsChunk, 4)), failing{})), nil
	default:
		return l.stream(ctx, -1), nil
	}
}

type failing struct{}

func (failing) Read([]byte) (int, error) { return 0, errors.New("connection reset by peer") }

func answer(status int, contentType string, body io.Reader) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(body)}
}

// stream answers an MPEG-TS stream sending chunks chunks, then nothing
// (keeping the connection open); -1 sends forever. It counts as open until
// closed.
func (l *liveSource) stream(ctx context.Context, chunks int) *http.Response {
	reader, writer := io.Pipe()
	l.mu.Lock()
	l.open++
	l.most = max(l.most, l.open)
	l.mu.Unlock()
	go func() {
		defer writer.Close()
		for n := 0; chunks < 0 || n < chunks; n++ {
			if _, err := writer.Write(tsChunk); err != nil {
				return
			}
			if l.interval > 0 {
				time.Sleep(l.interval)
			}
		}
		<-ctx.Done()
	}()
	body := &countedBody{ReadCloser: reader, close: func() {
		l.mu.Lock()
		l.open--
		l.mu.Unlock()
	}}
	context.AfterFunc(ctx, func() { _ = body.Close() })
	return answer(http.StatusOK, "video/mp2t", body)
}

type countedBody struct {
	io.ReadCloser
	once  sync.Once
	close func()
}

func (b *countedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.close)
	return err
}

func (l *liveSource) counts() (open, opened, most int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.open, l.opened, l.most
}

// shortLiveTimes are the times of live streams in tests.
var shortLiveTimes = liveTimes{grace: 200 * time.Millisecond, stall: 300 * time.Millisecond, answer: time.Second,
	body: 400 * time.Millisecond, slotWait: 2 * time.Second}

// liveService returns a service with shortLiveTimes.
func liveService(t *testing.T, opener *liveSource, ffprobe string) *Service {
	t.Helper()
	s := newService(t, opener, ffprobe, nil)
	s.feeds.times = shortLiveTimes
	return s
}

func channelVersion(n byte, name string, source accounts.ID) library.Version {
	return library.Version{ID: accounts.ID{n}, URL: "http://10.0.0.1/live/" + name, Origin: library.Origin{Addon: source}}
}

// eventually waits for cond, failing after a second.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A channel's stream is read through one connection however many read it:
// the analysis, FFmpeg, players. It stays open a moment after the last
// reader left, so that a quick return finds it, then closes.
func TestLiveStreamsShareOneConnection(t *testing.T) {
	src := &liveSource{interval: 5 * time.Millisecond}
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	path, runs := fakeProbe(t, string(probe), false)
	s := liveService(t, src, path)
	version := channelVersion(1, "1.ts", accounts.ID{9})
	if _, err := s.AnalyzeLive(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	first, err := s.openFeed(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.openFeed(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []*feedReader{first, second} {
		buf := make([]byte, 4096)
		if n, err := io.ReadFull(r, buf); err != nil || buf[0] != 0x47 {
			t.Fatalf("reading the feed: %d %v", n, err)
		}
	}
	if open, opened, _ := src.counts(); open != 1 || opened != 1 || runs() != 1 {
		t.Errorf("connections open %d, opened %d, analyses %d: want one connection for all", open, opened, runs())
	}
	_ = first.Close()
	_ = second.Close()
	if open, _, _ := src.counts(); open != 1 {
		t.Errorf("the connection closed with no grace")
	}
	eventually(t, "the connection to close after the grace", func() bool { open, _, _ := src.counts(); return open == 0 })
}

// Each kind of dead answer fails within a second, before any analysis,
// classified as the stream health records it; a silent source within the
// time given to its first bytes.
func TestDeadLiveStreamsFailAtOnce(t *testing.T) {
	path, runs := fakeProbe(t, "{}", false)
	s := liveService(t, &liveSource{}, path)
	var mu sync.Mutex
	reported := map[accounts.ID]string{}
	s.LiveSources(nil, func(_ context.Context, version library.Version, failure string) {
		mu.Lock()
		reported[version.ID] = failure
		mu.Unlock()
	})
	for n, tc := range []struct {
		name, failure string
		within        time.Duration
	}{
		{"html", LiveDead, time.Second},
		{"missing", LiveDead, time.Second},
		{"empty", LiveDead, time.Second},
		{"forbidden", LiveRefused, time.Second},
		{"garbage", LiveDead, time.Second},
		{"silent", LiveTimeout, shortLiveTimes.body + time.Second},
	} {
		version := channelVersion(byte(n+1), tc.name+".ts", accounts.ID{})
		started := time.Now()
		_, err := s.AnalyzeLive(t.Context(), version)
		took := time.Since(started)
		if LiveFailure(err) != tc.failure || took > tc.within {
			t.Errorf("%s: %v (%s) after %s, want %s within %s", tc.name, err, LiveFailure(err), took, tc.failure, tc.within)
		}
		mu.Lock()
		if reported[version.ID] != tc.failure {
			t.Errorf("%s: reported %q", tc.name, reported[version.ID])
		}
		mu.Unlock()
		// A refusal is tried again at the next start; the others are not
		// tried again for a while.
		if failed := s.Failed(version.ID); failed != (tc.failure != LiveRefused) {
			t.Errorf("%s: remembered as failed: %v", tc.name, failed)
		}
	}
	if runs() != 0 {
		t.Errorf("ffprobe ran %d times for dead streams", runs())
	}
}

// A stream that stops sending ends its feed after feedStall, and one cut
// mid-stream ends it at once: readers see the end rather than wait.
func TestStalledAndCutStreamsEndTheirFeed(t *testing.T) {
	s := liveService(t, &liveSource{}, "ffprobe-not-installed")
	for n, name := range []string{"stall.ts", "reset.ts"} {
		reader, err := s.openFeed(t.Context(), channelVersion(byte(n+1), name, accounts.ID{}))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := io.Copy(io.Discard, reader)
			done <- err
		}()
		select {
		case err := <-done:
			if name == "stall.ts" && !errors.Is(err, ErrLiveTimeout) {
				t.Errorf("%s: %v, want a timeout", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: the reader still waits", name)
		}
		_ = reader.Close()
	}
}

// A reader that falls further behind than the feed keeps is dropped; the
// others read on.
func TestSlowReadersAreDropped(t *testing.T) {
	s := liveService(t, &liveSource{}, "ffprobe-not-installed")
	version := channelVersion(1, "1.ts", accounts.ID{})
	slow, err := s.openFeed(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	fast, err := s.openFeed(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(io.Discard, fast, 3*feedBuffer); err != nil {
		t.Fatalf("the fast reader: %v", err)
	}
	if _, err := io.ReadAll(slow); !errors.Is(err, ErrSlowReader) {
		t.Errorf("the slow reader: %v, want it dropped", err)
	}
	_ = fast.Close()
}

// A source that plays one stream at a time: a user switching channels
// replaces their own stream, idle ones are closed first, and another
// user's stream is never taken: the new one is refused at once.
func TestSourceConnectionsAreKept(t *testing.T) {
	source := accounts.ID{7}
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	s.LiveSources(func(_ context.Context, id accounts.ID) (int, error) {
		if id == source {
			return 1, nil
		}
		return 0, nil
	}, nil)
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	first, err := s.openFeed(alice, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.openFeed(alice, channelVersion(2, "2.ts", source))
	if err != nil {
		t.Fatalf("switching channels: %v", err)
	}
	if _, err := first.Read(make([]byte, 10)); !errors.Is(err, ErrSlotsInUse) {
		t.Errorf("the replaced stream: %v", err)
	}
	if _, err := s.openFeed(bob, channelVersion(3, "3.ts", source)); !errors.Is(err, ErrSlotsInUse) || LiveFailure(err) != LiveRefused {
		t.Errorf("another user's channel: %v, want refused", err)
	}
	// Once alice left, her stream waiting out its grace makes room.
	_ = second.Close()
	third, err := s.openFeed(bob, channelVersion(3, "3.ts", source))
	if err != nil {
		t.Fatalf("after the other stream was left: %v", err)
	}
	_ = third.Close()
	if _, _, most := src.counts(); most != 1 {
		t.Errorf("%d connections at once, want 1", most)
	}
}

// A provider still counting the connection Polyfin just closed refuses
// the next one for a moment: the open is tried again.
func TestAnOpenRefusedAfterARoomWasMadeIsTriedAgain(t *testing.T) {
	source := accounts.ID{7}
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	s.LiveSources(func(context.Context, accounts.ID) (int, error) { return 1, nil }, nil)
	ctx := ForUser(t.Context(), accounts.ID{1})
	first, err := s.openFeed(ctx, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	src.mu.Lock()
	src.refuse = 1
	src.mu.Unlock()
	second, err := s.openFeed(ctx, channelVersion(2, "2.ts", source))
	if err != nil {
		t.Fatalf("the refused open was not tried again: %v", err)
	}
	_ = second.Close()
}

// A source of unknown limit that refuses a stream while another, read by
// no one, waits out its grace has that one closed, and is asked again.
func TestARefusalClosesStreamsNoOneReads(t *testing.T) {
	source := accounts.ID{7}
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	s.feeds.times.grace = time.Minute
	ctx := ForUser(t.Context(), accounts.ID{1})
	first, err := s.openFeed(ctx, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	src.mu.Lock()
	src.refuse = 1
	src.mu.Unlock()
	second, err := s.openFeed(ctx, channelVersion(2, "2.ts", source))
	if err != nil {
		t.Fatalf("refused while an idle stream was kept: %v", err)
	}
	_ = second.Close()
	s.feeds.mu.Lock()
	_, kept := s.feeds.feeds[channelVersion(1, "1.ts", source).ID]
	s.feeds.mu.Unlock()
	if kept {
		t.Error("the idle stream was kept")
	}
}

// What a channel's stream holds is kept: the next start, even after a
// restart, needs neither the source nor ffprobe.
func TestLiveShapesAreKept(t *testing.T) {
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	path, runs := fakeProbe(t, string(probe), false)
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, path)
	version := channelVersion(1, "1.ts", accounts.ID{})
	first, err := s.AnalyzeLive(t.Context(), version)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := New(s.db, src, path, s.signer, s.sources, s.segments, nil, s.logger, s.settings)
	if err != nil {
		t.Fatal(err)
	}
	restarted.feeds.times = shortLiveTimes
	defer restarted.Close()
	_, opened, _ := src.counts()
	again, err := restarted.AnalyzeLive(t.Context(), version)
	if _, now, _ := src.counts(); err != nil || now != opened || runs() != 1 || len(again.Streams) != len(first.Streams) {
		t.Errorf("after a restart: %v, opens %d → %d, analyses %d", err, opened, now, runs())
	}
	// A shape found wrong is analyzed again.
	restarted.forgetShape(t.Context(), version.ID)
	if _, err := restarted.AnalyzeLive(t.Context(), version); err != nil || runs() != 2 {
		t.Errorf("after forgetting it: %v, analyses %d", err, runs())
	}
}

// An HLS playlist has no feed: it is told apart at its first bytes and
// relayed file by file.
func TestPlaylistsHaveNoFeed(t *testing.T) {
	s := liveService(t, &liveSource{}, "ffprobe-not-installed")
	version := channelVersion(1, "playlist.m3u8", accounts.ID{})
	if _, err := s.openFeed(t.Context(), version); !errors.Is(err, errManifest) {
		t.Fatalf("a playlist: %v", err)
	}
	if !s.manifest(version.ID) {
		t.Errorf("the playlist is not remembered as one")
	}
}

// generatedTS encodes 4 seconds of video with a keyframe every second and
// audio, as MPEG-TS, with FFmpeg; without indicator, the random access
// indicators are cleared so that keyframes are found by their NAL units.
func generatedTS(t *testing.T, codec string, indicator bool) []byte {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	out := filepath.Join(t.TempDir(), "live.ts")
	command := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=4",
		"-f", "lavfi", "-i", "sine=duration=4", "-c:v", codec, "-g", "25", "-bf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "mpegts", out)
	if output, err := command.CombinedOutput(); err != nil {
		if codec != "libx264" {
			t.Logf("%s cannot be encoded here: %v %s", codec, err, output)
			return nil
		}
		t.Fatalf("encoding: %v %s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !indicator {
		for at := 0; at+tsPacket <= len(data); at += tsPacket {
			if p := data[at:]; p[3]&0x20 != 0 && p[4] > 0 {
				p[5] &^= 0x40
			}
		}
	}
	return data
}

// tsPID is the PID of the packet at at.
func tsPID(data []byte, at int64) int {
	return int(data[at+1]&0x1f)<<8 | int(data[at+2])
}

// tsVideo is the PID FFmpeg's muxer gives the first video.
const tsVideo = 0x100

// indicatedKeys lists the video packets the muxer marked as random access
// points (it marks audio ones too).
func indicatedKeys(data []byte) []int64 {
	var keys []int64
	for at := 0; at+tsPacket <= len(data); at += tsPacket {
		if p := data[at:]; p[3]&0x20 != 0 && p[4] > 0 && p[5]&0x40 != 0 && tsPID(data, int64(at)) == tsVideo {
			keys = append(keys, int64(at))
		}
	}
	return keys
}

// feedOf writes data to a new feed as a source would send it: after junk
// that is no packet, in chunks of odd sizes.
func feedOf(data []byte, junk int) *feed {
	f := &feed{buf: make([]byte, feedBuffer), changed: make(chan struct{}), readers: map[*feedReader]bool{}, index: newTSIndex(),
		ready: make(chan struct{})}
	f.write(bytes.Repeat([]byte{0xff}, junk))
	for len(data) > 0 {
		n := min(1000, len(data))
		f.write(data[:n])
		data = data[n:]
	}
	return f
}

// A feed finds where a stream's keyframes start, by their random access
// indicator or, without one, by their NAL units, packets split across
// writes included.
func TestFeedsFindTheKeyframesOfTheirStream(t *testing.T) {
	marked := generatedTS(t, "libx264", true)
	want := indicatedKeys(marked)
	if len(want) != 4 {
		t.Fatalf("the generated stream has %d keyframes, want 4", len(want))
	}
	const junk = 100
	for _, test := range []struct {
		name string
		data []byte
	}{{"indicated", marked}, {"NAL units", generatedTS(t, "libx264", false)}} {
		f := feedOf(test.data, junk)
		var got []int64
		for _, key := range f.index.keys {
			got = append(got, key-junk)
		}
		if !slices.Equal(got, want) || f.index.sync != junk {
			t.Errorf("%s: keyframes at %v (sync %d), want %v", test.name, got, f.index.sync, want)
		}
	}
	if hevc := generatedTS(t, "libx265", false); hevc != nil {
		if f := feedOf(hevc, 0); len(f.index.keys) != 4 {
			t.Errorf("HEVC: %d keyframes found by their NAL units, want 4", len(f.index.keys))
		}
	}
}

// readSome reads n bytes of a reader.
func readSome(t *testing.T, r *feedReader, n int) []byte {
	t.Helper()
	got := make([]byte, n)
	if _, err := io.ReadFull(r, got); err != nil {
		t.Fatal(err)
	}
	return got
}

// A reader joining a running feed starts with the stream's PAT and PMT,
// then its latest keyframe; the first reader of a new feed starts at its
// first keyframe. What it reads decodes from its first bytes, with
// FFmpeg's short probe.
func TestReadersStartAtAKeyframe(t *testing.T) {
	data := generatedTS(t, "libx264", true)
	keys := indicatedKeys(data)
	f := feedOf(data, 0)
	r := f.join(t.Context(), accounts.ID{1})
	head := readSome(t, r, 3*tsPacket)
	if tsPID(head, 0) != 0 || tsPID(head, tsPacket) != f.index.pmtPID || !bytes.Equal(head[2*tsPacket:], data[keys[3]:keys[3]+tsPacket]) {
		t.Fatalf("a joining reader starts with PIDs %d, %d, then %x, want 0, %d, then the last keyframe", tsPID(head, 0),
			tsPID(head, tsPacket), head[2*tsPacket:2*tsPacket+8], f.index.pmtPID)
	}
	if r.pos != keys[3]+tsPacket {
		t.Errorf("the reader is at %d, want just past the last keyframe, %d", r.pos, keys[3]+tsPacket)
	}

	// A new feed: its first reader waits for the first keyframe.
	fresh := &feed{buf: make([]byte, feedBuffer), changed: make(chan struct{}), readers: map[*feedReader]bool{}, index: newTSIndex(),
		ready: make(chan struct{})}
	first := fresh.join(t.Context(), accounts.ID{1})
	go func() {
		for rest := data; len(rest) > 0; {
			n := min(1316, len(rest))
			fresh.write(rest[:n])
			rest = rest[n:]
		}
	}()
	head = readSome(t, first, 3*tsPacket)
	if !bytes.Equal(head[2*tsPacket:], data[keys[0]:keys[0]+tsPacket]) {
		t.Fatalf("the first reader does not start at the first keyframe")
	}

	// What the joining reader read decodes with FFmpeg's short probe.
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	joined := append(slices.Clone(head[:2*tsPacket]), data[keys[3]:r.pos]...)
	joined = append(joined, readSome(t, r, len(data)-int(r.pos))...)
	file := filepath.Join(t.TempDir(), "joined.ts")
	if err := os.WriteFile(file, joined, 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-v", "error"}, shortProbe...)
	output, err := exec.Command(ffmpeg, append(args, "-i", file, "-map", "0", "-c", "copy", "-f", "null", "-")...).CombinedOutput()
	if err != nil || bytes.Contains(output, []byte("non-existing PPS")) || bytes.Contains(output, []byte("no frame")) {
		t.Errorf("a joined stream read with the short probe: %v %s", err, output)
	}
}

// Without a keyframe in its first keyframeWait bytes, or not MPEG-TS, a
// feed's readers start feedJoin back, on a packet.
func TestReadersOfStreamsWithoutKeyframesStillStart(t *testing.T) {
	f := feedOf(bytes.Repeat([]byte("not a stream "), 1000), 0)
	if !f.index.off {
		t.Error("a stream of no packets is indexed")
	}
	r := f.join(t.Context(), accounts.ID{1})
	if got := readSome(t, r, 4); string(got) != "not " {
		t.Errorf("read %q", got)
	}
}
