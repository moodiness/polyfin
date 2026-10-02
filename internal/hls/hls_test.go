package hls

import (
	"bytes"
	"context"
	"encoding/json"
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

func seconds(values ...float64) []time.Duration {
	durations := make([]time.Duration, len(values))
	for i, v := range values {
		durations[i] = time.Duration(math.Round(v * float64(time.Second)))
	}
	return durations
}

func TestSegmentsStartOnKeyframesFFmpegCanSeekTo(t *testing.T) {
	for name, c := range map[string]struct {
		keyframes []time.Duration
		duration  time.Duration
		starts    []time.Duration
	}{
		"each segment ends on the first keyframe 6 s after its start": {
			seconds(0, 2.5, 7, 8, 15.333, 16, 22.917, 27), 30 * time.Second, seconds(0, 7, 15.333, 22.917),
		},
		"a keyframe followed closely by another does not start a segment": {
			seconds(0, 6, 6.1, 12, 18), 20 * time.Second, seconds(0, 6.1, 18),
		},
		"a keyframe at the very end does not start a segment": {
			seconds(0, 6, 12, 19.5), 20 * time.Second, seconds(0, 6, 12),
		},
		"the first segment starts at zero, whatever the first keyframe": {
			seconds(0.083, 6.083, 12.083), 15 * time.Second, seconds(0, 6.083, 12.083),
		},
		"a version without keyframes past the first is one segment": {
			seconds(0), 9 * time.Second, seconds(0),
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan := NewPlan(c.keyframes, c.duration)
			if !slices.Equal(plan.starts, c.starts) {
				t.Fatalf("starts %v, want %v", plan.starts, c.starts)
			}
			if last := plan.Len() - 1; plan.End(last) != c.duration {
				t.Errorf("the last segment ends at %v", plan.End(last))
			}
			if got := plan.Segment(c.starts[len(c.starts)-1] + time.Millisecond); got != len(c.starts)-1 {
				t.Errorf("segment of the last start: %d", got)
			}
		})
	}
}

func TestPlaylistsListEverySegment(t *testing.T) {
	plan := NewPlan(seconds(0, 6.5, 13), 20*time.Second)
	var media bytes.Buffer
	if err := WriteMedia(&media, plan, FMP4, func(n int) string { return "hls1/main/" + strconv.Itoa(n) + ".mp4?a=b" }); err != nil {
		t.Fatal(err)
	}
	want := `#EXTM3U
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:7
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-MAP:URI="hls1/main/-1.mp4?a=b"
#EXTINF:6.500000, nodesc
hls1/main/0.mp4?a=b
#EXTINF:6.500000, nodesc
hls1/main/1.mp4?a=b
#EXTINF:7.000000, nodesc
hls1/main/2.mp4?a=b
#EXT-X-ENDLIST
`
	if media.String() != want {
		t.Errorf("media playlist:\n%s", media.String())
	}
	var master bytes.Buffer
	if err := WriteMaster(&master, Variant{Bandwidth: 5_000_000, Codecs: "avc1.640028,mp4a.40.2", Width: 1920, Height: 1080, FrameRate: 23.976, Range: "SDR"}, "main.m3u8?a=b"); err != nil {
		t.Fatal(err)
	}
	if want := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,AVERAGE-BANDWIDTH=5000000,VIDEO-RANGE=SDR,CODECS=\"avc1.640028,mp4a.40.2\",RESOLUTION=1920x1080,FRAME-RATE=23.976\nmain.m3u8?a=b\n"; master.String() != want {
		t.Errorf("master playlist:\n%s", master.String())
	}
}

func TestSubtitleRenditionsFollowHLSRules(t *testing.T) {
	var master bytes.Buffer
	err := WriteMaster(&master, Variant{Bandwidth: 1_000_000, Subtitles: []Rendition{
		{Name: "French", Language: "fr", URI: "s0.m3u8"},
		{Name: "French", Language: "fr", URI: "s1.m3u8"},
		{Name: `Forced "signs"`, Language: "fr", Forced: true, URI: "s2.m3u8"},
		{Name: "English", Language: "en", Default: true, URI: "s3.m3u8"},
	}}, "main.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	// Names differ within the group; a default track is autoselected, and
	// so is a forced one.
	want := `#EXTM3U
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French",LANGUAGE="fr",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO,URI="s0.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French 2",LANGUAGE="fr",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO,URI="s1.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Forced 'signs'",LANGUAGE="fr",DEFAULT=NO,AUTOSELECT=YES,FORCED=YES,URI="s2.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI="s3.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1000000,AVERAGE-BANDWIDTH=1000000,SUBTITLES="subs"
main.m3u8
`
	if master.String() != want {
		t.Errorf("master playlist:\n%s", master.String())
	}
}

// tools returns FFmpeg and ffprobe, named by POLYFIN_TEST_FFMPEG, or skips
// the test.
func tools(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// source writes a 30-second Matroska file with B-frames, keyframes at
// irregular times and audio in codec, and returns where ffprobe finds its
// keyframes.
func source(t *testing.T, ffmpeg, ffprobe, codec string) (string, []time.Duration) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "30", "-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "0,2.5,7,8,15.3,16,22.9,27", "-c:a", codec, "-b:a", "96k", path).CombinedOutput()
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
	if len(keyframes) != 8 {
		t.Fatalf("the source has keyframes at %v", keyframes)
	}
	return path, keyframes
}

type packet struct {
	Type  string `json:"codec_type"`
	PTS   string `json:"pts_time"`
	Flags string `json:"flags"`
}

func probePackets(t *testing.T, ffprobe, path string) []packet {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "packet=codec_type,pts_time,flags",
		"-show_entries", "stream=codec_type", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("probe %s: %v", filepath.Base(path), err)
	}
	var result struct{ Packets []packet }
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	return result.Packets
}

func TestRemuxedSegmentsJoinIntoTheSource(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	// AAC starts before zero, with its priming; the MP4 header of E-AC-3
	// needs its first packet.
	for _, codec := range []string{"aac", "eac3"} {
		input, keyframes := source(t, ffmpeg, ffprobe, codec)
		for _, format := range []Format{FMP4, TS} {
			t.Run(codec+" in "+format.Extension(), func(t *testing.T) {
				m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
				if err != nil {
					t.Fatal(err)
				}
				defer m.Close()
				plan := NewPlan(keyframes, 30*time.Second)
				released := make(chan struct{})
				open := func(context.Context) (Remux, func(), error) {
					return Remux{Input: input, Video: 0, Audio: 1, Format: format, Plan: plan}, func() { close(released) }, nil
				}
				key := Key{Session: "session", Audio: 1, Format: format}
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
				// A player resuming mid-file, then starting over: FFmpeg starts
				// at segment 2, then again at 0.
				read(m.Segment(ctx, key, open, 2))
				var init []byte
				if format == FMP4 {
					init = read(m.Init(ctx, key, open))
				}
				dir := t.TempDir()
				joined := slices.Clone(init)
				for n := range plan.Len() {
					data := read(m.Segment(ctx, key, open, n))
					joined = append(joined, data...)
					one := filepath.Join(dir, "segment"+strconv.Itoa(n))
					if err := os.WriteFile(one, append(slices.Clone(init), data...), 0o600); err != nil {
						t.Fatal(err)
					}
					packets := probePackets(t, ffprobe, one)
					i := slices.IndexFunc(packets, func(p packet) bool { return p.Type == "video" })
					if i < 0 {
						t.Fatalf("segment %d has no video", n)
					}
					at, _ := strconv.ParseFloat(packets[i].PTS, 64)
					at -= timestampOffset.Seconds()
					if want := plan.Start(n).Seconds(); !strings.Contains(packets[i].Flags, "K") || math.Abs(at-want) > 0.002 {
						t.Errorf("segment %d starts with %+v (at %.3f), want a keyframe at %.3f", n, packets[i], at, want)
					}
				}
				// Joined, the segments are the source's 720 frames, in order.
				all := filepath.Join(dir, "joined")
				if err := os.WriteFile(all, joined, 0o600); err != nil {
					t.Fatal(err)
				}
				frames, last := 0, -1.0
				for _, p := range probePackets(t, ffprobe, all) {
					if p.Type != "video" {
						continue
					}
					frames++
					if strings.Contains(p.Flags, "K") {
						at, _ := strconv.ParseFloat(p.PTS, 64)
						if at <= last {
							t.Errorf("keyframe at %.3f after %.3f", at, last)
						}
						last = at
					}
				}
				if frames != 720 {
					t.Errorf("%d video frames, want 720", frames)
				}
				if _, err := m.Segment(ctx, key, open, plan.Len()); err != ErrNotFound {
					t.Errorf("segment past the end: %v", err)
				}
				m.Stop("session")
				select {
				case <-released:
				case <-time.After(10 * time.Second):
					t.Error("the input was not released")
				}
			})
		}
	}
}

func TestARemuxThatFailsReportsItsError(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	input, keyframes := source(t, ffmpeg, ffprobe, "aac")
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Only the audio, as if it were the video: segments cannot be cut on
	// its keyframes.
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: input, Video: 1, Audio: -1, Format: FMP4, Plan: NewPlan(keyframes, 30*time.Second)}, func() {}, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, err = m.Segment(ctx, Key{Session: "session", Format: FMP4}, open, 0)
	if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "no video track") {
		t.Errorf("segment of a failing remux: %v", err)
	}
}
