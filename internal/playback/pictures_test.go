package playback

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// fakeReader reads every picture as "line N", N its place in the track,
// with the languages it has data for. With hold set, each read waits for
// a value from it, having told started.
type fakeReader struct {
	languages []string
	hold      chan struct{}
	started   chan struct{}

	mu            sync.Mutex
	reads         []string
	running, most int
}

func (f *fakeReader) Languages(context.Context) ([]string, error) { return f.languages, nil }

func (f *fakeReader) Read(_ context.Context, pictures []ocr.Picture, language string) ([]subtitles.Cue, error) {
	f.mu.Lock()
	f.reads = append(f.reads, language)
	f.running++
	f.most = max(f.most, f.running)
	f.mu.Unlock()
	if f.hold != nil {
		f.started <- struct{}{}
		<-f.hold
	}
	f.mu.Lock()
	f.running--
	f.mu.Unlock()
	cues := make([]subtitles.Cue, len(pictures))
	for i, p := range pictures {
		cues[i] = subtitles.Cue{Start: p.Start, End: p.End, Lines: []string{"line " + string(rune('1'+i))}}
	}
	return cues, nil
}

func (f *fakeReader) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads)
}

// localDisk stands for local folders: the files of dir by their keys,
// their states told by states, which a test changes.
type localDisk struct {
	dir    string
	mu     sync.Mutex
	states map[string]string
}

func (d *localDisk) open(_ context.Context, key string) (io.ReadSeekCloser, time.Time, error) {
	file, err := os.Open(filepath.Join(d.dir, key))
	return file, time.Time{}, err
}

func (d *localDisk) Input(context.Context, string) (media.Input, error) {
	return media.Input{}, errors.New("no disc")
}

func (d *localDisk) Keyframes(context.Context, string) ([]time.Duration, error) {
	return nil, errors.New("no disc")
}

func (d *localDisk) Identity(_ context.Context, key string) (path, state string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.states[key]
	if !ok {
		return "", "", errors.New("gone")
	}
	return "folder/" + key, state, nil
}

func (d *localDisk) setState(key, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.states[key] = state
}

// pictureSetup is a playback service reading pictures with a fake reader,
// its settings changed by set, on a local disk.
type pictureSetup struct {
	*Service
	reader *fakeReader
	disk   *localDisk
	base   string
	set    func(change func(*accounts.Settings))
}

// newPictureService starts a service on pool reading pictures with
// reader, from the files of disk, with the setting on.
func newPictureService(t *testing.T, pool *pgxpool.Pool, reader *fakeReader, disk *localDisk, ffmpeg string) pictureSetup {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sources, err := source.New(t.TempDir(), func() int64 { return 1 << 30 }, &fakeSource{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sources.Close() })
	segments, err := hls.NewManager("ffmpeg", t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(segments.Close)
	var mu sync.Mutex
	settings := accounts.DefaultSettings()
	settings.SubtitleOCR = "played"
	s, err := New(pool, &fakeSource{}, "ffprobe", NewSigner([]byte("test secret")), sources, segments, nil, logger, func() accounts.Settings {
		mu.Lock()
		defer mu.Unlock()
		return settings
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	base := s.LocalFiles(disk.open)
	s.LocalInputs(disk)
	s.UseTesseract(ffmpeg, t.TempDir(), reader)
	return pictureSetup{Service: s, reader: reader, disk: disk, base: base, set: func(change func(*accounts.Settings)) {
		mu.Lock()
		defer mu.Unlock()
		change(&settings)
	}}
}

// waitReads waits for the reads queued to end.
func (p pictureSetup) waitReads(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		p.pictures.mu.Lock()
		idle := p.pictures.running == 0 && len(p.pictures.queue) == 0
		p.pictures.mu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the reads did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// pgsCaptions are two captions, shown from 1 to 3 seconds and from 5 to 7.
var pgsCaptions = testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: []testsubtitles.Caption{
	{Lines: []string{"The lighthouse keeper waited"}, Start: time.Second, End: 3 * time.Second},
	{Lines: []string{"Seven boats came home"}, Start: 5 * time.Second, End: 7 * time.Second},
}}

// sideFiles writes the PGS files of names to dir, each the captions, and
// lists them as files beside a local video, in English.
func sideFiles(t *testing.T, p pictureSetup, names ...string) []library.ExternalSubtitle {
	t.Helper()
	var files []library.ExternalSubtitle
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(p.disk.dir, name), pgsCaptions.PGS(), 0o600); err != nil {
			t.Fatal(err)
		}
		p.disk.setState(name, "100 1")
		files = append(files, library.ExternalSubtitle{URL: p.base + name, Format: "sup", Language: "eng", Forced: true})
	}
	return files
}

// videoAnalysis is an analysis of a Matroska video with a single track.
var videoAnalysis = media.Analysis{Format: "matroska,webm", Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}}}

func TestTextCopiesAreReadOnceAndAgainWhenTheirFileChanges(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, pool, &fakeReader{languages: []string{"eng"}}, disk, "ffmpeg")
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := sideFiles(t, p, "sub.sup")
	if copies := p.TextCopies(t.Context(), version, videoAnalysis, files); len(copies) != 0 {
		t.Fatalf("copies before any read: %+v", copies)
	}
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	// The copy follows the file and the video's track.
	if copies := p.TextCopies(t.Context(), version, videoAnalysis, files); !slices.Equal(copies, []TextCopy{{Original: 0, Index: 2}}) {
		t.Fatalf("copies: %+v", copies)
	}
	cues, ok := p.TextCopyCues(t.Context(), version, videoAnalysis, files, 2)
	if !ok || len(cues) != 2 || cues[0].Start != time.Second || cues[0].End != 3*time.Second || cues[1].Lines[0] != "line 2" {
		t.Fatalf("the copy's text: %+v %v", cues, ok)
	}
	// Played again, and after a restart, it is not read again.
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	// A service started again serves its files at another address, as
	// the scans list them again.
	restarted := newPictureService(t, pool, p.reader, disk, "ffmpeg")
	version.URL = restarted.base + "video.mkv"
	files = sideFiles(t, restarted, "sub.sup")
	if copies := restarted.TextCopies(t.Context(), version, videoAnalysis, files); len(copies) != 1 {
		t.Fatalf("copies after a restart: %+v", copies)
	}
	restarted.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	restarted.waitReads(t)
	if reads := p.reader.count(); reads != 1 {
		t.Fatalf("read %d times", reads)
	}
	// A file that changed is read again, its copy not listed meanwhile.
	disk.setState("sub.sup", "200 2")
	if copies := restarted.TextCopies(t.Context(), library.Version{ID: accounts.ID{2}, URL: version.URL}, videoAnalysis, files); len(copies) != 0 {
		t.Fatalf("copies of a changed file: %+v", copies)
	}
	restarted.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	restarted.waitReads(t)
	if reads := p.reader.count(); reads != 2 {
		t.Fatalf("read %d times once changed", reads)
	}
	if copies := restarted.TextCopies(t.Context(), version, videoAnalysis, files); len(copies) != 1 {
		t.Fatalf("copies once read again: %+v", copies)
	}
}

func TestTextCopiesNeedTheSetting(t *testing.T) {
	t.Parallel()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}}, disk, "ffmpeg")
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := sideFiles(t, p, "sub.sup")
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	p.set(func(s *accounts.Settings) { s.SubtitleOCR = "off" })
	if copies := p.TextCopies(t.Context(), version, videoAnalysis, files); len(copies) != 0 {
		t.Errorf("copies with the setting off: %+v", copies)
	}
	if _, ok := p.TextCopyCues(t.Context(), version, videoAnalysis, files, 2); ok {
		t.Error("a copy served with the setting off")
	}
	disk.setState("sub.sup", "200 2")
	p.QueueTextCopies(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	if reads := p.reader.count(); reads != 1 {
		t.Errorf("read %d times", reads)
	}
}

func TestLanguagesWithoutDataAreNotRead(t *testing.T) {
	t.Parallel()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng", "fra"}}, disk, "ffmpeg")
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := sideFiles(t, p, "a.sup", "b.sup", "c.sup")
	files[0].Language, files[1].Language, files[2].Language = "jpn", "fre", ""
	p.set(func(s *accounts.Settings) { s.Language = "fr" })
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	// French by its code, and by the server's language for the file of no
	// stated language.
	p.reader.mu.Lock()
	reads := slices.Clone(p.reader.reads)
	p.reader.mu.Unlock()
	if !slices.Equal(reads, []string{"fra", "fra"}) {
		t.Errorf("read in %v", reads)
	}
	if missing := p.MissingLanguages(t.Context()); !slices.Equal(missing, []string{"jpn"}) {
		t.Errorf("missing languages: %v", missing)
	}
	if copies := p.TextCopies(t.Context(), version, videoAnalysis, files); len(copies) != 2 {
		t.Errorf("copies: %+v", copies)
	}
	// Once its data is there, the language is no longer missing.
	p.reader.languages = append(p.reader.languages, "jpn")
	if missing := p.MissingLanguages(t.Context()); len(missing) != 0 {
		t.Errorf("missing languages with the data: %v", missing)
	}
}

func TestNoMoreTracksThanTheSettingAllowsAreReadAtOnce(t *testing.T) {
	t.Parallel()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	reader := &fakeReader{languages: []string{"eng"}, hold: make(chan struct{}), started: make(chan struct{})}
	p := newPictureService(t, migrated.New(t), reader, disk, "ffmpeg")
	p.set(func(s *accounts.Settings) { s.SubtitleOCRTracks = 2 })
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := sideFiles(t, p, "a.sup", "b.sup", "c.sup", "d.sup")
	p.queueTracks(t.Context(), version, videoAnalysis, files, 2)
	// Two reads start; the others wait while those are held.
	<-reader.started
	<-reader.started
	p.pictures.mu.Lock()
	running := p.pictures.running
	var queued []int
	for _, job := range p.pictures.queue {
		queued = append(queued, job.track.index)
	}
	p.pictures.mu.Unlock()
	// The chosen file was read first, with the first other one.
	if running != 2 || !slices.Equal(queued, []int{1, 3}) {
		t.Errorf("%d reads running, files %v queued", running, queued)
	}
	for range 2 {
		reader.hold <- struct{}{}
		<-reader.started
	}
	reader.hold <- struct{}{}
	reader.hold <- struct{}{}
	p.waitReads(t)
	if reader.most != 2 || reader.count() != 4 {
		t.Errorf("%d reads, %d at most at once", reader.count(), reader.most)
	}
}

// testTools are POLYFIN_TEST_FFMPEG, with its ffprobe, and the Tesseract
// of POLYFIN_TEST_TESSERACT, with the data of POLYFIN_TEST_TESSDATA.
func testTools(t *testing.T) (ffmpeg, ffprobe string, reader *ocr.Tesseract) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	tesseract := os.Getenv("POLYFIN_TEST_TESSERACT")
	if ffmpeg == "" || tesseract == "" {
		t.Skip("POLYFIN_TEST_FFMPEG or POLYFIN_TEST_TESSERACT is not set")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), &ocr.Tesseract{Path: tesseract, DataDir: os.Getenv("POLYFIN_TEST_TESSDATA"), TempDir: t.TempDir()}
}

// TestPictureTracksAreReadIntoText reads a PGS track inside a Matroska
// file, a VobSub pair beside it, and a DVD's VobSub track, inside an MPEG
// program stream as a disc's cells are, into the captions' text and times.
func TestPictureTracksAreReadIntoText(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe, tesseract := testTools(t)
	dir := t.TempDir()
	disk := &localDisk{dir: dir, states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), nil, disk, ffmpeg)
	p.UseTesseract(ffmpeg, t.TempDir(), tesseract)
	dvd := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 22, Shown: pgsCaptions.Shown}
	idx, sub := dvd.VobSub("en")
	for name, data := range map[string][]byte{"captions.sup": pgsCaptions.PGS(), "pair.idx": []byte(idx), "pair.sub": sub} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(ffmpeg, append([]string{"-hide_banner", "-loglevel", "error"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v: %s", err, out)
		}
	}
	// The captions keep their times: -copyts, as their file starts with
	// the first.
	run("-copyts", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-i", filepath.Join(dir, "captions.sup"), "-t", "8", "-map", "0", "-map", "1",
		"-c:v", "libx264", "-preset", "veryfast", "-c:s", "copy", "-metadata:s:s:0", "language=eng", filepath.Join(dir, "movie.mkv"))
	run("-copyts", "-f", "lavfi", "-i", "testsrc2=size=720x480:rate=25", "-i", filepath.Join(dir, "pair.idx"), "-t", "8", "-map", "0", "-map", "1",
		"-c:v", "mpeg2video", "-c:s", "copy", "-f", "vob", filepath.Join(dir, "disc.vob"))
	for _, c := range []struct {
		name  string
		files []library.ExternalSubtitle
		video string
	}{
		{"a PGS track", nil, "movie.mkv"},
		{"a VobSub pair", []library.ExternalSubtitle{{URL: p.base + "pair.idx", Format: "idx", Language: "eng"}}, "movie.mkv"},
		{"a DVD's track", nil, "disc.vob"},
	} {
		t.Run(c.name, func(t *testing.T) {
			analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), filepath.Join(dir, c.video))
			if err != nil {
				t.Fatal(err)
			}
			// The source's start, as ffprobe tells it.
			out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=start_time", "-of", "csv=p=0", filepath.Join(dir, c.video)).Output()
			if err != nil {
				t.Fatal(err)
			}
			seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
			if err != nil {
				t.Fatal(err)
			}
			offset := time.Duration(seconds * float64(time.Second))
			if c.files != nil {
				offset = 0
			}
			if c.files != nil {
				// Only the pair: the video's own track is another case.
				analysis.Streams = analysis.Streams[:1]
			}
			disk.setState(c.video, "1 1")
			disk.setState("pair.idx", "1 1")
			version := library.Version{ID: accounts.ID{byte(len(c.name))}, URL: p.base + c.video}
			p.queueTracks(t.Context(), version, analysis, c.files, -1)
			p.waitReads(t)
			copies := p.TextCopies(t.Context(), version, analysis, c.files)
			if len(copies) != 1 {
				t.Fatalf("copies: %+v", copies)
			}
			cues, _ := p.TextCopyCues(t.Context(), version, analysis, c.files, copies[0].Index)
			if len(cues) != 2 {
				t.Fatalf("cues: %+v", cues)
			}
			for i, cue := range cues {
				want := pgsCaptions.Shown[i]
				if got := strings.ToLower(strings.Join(cue.Lines, " ")); got != strings.ToLower(want.Lines[0]) {
					t.Errorf("cue %d reads %q, want %q", i, got, want.Lines[0])
				}
				// A DVD's times count in 1024/90000 s; a program stream's
				// start late, by what its muxer gave its first pack, which
				// the source's timestamps keep, as remuxes do.
				start, end := want.Start+offset, want.End+offset
				if d := cue.Start - start; d < -50*time.Millisecond || d > 50*time.Millisecond {
					t.Errorf("cue %d starts at %v, want %v", i, cue.Start, start)
				}
				if d := cue.End - end; d < -50*time.Millisecond || d > 50*time.Millisecond {
					t.Errorf("cue %d ends at %v, want %v", i, cue.End, end)
				}
			}
		})
	}
}
