package playback

import (
	"context"
	"encoding/binary"
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
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// fakeReader reads every picture as "line N", N its place in the pictures
// read at once, with the languages it has data for. With hold set, each
// read waits for a value from it, having told started.
type fakeReader struct {
	languages []string
	hold      chan struct{}
	started   chan struct{}

	mu            sync.Mutex
	reads         []string
	running, most int
	// held are the bytes of the pictures of each read.
	held []int
}

func (f *fakeReader) Languages(context.Context) ([]string, error) { return f.languages, nil }

func (f *fakeReader) Read(_ context.Context, pictures []ocr.Picture, language string) ([]subtitles.Cue, error) {
	held := 0
	for _, p := range pictures {
		held += len(p.Image.Pix)
	}
	f.mu.Lock()
	f.reads = append(f.reads, language)
	f.held = append(f.held, held)
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
// their states told by states, which a test changes, and the inputs of
// discs by theirs.
type localDisk struct {
	dir    string
	mu     sync.Mutex
	states map[string]string
	discs  map[string]media.Input
}

func (d *localDisk) open(_ context.Context, key string) (io.ReadSeekCloser, time.Time, error) {
	file, err := os.Open(filepath.Join(d.dir, key))
	return file, time.Time{}, err
}

func (d *localDisk) Input(_ context.Context, key string) (media.Input, error) {
	if in, ok := d.discs[key]; ok {
		return in, nil
	}
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
	sources, err := source.New(t.TempDir(), func() source.Policy { return source.Policy{Limit: 1 << 30} }, &fakeSource{}, logger)
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
// file, a VobSub pair beside it, and a VobSub track inside a loose VOB, an
// MPEG program stream as a DVD's cells are, into the captions' text and
// times. The loose VOB plays over HLS from zero, its first frame's time
// (see playback.unindexedFormat): its text counts from there too.
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
		{"a loose VOB's track", nil, "disc.vob"},
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
			if c.files != nil || c.video == "disc.vob" {
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
				// A DVD's times count in 1024/90000 s. A Matroska file's
				// remuxes keep its timestamps; a loose VOB's start at zero.
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

// opaqueSUP is a SUP file showing n times, a second apart from zero, the
// same picture: the whole of a 1920 by 1080 video, opaque.
func opaqueSUP(n int) []byte {
	segment := func(kind byte, at time.Duration, payload []byte) []byte {
		header := make([]byte, 13, 13+len(payload))
		header[0], header[1], header[10] = 'P', 'G', kind
		binary.BigEndian.PutUint32(header[2:6], uint32(at*90000/time.Second))
		binary.BigEndian.PutUint16(header[11:13], uint16(len(payload)))
		return append(header, payload...)
	}
	// 1920 by 1080, starting an epoch the first time, placing object 0 at
	// 0, 0 through palette 0.
	composition := func(epoch bool) []byte {
		data := []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
		if epoch {
			data[7] = 0x80
		}
		return data
	}
	// Object 0, first and last of its sequence, 1920 by 1080, each line a
	// run of color 1, opaque white.
	object := []byte{0, 0, 0, 0xc0, 0, 0, 0, 0x07, 0x80, 0x04, 0x38}
	for range 1080 {
		object = append(object, 0, 0xc0|1920>>8, 1920&0xff, 1, 0, 0)
	}
	var sup []byte
	for i := range n {
		at := time.Duration(i) * time.Second
		sup = append(sup, segment(0x16, at, composition(i == 0))...)
		if i == 0 {
			sup = append(sup, segment(0x14, at, []byte{0, 0, 1, 235, 128, 128, 255})...)
			sup = append(sup, segment(0x15, at, object)...)
		}
		sup = append(sup, segment(0x80, at, nil)...)
	}
	return sup
}

// A track's pictures are read by batches, each dropped once read: no more
// than ocrBatchBytes of them are held at once, however many the track
// shows, and the text keeps them all, in order.
func TestPicturesAreReadByBatches(t *testing.T) {
	t.Parallel()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}}, disk, "ffmpeg")
	// 40 pictures of some 2 MB each.
	const shown = 40
	if err := os.WriteFile(filepath.Join(disk.dir, "sub.sup"), opaqueSUP(shown), 0o600); err != nil {
		t.Fatal(err)
	}
	disk.setState("sub.sup", "1 1")
	files := []library.ExternalSubtitle{{URL: p.base + "sub.sup", Format: "sup", Language: "eng"}}
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	p.reader.mu.Lock()
	held := slices.Clone(p.reader.held)
	p.reader.mu.Unlock()
	total := 0
	for _, bytes := range held {
		total += bytes
		if bytes > ocrBatchBytes {
			t.Errorf("a read of %d MB of pictures", bytes>>20)
		}
	}
	if len(held) < 2 || total <= ocrBatchBytes {
		t.Errorf("%d reads of %d MB in all", len(held), total>>20)
	}
	cues, ok := p.TextCopyCues(t.Context(), version, videoAnalysis, files, 2)
	if !ok || len(cues) != shown {
		t.Fatalf("%d cues kept, %v", len(cues), ok)
	}
	for i, cue := range cues {
		if cue.Start != time.Duration(i)*time.Second {
			t.Errorf("cue %d starts at %v", i, cue.Start)
		}
	}
}

// The text copy of a track inside an MPEG-TS file, as a Blu-ray's .m2ts
// file is, starts at zero as the file's remuxes do, rather than at the
// file's first timestamp.
func TestTextCopiesOfTransportStreamsStartAtZero(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	disk := &localDisk{dir: dir, states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}}, disk, ffmpeg)
	if err := os.WriteFile(filepath.Join(dir, "captions.sup"), pgsCaptions.PGS(), 0o600); err != nil {
		t.Fatal(err)
	}
	// Its timestamps start 600 seconds in, as a Blu-ray's do; the captions
	// keep their times from its first frame (-copyts).
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-i", filepath.Join(dir, "captions.sup"), "-t", "8", "-map", "0", "-map", "1", "-c:v", "libx264", "-preset", "veryfast",
		"-c:s", "copy", "-metadata:s:s:0", "language=eng", "-output_ts_offset", "600", "-f", "mpegts", filepath.Join(dir, "movie.m2ts")).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	analysis, err := media.Prober{Path: filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), Timeout: time.Minute}.Probe(t.Context(), filepath.Join(dir, "movie.m2ts"))
	if err != nil {
		t.Fatal(err)
	}
	disk.setState("movie.m2ts", "1 1")
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "movie.m2ts"}
	p.queueTracks(t.Context(), version, analysis, nil, -1)
	p.waitReads(t)
	copies := p.TextCopies(t.Context(), version, analysis, nil)
	if len(copies) != 1 {
		t.Fatalf("copies: %+v", copies)
	}
	cues, _ := p.TextCopyCues(t.Context(), version, analysis, nil, copies[0].Index)
	if len(cues) != len(pgsCaptions.Shown) {
		t.Fatalf("cues: %+v", cues)
	}
	for i, cue := range cues {
		want := pgsCaptions.Shown[i]
		if d := cue.Start - want.Start; d < -100*time.Millisecond || d > 100*time.Millisecond {
			t.Errorf("cue %d starts at %v, want %v", i, cue.Start, want.Start)
		}
		if d := cue.End - want.End; d < -100*time.Millisecond || d > 100*time.Millisecond {
			t.Errorf("cue %d ends at %v, want %v", i, cue.End, want.End)
		}
	}
}

// The text copy of a VobSub track inside an MPEG program stream lines up
// with the video its HLS output plays, whose timestamps it starts at the
// first frame's: the frame the HLS output shows at each cue's start is the
// one the source shows with the caption's picture. So for a loose VOB,
// played as a file, and for a DVD's title on a share, read from its input
// as one program stream.
func TestTextCopiesOfProgramStreamsMatchTheirHLSVideo(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	disk := &localDisk{dir: dir, states: map[string]string{}, discs: map[string]media.Input{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}}, disk, ffmpeg)
	idx, sub := testsubtitles.Captions{CanvasWidth: 720, CanvasHeight: 480, Size: 22, Shown: pgsCaptions.Shown}.VobSub("en")
	for name, data := range map[string][]byte{"pair.idx": []byte(idx), "pair.sub": sub} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	vob := filepath.Join(dir, "disc.vob")
	// testsrc2 draws each frame apart; the muxer starts the stream late,
	// its captions with it (-copyts).
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-f", "lavfi", "-i", "testsrc2=size=720x480:rate=25",
		"-i", filepath.Join(dir, "pair.idx"), "-t", "8", "-map", "0", "-map", "1", "-c:v", "mpeg2video", "-g", "12", "-c:s", "copy", "-f", "vob",
		vob).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), vob)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "s:0", "-show_entries", "packet=pts_time", "-of", "csv=p=0", vob).Output()
	if err != nil {
		t.Fatal(err)
	}
	var shown []float64
	for line := range strings.Lines(string(out)) {
		if at, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(line), ","), 64); err == nil {
			shown = append(shown, at)
		}
	}
	source := framesOf(t, ffmpeg, nil, vob)
	disk.setState("disc.vob", "1 1")
	disk.setState("title", "1 1")
	disk.discs["title"] = media.Input{Format: "mpeg", Path: vob}
	info, err := os.Stat(vob)
	if err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]library.Version{
		"a loose VOB":   {ID: accounts.ID{1}, URL: p.base + "disc.vob", Size: info.Size()},
		"a DVD's title": {ID: accounts.ID{2}, URL: p.LocalInputs(disk) + "title"},
	} {
		t.Run(name, func(t *testing.T) {
			p.analyses.Put(version.ID, analysis)
			// The test's sources serve no bytes: the program stream's lack of
			// an index is known.
			p.unindexed.Put(version.ID, container.ErrNoIndex)
			p.queueTracks(t.Context(), version, analysis, nil, -1)
			p.waitReads(t)
			copies := p.TextCopies(t.Context(), version, analysis, nil)
			if len(copies) != 1 {
				t.Fatalf("copies: %+v", copies)
			}
			cues, _ := p.TextCopyCues(t.Context(), version, analysis, nil, copies[0].Index)
			if len(cues) != len(shown) {
				t.Fatalf("cues %+v, shown at %v", cues, shown)
			}
			convert := &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 720, Height: 480, Bitrate: 2_000_000}
			r, release, err := p.remuxOpener(Remux{Session: name, Version: version, Audio: -1, Format: hls.TS, ConvertVideo: convert})(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !r.Plan.Grid() || !r.Plan.IsSearched() {
				t.Errorf("the remux's plan: grid %v, searched %v", r.Plan.Grid(), r.Plan.IsSearched())
			}
			// The remux's options, on the file itself, which a loose VOB's
			// remux reads through the source cache.
			played := framesOf(t, ffmpeg, r.InputOptions, vob)
			release()
			for i, cue := range cues {
				if got, want := frameAt(played, cue.Start.Seconds()), frameAt(source, shown[i]); got == "" || got != want {
					t.Errorf("cue %d at %v: the remux shows frame %q, the source %q with its picture at %.3f s", i, cue.Start, got, want, shown[i])
				}
			}
		})
	}
}

// decodedFrame is a video frame FFmpeg decodes: its time and its checksum.
type decodedFrame struct {
	at  float64
	sum string
}

// framesOf are the video frames FFmpeg decodes of input read with options,
// keeping their timestamps, as a remux does.
func framesOf(t *testing.T, ffmpeg string, options []string, input string) []decodedFrame {
	t.Helper()
	args := slices.Concat([]string{"-hide_banner", "-nostdin"}, options, []string{"-copyts", "-i", input, "-map", "0:v:0", "-vf", "showinfo", "-f", "null", "-"})
	out, err := exec.Command(ffmpeg, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	var frames []decodedFrame
	for line := range strings.Lines(string(out)) {
		_, at, ok := strings.Cut(line, " pts_time:")
		_, sum, found := strings.Cut(line, " checksum:")
		if !ok || !found {
			continue
		}
		seconds, err := strconv.ParseFloat(strings.Fields(at)[0], 64)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, decodedFrame{at: seconds, sum: strings.Fields(sum)[0]})
	}
	if len(frames) == 0 {
		t.Fatalf("no frame decoded: %s", out)
	}
	return frames
}

// frameAt is the checksum of the frame shown at a time: the last one
// starting by then, within a millisecond.
func frameAt(frames []decodedFrame, at float64) string {
	sum := ""
	for _, f := range frames {
		if f.at <= at+0.001 {
			sum = f.sum
		}
	}
	return sum
}
