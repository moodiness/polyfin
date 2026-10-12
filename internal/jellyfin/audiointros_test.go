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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
	user := s.user("member", nil)
	token := s.signIn("member", "tv")

	dir := t.TempDir()
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
	// The databases give the second episode an intro, from 60 to 95
	// seconds, and the third one from 321.4 seconds, which its chapters'
	// replace.
	starts := map[int]time.Duration{1: 4 * time.Second, 2: 11500 * time.Millisecond, 3: 15 * time.Second, 5: 18750 * time.Millisecond}
	for number, at := range starts {
		withChapters := ""
		if number == 3 {
			withChapters = chapters
		}
		introEpisode(t, ffmpeg, filepath.Join(dir, "Show", "Season 01", fmt.Sprintf("Show - S01E0%d.mkv", number)), intro, at, number,
			withChapters)
	}
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
	s.get(t, "/UserViews", token, &views)
	var shows QueryResult
	if len(views.Items) != 1 || s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &shows) != http.StatusOK || len(shows.Items) != 1 {
		t.Fatalf("views %+v, shows %+v", views.Items, shows.Items)
	}
	var list QueryResult
	s.get(t, "/Shows/"+shows.Items[0].Id+"/Episodes?UserId="+user.ID.String(), token, &list)
	episodes := map[int]string{}
	for _, episode := range list.Items {
		if episode.IndexNumber != nil {
			episodes[*episode.IndexNumber] = episode.Id
		}
	}
	if len(episodes) != 4 {
		t.Fatalf("episodes: %+v", list.Items)
	}
	// PlaybackInfo analyzes the version it gives, chapters included.
	playbackInfo := func(number int) string {
		t.Helper()
		status, data := s.call(http.MethodPost, "/Items/"+episodes[number]+"/PlaybackInfo", app("tv", token),
			map[string]any{"UserId": user.ID.String()})
		var info playbackInfoResponse
		if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		return info.MediaSources[0].Id
	}
	intros := func(number int) []span {
		t.Helper()
		var result MediaSegmentDtoQueryResult
		s.get(t, "/MediaSegments/"+episodes[number]+"?includeSegmentTypes=Intro", token, &result)
		return spans(result)
	}
	playbackInfo(3)
	if got := intros(1); len(got) != 0 {
		t.Errorf("an intro before any play: %v", got)
	}
	source := playbackInfo(1)
	if status, data := s.call(http.MethodPost, "/Sessions/Playing", app("tv", token),
		map[string]any{"ItemId": episodes[1], "MediaSourceId": source, "PositionTicks": 0}); status != http.StatusNoContent {
		t.Fatalf("playing: %d %s", status, data)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for len(intros(5)) == 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	near := func(got []span, at time.Duration) bool {
		const tick = 100 * time.Nanosecond
		return len(got) == 1 && (time.Duration(got[0].Start)*tick-at).Abs() <= time.Second/2 &&
			(time.Duration(got[0].End)*tick-at-40*time.Second).Abs() <= time.Second/2
	}
	for _, number := range []int{1, 5} {
		if got := intros(number); !near(got, starts[number]) {
			t.Errorf("episode %d: %v, want an intro from %v", number, got, starts[number])
		}
	}
	if got := intros(2); !slices.Equal(got, []span{spanMS("Intro", 60_000, 95_000)}) {
		t.Errorf("episode 2: %v, want the databases' intro", got)
	}
	if got := intros(3); !slices.Equal(got, []span{spanMS("Intro", 30_000, 50_000)}) {
		t.Errorf("episode 3: %v, want its chapters' intro", got)
	}
}
