package hls

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A play starts with fast segments only where its segment is long: the
// segment playing where it starts is cut every 2 seconds, its last part up
// to 4 seconds long, and the segments around it keep their times.
func TestAFastStartCutsALongSegmentIntoShortOnes(t *testing.T) {
	for name, c := range map[string]struct {
		keyframes  []time.Duration
		duration   time.Duration
		at         time.Duration
		starts     []time.Duration
		from, to   int
		fastStarts bool
	}{
		"10 s keyframes, from the start": {
			seconds(0, 10, 20, 30), 40 * time.Second, 0, seconds(0, 2, 4, 6, 8, 10, 20, 30), 0, 5, true,
		},
		"10 s keyframes, resumed 25 s in": {
			seconds(0, 10, 20, 30), 40 * time.Second, 25 * time.Second, seconds(0, 10, 20, 22, 24, 26, 28, 30), 2, 7, true,
		},
		"a first keyframe 5 s in": {
			seconds(0, 5, 10), 15 * time.Second, 0, seconds(0, 2, 5, 10), 0, 2, true,
		},
		"the last segment": {
			seconds(0, 10, 20), 29 * time.Second, 28 * time.Second, seconds(0, 10, 20, 22, 24, 26), 2, 6, true,
		},
		"2 s keyframes make short segments already": {
			seconds(0, 2, 4, 6, 8, 10), 12 * time.Second, 0, seconds(0, 2, 6, 10), 0, 0, false,
		},
		"a 4 s segment is no longer than two fast ones": {
			seconds(0, 2, 4, 6, 8, 10), 12 * time.Second, 7 * time.Second, seconds(0, 2, 6, 10), 0, 0, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan, ok := NewPlan(c.keyframes, c.duration).FastFrom(c.at)
			if ok != c.fastStarts || !slices.Equal(plan.starts, c.starts) || plan.fastFrom != c.from || plan.fastTo != c.to {
				t.Fatalf("%v: starts %v, fast from %d to %d", ok, plan.starts, plan.fastFrom, plan.fastTo)
			}
			for n := range plan.Len() {
				if fast := n >= c.from && n < c.to; plan.Fast(n) != fast {
					t.Errorf("segment %d fast: %v", n, plan.Fast(n))
				}
			}
			if plan.End(plan.Len()-1) != c.duration {
				t.Errorf("the last segment ends at %v", plan.End(plan.Len()-1))
			}
		})
	}
}

// The playlist marks where the fast segments start, unless they start
// the play, and where they end, with a discontinuity; in fragmented MP4,
// each kind of segment has its initialization segment.
func TestPlaylistsMarkTheFastSegments(t *testing.T) {
	uri := func(n int) string { return strconv.Itoa(n) }
	for name, c := range map[string]struct {
		at     time.Duration
		format Format
		want   string
	}{
		"from the start, in fragmented MP4": {0, FMP4, "map -2, 0, 1, 2, 3, 4, discontinuity, map -1, 5, 6"},
		"from the start, in MPEG-TS":        {0, TS, "0, 1, 2, 3, 4, discontinuity, 5, 6"},
		"resumed, in fragmented MP4":        {12 * time.Second, FMP4, "map -1, 0, discontinuity, map -2, 1, 2, 3, 4, 5, discontinuity, map -1, 6"},
	} {
		t.Run(name, func(t *testing.T) {
			plan, _ := NewPlan(seconds(0, 10, 20), 30*time.Second).FastFrom(c.at)
			var b bytes.Buffer
			if err := WriteMedia(&b, plan, c.format, uri); err != nil {
				t.Fatal(err)
			}
			var got []string
			for line := range strings.SplitSeq(b.String(), "\n") {
				switch {
				case line == "#EXT-X-DISCONTINUITY":
					got = append(got, "discontinuity")
				case strings.HasPrefix(line, "#EXT-X-MAP:"):
					got = append(got, "map "+strings.Trim(strings.TrimPrefix(line, "#EXT-X-MAP:URI="), `"`))
				case line != "" && !strings.HasPrefix(line, "#"):
					got = append(got, line)
				}
			}
			if strings.Join(got, ", ") != c.want {
				t.Errorf("playlist:\n%s", b.String())
			}
			if strings.Contains(b.String(), "#EXT-X-TARGETDURATION:10\n") == false {
				t.Errorf("target duration:\n%s", b.String())
			}
		})
	}
}

// fastSource writes 20 seconds of 640×480 video with a keyframe every 10
// seconds, and AAC audio, in Matroska, and returns its keyframes.
func fastSource(t *testing.T, ffmpeg, ffprobe string) (string, []time.Duration) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x480:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-g", "240", "-keyint_min", "240", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "96k", path).CombinedOutput()
	if err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	var keyframes []time.Duration
	for _, p := range probePackets(t, ffprobe, path) {
		if p.Type == "video" && strings.Contains(p.Flags, "K") {
			at, _ := strconv.ParseFloat(p.PTS, 64)
			keyframes = append(keyframes, seconds(at)...)
		}
	}
	if len(keyframes) != 2 {
		t.Fatalf("the source has keyframes at %v", keyframes)
	}
	return path, keyframes
}

// A repackaged play starting with fast segments gets them converted, at
// the size and in the encoder asked, each starting on a keyframe at its
// time; the segments after are the version's own, copied. One FFmpeg makes
// the fast segments and hands over to another at the version's keyframe:
// asked one after the other, the segments take two runs. A play resumed
// in the last segment gets its fast segments there, and the copied
// segment before them when it goes back.
func TestFastSegmentsAreConvertedThenTheVideoIsCopied(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	input, keyframes := fastSource(t, ffmpeg, ffprobe)
	for _, c := range []struct {
		format Format
		at     time.Duration
	}{{FMP4, 0}, {TS, 0}, {FMP4, 12 * time.Second}, {TS, 12 * time.Second}} {
		t.Run(c.format.Extension()+" from "+c.at.String(), func(t *testing.T) {
			m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if !slices.Contains(m.Encoders(), "libx264") {
				t.Skip("FFmpeg has no libx264")
			}
			plan, ok := NewPlan(keyframes, 20*time.Second).FastFrom(c.at)
			if !ok {
				t.Fatal("no fast segments")
			}
			open := func(context.Context) (Remux, func(), error) {
				return Remux{Input: input, Video: 0, Audio: 1, Format: c.format, Plan: plan,
					Fast: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 320, Height: 240, Bitrate: 400_000, FrameRate: 24, Preset: "ultrafast"}}, func() {}, nil
			}
			key := Key{Session: "session", Audio: 1, Format: c.format, FastStart: true}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			read := func(f *os.File, err error) []byte {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				data, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			first := plan.Segment(c.at)
			read(m.Segment(ctx, key, open, first))
			var init, fastInit []byte
			if c.format == FMP4 {
				fastInit = read(m.FastInit(ctx, key, open))
				init = read(m.Init(ctx, key, open))
				if bytes.Equal(init, fastInit) {
					t.Fatal("one initialization segment for both kinds of segments")
				}
			}
			dir := t.TempDir()
			for n := range plan.Len() {
				data := read(m.Segment(ctx, key, open, n))
				header, size := init, "640x480"
				if plan.Fast(n) {
					header, size = fastInit, "320x240"
				}
				one := filepath.Join(dir, "segment"+strconv.Itoa(n))
				if err := os.WriteFile(one, append(slices.Clone(header), data...), 0o600); err != nil {
					t.Fatal(err)
				}
				out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v", "-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", one).Output()
				if got, _, _ := firstLine(out); err != nil || got != size {
					t.Errorf("segment %d is %q, want %s", n, got, size)
				}
				packets := probePackets(t, ffprobe, one)
				i := slices.IndexFunc(packets, func(p packet) bool { return p.Type == "video" })
				if i < 0 {
					t.Fatalf("segment %d has no video", n)
				}
				at, _ := strconv.ParseFloat(packets[i].PTS, 64)
				at -= timestampOffset.Seconds()
				if want := plan.Start(n).Seconds(); !strings.Contains(packets[i].Flags, "K") || math.Abs(at-want) > 0.05 {
					t.Errorf("segment %d starts with %+v (at %.3f), want a keyframe at %.3f", n, packets[i], at, want)
				}
			}
			if c.at > 0 {
				return
			}
			m.mu.Lock()
			e := m.encodings[key]
			m.mu.Unlock()
			e.mu.Lock()
			runs := e.runs
			e.mu.Unlock()
			if runs != 2 {
				t.Errorf("%d FFmpeg runs, want 2: the fast segments, then the copy", runs)
			}
		})
	}
}
