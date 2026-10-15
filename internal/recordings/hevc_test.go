package recordings

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/userdata"
)

// fakeConverter writes copies of copyBytes bytes, and answers the
// analyses fakeProber is given.
type fakeConverter struct {
	hardware *hls.Hardware
	encoders []string
	// copyBytes is the size of the copies written; during, when set, runs
	// while a copy is written, before it returns.
	copyBytes int
	during    func(ctx context.Context) error

	mu    sync.Mutex
	calls []hls.FileConversion
}

func (c *fakeConverter) Hardware() *hls.Hardware { return c.hardware }
func (c *fakeConverter) Encoders() []string      { return c.encoders }

func (c *fakeConverter) ConvertFile(ctx context.Context, conversion hls.FileConversion, _ func(time.Duration)) error {
	c.mu.Lock()
	c.calls = append(c.calls, conversion)
	c.mu.Unlock()
	if err := os.WriteFile(conversion.Output, make([]byte, c.copyBytes), 0o600); err != nil {
		return err
	}
	if c.during != nil {
		return c.during(ctx)
	}
	return nil
}

func (c *fakeConverter) DecodedLength(_ context.Context, path string) (time.Duration, error) {
	return 10 * time.Second, nil
}

func (c *fakeConverter) conversions() []hls.FileConversion {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

// fakeProber describes the recordings as ten seconds of 720p H.264 with
// stereo AAC, and their copies as copyLength of HEVC.
type fakeProber struct{ copyLength time.Duration }

func (p fakeProber) ProbeInput(_ context.Context, in media.Input) (media.Analysis, error) {
	if _, err := os.Stat(in.Path); err != nil {
		return media.Analysis{}, err
	}
	video := media.Stream{Index: 0, Type: "video", Codec: "h264", Width: 1280, Height: 720, FrameRate: 25, BitDepth: 8}
	duration := 10 * time.Second
	if strings.HasSuffix(in.Path, ".hevc.mkv") {
		video.Codec, duration = "hevc", p.copyLength
	}
	return media.Analysis{Duration: duration, Streams: []media.Stream{video,
		{Index: 1, Type: "audio", Codec: "aac", Channels: 2, Language: "fre"}}}, nil
}

// conversionFixture is a fixture converting with c, ConvertRecordings on
// from 1:00 to 6:00 and the processor allowed, the clock at 2:00 in a
// time zone of its own.
func conversionFixture(t *testing.T, c Converter, p Prober) (fixture, *atomic.Bool) {
	t.Helper()
	f := newFixture(t)
	playing := &atomic.Bool{}
	f.service.converter, f.service.prober = c, p
	f.service.inUse = func(accounts.ID) bool { return playing.Load() }
	f.service.free = func(string) (int64, bool) { return 1 << 40, true }
	f.at(time.Date(2026, 10, 15, 2, 0, 0, 0, time.FixedZone("UTC+2", 2*3600)))
	f.settings(t, func(s *accounts.Settings) { s.ConvertRecordings, s.ConvertRecordingsOnProcessor = true, true })
	return f, playing
}

func (f fixture) at(now time.Time) { f.service.now = func() time.Time { return now } }

func (f fixture) settings(t *testing.T, change func(*accounts.Settings)) {
	t.Helper()
	settings := f.store.Settings()
	change(&settings)
	if _, err := f.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

// finished adds a finished recording whose file holds data.
func (f fixture) finished(t *testing.T, data []byte) Recording {
	t.Helper()
	var r Recording
	start := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
	if err := f.service.db.QueryRow(t.Context(), `INSERT INTO live_recordings (channel_id, name, start_at, end_at, started_at, ended_at, status, file, size)
		VALUES ($1, 'The Evening Show', $2, $3, $2, $3, 'Completed', 'pending.ts', $4) RETURNING id`,
		id(t, channelX), start, start.Add(time.Hour), len(data)).Scan(&r.ID); err != nil {
		t.Fatal(err)
	}
	r.File = r.ID.String() + ".ts"
	if _, err := f.service.db.Exec(t.Context(), "UPDATE live_recordings SET file = $2 WHERE id = $1", r.ID, r.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, r.File), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return r
}

// hevcState is where a recording is in its conversion.
func (f fixture) hevcState(t *testing.T, r accounts.ID) (state, file string, size int64) {
	t.Helper()
	if err := f.service.db.QueryRow(t.Context(), "SELECT hevc, file, size FROM live_recordings WHERE id = $1", r).Scan(&state, &file, &size); err != nil {
		t.Fatal(err)
	}
	return state, file, size
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The hours run in the server's time zone, past midnight too.
func TestTheConversionHours(t *testing.T) {
	zone := time.FixedZone("UTC-5", -5*3600)
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 15, hour, minute, 0, 0, zone) }
	for _, tc := range []struct {
		name        string
		now         time.Time
		from, until int
		end         time.Time
		within      bool
	}{
		{"at the start", at(1, 0), 1, 6, at(6, 0), true},
		{"within", at(5, 59), 1, 6, at(6, 0), true},
		{"at the end", at(6, 0), 1, 6, time.Time{}, false},
		{"before", at(0, 30), 1, 6, time.Time{}, false},
		{"in the afternoon", at(14, 0), 1, 6, time.Time{}, false},
		{"before midnight", at(23, 0), 22, 5, at(29, 0), true},
		{"after midnight", at(2, 0), 22, 5, at(5, 0), true},
		{"in the day, past midnight hours", at(12, 0), 22, 5, time.Time{}, false},
	} {
		end, within := conversionEnd(tc.now, tc.from, tc.until)
		if within != tc.within || !end.Equal(tc.end) {
			t.Errorf("%s: %v, %v", tc.name, end, within)
		}
	}
}

// A conversion outside its hours does not start, nor with the setting
// off.
func TestAConversionOutsideItsHoursDoesNotStart(t *testing.T) {
	c := &fakeConverter{encoders: []string{"libx265"}, copyBytes: 10}
	f, _ := conversionFixture(t, c, fakeProber{copyLength: 10 * time.Second})
	r := f.finished(t, make([]byte, 100))
	f.at(time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local))
	if wait := f.service.ConvertNext(t.Context()); wait != maxWait {
		t.Errorf("waits %v", wait)
	}
	f.at(time.Date(2026, 10, 15, 2, 0, 0, 0, time.Local))
	f.settings(t, func(s *accounts.Settings) { s.ConvertRecordings = false })
	f.service.ConvertNext(t.Context())
	if calls := c.conversions(); len(calls) != 0 {
		t.Fatalf("converted: %+v", calls)
	}
	if state, file, _ := f.hevcState(t, r.ID); state != hevcToDo || file != r.File || exists(copyPath(f.dir, r.ID)) {
		t.Errorf("%q, %q", state, file)
	}
	// Within its hours, it starts.
	f.settings(t, func(s *accounts.Settings) { s.ConvertRecordings = true })
	if wait := f.service.ConvertNext(t.Context()); wait != 0 || len(c.conversions()) != 1 {
		t.Errorf("within the hours: waits %v, %d conversions", wait, len(c.conversions()))
	}
}

// Without a GPU and with the processor setting off, nothing is converted;
// a GPU with an HEVC encoder converts whatever the processor setting.
func TestNothingIsConvertedWithoutAGPUOrTheProcessor(t *testing.T) {
	c := &fakeConverter{encoders: []string{"libx264", "libx265"}, copyBytes: 10}
	f, _ := conversionFixture(t, c, fakeProber{copyLength: 10 * time.Second})
	r := f.finished(t, make([]byte, 100))
	f.settings(t, func(s *accounts.Settings) { s.ConvertRecordingsOnProcessor = false })
	f.service.ConvertNext(t.Context())
	// A GPU without an HEVC encoder is no better.
	c.hardware = &hls.Hardware{Method: "vaapi", Encoders: []string{"h264_vaapi"}}
	f.service.ConvertNext(t.Context())
	if calls := c.conversions(); len(calls) != 0 {
		t.Fatalf("converted: %+v", calls)
	}
	if state, _, _ := f.hevcState(t, r.ID); state != hevcToDo {
		t.Errorf("state %q", state)
	}
	c.hardware = &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}}
	f.service.ConvertNext(t.Context())
	if calls := c.conversions(); len(calls) != 1 || calls[0].Video.Encoder != "hevc_nvenc" || calls[0].Video.Hardware != c.hardware {
		t.Fatalf("on the GPU: %+v", calls)
	}
}

// A copy shorter than the original is deleted, the original kept, and the
// recording not tried again.
func TestAShorterCopyIsDeletedAndTheOriginalKept(t *testing.T) {
	c := &fakeConverter{encoders: []string{"libx265"}, copyBytes: 10}
	f, _ := conversionFixture(t, c, fakeProber{copyLength: 8 * time.Second})
	original := []byte("the original recording, as it was written")
	r := f.finished(t, original)
	f.service.ConvertNext(t.Context())
	if len(c.conversions()) != 1 {
		t.Fatalf("%d conversions", len(c.conversions()))
	}
	state, file, size := f.hevcState(t, r.ID)
	if state != hevcKept || file != r.File || size != int64(len(original)) || exists(copyPath(f.dir, r.ID)) {
		t.Errorf("%q, %q, %d, copy kept %v", state, file, size, exists(copyPath(f.dir, r.ID)))
	}
	if data, err := os.ReadFile(filepath.Join(f.dir, r.File)); err != nil || string(data) != string(original) {
		t.Errorf("the original: %q, %v", data, err)
	}
	f.service.ConvertNext(t.Context())
	if len(c.conversions()) != 1 {
		t.Errorf("tried again: %d conversions", len(c.conversions()))
	}
	// A copy as large as the original is not kept either.
	c.copyBytes = 200
	large := f.finished(t, make([]byte, 200))
	f.service.prober = fakeProber{copyLength: 10 * time.Second}
	f.service.ConvertNext(t.Context())
	if state, file, _ := f.hevcState(t, large.ID); state != hevcKept || file != large.File || exists(copyPath(f.dir, large.ID)) {
		t.Errorf("a copy as large: %q, %q", state, file)
	}
}

// notMediaProber is fakeProber, but ffprobe cannot read the file named bad.
type notMediaProber struct {
	fakeProber
	bad string
}

func (p notMediaProber) ProbeInput(ctx context.Context, in media.Input) (media.Analysis, error) {
	if filepath.Base(in.Path) == p.bad {
		return media.Analysis{}, fmt.Errorf("%w: ffprobe: exit status 1: Invalid data found when processing input", media.ErrNotMedia)
	}
	return p.fakeProber.ProbeInput(ctx, in)
}

// A recording that cannot be read for a reason that says nothing of it is
// tried again, never kept: with the folder missing, as a disk not mounted
// yet, or ffprobe missing, the pass stops; with its file alone missing,
// the pass goes on with the next recordings. Only one ffprobe cannot read
// is kept as it was.
func TestAnUnreadableRecordingIsTriedAgain(t *testing.T) {
	c := &fakeConverter{encoders: []string{"libx265"}, copyBytes: 10}
	f, _ := conversionFixture(t, c, fakeProber{copyLength: 10 * time.Second})
	var recordings []Recording
	for range 3 {
		recordings = append(recordings, f.finished(t, make([]byte, 100)))
	}
	unchanged := func(when string) {
		t.Helper()
		for _, r := range recordings {
			if state, file, _ := f.hevcState(t, r.ID); state != hevcToDo || file != r.File {
				t.Errorf("%s: recording %s: %q, %q", when, r.ID, state, file)
			}
		}
		if calls := c.conversions(); len(calls) != 0 {
			t.Errorf("%s: converted: %+v", when, calls)
		}
	}
	// pass runs ConvertNext until it waits, and counts the recordings it
	// went through.
	pass := func(when string) int {
		t.Helper()
		for n := range 10 {
			if wait := f.service.ConvertNext(t.Context()); wait != 0 {
				if wait != maxWait {
					t.Errorf("%s: waits %v", when, wait)
				}
				return n
			}
		}
		t.Fatalf("%s: the pass does not end", when)
		return 0
	}
	// The disk is not mounted yet: the folder is missing.
	absent := filepath.Join(t.TempDir(), "recordings")
	f.folder.Store(&absent)
	if n := pass("the folder missing"); n != 0 {
		t.Errorf("the folder missing: %d recordings tried", n)
	}
	unchanged("the folder missing")
	// The mount point is there, but empty: each file is missing alone.
	mountPoint := t.TempDir()
	f.folder.Store(&mountPoint)
	if n := pass("the mount point empty"); n != len(recordings) {
		t.Errorf("the mount point empty: %d recordings tried", n)
	}
	unchanged("the mount point empty")
	// The disk is back, but ffprobe is missing.
	f.folder.Store(&f.dir)
	f.service.prober = media.Prober{Path: filepath.Join(t.TempDir(), "ffprobe"), Timeout: time.Minute}
	if n := pass("ffprobe missing"); n != 0 {
		t.Errorf("ffprobe missing: %d recordings tried", n)
	}
	unchanged("ffprobe missing")
	// ffprobe is back, but one recording's file is missing: the others are
	// converted in the same pass.
	f.service.prober = fakeProber{copyLength: 10 * time.Second}
	missing := recordings[1]
	aside := filepath.Join(t.TempDir(), missing.File)
	if err := os.Rename(filepath.Join(f.dir, missing.File), aside); err != nil {
		t.Fatal(err)
	}
	if n := pass("a file missing"); n != len(recordings) || len(c.conversions()) != len(recordings)-1 {
		t.Errorf("a file missing: %d recordings tried, %d conversions", n, len(c.conversions()))
	}
	for _, r := range recordings {
		want := hevcConverted
		if r.ID == missing.ID {
			want = hevcToDo
		}
		if state, _, _ := f.hevcState(t, r.ID); state != want {
			t.Errorf("a file missing: recording %s: %q", r.ID, state)
		}
	}
	// Once its file is back, it is converted.
	if err := os.Rename(aside, filepath.Join(f.dir, missing.File)); err != nil {
		t.Fatal(err)
	}
	if n := pass("the file back"); n != 1 || len(c.conversions()) != len(recordings) {
		t.Errorf("the file back: %d recordings tried, %d conversions", n, len(c.conversions()))
	}
	if state, _, _ := f.hevcState(t, missing.ID); state != hevcConverted {
		t.Errorf("the file back: %q", state)
	}
	// A recording ffprobe cannot read is kept, not tried again.
	bad := f.finished(t, []byte("not a video"))
	f.service.prober = notMediaProber{fakeProber{copyLength: 10 * time.Second}, bad.File}
	if wait := f.service.ConvertNext(t.Context()); wait != 0 {
		t.Errorf("not a video: waits %v", wait)
	}
	if state, file, _ := f.hevcState(t, bad.ID); state != hevcKept || file != bad.File {
		t.Errorf("not a video: %q, %q", state, file)
	}
	if wait := f.service.ConvertNext(t.Context()); wait != maxWait || len(c.conversions()) != len(recordings) {
		t.Errorf("not a video, tried again: waits %v, %d conversions", wait, len(c.conversions()))
	}
}

// A conversion still running at the end of its hours stops, its copy is
// deleted, and the recording starts over next time.
func TestAConversionStopsAtTheEndOfItsHours(t *testing.T) {
	stopped := make(chan struct{})
	c := &fakeConverter{encoders: []string{"libx265"}, copyBytes: 10, during: func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}}
	f, _ := conversionFixture(t, c, fakeProber{copyLength: 10 * time.Second})
	r := f.finished(t, make([]byte, 100))
	// Half a second before 6:00.
	f.at(time.Date(2026, 10, 15, 5, 59, 59, 500_000_000, time.Local))
	if wait := f.service.ConvertNext(t.Context()); wait != maxWait {
		t.Errorf("waits %v", wait)
	}
	<-stopped
	if state, file, _ := f.hevcState(t, r.ID); state != hevcToDo || file != r.File || exists(copyPath(f.dir, r.ID)) {
		t.Errorf("%q, %q, copy kept %v", state, file, exists(copyPath(f.dir, r.ID)))
	}
}

// A recording someone plays while it is converted is replaced once the
// playback stops, whatever the hour.
func TestARecordingPlayedDuringItsConversionIsReplacedAfterward(t *testing.T) {
	c := &fakeConverter{encoders: []string{"libx265"}, copyBytes: 10}
	f, playing := conversionFixture(t, c, fakeProber{copyLength: 10*time.Second - 500*time.Millisecond})
	// The playback starts while the copy is written.
	c.during = func(context.Context) error {
		playing.Store(true)
		return nil
	}
	original := make([]byte, 100)
	r := f.finished(t, original)
	f.service.ConvertNext(t.Context())
	state, file, size := f.hevcState(t, r.ID)
	if state != hevcPending || file != r.File || size != int64(len(original)) || !exists(copyPath(f.dir, r.ID)) {
		t.Fatalf("while played: %q, %q, %d, copy %v", state, file, size, exists(copyPath(f.dir, r.ID)))
	}
	if data, err := os.ReadFile(filepath.Join(f.dir, r.File)); err != nil || len(data) != len(original) {
		t.Errorf("the original while played: %d bytes, %v", len(data), err)
	}
	f.service.ConvertNext(t.Context())
	if state, _, _ := f.hevcState(t, r.ID); state != hevcPending {
		t.Errorf("replaced while still played: %q", state)
	}
	// Stopped, at noon.
	playing.Store(false)
	f.at(time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local))
	f.service.ConvertNext(t.Context())
	state, file, size = f.hevcState(t, r.ID)
	if state != hevcConverted || file != r.ID.String()+".mkv" || size != 10 || exists(copyPath(f.dir, r.ID)) || exists(filepath.Join(f.dir, r.File)) {
		t.Errorf("after the playback: %q, %q, %d", state, file, size)
	}
	if recordings, saved, err := f.service.SpaceSaved(t.Context()); err != nil || recordings != 1 || saved != 90 {
		t.Errorf("space saved: %d recordings, %d bytes, %v", recordings, saved, err)
	}
	if len(c.conversions()) != 1 {
		t.Errorf("%d conversions", len(c.conversions()))
	}
}

// testTools are the FFmpeg and ffprobe of POLYFIN_TEST_FFMPEG, skipping
// the test when it is not set.
func testTools(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// A recording converted by the processor keeps its identifier, played
// state and resume point; what playback learned of its file is forgotten.
func TestAConvertedRecordingKeepsItsIdentifierAndResumePoint(t *testing.T) {
	ffmpeg, ffprobe := testTools(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	segments, err := hls.NewManager(ffmpeg, t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	if !slices.Contains(segments.Encoders(), "libx265") {
		t.Skip("FFmpeg has no libx265")
	}
	prober := media.Prober{Path: ffprobe, Timeout: time.Minute}
	f, _ := conversionFixture(t, segments, prober)
	forgotten := &forgettingRecorder{}
	f.service.recorder = forgotten
	// Five seconds of interlaced MPEG-2 at a high bitrate, with stereo
	// audio, as a broadcast records.
	var r Recording
	start := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
	if err := f.service.db.QueryRow(t.Context(), `INSERT INTO live_recordings (channel_id, name, start_at, end_at, started_at, ended_at, status)
		VALUES ($1, 'The Evening Show', $2, $3, $2, $3, 'Completed') RETURNING id`, id(t, channelX), start, start.Add(time.Hour)).Scan(&r.ID); err != nil {
		t.Fatal(err)
	}
	r.File = r.ID.String() + ".mkv"
	source := filepath.Join(f.dir, r.File)
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "5", "-c:v", "mpeg2video", "-q:v", "2", "-flags", "+ilme+ildct",
		"-c:a", "aac", "-ac", "2", "-metadata:s:a:0", "language=fre", source).CombinedOutput(); err != nil {
		t.Fatalf("make the recording: %v: %s", err, out)
	}
	original := size(source)
	if _, err := f.service.db.Exec(t.Context(), "UPDATE live_recordings SET file = $2, size = $3 WHERE id = $1", r.ID, r.File, original); err != nil {
		t.Fatal(err)
	}
	data := userdata.New(f.service.db)
	if _, err := data.Change(t.Context(), f.user.ID, []userdata.Item{{ID: r.ID}}, func(d *userdata.Data) {
		d.Position, d.PlayCount = 2*time.Second, 1
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.db.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, '{}')", r.ID); err != nil {
		t.Fatal(err)
	}

	f.service.ConvertNext(t.Context())
	state, file, converted := f.hevcState(t, r.ID)
	if state != hevcConverted || file != r.File || converted <= 0 || converted >= original || exists(copyPath(f.dir, r.ID)) {
		t.Fatalf("%q, %q, %d bytes from %d", state, file, converted, original)
	}
	after, err := prober.ProbeInput(t.Context(), media.Input{Path: source})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Streams) != 2 || after.Streams[0].Codec != "hevc" || after.Streams[0].Width != 320 || after.Streams[1].Codec != "aac" ||
		after.Streams[1].Language != "fre" {
		t.Errorf("the converted file: %+v", after.Streams)
	}
	if list, err := f.service.Recordings(t.Context()); err != nil || len(list) != 1 || list[0].ID != r.ID || list[0].Size != converted {
		t.Errorf("recordings: %+v, %v", list, err)
	}
	got, err := data.Get(t.Context(), f.user.ID, []accounts.ID{r.ID})
	if err != nil || got[r.ID].Position != 2*time.Second || got[r.ID].PlayCount != 1 {
		t.Errorf("user data: %+v, %v", got[r.ID], err)
	}
	var analyses int
	if err := f.service.db.QueryRow(t.Context(), "SELECT count(*) FROM media_analyses WHERE version_id = $1", r.ID).Scan(&analyses); err != nil || analyses != 0 {
		t.Errorf("the analysis of the original was kept: %d, %v", analyses, err)
	}
	if !slices.Equal(forgotten.forgotten(), []accounts.ID{r.ID}) {
		t.Errorf("forgotten: %v", forgotten.forgotten())
	}
	if recordings, saved, err := f.service.SpaceSaved(t.Context()); err != nil || recordings != 1 || saved != original-converted {
		t.Errorf("space saved: %d recordings, %d bytes, %v", recordings, saved, err)
	}
}

// forgettingRecorder tells the files it was asked to forget.
type forgettingRecorder struct {
	recorder
	mu    sync.Mutex
	files []accounts.ID
}

func (r *forgettingRecorder) ForgetFile(id accounts.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files = append(r.files, id)
}

func (r *forgettingRecorder) forgotten() []accounts.ID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.files)
}
