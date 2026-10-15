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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/commercials"
	"github.com/moodiness/polyfin/internal/recordings"
)

// makeBreaksRecording writes at path a recording of three parts of a
// programme parted by two commercial breaks, each break opened, parted and
// closed by black and silent pauses of a second: two adverts of 15 and 20
// seconds from 100 seconds, then three of 12 seconds from 238 seconds.
func makeBreaksRecording(t *testing.T, ffmpeg, path string) {
	t.Helper()
	const programme, advert, pause = "testsrc2", "smptebars", "black"
	parts := []struct {
		kind    string
		seconds int
	}{
		{programme, 100}, {pause, 1}, {advert, 15}, {pause, 1}, {advert, 20}, {pause, 1}, {programme, 100}, {pause, 1}, {advert, 12},
		{pause, 1}, {advert, 12}, {pause, 1}, {advert, 12}, {pause, 1}, {programme, 100},
	}
	var args []string
	var graph strings.Builder
	for i, p := range parts {
		video := fmt.Sprintf("%s=size=160x90:rate=5:duration=%d", p.kind, p.seconds)
		audio := fmt.Sprintf("sine=frequency=440:sample_rate=48000:duration=%d", p.seconds)
		switch p.kind {
		case pause:
			video, audio = fmt.Sprintf("color=c=black:size=160x90:rate=5:duration=%d", p.seconds), fmt.Sprintf("aevalsrc=0:s=48000:d=%d", p.seconds)
		case advert:
			audio = fmt.Sprintf("sine=frequency=880:sample_rate=48000:duration=%d", p.seconds)
		}
		args = append(args, "-f", "lavfi", "-i", video, "-f", "lavfi", "-i", audio)
		fmt.Fprintf(&graph, "[%d:v]format=yuv420p[v%d];[%d:a]aformat=sample_rates=48000:channel_layouts=mono[a%d];", 2*i, i, 2*i+1, i)
	}
	for i := range parts {
		fmt.Fprintf(&graph, "[v%d][a%d]", i, i)
	}
	fmt.Fprintf(&graph, "concat=n=%d:v=1:a=1[v][a]", len(parts))
	args = append(args, "-filter_complex", graph.String(), "-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", path)
	if out, err := exec.Command(ffmpeg, append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
}

// A recording with two commercial breaks, read for them, gets them as two
// Commercial segments from /MediaSegments, and HasSegments on its media
// source, for the user who sees it; with the skip buttons off, it has none.
func TestRecordingCommercialsAreSegments(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	folder := t.TempDir()
	var breaks *commercials.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		service := recordings.New(recordings.Config{DB: pool, Folder: func() string { return folder }, Guide: o.Library, Recorder: o.Playback,
			Users: o.Accounts, Logger: logger})
		o.Recordings = service
		breaks = commercials.New(commercials.Options{DB: pool, FFmpeg: ffmpeg, FFprobe: ffprobe, Recordings: service,
			Settings: o.Accounts.Settings, Logger: logger})
		t.Cleanup(breaks.Close)
		o.Commercials = breaks
	})
	s.setting(t, func(settings *accounts.Settings) { settings.RecordingCommercials = true })
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	s.user("other", nil)
	other := s.signIn("other", "tv")
	// The member's recording, on a channel no one reaches.
	var id accounts.ID
	if err := s.pool.QueryRow(t.Context(), `INSERT INTO live_recordings (user_id, channel_id, name, start_at, end_at, started_at, ended_at, status)
		VALUES ($1, gen_random_uuid(), 'Evening at the harbour', now(), now(), now(), now(), 'Completed') RETURNING id`, member.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	makeBreaksRecording(t, ffmpeg, filepath.Join(folder, id.String()+".mkv"))
	if _, err := s.pool.Exec(t.Context(), "UPDATE live_recordings SET file = $2 WHERE id = $1", id, id.String()+".mkv"); err != nil {
		t.Fatal(err)
	}
	recording := strings.ReplaceAll(id.String(), "-", "")

	segments := func(token, query string) (int, MediaSegmentDtoQueryResult) {
		t.Helper()
		var result MediaSegmentDtoQueryResult
		status, body := s.call(http.MethodGet, "/MediaSegments/"+recording+query, app("tv", token), nil)
		if status == http.StatusOK {
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
		}
		return status, result
	}
	hasSegments := func() bool {
		t.Helper()
		status, body := s.call(http.MethodPost, "/Items/"+recording+"/PlaybackInfo", app("tv", token), map[string]any{})
		var info playbackInfoResponse
		if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo of the recording: %d %s", status, body)
		}
		return info.MediaSources[0].HasSegments
	}

	// Not read yet, it has none.
	if status, result := segments(token, ""); status != http.StatusOK || len(result.Items) != 0 || hasSegments() {
		t.Fatalf("before it is read: %d %+v", status, result)
	}
	if err := breaks.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, result := segments(token, "")
	want := [][2]time.Duration{{100 * time.Second, 138 * time.Second}, {238 * time.Second, 278 * time.Second}}
	if status != http.StatusOK || len(result.Items) != len(want) || result.TotalRecordCount != len(want) {
		t.Fatalf("segments: %d %+v", status, result)
	}
	for i, segment := range result.Items {
		start, end := time.Duration(segment.StartTicks*100), time.Duration(segment.EndTicks*100)
		if segment.Type != "Commercial" || segment.ItemId != recording || (start-want[i][0]).Abs() > time.Second/2 || (end-want[i][1]).Abs() > time.Second/2 {
			t.Errorf("segment %d: %+v, want a commercial from %v to %v", i, segment, want[i][0], want[i][1])
		}
	}
	if !hasSegments() {
		t.Error("the recording's media source has no segments")
	}
	if status, result := segments(token, "?includeSegmentTypes=Intro"); status != http.StatusOK || len(result.Items) != 0 {
		t.Errorf("intros asked: %d %+v", status, result)
	}
	if status, result := segments(token, "?includeSegmentTypes=Commercial"); status != http.StatusOK || len(result.Items) != 2 {
		t.Errorf("commercials asked: %d %+v", status, result)
	}
	// A user who does not see the recording does not reach its segments.
	if status, _ := segments(other, ""); status != http.StatusNotFound {
		t.Errorf("segments for another user: %d", status)
	}

	// With the skip buttons off, none is served.
	s.setting(t, func(settings *accounts.Settings) { settings.SkipButtons = false })
	if status, result := segments(token, ""); status != http.StatusOK || len(result.Items) != 0 || hasSegments() {
		t.Errorf("with the skip buttons off: %d %+v", status, result)
	}
}
