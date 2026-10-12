package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/audiosegments"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ffmpegRun runs FFmpeg, failing the test when it fails.
func ffmpegRun(t *testing.T, ffmpeg string, args ...string) {
	t.Helper()
	if out, err := exec.Command(ffmpeg, append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

// introEpisode writes a 4-minute episode at path: a gray picture, and noise
// made from seed but for the 40-second intro, from at, with the chapters of
// chapters when not empty, an FFmpeg metadata file.
func introEpisode(t *testing.T, ffmpeg, path, intro string, at time.Duration, seed int, chapters string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	after := 200*time.Second - at
	args := []string{
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=brown:seed=%d:a=0.3:d=%.3f:r=22050", seed, at.Seconds()),
		"-i", intro,
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%.3f:r=22050", seed+100, after.Seconds()),
		"-f", "lavfi", "-i", "color=c=gray:s=64x36:r=2:d=240",
	}
	if chapters != "" {
		args = append(args, "-f", "ffmetadata", "-i", chapters, "-map_chapters", "4")
	}
	args = append(args, "-filter_complex", "[0][1][2]concat=n=3:v=0:a=1[a]", "-map", "3:v", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", path)
	ffmpegRun(t, ffmpeg, args...)
}

// audioSeason is a shows folder holding a season of the series the fake
// segment databases know, with the search in local episodes' audio on.
type audioSeason struct {
	testServer
	token    string
	user     accounts.User
	episodes map[int]string
}

// newAudioSeason serves the episodes write writes in dir, under
// Show/Season 01, once matched and linked to the series; there must be
// count of them.
func newAudioSeason(t *testing.T, ffmpeg string, count int, write func(dir string)) audioSeason {
	t.Helper()
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	databases := newSegmentDatabases(t)
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		o.Segments = mediasegments.New(pool, []mediasegments.Source{{Name: mediasegments.TheIntroDB, URL: databases.url}}, "test", logger,
			o.Accounts.Settings)
		o.AudioSegments = audiosegments.New(audiosegments.Options{DB: pool, FFmpeg: ffmpeg, FFprobe: ffprobe, Files: folders,
			Settings: o.Accounts.Settings, Logger: logger})
		t.Cleanup(o.AudioSegments.Close)
	})
	a := audioSeason{testServer: s, user: s.user("member", nil), token: s.signIn("member", "tv"), episodes: map[int]string{}}
	dir := t.TempDir()
	write(dir)
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shows", Path: dir, Kind: localfiles.KindShows})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Show", "tt7654321"); err != nil {
		t.Fatal(err)
	}
	var views QueryResult
	s.get(t, "/UserViews", a.token, &views)
	var shows QueryResult
	if len(views.Items) != 1 || s.get(t, "/Items?ParentId="+views.Items[0].Id, a.token, &shows) != http.StatusOK || len(shows.Items) != 1 {
		t.Fatalf("views %+v, shows %+v", views.Items, shows.Items)
	}
	var list QueryResult
	s.get(t, "/Shows/"+shows.Items[0].Id+"/Episodes?UserId="+a.user.ID.String(), a.token, &list)
	for _, episode := range list.Items {
		if episode.IndexNumber != nil {
			a.episodes[*episode.IndexNumber] = episode.Id
		}
	}
	if len(a.episodes) != count {
		t.Fatalf("episodes: %+v", list.Items)
	}
	return a
}

// playbackInfo has the version of an episode given, analyzing it, its
// chapters included, and returns it.
func (a audioSeason) playbackInfo(t *testing.T, number int) MediaSourceInfo {
	t.Helper()
	status, data := a.call(http.MethodPost, "/Items/"+a.episodes[number]+"/PlaybackInfo", app("tv", a.token),
		map[string]any{"UserId": a.user.ID.String()})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return info.MediaSources[0]
}

// play starts playing an episode, as an app does after PlaybackInfo.
func (a audioSeason) play(t *testing.T, number int) {
	t.Helper()
	source := a.playbackInfo(t, number).Id
	if status, data := a.call(http.MethodPost, "/Sessions/Playing", app("tv", a.token),
		map[string]any{"ItemId": a.episodes[number], "MediaSourceId": source, "PositionTicks": 0}); status != http.StatusNoContent {
		t.Fatalf("playing: %d %s", status, data)
	}
}

// segments lists an episode's segments of type kind.
func (a audioSeason) segments(t *testing.T, number int, kind string) []span {
	t.Helper()
	var result MediaSegmentDtoQueryResult
	a.get(t, "/MediaSegments/"+a.episodes[number]+"?includeSegmentTypes="+kind, a.token, &result)
	return spans(result)
}

// waitSegments waits, two minutes at most, for an episode to have a
// segment of type kind.
func (a audioSeason) waitSegments(t *testing.T, number int, kind string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for len(a.segments(t, number, kind)) == 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
}

// near reports whether got is a single segment from start to end, to the
// second.
func near(got []span, start, end time.Duration) bool {
	const tick = 100 * time.Nanosecond
	return len(got) == 1 && (time.Duration(got[0].Start)*tick-start).Abs() <= time.Second/2 &&
		(time.Duration(got[0].End)*tick-end).Abs() <= time.Second/2
}

// Local episodes played without an intro have their season compared in
// the background: the episodes sharing a 40-second intro at different
// times are given it, to the second, from their audio, but for those whose
// intro the databases or their chapters give, which come first.
func TestLocalEpisodesShareTheirIntro(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	// The databases give the second episode an intro, from 60 to 95
	// seconds, and the third one from 321.4 seconds, which its chapters'
	// replace.
	starts := map[int]time.Duration{1: 4 * time.Second, 2: 11500 * time.Millisecond, 3: 15 * time.Second, 5: 18750 * time.Millisecond}
	a := newAudioSeason(t, ffmpeg, 4, func(dir string) {
		intro := filepath.Join(t.TempDir(), "intro.wav")
		ffmpegRun(t, ffmpeg, "-f", "lavfi", "-i",
			"aevalsrc='0.3*sin(2*PI*t*(330+110*floor(mod(t*2,5))))+0.2*sin(2*PI*t*(523+130*floor(mod(t*3,4))))':s=22050:d=40",
			"-f", "lavfi", "-i", "anoisesrc=color=pink:seed=7:a=0.1:d=40:r=22050",
			"-filter_complex", "[0][1]amix=inputs=2:normalize=0", intro)
		chapters := filepath.Join(t.TempDir(), "chapters.txt")
		if err := os.WriteFile(chapters, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=30000\ntitle=Chapter 1\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=30000\nEND=50000\ntitle=Opening\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=50000\nEND=240000\ntitle=Chapter 3\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for number, at := range starts {
			withChapters := ""
			if number == 3 {
				withChapters = chapters
			}
			introEpisode(t, ffmpeg, filepath.Join(dir, "Show", "Season 01", fmt.Sprintf("Show - S01E0%d.mkv", number)), intro, at, number,
				withChapters)
		}
	})
	a.playbackInfo(t, 3)
	if got := a.segments(t, 1, "Intro"); len(got) != 0 {
		t.Errorf("an intro before any play: %v", got)
	}
	a.play(t, 1)
	a.waitSegments(t, 5, "Intro")
	for _, number := range []int{1, 5} {
		if got := a.segments(t, number, "Intro"); !near(got, starts[number], starts[number]+40*time.Second) {
			t.Errorf("episode %d: %v, want an intro from %v", number, got, starts[number])
		}
	}
	if got := a.segments(t, 2, "Intro"); !slices.Equal(got, []span{spanMS("Intro", 60_000, 95_000)}) {
		t.Errorf("episode 2: %v, want the databases' intro", got)
	}
	if got := a.segments(t, 3, "Intro"); !slices.Equal(got, []span{spanMS("Intro", 30_000, 50_000)}) {
		t.Errorf("episode 3: %v, want its chapters' intro", got)
	}
}

// creditsEpisode writes a 5-minute episode at path: a gray picture, and
// noise made from seed but for the 60-second credits, followed by a scene
// lasting scene, with the chapters of chapters when not empty, an FFmpeg
// metadata file.
func creditsEpisode(t *testing.T, ffmpeg, path, credits string, scene time.Duration, seed int, chapters string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"-f", "lavfi", "-i", "color=c=gray:s=64x36:r=2:d=300",
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=brown:seed=%d:a=0.3:d=%.3f:r=22050", seed, (240*time.Second - scene).Seconds()),
		"-i", credits,
	}
	audio, inputs := "[1][2]concat=n=2:v=0:a=1[a]", 3
	if scene > 0 {
		args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=color=pink:seed=%d:a=0.2:d=%.3f:r=22050", seed+100, scene.Seconds()))
		audio, inputs = "[1][2][3]concat=n=3:v=0:a=1[a]", 4
	}
	if chapters != "" {
		args = append(args, "-f", "ffmetadata", "-i", chapters, "-map_chapters", strconv.Itoa(inputs))
	}
	args = append(args, "-filter_complex", audio, "-map", "0:v", "-map", "[a]",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", path)
	ffmpegRun(t, ffmpeg, args...)
}

// Local episodes ending with the same 60-second credits are given them,
// to the second, from their audio once one of them is played: the scene
// after them in some is left out, and the credits running to the end of
// the others end at the end, for jellyfin-web's Up Next card, as the
// listed runtime falls short of the files. The credits of the third
// episode's chapters win; the fourth's from the databases, past its end,
// leave the gap to the audio; the sixth, ending otherwise, gets none.
func TestLocalEpisodesShareTheirCredits(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	scenes := map[int]time.Duration{1: 0, 2: 12500 * time.Millisecond, 3: 0, 4: 0, 5: 5 * time.Second}
	a := newAudioSeason(t, ffmpeg, 6, func(dir string) {
		credits := filepath.Join(t.TempDir(), "credits.wav")
		ffmpegRun(t, ffmpeg, "-f", "lavfi", "-i",
			"aevalsrc='0.35*sin(2*PI*t*(220+55*floor(mod(t*1.5,6))))+0.15*sin(2*PI*t*(660+90*floor(mod(t*2.5,3))))':s=22050:d=60",
			"-f", "lavfi", "-i", "anoisesrc=color=white:seed=9:a=0.05:d=60:r=22050",
			"-filter_complex", "[0][1]amix=inputs=2:normalize=0", credits)
		chapters := filepath.Join(t.TempDir(), "chapters.txt")
		if err := os.WriteFile(chapters, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=250000\ntitle=Chapter 1\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=250000\nEND=300000\ntitle=End Credits\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		episode := func(number int) string {
			return filepath.Join(dir, "Show", "Season 01", fmt.Sprintf("Show - S01E0%d.mkv", number))
		}
		for number, scene := range scenes {
			withChapters := ""
			if number == 3 {
				withChapters = chapters
			}
			creditsEpisode(t, ffmpeg, episode(number), credits, scene, number, withChapters)
		}
		ffmpegRun(t, ffmpeg, "-f", "lavfi", "-i", "anoisesrc=color=pink:seed=6:a=0.2:d=300:r=22050",
			"-f", "lavfi", "-i", "color=c=gray:s=64x36:r=2:d=300", "-map", "1:v", "-map", "0:a",
			"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", episode(6))
	})
	// Each version's end, from its analysis.
	ends := map[int]time.Duration{}
	for number := 1; number <= 6; number++ {
		source := a.playbackInfo(t, number)
		if source.RunTimeTicks == nil {
			t.Fatalf("episode %d: no runtime", number)
		}
		ends[number] = time.Duration(*source.RunTimeTicks) * 100
	}
	if got := a.segments(t, 1, "Outro"); len(got) != 0 {
		t.Errorf("credits before any play: %v", got)
	}
	a.play(t, 1)
	a.waitSegments(t, 5, "Outro")
	for _, number := range []int{1, 2, 4, 5} {
		start, end := 240*time.Second-scenes[number], 300*time.Second-scenes[number]
		if scenes[number] == 0 {
			end = ends[number]
		}
		if got := a.segments(t, number, "Outro"); !near(got, start, end) {
			t.Errorf("episode %d: %v, want credits from %v to %v", number, got, start, end)
		}
	}
	if got := a.segments(t, 3, "Outro"); !slices.Equal(got, []span{{"Outro", 250_000 * 10_000, int64(ends[3] / 100)}}) {
		t.Errorf("episode 3: %v, want its chapters' credits, to its end at %v", got, ends[3])
	}
	if got := a.segments(t, 6, "Outro"); len(got) != 0 {
		t.Errorf("episode 6: %v, want none", got)
	}
}
