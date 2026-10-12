package audiosegments

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/mediasegments"
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

// introLength is the length of the intro the episodes share.
const introLength = 40 * time.Second

// makeIntro writes, in dir, a 40-second intro: two melodies over a soft
// noise.
func makeIntro(t *testing.T, ffmpeg, dir string) string {
	t.Helper()
	intro := filepath.Join(dir, "intro.wav")
	run(t, ffmpeg, "-f", "lavfi", "-i",
		"aevalsrc='0.3*sin(2*PI*t*(330+110*floor(mod(t*2,5))))+0.2*sin(2*PI*t*(523+130*floor(mod(t*3,4))))':s=22050:d=40",
		"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=7:a=0.1:d=40:r=22050",
		"-filter_complex", "[0][1]amix=inputs=2:normalize=0", intro)
	return intro
}

// makeEpisode writes, in dir, a 4-minute episode named name, of noise
// made from seed, with the intro from at when intro is not empty.
func makeEpisode(t *testing.T, ffmpeg, dir, name, intro string, at time.Duration, seed int) string {
	t.Helper()
	file := filepath.Join(dir, name)
	const length = 240 * time.Second
	if intro == "" {
		run(t, ffmpeg, "-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%s:r=22050", seed, seconds(length)),
			"-c:a", "aac", file)
		return file
	}
	run(t, ffmpeg, "-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=brown:seed=%d:a=0.3:d=%s:r=22050", seed, seconds(at)),
		"-i", intro,
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%s:r=22050", seed+100, seconds(length-at-introLength)),
		"-filter_complex", "[0][1][2]concat=n=3:v=0:a=1", "-c:a", "aac", file)
	return file
}

// creditsLength is the length of the credits the episodes share, and
// endingLength that of the episodes ending with them: their last quarter,
// 75 seconds, holds the credits and a scene after them.
const (
	creditsLength = 60 * time.Second
	endingLength  = 300 * time.Second
)

// makeCredits writes, in dir, 60-second credits: a melody over a soft
// noise, unlike the intro's.
func makeCredits(t *testing.T, ffmpeg, dir string) string {
	t.Helper()
	credits := filepath.Join(dir, "credits.wav")
	run(t, ffmpeg, "-f", "lavfi", "-i",
		"aevalsrc='0.35*sin(2*PI*t*(220+55*floor(mod(t*1.5,6))))+0.15*sin(2*PI*t*(660+90*floor(mod(t*2.5,3))))':s=22050:d=60",
		"-f", "lavfi", "-i", "anoisesrc=color=white:seed=9:a=0.05:d=60:r=22050",
		"-filter_complex", "[0][1]amix=inputs=2:normalize=0", credits)
	return credits
}

// makeEnding writes, in dir, a 5-minute episode named name, of noise made
// from seed, ending with the credits when credits is not empty, followed
// by a scene lasting scene.
func makeEnding(t *testing.T, ffmpeg, dir, name, credits string, scene time.Duration, seed int) string {
	t.Helper()
	file := filepath.Join(dir, name)
	if credits == "" {
		run(t, ffmpeg, "-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%s:r=22050", seed, seconds(endingLength)),
			"-c:a", "aac", file)
		return file
	}
	args := []string{"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=brown:seed=%d:a=0.3:d=%s:r=22050", seed, seconds(endingLength-creditsLength-scene)),
		"-i", credits}
	parts := "[0][1]concat=n=2:v=0:a=1"
	if scene > 0 {
		args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%s:r=22050", seed+100, seconds(scene)))
		parts = "[0][1][2]concat=n=3:v=0:a=1"
	}
	run(t, ffmpeg, append(args, "-filter_complex", parts, "-c:a", "aac", file)...)
	return file
}

func run(t *testing.T, ffmpeg string, args ...string) {
	t.Helper()
	out, err := exec.Command(ffmpeg, append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

// season stands in for the local folders: the files of one season, read
// at their paths.
type season struct {
	mu    sync.Mutex
	files []localfiles.EpisodeFile
}

// add adds the file at path, of episode number, as it is on the disk.
func (s *season) add(t *testing.T, path string, number int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, localfiles.EpisodeFile{Path: path, URL: path, Size: info.Size(), Modified: info.ModTime().Truncate(time.Microsecond),
		Title: "tt1", Season: 1, First: number, Last: number})
}

// touch makes the file at path look changed.
func (s *season) touch(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.files {
		if s.files[i].Path == path {
			s.files[i].Modified = s.files[i].Modified.Add(time.Second)
		}
	}
}

func (s *season) EpisodeFile(_ context.Context, address string) (localfiles.EpisodeFile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.files, func(f localfiles.EpisodeFile) bool { return f.URL == address })
	if i < 0 {
		return localfiles.EpisodeFile{}, false, nil
	}
	return s.files[i], true, nil
}

func (s *season) SeasonFiles(context.Context, localfiles.EpisodeFile) ([]localfiles.EpisodeFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.files), nil
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

func newService(t *testing.T, files Files) (*Service, *settings) {
	t.Helper()
	ffmpeg, ffprobe := testFFmpeg(t)
	current := &settings{value: accounts.DefaultSettings()}
	s := New(Options{DB: migrated.New(t), FFmpeg: ffmpeg, FFprobe: ffprobe, Files: files, Settings: current.get,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(s.Close)
	return s, current
}

// compareSeason compares the season of the file at path for what l looks
// for, as its play would in the background.
func compareSeason(t *testing.T, s *Service, files *season, path string, l look) {
	t.Helper()
	file, ok, _ := files.EpisodeFile(t.Context(), path)
	if !ok {
		t.Fatalf("no file %s", path)
	}
	if err := s.compare(t.Context(), file, l); err != nil {
		t.Fatal(err)
	}
}

// checkSegment checks the segment of type kind found in the file at path
// runs from at for length, to the second; none for at below 0.
func checkSegment(t *testing.T, s *Service, path string, kind mediasegments.Type, at, length time.Duration) {
	t.Helper()
	found := slices.DeleteFunc(s.Segments(t.Context(), path), func(segment mediasegments.Segment) bool { return segment.Type != kind })
	switch {
	case at < 0 && len(found) > 0:
		t.Errorf("%s: %s %v", filepath.Base(path), kind, found)
	case at < 0:
	case len(found) != 1:
		t.Errorf("%s: %v, want one %s at %v", filepath.Base(path), found, kind, at)
	case (found[0].Start-at).Abs() > time.Second/2 || (found[0].End-at-length).Abs() > time.Second/2:
		t.Errorf("%s: %s %v to %v, want %v to %v", filepath.Base(path), kind, found[0].Start, found[0].End, at, at+length)
	}
}

// checkIntro checks the intro of the file at path starts at at, to the
// second; none for at below 0.
func checkIntro(t *testing.T, s *Service, path string, at time.Duration) {
	t.Helper()
	checkSegment(t, s, path, mediasegments.Intro, at, introLength)
}

// Episodes sharing an intro at different times are given it, to the
// second; one sharing nothing gets none. Two files of the same episode,
// which share all their audio, are not compared with each other.
func TestEpisodesSharingAnIntroAreGivenIt(t *testing.T) {
	t.Parallel()
	files := &season{}
	s, _ := newService(t, files)
	dir := t.TempDir()
	intro := makeIntro(t, s.FFmpeg, dir)
	starts := map[string]time.Duration{}
	for i, at := range []time.Duration{4 * time.Second, 11500 * time.Millisecond, 18750 * time.Millisecond} {
		path := makeEpisode(t, s.FFmpeg, dir, fmt.Sprintf("S01E0%d.mka", i+1), intro, at, i+1)
		files.add(t, path, i+1)
		starts[path] = at
	}
	copied := filepath.Join(dir, "S01E01 copy.mka")
	run(t, s.FFmpeg, "-i", filepath.Join(dir, "S01E01.mka"), "-c:a", "aac", "-b:a", "96k", copied)
	files.add(t, copied, 1)
	starts[copied] = starts[filepath.Join(dir, "S01E01.mka")]
	unrelated := makeEpisode(t, s.FFmpeg, dir, "S01E04.mka", "", 0, 4)
	files.add(t, unrelated, 4)
	starts[unrelated] = -1

	compareSeason(t, s, files, filepath.Join(dir, "S01E02.mka"), intros)
	for path, at := range starts {
		checkIntro(t, s, path, at)
	}
}

// Episodes ending with the same credits are given them, to the second,
// ending where the credits end: the scene after them in some is left out.
// One sharing no end gets none.
func TestEpisodesSharingCreditsAreGivenThem(t *testing.T) {
	t.Parallel()
	files := &season{}
	s, _ := newService(t, files)
	dir := t.TempDir()
	music := makeCredits(t, s.FFmpeg, dir)
	starts := map[string]time.Duration{}
	for i, scene := range []time.Duration{0, 12500 * time.Millisecond, 5 * time.Second} {
		path := makeEnding(t, s.FFmpeg, dir, fmt.Sprintf("S01E0%d.mka", i+1), music, scene, i+1)
		files.add(t, path, i+1)
		starts[path] = endingLength - creditsLength - scene
	}
	unrelated := makeEnding(t, s.FFmpeg, dir, "S01E04.mka", "", 0, 4)
	files.add(t, unrelated, 4)
	starts[unrelated] = -1

	compareSeason(t, s, files, unrelated, credits)
	for path, at := range starts {
		checkSegment(t, s, path, mediasegments.Outro, at, creditsLength)
	}
}

// A file is read once: its fingerprint is kept until its size or
// modification time changes, and the intro found in it is no longer
// given once it changed.
func TestFilesAreReadAgainOnlyOnceTheyChange(t *testing.T) {
	t.Parallel()
	files := &season{}
	s, _ := newService(t, files)
	dir := t.TempDir()
	intro := makeIntro(t, s.FFmpeg, dir)
	first := makeEpisode(t, s.FFmpeg, dir, "S01E01.mka", intro, 6*time.Second, 1)
	second := makeEpisode(t, s.FFmpeg, dir, "S01E02.mka", intro, 15*time.Second, 2)
	files.add(t, first, 1)
	files.add(t, second, 2)
	compareSeason(t, s, files, first, intros)
	checkIntro(t, s, second, 15*time.Second)

	// Its audio replaced, the file still looks the same: what was read
	// of it holds.
	makeEpisode(t, s.FFmpeg, dir, "S01E02.mka", "", 0, 5)
	compareSeason(t, s, files, first, intros)
	checkIntro(t, s, first, 6*time.Second)
	checkIntro(t, s, second, 15*time.Second)

	// Once it looks changed, its intro is gone, and it is read again.
	files.touch(second)
	checkIntro(t, s, second, -1)
	compareSeason(t, s, files, first, intros)
	checkIntro(t, s, first, -1)
	checkIntro(t, s, second, -1)
}

// The intros found are given only while the skip buttons and the search
// in local episodes' audio are both on.
func TestSettingsTurnAudioIntrosOff(t *testing.T) {
	t.Parallel()
	files := &season{}
	s, current := newService(t, files)
	dir := t.TempDir()
	intro := makeIntro(t, s.FFmpeg, dir)
	first := makeEpisode(t, s.FFmpeg, dir, "S01E01.mka", intro, 6*time.Second, 1)
	second := makeEpisode(t, s.FFmpeg, dir, "S01E02.mka", intro, 15*time.Second, 2)
	files.add(t, first, 1)
	files.add(t, second, 2)
	compareSeason(t, s, files, first, intros)

	current.set(func(s *accounts.Settings) { s.AudioIntros = false })
	checkIntro(t, s, first, -1)
	current.set(func(s *accounts.Settings) { s.AudioIntros, s.SkipButtons = true, false })
	checkIntro(t, s, first, -1)
	current.set(func(s *accounts.Settings) { s.SkipButtons = true })
	checkIntro(t, s, first, 6*time.Second)
}
