package hls

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// contains reports whether args hold want, in a row.
func contains(args []string, want ...string) bool {
	for i := range args {
		if i+len(want) <= len(args) && slices.Equal(args[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// Each setting changes the FFmpeg command line of conversions as it says,
// for every encoder, in files and live alike.
func TestSettingsChangeTheArguments(t *testing.T) {
	plan := NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	nvidia := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}}
	nvidiaToneMapping := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}}
	vaapiQVBR := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, QVBR: true}
	file := func(e VideoEncoding) []string {
		return Remux{Input: "in", Audio: -1, Format: TS, Plan: plan, Encode: &e}.args(0)
	}
	encoding := func(encoder string, hw *Hardware) VideoEncoding {
		return VideoEncoding{Encoder: encoder, Level: "4.1", Width: 1280, Height: 720, Bitrate: 4_000_000, FrameRate: 25, Hardware: hw}
	}
	with := func(e VideoEncoding, change func(*VideoEncoding)) VideoEncoding {
		change(&e)
		return e
	}
	for _, tc := range []struct {
		name  string
		args  []string
		has   [][]string
		lacks []string
	}{
		// The preset follows each encoder's scale.
		{"x264 preset", file(with(encoding("libx264", nil), func(e *VideoEncoding) { e.Preset = "slow" })), [][]string{{"-preset", "slow"}}, nil},
		{"x265 preset", file(with(encoding("libx265", nil), func(e *VideoEncoding) { e.Preset = "ultrafast" })), [][]string{{"-preset", "ultrafast"}}, nil},
		{"NVENC preset", file(with(encoding("h264_nvenc", nvidia), func(e *VideoEncoding) { e.Preset = "slow" })), [][]string{{"-preset", "p5"}}, nil},
		{"NVENC fastest preset", file(with(encoding("hevc_nvenc", nvidia), func(e *VideoEncoding) { e.Preset = "superfast" })), [][]string{{"-preset", "p1"}}, nil},
		{"VAAPI preset", file(with(encoding("h264_vaapi", vaapi), func(e *VideoEncoding) { e.Preset = "veryslow" })),
			[][]string{{"-compression_level", "1"}, {"-rc_mode", "VBR"}}, nil},
		// A quality factor rules within the maximum rate.
		{"x264 quality", file(with(encoding("libx264", nil), func(e *VideoEncoding) { e.Quality = 20 })),
			[][]string{{"-crf", "20"}, {"-maxrate", "6000000", "-bufsize", "8000000"}}, []string{"-b:v"}},
		{"x265 quality", file(with(encoding("libx265", nil), func(e *VideoEncoding) { e.Quality = 28 })), [][]string{{"-crf", "28"}}, []string{"-b:v"}},
		{"NVENC quality", file(with(encoding("h264_nvenc", nvidia), func(e *VideoEncoding) { e.Quality = 25 })),
			[][]string{{"-b:v", "0", "-maxrate", "6000000"}, {"-rc", "vbr"}, {"-cq", "25"}}, nil},
		{"VAAPI quality", file(with(encoding("h264_vaapi", vaapiQVBR), func(e *VideoEncoding) { e.Quality = 24 })),
			[][]string{{"-b:v", "4000000"}, {"-rc_mode", "QVBR"}, {"-global_quality", "24"}}, nil},
		{"VAAPI quality without QVBR", file(with(encoding("h264_vaapi", vaapi), func(e *VideoEncoding) { e.Quality = 24 })),
			[][]string{{"-b:v", "4000000"}, {"-rc_mode", "VBR"}}, []string{"-global_quality", "QVBR"}},
		// Tone mapping takes the curve on either side, the peak and the
		// desaturation on the processor.
		{"GPU curve", file(with(encoding("h264_nvenc", nvidiaToneMapping), func(e *VideoEncoding) { e.ToneMap, e.ToneMapCurve, e.ToneMapPeak = true, "reinhard", 1000 })),
			nil, []string{"bt.2390", "peak"}},
		{"CPU curve, peak and desaturation", file(with(encoding("libx264", nil), func(e *VideoEncoding) {
			e.ToneMap, e.ToneMapCurve, e.ToneMapPeak, e.ToneMapDesat = true, "mobius", 1000, 0.5
		})), nil, []string{"hable"}},
		{"BT.2390 on the processor", file(with(encoding("libx264", nil), func(e *VideoEncoding) { e.ToneMap, e.ToneMapCurve = true, "bt2390" })), nil, []string{"bt.2390"}},
		// The deinterlacer, and frame doubling.
		{"bwdif", file(with(encoding("libx264", nil), func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer = true, "bwdif" })), nil, []string{"yadif"}},
		{"yadif doubling", file(with(encoding("libx264", nil), func(e *VideoEncoding) { e.Deinterlace, e.DoubleRate, e.FrameRate = true, true, 50 })), nil, nil},
		{"bwdif doubling", file(with(encoding("h264_vaapi", vaapi), func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer, e.DoubleRate = true, "bwdif", true })), nil, nil},
		// Decoding left to the processor keeps the GPU for the rest.
		{"NVENC decoding in software", file(with(encoding("h264_nvenc", nvidia), func(e *VideoEncoding) { e.DecodeOnCPU = true })), nil, []string{"-hwaccel"}},
		{"VAAPI decoding in software", file(with(encoding("h264_vaapi", vaapi), func(e *VideoEncoding) { e.DecodeOnCPU = true })),
			[][]string{{"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va", "-copyts"}}, []string{"-hwaccel"}},
		{"tone mapping on the GPU, decoding in software", file(with(encoding("h264_nvenc", nvidiaToneMapping), func(e *VideoEncoding) { e.ToneMap, e.DecodeOnCPU = true, true })),
			[][]string{{"-filter_hw_device", "vk", "-copyts"}}, []string{"-hwaccel"}},
	} {
		for _, want := range tc.has {
			if !contains(tc.args, want...) {
				t.Errorf("%s: no %q in %q", tc.name, want, tc.args)
			}
		}
		for _, unwanted := range tc.lacks {
			if slices.ContainsFunc(tc.args, func(arg string) bool { return strings.Contains(arg, unwanted) }) {
				t.Errorf("%s: %q in %q", tc.name, unwanted, tc.args)
			}
		}
	}
	// The filters, whole.
	for _, tc := range []struct {
		name string
		e    VideoEncoding
		want string
	}{
		{"GPU curve", with(encoding("h264_nvenc", nvidiaToneMapping), func(e *VideoEncoding) { e.ToneMap, e.ToneMapCurve = true, "reinhard" }),
			"libplacebo=w=1280:h=720:format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping=reinhard,format=yuv420p"},
		{"CPU curve, peak and desaturation", with(encoding("libx264", nil), func(e *VideoEncoding) {
			e.ToneMap, e.ToneMapCurve, e.ToneMapPeak, e.ToneMapDesat = true, "mobius", 1000, 0.5
		}), "scale=w=1280:h=720,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=mobius:desat=0.5:peak=1000,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"},
		{"BT.2390 on the processor", with(encoding("libx264", nil), func(e *VideoEncoding) { e.ToneMap, e.ToneMapCurve = true, "bt2390" }),
			"scale=w=1280:h=720,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"},
		{"bwdif", with(encoding("libx264", nil), func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer = true, "bwdif" }), "bwdif=0,scale=w=1280:h=720,format=yuv420p"},
		{"yadif doubling", with(encoding("libx264", nil), func(e *VideoEncoding) { e.Deinterlace, e.DoubleRate = true, true }), "yadif=1,scale=w=1280:h=720,format=yuv420p"},
		{"bwdif doubling", with(encoding("h264_vaapi", vaapi), func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer, e.DoubleRate = true, "bwdif", true }),
			"bwdif=1,scale=w=1280:h=720,format=nv12,hwupload"},
	} {
		if got := tc.e.filters(); got != tc.want {
			t.Errorf("%s: filters %q, want %q", tc.name, got, tc.want)
		}
	}
	// The audio filter and the threads, in files and live, for conversions
	// only.
	audio := Remux{Input: "in", Audio: 1, Format: TS, Plan: plan, AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
		AudioFilter: "pan=stereo|c0=c0|c1=c1,volume=2", Threads: 3}
	for name, args := range map[string][]string{"file": audio.args(0), "live": audio.liveArgs("/tmp/live", 0)} {
		if !contains(args, "-c:a", "aac", "-ac", "2", "-b:a", "192000", "-af", "pan=stereo|c0=c0|c1=c1,volume=2") || !contains(args, "-threads", "3") {
			t.Errorf("%s: %q", name, args)
		}
	}
	copied := Remux{Input: "in", Audio: 1, Format: TS, Plan: plan, Threads: 3}
	if args := copied.args(0); slices.Contains(args, "-threads") {
		t.Errorf("a copy runs with threads: %q", args)
	}
}

// FFmpeg makes the segments starting within as many seconds past the end
// of the last one asked for as the settings say, then waits for the
// player: counted in time, the 2 s first segment lets fewer seconds
// through than its count would.
func TestSegmentsAheadFollowTheSettings(t *testing.T) {
	ffmpeg, _ := tools(t)
	input := filepath.Join(t.TempDir(), "long.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24",
		"-t", "60", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48", input).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	var keyframes []time.Duration
	for at := time.Duration(0); at < time.Minute; at += 2 * time.Second {
		keyframes = append(keyframes, at)
	}
	// Segments start at 0, 2, 6, 10, 14, 18 s and so on.
	plan := NewPlan(keyframes, time.Minute)
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.LimitAhead(func() time.Duration { return 10 * time.Second })
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: input, Video: 0, Audio: -1, Format: TS, Plan: plan}, func() {}, nil
	}
	key := Key{Session: "session", Audio: -1, Format: TS}
	made := func() int {
		m.mu.Lock()
		e := m.encodings[key]
		m.mu.Unlock()
		e.mu.Lock()
		defer e.mu.Unlock()
		n := 0
		for _, ready := range e.ready {
			if ready {
				n++
			}
		}
		return n
	}
	// Segment 0 ends at 2 s: those starting before 12 s, up to segment 3
	// at 10 s. Segment 1 ends at 6 s: up to segment 4, at 14 s.
	for n, want := range []int{4, 5} {
		f, err := m.Segment(t.Context(), key, open, n)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		deadline := time.Now().Add(20 * time.Second)
		for made() < want && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(2 * time.Second)
		if got := made(); got != want {
			t.Errorf("segment %d asked for, 10 s ahead: %d segments made, want %d", n, got, want)
		}
	}
}

// The ahead limit is a length of picture past the end of the segment
// asked for: at least the next segment, 120 s unless set.
func TestAheadLimitCountsTime(t *testing.T) {
	m := &Manager{}
	// Segments start at 0, 2, 6, 10 s, then every 4 s to 600 s.
	var keyframes []time.Duration
	for at := time.Duration(0); at < 10*time.Minute; at += 2 * time.Second {
		keyframes = append(keyframes, at)
	}
	plan := NewPlan(keyframes, 10*time.Minute)
	for _, tc := range []struct {
		ahead   time.Duration
		n, want int
	}{
		// Unset: 120 s past the end of segment 0, at 2 s, segments start
		// before 122 s: the last at 118 s.
		{0, 0, plan.Segment(118 * time.Second)},
		{0, 10, plan.Segment(plan.End(10) + 116*time.Second)},
		{30 * time.Second, 0, plan.Segment(30 * time.Second)},
		// A limit shorter than a segment still lets the next one through.
		{time.Second, 5, 6},
		// Near the end, the last segment.
		{0, plan.Len() - 2, plan.Len() - 1},
	} {
		ahead := tc.ahead
		m.LimitAhead(func() time.Duration { return ahead })
		if got := m.aheadLimit(plan, tc.n); got != tc.want {
			t.Errorf("%v ahead of segment %d: up to %d, want %d", tc.ahead, tc.n, got, tc.want)
		}
	}
	if plan.Start(plan.Segment(118*time.Second)) != 118*time.Second {
		t.Fatalf("the plan has no segment at 118 s: %v", plan.starts[:40])
	}
}
