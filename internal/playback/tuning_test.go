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

// A new server's settings tune nothing but the volume of stereo mixes,
// Jellyfin's 2: conversions otherwise run as before they were settings
// (see hls.TestDefaultSettingsKeepTheArguments).
func TestDefaultSettingsTuneOnlyTheStereoVolume(t *testing.T) {
	s := newService(t, &fakeSource{}, "ffprobe", nil)
	if got := s.Capabilities().Tuning; !reflect.DeepEqual(got, Tuning{DownmixBoost: 2}) {
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
	// and the boost, which apply only when mixing down to stereo; a boost
	// above 1 goes through the limiter, one below 1 needs none. Without an
	// algorithm, FFmpeg's downmix comes first, so that the limiter works on
	// the stereo sent.
	dave := "pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3"
	ownMix := "aformat=channel_layouts=stereo,"
	limited := "alimiter=limit=0.841395:level=disabled"
	normalized := "loudnorm=I=-16:TP=-1.5:LRA=11,aresample="
	for _, tc := range []struct {
		name     string
		codecs   string
		channels int
		layout   string
		tuning   Tuning
		want     *AudioConversion
	}{
		{"at most stereo", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000}},
		{"at most mono", "flac", 6, "5.1", Tuning{MaxAudioChannels: 1}, &AudioConversion{Codec: "flac", Channels: 1}},
		{"96 kb/s a channel", "aac", 6, "5.1", Tuning{AudioBitrate: 96_000}, &AudioConversion{Codec: "aac", Channels: 6, Bitrate: 576_000}},
		{"96 kb/s a channel in stereo", "aac", 2, "stereo", Tuning{AudioBitrate: 96_000}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"Dave750 downmix", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, Downmix: "Dave750"},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: dave}},
		{"downmix with a guessed layout, boosted", "ac3", 6, "", Tuning{MaxAudioChannels: 2, Downmix: "Dave750", DownmixBoost: 1.5},
			&AudioConversion{Codec: "ac3", Channels: 2, Bitrate: 384_000, Filter: dave + ",volume=1.5," + limited}},
		{"FFmpeg's downmix at Jellyfin's volume", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, DownmixBoost: 2},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: ownMix + "volume=2," + limited}},
		{"a quieter downmix is not limited", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, Downmix: "Dave750", DownmixBoost: 0.5},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: dave + ",volume=0.5"}},
		{"7.1 downmixed through 5.1", "aac", 8, "7.1", Tuning{MaxAudioChannels: 2, Downmix: "Ac4"},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000,
				Filter: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7,pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5"}},
		{"a layout the algorithm lacks keeps FFmpeg's downmix, boosted", "aac", 6, "6.0", Tuning{MaxAudioChannels: 2, Downmix: "NightmodeDialogue", DownmixBoost: 2},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: ownMix + "volume=2," + limited}},
		{"stereo is not mixed down", "aac", 2, "stereo", Tuning{Downmix: "Rfc7845", DownmixBoost: 2}, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000}},
		{"5.1 kept is not mixed down", "aac", 6, "5.1", Tuning{Downmix: "Rfc7845", DownmixBoost: 2}, &AudioConversion{Codec: "aac", Channels: 6, Bitrate: 640_000}},
		// Normalized: after the downmix and its volume, back to the
		// source's rate (48 kHz when unknown), then limited, whatever the
		// volume.
		{"mixed down at Jellyfin's volume, normalized", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, DownmixBoost: 2, Normalize: true},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: ownMix + "volume=2," + normalized + "48000," + limited}},
		{"mixed down quieter, normalized", "aac", 6, "5.1(side)", Tuning{MaxAudioChannels: 2, Downmix: "Dave750", DownmixBoost: 0.5, Normalize: true},
			&AudioConversion{Codec: "aac", Channels: 2, Bitrate: 384_000, Filter: dave + ",volume=0.5," + normalized + "48000," + limited}},
		{"5.1 kept, normalized", "aac", 6, "5.1", Tuning{DownmixBoost: 2, Normalize: true},
			&AudioConversion{Codec: "aac", Channels: 6, Bitrate: 640_000, Filter: normalized + "48000," + limited}},
		{"stereo normalized", "ac3", 2, "stereo", Tuning{DownmixBoost: 2, Normalize: true},
			&AudioConversion{Codec: "ac3", Channels: 2, Bitrate: 384_000, Filter: normalized + "48000," + limited}},
	} {
		if got := ConvertAudio(tc.codecs, "", AudioSource{Channels: tc.channels, ChannelLayout: tc.layout}, 0, tc.tuning); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
	if got := ConvertAudio("aac", "", AudioSource{Channels: 2, SampleRate: 44_100}, 0, Tuning{Normalize: true}); got == nil || got.Filter != normalized+"44100,"+limited {
		t.Errorf("normalized at 44.1 kHz: %+v", got)
	}
	// Converted AC-3 and E-AC-3 apply their dynamic range compression as
	// chosen, through their decoder; other codecs have none to apply.
	for _, tc := range []struct {
		codec, drc string
		want       []string
	}{
		{"eac3", "", nil},
		{"eac3", "none", []string{"drc_scale", "0"}},
		{"eac3", "night", []string{"drc_scale", "2"}},
		{"ac3", "none", []string{"drc_scale", "0"}},
		{"AC3", "night", []string{"drc_scale", "2"}},
		{"aac", "none", nil},
		{"truehd", "night", nil},
	} {
		got := ConvertAudio("aac", "2", AudioSource{Codec: tc.codec, Channels: 6, ChannelLayout: "5.1(side)"}, 0, Tuning{DRC: tc.drc})
		if got == nil || !slices.Equal(got.Decoder, tc.want) {
			t.Errorf("%s, %q: %+v, want the decoder's %q", tc.codec, tc.drc, got, tc.want)
			continue
		}
		r := hls.Remux{Audio: 1}
		(Remux{ConvertAudio: got}).convert(&r, media.Stream{Codec: "h264"}, Tuning{})
		if !slices.Equal(r.AudioDecoder, tc.want) {
			t.Errorf("%s, %q: the encoding's decoder %q, want %q", tc.codec, tc.drc, r.AudioDecoder, tc.want)
		}
	}

	// A track converted on its own, progressively or into HLS segments,
	// is mixed down to stereo the same way, as Jellyfin 12.2 does.
	profile := &DeviceProfile{TranscodingProfiles: []TranscodingProfile{{Type: "Audio", Container: "mp3", AudioCodec: "mp3",
		Protocol: "http", MaxAudioChannels: "2"}}}
	source := AudioSource{Container: "flac", Codec: "flac", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48_000}
	decision := DecideAudio(profile, source, AudioOptions{Can: Capabilities{Encoders: []string{"libmp3lame"}, Tuning: Tuning{Downmix: "Dave750"}}})
	// Music is never normalized: apps even it with its NormalizationGain.
	if d := DecideAudio(profile, source, AudioOptions{Can: Capabilities{Encoders: []string{"libmp3lame"}, Tuning: Tuning{Downmix: "Dave750", Normalize: true}}}); d.Target == nil || d.Target.Filter != dave {
		t.Errorf("music normalized: %+v", d.Target)
	}
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

// A source switched to during a play converts its audio as the play did:
// normalized when the play's audio was converted, at the play's rate, with
// the dynamic range compression chosen; only mixed down to stereo when it
// stands for audio the play copied.
func TestSwitchedSourceConvertsAsThePlay(t *testing.T) {
	before := media.Analysis{Streams: []media.Stream{{Index: 1, Type: "audio", Codec: "aac", Channels: 2, SampleRate: 48_000}}}
	after := media.Analysis{Streams: []media.Stream{{Index: 1, Type: "audio", Codec: "eac3", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 44_100}}}
	c := &carry{remux: Remux{Audio: 1, Format: hls.TS}, analysis: before}
	tuning := Tuning{DownmixBoost: 2, Normalize: true, DRC: "night"}
	var converted hls.Remux
	if err := c.audio(&converted, after, hls.Remux{AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 384_000}, tuning); err != nil {
		t.Fatal(err)
	}
	if want := "aformat=channel_layouts=stereo,volume=2,loudnorm=I=-16:TP=-1.5:LRA=11,aresample=48000,alimiter=limit=0.841395:level=disabled"; converted.AudioFilter != want ||
		converted.AudioRate != 48_000 || !slices.Equal(converted.AudioDecoder, []string{"drc_scale", "2"}) {
		t.Errorf("after a conversion: %q at %d Hz, decoder %q, want %q", converted.AudioFilter, converted.AudioRate, converted.AudioDecoder, want)
	}
	var copied hls.Remux
	if err := c.audio(&copied, after, hls.Remux{}, tuning); err != nil {
		t.Fatal(err)
	}
	if want := "aformat=channel_layouts=stereo,volume=2,alimiter=limit=0.841395:level=disabled"; copied.AudioFilter != want || copied.AudioDecoder != nil {
		t.Errorf("after a copy: %q, decoder %q, want %q", copied.AudioFilter, copied.AudioDecoder, want)
	}
}

// A conversion with a slower preset, a quality factor, HEVC and a downmix
// to stereo, normalized, its AC-3 decoded with night compression, plays.
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
	tuning := Tuning{Preset: "faster", HEVCQuality: 30, PreferHEVC: true, Downmix: "Dave750", DownmixBoost: 1.5, MaxAudioChannels: 2, Normalize: true,
		DRC: "night"}
	can := Capabilities{Encoders: m.Encoders(), Tuning: tuning}
	stream := media.Stream{Index: 0, Type: "video", Codec: "h264", Width: 320, Height: 180, AverageRate: 24, BitDepth: 8}
	remux := Remux{Format: hls.FMP4,
		ConvertVideo: ConvertVideo("hevc,h264", Limits{}, MediaStream{Codec: "h264", VideoRange: "SDR", Width: new(320), Height: new(180)}, can),
		ConvertAudio: ConvertAudio("aac", "", AudioSource{Codec: "ac3", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48_000}, 0, tuning)}
	plan := hls.NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	r := hls.Remux{Input: input, Video: 0, Audio: 1, Format: hls.FMP4, Plan: plan}
	remux.convert(&r, stream, tuning)
	if e := r.Encode; e == nil || e.Encoder != "libx265" || e.Preset != "faster" || e.Quality != 30 || r.AudioChannels != 2 || !strings.HasPrefix(r.AudioFilter, "pan=stereo") ||
		!slices.Equal(r.AudioDecoder, []string{"drc_scale", "2"}) {
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
