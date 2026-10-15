package commercials

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// testFFmpeg is the FFmpeg of POLYFIN_TEST_FFMPEG, with the ffprobe beside
// it, skipping the test when it is not set.
func testFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// piece is a part of a recording makeRecording writes.
type piece struct {
	kind   pieceKind
	length time.Duration
}

// pieceKind is what a piece shows and sounds.
type pieceKind int

const (
	// programme is a moving picture with a tone.
	programme pieceKind = iota
	// advert is other bars with another tone.
	advert
	// pause is black and silent, as between a programme and its
	// commercials.
	pause
)

// twoBreaks is a recording with two commercial breaks, each opened,
// parted and closed by black and silent pauses: two adverts of 15 and 20
// seconds after 100 seconds, then three of 12 seconds. Its breaks are
// twoBreaksFound.
var twoBreaks = []piece{
	{programme, 100 * time.Second}, {pause, time.Second}, {advert, 15 * time.Second}, {pause, time.Second}, {advert, 20 * time.Second},
	{pause, time.Second}, {programme, 100 * time.Second}, {pause, time.Second}, {advert, 12 * time.Second}, {pause, time.Second},
	{advert, 12 * time.Second}, {pause, time.Second}, {advert, 12 * time.Second}, {pause, time.Second}, {programme, 100 * time.Second},
}

// twoBreaksFound are the breaks of twoBreaks, their pauses included.
var twoBreaksFound = []span{{100 * time.Second, 138 * time.Second}, {238 * time.Second, 278 * time.Second}}

// makeRecording writes a Matroska recording of pieces at path, a small
// picture at 5 frames a second, as a channel airs it.
func makeRecording(t *testing.T, ffmpeg, path string, pieces []piece) {
	t.Helper()
	var args []string
	var graph strings.Builder
	for i, p := range pieces {
		seconds := fmt.Sprintf("%.3f", p.length.Seconds())
		var video, audio string
		switch p.kind {
		case programme:
			video, audio = "testsrc2=size=160x90:rate=5:duration="+seconds, "sine=frequency=440:sample_rate=48000:duration="+seconds
		case advert:
			video, audio = "smptebars=size=160x90:rate=5:duration="+seconds, "sine=frequency=880:sample_rate=48000:duration="+seconds
		case pause:
			video, audio = "color=c=black:size=160x90:rate=5:duration="+seconds, "aevalsrc=0:s=48000:d="+seconds
		}
		args = append(args, "-f", "lavfi", "-i", video, "-f", "lavfi", "-i", audio)
		fmt.Fprintf(&graph, "[%d:v]format=yuv420p[v%d];[%d:a]aformat=sample_rates=48000:channel_layouts=mono[a%d];", 2*i, i, 2*i+1, i)
	}
	for i := range pieces {
		fmt.Fprintf(&graph, "[v%d][a%d]", i, i)
	}
	fmt.Fprintf(&graph, "concat=n=%d:v=1:a=1[v][a]", len(pieces))
	args = append(args, "-filter_complex", graph.String(), "-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", path)
	out, err := exec.Command(ffmpeg, append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

// settings are the settings a test changes.
type settings struct {
	mu    sync.Mutex
	value accounts.Settings
}

func (s *settings) get() accounts.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *settings) set(change func(*accounts.Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&s.value)
}

// fixture is a Service reading the recordings of a folder.
type fixture struct {
	service  *Service
	settings *settings
	pool     *pgxpool.Pool
	dir      string
	ffmpeg   string
}

// newFixture starts a Service with the search on.
func newFixture(t *testing.T) fixture {
	t.Helper()
	ffmpeg, ffprobe := testFFmpeg(t)
	pool := migrated.New(t)
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	current := &settings{value: accounts.DefaultSettings()}
	current.set(func(s *accounts.Settings) { s.RecordingCommercials = true })
	finished := recordings.New(recordings.Config{DB: pool, Folder: func() string { return dir }, Logger: logger})
	s := New(Options{DB: pool, FFmpeg: ffmpeg, FFprobe: ffprobe, Recordings: finished, Settings: current.get, Logger: logger})
	t.Cleanup(s.Close)
	return fixture{service: s, settings: current, pool: pool, dir: dir, ffmpeg: ffmpeg}
}

// record writes a finished recording of pieces, and returns its
// identifier.
func (f fixture) record(t *testing.T, pieces []piece) accounts.ID {
	t.Helper()
	var id accounts.ID
	if err := f.pool.QueryRow(t.Context(), `INSERT INTO live_recordings (channel_id, name, start_at, end_at, started_at, ended_at, status)
		VALUES (gen_random_uuid(), 'Evening at the harbour', now(), now(), now(), now(), 'Completed') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	file := id.String() + ".mkv"
	makeRecording(t, f.ffmpeg, filepath.Join(f.dir, file), pieces)
	if _, err := f.pool.Exec(t.Context(), "UPDATE live_recordings SET file = $2 WHERE id = $1", id, file); err != nil {
		t.Fatal(err)
	}
	return id
}

// read tells whether a recording was read.
func (f fixture) read(t *testing.T, id accounts.ID) bool {
	t.Helper()
	var read bool
	if err := f.pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM recording_commercial_reads WHERE recording_id = $1)", id).Scan(&read); err != nil {
		t.Fatal(err)
	}
	return read
}

// checkSegments checks the segments served for a recording are
// commercials of want, to half a second.
func checkSegments(t *testing.T, s *Service, id accounts.ID, want []span) {
	t.Helper()
	got := s.Segments(t.Context(), id)
	if len(got) != len(want) {
		t.Fatalf("segments: %v, want %v", got, want)
	}
	for i, segment := range got {
		if segment.Type != mediasegments.Commercial || (segment.Start-want[i].start).Abs() > time.Second/2 || (segment.End-want[i].end).Abs() > time.Second/2 {
			t.Errorf("segment %d: %+v, want a commercial from %v to %v", i, segment, want[i].start, want[i].end)
		}
	}
	if has := s.Has(t.Context(), id); has != (len(want) > 0) {
		t.Errorf("has segments: %v", has)
	}
}

// A recording with two commercial breaks, each opened, parted and closed
// by black and silent pauses, gets them as two Commercial segments once
// the task reads it; with the skip buttons off, none is served. They are
// kept by the recording's identifier, and deleted with it.
func TestTwoBreaksAreFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	id := f.record(t, twoBreaks)
	if err := f.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkSegments(t, f.service, id, twoBreaksFound)

	f.settings.set(func(s *accounts.Settings) { s.SkipButtons = false })
	checkSegments(t, f.service, id, nil)
	f.settings.set(func(s *accounts.Settings) { s.SkipButtons = true })

	// Its file replaced, as by a conversion, it keeps them.
	if err := os.Rename(filepath.Join(f.dir, id.String()+".mkv"), filepath.Join(f.dir, "replaced.mkv")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE live_recordings SET file = 'replaced.mkv', size = 1 WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkSegments(t, f.service, id, twoBreaksFound)

	if _, err := f.pool.Exec(t.Context(), "DELETE FROM live_recordings WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM recording_commercials WHERE recording_id = $1) +
		(SELECT count(*) FROM recording_commercial_reads WHERE recording_id = $1)`, id).Scan(&left); err != nil || left != 0 {
		t.Errorf("left after the recording was deleted: %d %v", left, err)
	}
}

// A programme with a black scene in silence has no break: one black and
// silent moment parts no commercials. The recording is read all the same,
// and not again.
func TestBlackSceneInSilenceIsNoBreak(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	id := f.record(t, []piece{{programme, 100 * time.Second}, {pause, 3 * time.Second}, {programme, 100 * time.Second}})
	if err := f.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !f.read(t, id) {
		t.Fatal("the recording was not read")
	}
	checkSegments(t, f.service, id, nil)
}

// A recording longer than the bound is not read, and a reading that takes
// longer than the recording lasts stops: both get none, and are not read
// again.
func TestBoundsLeaveRecordingsWithoutBreaks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		bound func(s *Service)
	}{
		{"too long", func(s *Service) { s.longest = time.Minute }},
		{"read too slowly", func(s *Service) { s.budget = func(time.Duration) time.Duration { return time.Millisecond } }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tc.bound(f.service)
			id := f.record(t, twoBreaks)
			if err := f.service.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !f.read(t, id) {
				t.Fatal("the recording was not marked read")
			}
			checkSegments(t, f.service, id, nil)
		})
	}
}

// A recording that ends is read in the background while the search is on;
// one that ends while it is off is left to the task, which does nothing
// while it is off either.
func TestEndedRecordingsAreRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	pieces := []piece{{advert, 20 * time.Second}, {pause, time.Second}, {advert, 20 * time.Second}, {pause, time.Second}, {programme, 100 * time.Second}}
	off := f.record(t, pieces)
	on := f.record(t, pieces)

	f.settings.set(func(s *accounts.Settings) { s.RecordingCommercials = false })
	f.service.Ended(t.Context(), recordings.Ended{Recording: off, Partial: true})
	if err := f.service.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f.read(t, off) {
		t.Fatal("the task read a recording while the search is off")
	}
	f.settings.set(func(s *accounts.Settings) { s.RecordingCommercials = true })
	f.service.Ended(t.Context(), recordings.Ended{Recording: on, Partial: true})
	// The queue reads in order: once the second is read, the first would
	// have been.
	deadline := time.Now().Add(time.Minute)
	for !f.read(t, on) {
		if time.Now().After(deadline) {
			t.Fatal("the recording that ended was not read")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if f.read(t, off) {
		t.Error("a recording that ended while the search was off was read")
	}
	// Its break, from its start, counts.
	checkSegments(t, f.service, on, []span{{0, 42 * time.Second}})
}

// Breaks are runs of pieces, each 5 to 90 seconds long, between black and
// silent moments, of 30 seconds to 8 minutes in all, the recording's start
// counting as such a moment.
func TestBreaks(t *testing.T) {
	t.Parallel()
	s := func(start, end float64) span {
		return span{time.Duration(start * float64(time.Second)), time.Duration(end * float64(time.Second))}
	}
	for _, tc := range []struct {
		name    string
		moments []span
		want    []span
	}{
		{"none", nil, nil},
		{"two breaks", []span{s(100, 101), s(116, 117), s(137, 138), s(238, 239), s(251, 252), s(264, 265), s(277, 278)},
			[]span{s(100, 138), s(238, 278)}},
		{"a black scene", []span{s(100, 103)}, nil},
		{"from the start", []span{s(20, 21), s(41, 42)}, []span{s(0, 42)}},
		{"a moment at the start", []span{s(0, 0.5), s(20, 21), s(41, 42)}, []span{s(0, 42)}},
		{"too short in all", []span{s(100, 101), s(110, 111), s(120, 121)}, nil},
		{"too long in all", func() []span {
			var m []span
			for at := 100.0; at <= 100+11*60; at += 60 {
				m = append(m, s(at, at+1))
			}
			return m
		}(), nil},
		{"a piece too short parts runs", []span{s(100, 101), s(120, 121), s(140, 141), s(142.5, 143), s(163, 164), s(184, 185)},
			[]span{s(100, 141), s(142.5, 185)}},
		{"a piece too long parts runs", []span{s(100, 101), s(120, 121), s(140, 141), s(240, 241), s(260, 261)}, []span{s(100, 141)}},
		{"a flicker is one moment", []span{s(100, 100.3), s(100.8, 101), s(120, 121), s(140, 141)}, []span{s(100, 141)}},
	} {
		if got := breaks(tc.moments); fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// What blackdetect and silencedetect report is read from FFmpeg's log, a
// silence still under way at the end ending there, and the moments both
// black and silent are where they overlap.
func TestParse(t *testing.T) {
	t.Parallel()
	log := strings.Join([]string{
		"Input #0, matroska,webm, from 'x.mkv':",
		"[Parsed_silencedetect_0 @ 0x1] silence_start: 99.75",
		"[Parsed_blackdetect_1 @ 0x2] black_start:100 black_end:101.25 black_duration:1.25",
		"[Parsed_silencedetect_0 @ 0x1] silence_end: 101 | silence_duration: 1.25",
		"[Parsed_blackdetect_1 @ 0x2] black_start:150 black_end:151 black_duration:1",
		"[Parsed_silencedetect_0 @ 0x1] silence_start: 299.5",
		"[out#0/null @ 0x3] video:1kB audio:1kB",
	}, "\n")
	ms := func(start, end int64) span {
		return span{time.Duration(start) * time.Millisecond, time.Duration(end) * time.Millisecond}
	}
	black, silent, last := parse(strings.NewReader(log), 300*time.Second)
	if !slices.Equal(black, []span{ms(100000, 101250), ms(150000, 151000)}) || !slices.Equal(silent, []span{ms(99750, 101000), ms(299500, 300000)}) ||
		!strings.HasPrefix(last, "[out#0") {
		t.Fatalf("parsed: %v %v %q", black, silent, last)
	}
	if both := overlaps(black, silent); !slices.Equal(both, []span{ms(100000, 101000)}) {
		t.Errorf("overlaps: %v", both)
	}
}
