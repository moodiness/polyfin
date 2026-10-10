package playback

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// A new server's settings tune nothing: conversions run as before they
// were settings (see hls.TestDefaultSettingsKeepTheArguments).
func TestDefaultSettingsTuneNothing(t *testing.T) {
	s := newService(t, &fakeSource{}, "ffprobe", nil)
	if got := s.Capabilities().Tuning; !reflect.DeepEqual(got, Tuning{}) {
		t.Errorf("default tuning: %+v", got)
	}
}

func TestTuningChangesConversions(t *testing.T) {
	can := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true}
	sdr := MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(1920), Height: new(1080)}
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", Width: new(3840), Height: new(2160)}
	dolby := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "DOVI", Width: new(3840), Height: new(2160)}
	tuned := func(t Tuning) Capabilities {
		c := can
		c.Tuning = t
		return c
	}
	// HEVC comes first for the apps listing it first, once allowed.
	for _, tc := range []struct {
		codecs string
		prefer bool
		want   string
	}{{"hevc,h264", false, "h264"}, {"hevc,h264", true, "hevc"}, {"h264,hevc", true, "h264"}, {"hevc", false, "hevc"}} {
		if got := ConvertVideo(tc.codecs, Limits{}, sdr, tuned(Tuning{PreferHEVC: tc.prefer})); got == nil || got.Codec != tc.want {
			t.Errorf("%s, HEVC preferred %v: %+v, want %s", tc.codecs, tc.prefer, got, tc.want)
		}
	}
	// Without tone mapping, HDR converts as it is, at full height, but not
	// Dolby Vision that other players cannot read.
	if got := ConvertVideo("h264", Limits{}, hdr, tuned(Tuning{NoToneMapping: true})); got == nil || got.ToneMap || got.Height != 1080 {
		t.Errorf("HDR without tone mapping: %+v", got)
	}
	if got := ConvertVideo("h264", Limits{}, dolby, tuned(Tuning{NoToneMapping: true})); got != nil {
		t.Errorf("Dolby Vision without tone mapping: %+v", got)
	}

	// Audio: the channel cap, the bitrate per channel, Jellyfin's downmix
	// and the boost, which apply only when mixing down to stereo.
	dave := "pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3"
	for _, tc := range []struct {
		name     string
		codecs   string
		channels int
		layout   string
		tuning   Tuning
		want     *AudioConversion
	}{
		{"at most stereo", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"at most mono", "flac", 6, "5.1", Tuning{MaxAudioChannels: 1}, &AudioConversion{Codec: "flac", Channels: 1}},
		{"96 kb/s a channel", "aac", 6, "5.1", Tuning{AudioBitrate: 96_000}, &AudioConversion{Codec: "aac", Channels: 6, Bitrate: 576_000}},
		{"96 kb/s a channel in stereo", "aac", 2, "stereo", Tuning{AudioBitrate: 96_000}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"Dave750 downmix", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, Downmix: "Dave750"},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000, Filter: dave}},
		{"downmix with a guessed layout, boosted", "ac3", 6, "", Tuning{MaxAudioChannels: 2, Downmix: "Dave750", DownmixBoost: 1.5},
			&AudioConversion{Codec: "ac3", Channels: 2, Bitrate: 192_000, Filter: dave + ",volume=1.5"}},
		{"7.1 downmixed through 5.1", "aac", 8, "7.1", Tuning{MaxAudioChannels: 2, Downmix: "Ac4"},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000,
				Filter: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7,pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5"}},
		{"a layout the algorithm lacks keeps FFmpeg's downmix, boosted", "aac", 6, "6.0", Tuning{MaxAudioChannels: 2, Downmix: "NightmodeDialogue", DownmixBoost: 2},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000, Filter: "volume=2"}},
		{"stereo is not mixed down", "aac", 2, "stereo", Tuning{Downmix: "Rfc7845", DownmixBoost: 2}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"5.1 kept is not mixed down", "aac", 6, "5.1", Tuning{Downmix: "Rfc7845", DownmixBoost: 2}, &AudioConversion{Codec: "aac", Channels: 6, Bitrate: 384_000}},
	} {
		if got := ConvertAudio(tc.codecs, "", tc.channels, tc.layout, tc.tuning); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}

	// A track converted on its own, progressively or into HLS segments,
	// is mixed down to stereo the same way, as Jellyfin 12.2 does.
	profile := &DeviceProfile{TranscodingProfiles: []TranscodingProfile{{Type: "Audio", Container: "mp3", AudioCodec: "mp3",
		Protocol: "http", MaxAudioChannels: "2"}}}
	source := AudioSource{Container: "flac", Codec: "flac", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48_000}
	decision := DecideAudio(profile, source, AudioOptions{Can: Capabilities{Encoders: []string{"libmp3lame"}, Tuning: Tuning{Downmix: "Dave750"}}})
	if decision.Target == nil || decision.Target.Channels != 2 || decision.Target.Filter != dave {
		t.Fatalf("progressive downmix: %+v", decision.Target)
	}
	if a := audioConversion("http://127.0.0.1:1/a.flac", nil, decision.Target); a.Channels != 2 || a.Filter != dave {
		t.Errorf("progressive downmix conversion: %+v", a)
	}
	stereo := source
	stereo.Channels, stereo.ChannelLayout = 2, "stereo"
	if d := DecideAudio(profile, stereo, AudioOptions{Can: Capabilities{Encoders: []string{"libmp3lame"}, Tuning: Tuning{Downmix: "Dave750"}}}); d.Target == nil || d.Target.Filter != "" {
		t.Errorf("stereo mixed down: %+v", d.Target)
	}

	// The encoding takes the preset, the quality of its codec, the curve,
	// the deinterlacer, frame doubling up to 30 frames a second, the GPU's
	// decoding and the threads.
	gpu := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}}
	tuning := Tuning{Preset: "slow", H264Quality: 21, HEVCQuality: 27, ToneMapCurve: "mobius", ToneMapPeak: 400, ToneMapDesat: 0.5,
		Deinterlacer: "bwdif", DoubleRate: true, CPUDecoded: []string{"hevc_10bit"}, Threads: 4}
	interlaced := MediaStream{Codec: "hevc", VideoRange: "SDR", Width: new(1920), Height: new(1080), IsInterlaced: true}
	for _, tc := range []struct {
		name   string
		codecs string
		stream media.Stream
		want   hls.VideoEncoding
	}{
		{"25 frames a second doubled, 10-bit HEVC decoded in software", "h264",
			media.Stream{Codec: "hevc", BitDepth: 10, AverageRate: 25},
			hls.VideoEncoding{Encoder: "h264_nvenc", Level: "4.2", Width: 1920, Height: 1080, Bitrate: 10_000_000, FrameRate: 50, Deinterlace: true, Hardware: gpu,
				Preset: "slow", Quality: 21, ToneMapCurve: "mobius", ToneMapPeak: 400, ToneMapDesat: 0.5, Deinterlacer: "bwdif", DoubleRate: true, DecodeOnCPU: true}},
		{"50 frames a second kept, HEVC's quality, 8-bit decoded on the GPU", "hevc",
			media.Stream{Codec: "hevc", BitDepth: 8, AverageRate: 50},
			hls.VideoEncoding{Encoder: "libx265", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 10_000_000, FrameRate: 50, Deinterlace: true,
				Preset: "slow", Quality: 27, ToneMapCurve: "mobius", ToneMapPeak: 400, ToneMapDesat: 0.5, Deinterlacer: "bwdif"}},
	} {
		c := can
		c.Hardware, c.Tuning = gpu, tuning
		remux := Remux{ConvertVideo: ConvertVideo(tc.codecs, Limits{}, interlaced, c), ConvertAudio: &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}}
		r := hls.Remux{Audio: 1}
		remux.convert(&r, tc.stream, tuning)
		if r.Encode == nil || !reflect.DeepEqual(*r.Encode, tc.want) || r.Threads != 4 {
			t.Errorf("%s: %+v, %d threads, want %+v", tc.name, r.Encode, r.Threads, tc.want)
		}
	}
}

// Untuned, a conversion keeps the frame rate, decodes on the GPU and lets
// FFmpeg choose its threads, as before the settings.
func TestUntunedConversionKeepsItsFrameRate(t *testing.T) {
	gpu := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}}
	can := Capabilities{Encoders: []string{"libx264"}, Hardware: gpu}
	interlaced := MediaStream{Codec: "hevc", VideoRange: "SDR", Width: new(1920), Height: new(1080), IsInterlaced: true}
	remux := Remux{ConvertVideo: ConvertVideo("h264", Limits{}, interlaced, can), ConvertAudio: &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}}
	r := hls.Remux{Audio: 1}
	remux.convert(&r, media.Stream{Codec: "hevc", BitDepth: 10, AverageRate: 25}, Tuning{})
	want := hls.VideoEncoding{Encoder: "h264_nvenc", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 10_000_000, FrameRate: 25, Deinterlace: true, Hardware: gpu}
	if r.Encode == nil || !reflect.DeepEqual(*r.Encode, want) || r.Threads != 0 || r.AudioFilter != "" {
		t.Errorf("%+v, %d threads, audio filter %q, want %+v", r.Encode, r.Threads, r.AudioFilter, want)
	}
}

// A conversion with a slower preset, a quality factor, HEVC and a downmix
// to stereo plays.
func TestTunedConversionPlays(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	input := filepath.Join(t.TempDir(), "surround.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "aevalsrc=sin(440*2*PI*t)|sin(550*2*PI*t)|sin(660*2*PI*t)|0.1*sin(50*2*PI*t)|sin(770*2*PI*t)|sin(880*2*PI*t):s=48000:c=5.1",
		"-t", "12", "-c:v", "libx264", "-preset", "ultrafast", "-g", "144", "-c:a", "ac3", "-b:a", "448k", input).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	m, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	if !slices.Contains(m.Encoders(), "libx265") {
		t.Skip("FFmpeg has no libx265")
	}
	tuning := Tuning{Preset: "faster", HEVCQuality: 30, PreferHEVC: true, Downmix: "Dave750", DownmixBoost: 1.5, MaxAudioChannels: 2}
	can := Capabilities{Encoders: m.Encoders(), Tuning: tuning}
	stream := media.Stream{Index: 0, Type: "video", Codec: "h264", Width: 320, Height: 180, AverageRate: 24, BitDepth: 8}
	remux := Remux{Format: hls.FMP4,
		ConvertVideo: ConvertVideo("hevc,h264", Limits{}, MediaStream{Codec: "h264", VideoRange: "SDR", Width: new(320), Height: new(180)}, can),
		ConvertAudio: ConvertAudio("aac", "", 6, "5.1(side)", tuning)}
	plan := hls.NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	r := hls.Remux{Input: input, Video: 0, Audio: 1, Format: hls.FMP4, Plan: plan}
	remux.convert(&r, stream, tuning)
	if e := r.Encode; e == nil || e.Encoder != "libx265" || e.Preset != "faster" || e.Quality != 30 || r.AudioChannels != 2 || !strings.HasPrefix(r.AudioFilter, "pan=stereo") {
		t.Fatalf("conversion: %+v, audio %s %d %q", e, r.AudioCodec, r.AudioChannels, r.AudioFilter)
	}
	open := func(context.Context) (hls.Remux, func(), error) { return r, func() {}, nil }
	key := hls.Key{Session: "session", Audio: 1, Format: hls.FMP4}
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
	joined := read(m.Init(ctx, key, open))
	for n := range plan.Len() {
		joined = append(joined, read(m.Segment(ctx, key, open, n))...)
	}
	path := filepath.Join(t.TempDir(), "joined.mp4")
	if err := os.WriteFile(path, joined, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_name,channels,width,height", "-of", "csv=p=0", path).Output()
	if got := strings.Fields(strings.TrimSpace(string(out))); err != nil || !slices.Equal(got, []string{"hevc,320,180", "aac,2"}) {
		t.Errorf("streams: %q (%v)", got, err)
	}
	// It decodes from start to end without an error.
	if out, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil || len(out) > 0 {
		t.Errorf("decoding: %v: %s", err, out)
	}
}

// "Tone map HDR on the graphics card" turned off reaches the encoding: the
// tuning tone maps on the processor, and so does the encoding of a
// conversion planned with it, on NVIDIA's GPU as on Intel's.
func TestGPUToneMappingTurnedOffReachesTheEncoding(t *testing.T) {
	settings := accounts.DefaultSettings()
	if TuningOf(settings).CPUToneMapping {
		t.Error("the processor tone maps by default")
	}
	settings.GPUToneMapping = false
	tuning := TuningOf(settings)
	if !tuning.CPUToneMapping {
		t.Fatal("turned off, the GPU still tone maps")
	}
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", ColorTransfer: "smpte2084", Width: new(3840), Height: new(2160),
		MasteringDisplay: new(true)}
	for _, gpu := range []*hls.Hardware{
		{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true},
		{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true},
	} {
		can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true, Hardware: gpu, Tuning: tuning}
		remux := Remux{ConvertVideo: ConvertVideo("h264", Limits{}, hdr, can)}
		r := hls.Remux{Audio: -1}
		remux.convert(&r, media.Stream{Codec: "hevc", BitDepth: 10, AverageRate: 24}, tuning)
		if e := r.Encode; e == nil || !e.ToneMap || !e.ToneMapOnCPU || e.Hardware != gpu || e.Height != 720 {
			t.Errorf("%s: %+v", gpu.Method, e)
		}
	}
}
