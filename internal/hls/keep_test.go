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

// keptPlayback plays, its video converted and kept apart from its audio, a
// 30-second source with two audio tracks: stream 1 in stereo, stream 2 in
// mono, so that a segment tells which it carries.
type keptPlayback struct {
	t       *testing.T
	m       *Manager
	plan    Plan
	input   string
	ffmpeg  string
	ffprobe string
	format  Format
}

func newKeptPlayback(t *testing.T, format Format) *keptPlayback {
	t.Helper()
	ffmpeg, ffprobe := tools(t)
	input := filepath.Join(t.TempDir(), "source.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000", "-t", "30", "-map", "0", "-map", "1", "-map", "2",
		"-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "0,2.5,7,8,15.3,16,22.9,27", "-c:a", "aac", "-b:a", "96k", "-ac:a:0", "2", "-ac:a:1", "1",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fra", input).CombinedOutput()
	if err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	var keyframes []time.Duration
	for _, p := range probePackets(t, ffprobe, input) {
		if p.Type == "video" && strings.HasPrefix(p.Flags, "K") {
			at, _ := strconv.ParseFloat(p.PTS, 64)
			keyframes = append(keyframes, seconds(at)...)
		}
	}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.KeepVideo(func() bool { return true })
	m.LimitConversions(func() int { return 1 })
	return &keptPlayback{t: t, m: m, plan: NewPlan(keyframes, 30*time.Second), input: input, ffmpeg: ffmpeg, ffprobe: ffprobe, format: format}
}

// key is what session plays of audio track audio, on a TV, at the
// conversion video describes.
func (k *keptPlayback) key(session string, audio int) Key {
	return Key{Session: session, Audio: audio, Format: k.format, User: "alice", Device: "tv", Version: "v1", Converts: true, Video: "h264 160x90"}
}

// opener opens the source at 160x90, or at height 72 for another quality
// or a lower variant.
func (k *keptPlayback) opener(key Key) Opener {
	width, height := 160, 90
	if key.Video != "h264 160x90" || key.Variant > 0 {
		width, height = 128, 72
	}
	return func(context.Context) (Remux, func(), error) {
		return Remux{Input: k.input, Video: 0, Audio: key.Audio, Format: k.format, Plan: k.plan, Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1",
			Width: width, Height: height, Bitrate: 300_000, FrameRate: 24}}, func() {}, nil
	}
}

// The variants of a kept video each convert with their own FFmpeg, and
// one audio FFmpeg serves them all.
func TestOneAudioServesEveryVariant(t *testing.T) {
	k := newKeptPlayback(t, TS)
	first := k.key("web-1", 1)
	lower := first
	lower.Variant = 1
	for n := range 3 {
		for _, key := range []Key{first, lower} {
			if got := k.probe(k.segment(key, n)).channels(); got != 2 {
				t.Fatalf("variant %d, segment %d: %d channels", key.Variant, n, got)
			}
		}
	}
	high, _ := k.video(first)
	low, _ := k.video(lower)
	k.m.mu.Lock()
	kept, audio := len(k.m.kept), k.m.kept[first.keptVideo().playback()].track
	k.m.mu.Unlock()
	audio.mu.Lock()
	runs := audio.runs
	audio.mu.Unlock()
	if high == low || kept != 1 || runs != 1 {
		t.Errorf("variants share one video: %v, %d kept, %d audio runs", high == low, kept, runs)
	}
}

func (k *keptPlayback) read(f *os.File, err error) []byte {
	k.t.Helper()
	if err != nil {
		k.t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		k.t.Fatal(err)
	}
	return data
}

// segment returns segment n of what key plays, after its initialization
// segment in fragmented MP4, so that it decodes on its own.
func (k *keptPlayback) segment(key Key, n int) []byte {
	k.t.Helper()
	var init []byte
	if k.format == FMP4 {
		init = k.read(k.m.Init(k.t.Context(), key, k.opener(key)))
	}
	return append(init, k.read(k.m.Segment(k.t.Context(), key, k.opener(key), n))...)
}

// video returns the kept video key plays, and how many FFmpeg processes it
// started.
func (k *keptPlayback) video(key Key) (*encoding, int) {
	k.t.Helper()
	k.m.mu.Lock()
	e := k.m.encodings[key.keptVideo()]
	k.m.mu.Unlock()
	if e == nil {
		k.t.Fatalf("no kept video for %+v", key)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e, e.runs
}

// streams describes what a segment holds: its packets, and the channels
// of its audio.
type streams struct {
	Packets []struct {
		Type     string `json:"codec_type"`
		PTS      string `json:"pts_time"`
		Duration string `json:"duration_time"`
		Hash     string `json:"data_hash"`
	}
	Streams []struct {
		Type     string `json:"codec_type"`
		Channels int    `json:"channels"`
	}
}

func (k *keptPlayback) probe(data []byte) streams {
	k.t.Helper()
	path := filepath.Join(k.t.TempDir(), "segment."+k.format.Extension())
	if err := os.WriteFile(path, data, 0o600); err != nil {
		k.t.Fatal(err)
	}
	out, err := exec.Command(k.ffprobe, "-v", "error", "-show_data_hash", "adler32", "-show_entries",
		"packet=codec_type,pts_time,duration_time,data_hash:stream=codec_type,channels", "-of", "json", path).Output()
	if err != nil {
		k.t.Fatalf("probe a segment: %v", err)
	}
	var s streams
	if err := json.Unmarshal(out, &s); err != nil {
		k.t.Fatal(err)
	}
	return s
}

// channels is how many channels the segment's audio has, 0 for none.
func (s streams) channels() int {
	for _, stream := range s.Streams {
		if stream.Type == "audio" {
			return stream.Channels
		}
	}
	return 0
}

// video lists the hashes of the video's packets.
func (s streams) video() []string {
	var hashes []string
	for _, p := range s.Packets {
		if p.Type == "video" {
			hashes = append(hashes, p.Hash)
		}
	}
	return hashes
}

// starts returns when the earliest packet of each kind is presented.
func (s streams) starts() (video, audio float64) {
	video, audio = math.Inf(1), math.Inf(1)
	for _, p := range s.Packets {
		at, err := strconv.ParseFloat(p.PTS, 64)
		if err != nil {
			continue
		}
		if p.Type == "video" {
			video = min(video, at)
		} else if p.Type == "audio" {
			audio = min(audio, at)
		}
	}
	return video, audio
}

// In jellyfin-web's order, a new play session asks for another audio track
// once the old one stopped: the video's FFmpeg goes on, the same, and the
// segments after the switch carry the new track with the video made
// before. The old session's second stop leaves the new one playing, and
// the kept video counts once toward the conversions at once.
func TestTrackChangeKeepsTheConvertedVideo(t *testing.T) {
	for _, format := range []Format{FMP4, TS} {
		t.Run(format.Extension(), func(t *testing.T) {
			k := newKeptPlayback(t, format)
			old := k.key("web-1", 1)
			for n := range 3 {
				if got := k.probe(k.segment(old, n)).channels(); got != 2 {
					t.Fatalf("segment %d of the first track: %d channels", n, got)
				}
			}
			before := k.probe(k.segment(old, 3)).video()
			e, runs := k.video(old)
			if runs != 1 {
				t.Fatalf("%d FFmpeg runs before the switch", runs)
			}

			// jellyfin-web: PlaybackInfo for the new track, DELETE of the old
			// session, then the new master playlist and its segments.
			k.m.Stop("web-1")
			switched := k.key("web-2", 2)
			after := k.probe(k.segment(switched, 3))
			if after.channels() != 1 {
				t.Errorf("the segment after the switch has %d channels, want the mono track", after.channels())
			}
			if !slices.Equal(after.video(), before) || len(before) == 0 {
				t.Errorf("the video after the switch differs: %d packets, %d before", len(after.video()), len(before))
			}
			if again, runs := k.video(switched); again != e || runs != 1 {
				t.Errorf("the video's FFmpeg started again: %d runs", runs)
			}
			// The second DELETE of the old session, once the new one plays.
			k.m.Stop("web-1")
			if got := k.probe(k.segment(switched, 4)).channels(); got != 1 {
				t.Errorf("segment 4 after the old session's second stop: %d channels", got)
			}
			if again, runs := k.video(switched); again != e || runs != 1 {
				t.Errorf("the old session's second stop restarted the video: %d runs", runs)
			}
			running := k.m.Running()
			if len(running) != 1 || running[0].Key.Session != "web-2" || running[0].Key.Audio != 2 {
				t.Errorf("running: %+v", running)
			}
			if count, _ := k.m.Conversions(); count != 1 {
				t.Errorf("%d conversions at once", count)
			}
		})
	}
}

// Each joined segment's audio starts with its video, before and after a
// switch, and consecutive joined segments decode as one stream, their
// audio following on without a gap.
func TestJoinedSegmentsStartWithTheirVideo(t *testing.T) {
	for _, format := range []Format{FMP4, TS} {
		t.Run(format.Extension(), func(t *testing.T) {
			k := newKeptPlayback(t, format)
			check := func(key Key, from int) {
				t.Helper()
				var init, joined []byte
				if format == FMP4 {
					init = k.read(k.m.Init(t.Context(), key, k.opener(key)))
					joined = slices.Clone(init)
				}
				for n := from; n < k.plan.Len(); n++ {
					data := k.read(k.m.Segment(t.Context(), key, k.opener(key), n))
					joined = append(joined, data...)
					video, audio := k.probe(append(slices.Clone(init), data...)).starts()
					want := k.plan.Start(n).Seconds() + timestampOffset.Seconds()
					// Within an audio frame, and the lead of the video's
					// keyframe (see keyframeLead).
					if math.Abs(video-want) > 0.02 || math.Abs(audio-video) > 0.045 {
						t.Errorf("segment %d: video starts at %.3f, audio at %.3f, the segment at %.3f", n, video, audio, want)
					}
				}
				path := filepath.Join(t.TempDir(), "joined."+format.Extension())
				if err := os.WriteFile(path, joined, 0o600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(k.ffmpeg, "-hide_banner", "-loglevel", "error", "-xerror", "-i", path, "-map", "0", "-f", "null", "-").CombinedOutput(); err != nil {
					t.Errorf("the joined segments do not decode: %v: %s", err, out)
				}
				var last float64
				for i, p := range k.probe(joined).Packets {
					if p.Type != "audio" {
						continue
					}
					at, _ := strconv.ParseFloat(p.PTS, 64)
					duration, _ := strconv.ParseFloat(p.Duration, 64)
					if i > 0 && last != 0 && math.Abs(at-last) > 0.001 {
						t.Errorf("the audio jumps from %.4f to %.4f", last, at)
						break
					}
					last = at + duration
				}
			}
			check(k.key("web-1", 1), 0)
			k.m.Stop("web-1")
			check(k.key("web-2", 2), 2)
		})
	}
}

// Another user, another device, or another conversion of the video,
// another quality or burned subtitle, converts its own: only the same
// playback takes a kept video over. Another conversion on the same device
// ends the video kept there.
func TestOnlyTheSamePlaybackTakesKeptVideoOver(t *testing.T) {
	k := newKeptPlayback(t, TS)
	k.m.LimitConversions(func() int { return 0 })
	first := k.key("web-1", 1)
	k.segment(first, 0)
	k.m.Stop("web-1")
	kept, _ := k.video(first)
	bob := k.key("bob-1", 2)
	bob.User = "bob"
	phone := k.key("phone-1", 2)
	phone.Device = "phone"
	for _, other := range []Key{bob, phone} {
		k.segment(other, 0)
		if e, runs := k.video(other); e == kept || runs != 1 {
			t.Errorf("%s/%s took the kept video over", other.User, other.Device)
		}
	}
	if again, _ := k.video(first); again != kept {
		t.Error("another user's or device's play ended the kept video")
	}
	lower := k.key("web-2", 1)
	lower.Video = "h264 128x72"
	k.segment(lower, 0)
	if e, _ := k.video(lower); e == kept {
		t.Error("another quality took the kept video over")
	}
	k.m.mu.Lock()
	_, still := k.m.encodings[first.keptVideo()]
	k.m.mu.Unlock()
	if still {
		t.Error("the video kept on the device runs on beside its other quality")
	}
	if _, err := os.Stat(kept.dir); !os.IsNotExist(err) {
		t.Errorf("the segments of the ended kept video: %v", err)
	}
}

// Kept video no session takes over stops once kept long enough, and its
// segments, video and audio, are removed.
func TestUnclaimedKeptVideoStops(t *testing.T) {
	k := newKeptPlayback(t, FMP4)
	k.m.keepFor.Store(int64(time.Second))
	key := k.key("web-1", 1)
	k.segment(key, 0)
	e, _ := k.video(key)
	k.m.mu.Lock()
	audio := k.m.kept[key.keptVideo().playback()].track.dir
	k.m.mu.Unlock()
	k.m.Stop("web-1")
	deadline := time.Now().Add(30 * time.Second)
	for {
		k.m.mu.Lock()
		left := len(k.m.encodings) + len(k.m.kept)
		k.m.mu.Unlock()
		_, videoErr := os.Stat(e.dir)
		_, audioErr := os.Stat(audio)
		if left == 0 && os.IsNotExist(videoErr) && os.IsNotExist(audioErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the kept video still runs: %d left, %v, %v", left, videoErr, audioErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if count, _ := k.m.Conversions(); count != 0 {
		t.Errorf("%d conversions at once", count)
	}
}

// With kept video off, a new session for another track converts the video
// again, its own, as an encoding of the session.
func TestTrackChangeConvertsAgainWithoutKeeping(t *testing.T) {
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	first := k.key("web-1", 1)
	k.segment(first, 2)
	k.m.Stop("web-1")
	switched := k.key("web-2", 2)
	if got := k.probe(k.segment(switched, 2)).channels(); got != 1 {
		t.Errorf("the new track: %d channels", got)
	}
	k.m.mu.Lock()
	defer k.m.mu.Unlock()
	if len(k.m.kept) != 0 || len(k.m.encodings) != 1 || k.m.encodings[switched] == nil {
		t.Errorf("encodings: %v, kept: %d", k.m.encodings, len(k.m.kept))
	}
}

// The joined program map's CRC is MPEG-TS's.
func TestProgramMapCRC(t *testing.T) {
	// A PAT as FFmpeg writes it: program 1 at PID 0x1000.
	section := []byte{0x00, 0xb0, 0x0d, 0x00, 0x01, 0xc1, 0x00, 0x00, 0x00, 0x01, 0xf0, 0x00}
	if got, want := mpegCRC(section), []byte{0x2a, 0xb1, 0x04, 0xb2}; !bytes.Equal(got32(got), want) {
		t.Errorf("CRC %x, want %x", got, want)
	}
}

func got32(v uint32) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
