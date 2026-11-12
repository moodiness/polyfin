package hls

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// The playlist of a playout starts a discontinuity where a piece starts,
// and counts those that left it.
func TestPlayoutPlaylistMarksPieces(t *testing.T) {
	t.Parallel()
	var segments []playoutSegment
	for n := range 8 {
		piece := 0
		if n >= 5 {
			piece = 1
		}
		segments = append(segments, playoutSegment{number: n, duration: 3, piece: piece})
	}
	uri := func(name string) string { return "seg/" + name }
	playlist := string(playoutPlaylist(segments, uri))
	if n := strings.Count(playlist, "#EXT-X-DISCONTINUITY\n"); n != 1 {
		t.Fatalf("%d discontinuities, want 1:\n%s", n, playlist)
	}
	if !strings.Contains(playlist, "seg/4.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:3.000000,\nseg/5.ts\n") {
		t.Fatalf("the discontinuity is not before the second piece:\n%s", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-DISCONTINUITY-SEQUENCE:0\n") {
		t.Fatalf("wrong sequences:\n%s", playlist)
	}
	// Once the first piece left the playlist, its discontinuity is
	// counted.
	for n := 8; n < 16; n++ {
		segments = append(segments, playoutSegment{number: n, duration: 3, piece: 1})
	}
	playlist = string(playoutPlaylist(segments, uri))
	if strings.Contains(playlist, "#EXT-X-DISCONTINUITY\n") ||
		!strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:6\n#EXT-X-DISCONTINUITY-SEQUENCE:1\n") {
		t.Fatalf("wrong playlist once the first piece left:\n%s", playlist)
	}
}

// A programme tuned 90 seconds after it started is read from 90 seconds
// in, through to its end.
func TestPlayoutPieceStartsAtItsOffset(t *testing.T) {
	t.Parallel()
	piece := PlayoutPiece{Remux: Remux{Input: "http://127.0.0.1/file.mkv", Video: 0, Audio: 1, Encode: &VideoEncoding{Encoder: "libx264", Width: 1280,
		Height: 720, Bitrate: 4_000_000}}, Offset: 90 * time.Second, Length: 25*time.Minute - 90*time.Second}
	args := piece.args("/segments", 12, playoutLead, time.Minute)
	input := slices.Index(args, "-i")
	if input < 2 || args[input-2] != "-ss" || args[input-1] != "90.000" || args[input+1] != piece.Remux.Input {
		t.Fatalf("the input is not read from 90 seconds: %q", args)
	}
	if i := slices.Index(args, "-t"); i < 0 || args[i+1] != "1410.000" {
		t.Fatalf("the piece does not last until the programme's end: %q", args)
	}
	if i := slices.Index(args, "-start_number"); i < 0 || args[i+1] != "12" {
		t.Fatalf("the segments are not numbered on: %q", args)
	}
}

// A playout airs its pieces one after the other, each from its offset,
// asking for the next where the one before ended, and its playlist starts
// a discontinuity at the second.
func TestPlayoutAirsPiecesInTurn(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := tools(t)
	path, _ := source(t, ffmpeg, ffprobe, "aac")
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	var mu sync.Mutex
	var asked []time.Time
	var failures []error
	next := func(_ context.Context, at time.Time, failure error) (PlayoutPiece, func(), error) {
		mu.Lock()
		defer mu.Unlock()
		asked, failures = append(asked, at), append(failures, failure)
		if len(asked) == 1 {
			// A programme that started 20 seconds ago, 4 seconds from its
			// end.
			return PlayoutPiece{Remux: Remux{Input: path, Video: 0, Audio: 1, Encode: &VideoEncoding{Encoder: "libx264", Level: "4.0", Width: 160,
				Height: 90, Bitrate: 300_000, FrameRate: 24}}, Offset: 20 * time.Second, Length: 4 * time.Second}, func() {}, nil
		}
		return PlayoutPiece{Slate: true, Length: time.Minute}, func() {}, nil
	}
	uri := func(name string) string { return name }
	deadline := time.Now().Add(time.Minute)
	var playlist string
	for !strings.Contains(playlist, "#EXT-X-DISCONTINUITY\n") {
		if time.Now().After(deadline) {
			t.Fatalf("no discontinuity within a minute:\n%s", playlist)
		}
		data, err := m.Playout(t.Context(), "channel", "Channel", next, uri, 1)
		if err != nil {
			t.Fatal(err)
		}
		playlist = string(data)
		time.Sleep(200 * time.Millisecond)
	}
	if n := strings.Count(playlist, "#EXT-X-DISCONTINUITY\n"); n != 1 {
		t.Fatalf("%d discontinuities, want 1:\n%s", n, playlist)
	}
	// The first piece aired its 4 seconds, and the second was asked for
	// right where it ended.
	first, _, _ := strings.Cut(playlist, "#EXT-X-DISCONTINUITY\n")
	if _, listed := listedSegments([]byte(first)); listed < 3500*time.Millisecond || listed > 4500*time.Millisecond {
		t.Fatalf("the first piece aired %v, want 4s:\n%s", listed, playlist)
	}
	mu.Lock()
	if len(asked) < 2 || asked[1].Sub(asked[0]) != 4*time.Second || failures[1] != nil {
		t.Fatalf("asked at %v with %v, want the second 4s after the first, with no failure", asked, failures)
	}
	mu.Unlock()
	for line := range strings.Lines(playlist) {
		if name := strings.TrimSpace(line); strings.HasSuffix(name, ".ts") {
			file, err := m.PlayoutSegment("channel", name)
			if err != nil {
				t.Fatalf("segment %s: %v", name, err)
			}
			data, _ := io.ReadAll(file)
			file.Close()
			if len(data) == 0 || data[0] != 0x47 {
				t.Fatalf("segment %s is no MPEG-TS", name)
			}
		}
	}
	m.StopPlayout("channel")
	if _, err := m.PlayoutSegment("channel", "0.ts"); err == nil {
		t.Fatal("a stopped playout still serves its segments")
	}
}

// unrenamedFFmpeg writes a script that runs as FFmpeg's HLS muxer does on
// Windows when the playout reads a run's playlist the moment FFmpeg
// renames its last one over it: for -t seconds, it writes segments of 3
// seconds at most, numbered from -start_number, and after each the
// playlist listing them to a temporary file, renamed over the run's
// playlist but for the last one, then exits cleanly.
func unrenamedFFmpeg(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake FFmpeg is a shell script")
	}
	script := `#!/bin/sh
case "$*" in *"-f hls"*) ;; *) exit 0 ;; esac
for playlist; do :; done
start=0; length=0
while [ $# -gt 1 ]; do
  case "$1" in
    -start_number) start=$2 ;;
    -t) length=${2%.*} ;;
  esac
  shift
done
dir=$(dirname "$playlist")
n=$start; list=""
while [ "$length" -gt 0 ]; do
  d=3; [ "$length" -lt 3 ] && d=$length
  printf 'segment' > "$dir/$n.ts"
  list="$list#EXTINF:$d.000000,
$n.ts
"
  printf '#EXTM3U\n%s' "$list" > "$playlist.tmp"
  length=$((length-d)); n=$((n+1))
  if [ "$length" -gt 0 ]; then mv "$playlist.tmp" "$playlist"; fi
done
`
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := process.WriteExecutable(path, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// On Windows, FFmpeg cannot rename a run's playlist over the one the
// playout is reading, and goes on: the last playlist of a run can stay in
// its temporary file. The playout reads it once the run ended, so that a
// piece airs whole, and the next one is numbered on rather than airing
// early in place of the last segment of the one before (seen on CI, where
// TestPlayoutAirsPiecesInTurn found its 4 seconds piece aired 3).
func TestPlayoutReadsTheLastPlaylistFFmpegCouldNotRename(t *testing.T) {
	t.Parallel()
	m, err := NewManager(unrenamedFFmpeg(t), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	lengths := []time.Duration{4 * time.Second, 6 * time.Second}
	aired := make(chan struct{})
	next := func(ctx context.Context, _ time.Time, _ error) (PlayoutPiece, func(), error) {
		if len(lengths) == 0 {
			close(aired)
			<-ctx.Done()
			return PlayoutPiece{}, nil, ctx.Err()
		}
		piece := PlayoutPiece{Slate: true, Length: lengths[0]}
		lengths = lengths[1:]
		return piece, func() {}, nil
	}
	uri := func(name string) string { return name }
	if _, err := m.Playout(t.Context(), "channel", "Channel", next, uri, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-aired:
	case <-time.After(time.Minute):
		t.Fatal("the two pieces did not air within a minute")
	}
	playlist, err := m.Playout(t.Context(), "channel", "Channel", next, uri, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:3\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-DISCONTINUITY-SEQUENCE:0\n" +
		"#EXTINF:3.000000,\n0.ts\n#EXTINF:1.000000,\n1.ts\n#EXT-X-DISCONTINUITY\n" +
		"#EXTINF:3.000000,\n2.ts\n#EXTINF:3.000000,\n3.ts\n"
	if string(playlist) != want {
		t.Fatalf("the playlist:\n%s\nwant the first piece's 4 seconds, then the second's numbered on:\n%s", playlist, want)
	}
}
