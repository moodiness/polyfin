package playback

import (
	"cmp"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// AudioSource is a track's stream as an audio decision sees it, with
// Jellyfin's names: its container (flac, mp3, m4a, ogg, wav, hls…), its
// codec, and what is known of its bitrate, channels, sample rate and bit
// depth, zero when unknown.
type AudioSource struct {
	Container  string
	Codec      string
	Profile    string
	Bitrate    int64
	Channels   int
	SampleRate int
	BitDepth   int
}

// AudioOptions are what a PlaybackInfo request chose for a track.
type AudioOptions struct {
	// MaxStreamingBitrate caps the source bitrate for direct play and the
	// converted bitrate; 0 falls back to the profile's.
	MaxStreamingBitrate int64
	EnableDirectPlay    bool
	// ConvertAudio converts the audio even when it could be copied, as apps
	// ask with AllowAudioStreamCopy false.
	ConvertAudio bool
	// Can is what the installed FFmpeg converts with.
	Can Capabilities
}

// AudioDecision is how a track reaches an app: as it is (DirectPlay), else
// through Transcoding, the profile Jellyfin transcodes with, over HLS or
// progressively; Target is what the audio is converted to, nil when it is
// copied, as its codec is one the app takes, into the profile's container.
// Transcoding is nil when the app takes no format Polyfin makes.
type AudioDecision struct {
	DirectPlay  bool
	Container   string
	Reasons     []string
	Transcoding *TranscodingProfile
	HLS         bool
	Target      *AudioTarget
}

// AudioTarget is what a track is converted to: Codec is Jellyfin's codec
// name (aac, mp3, opus, flac, wav…), Bitrate in bits per second (zero for
// lossless codecs), SampleRate and Channels zero to keep the source's.
type AudioTarget struct {
	Codec      string
	Bitrate    int64
	SampleRate int
	Channels   int
}

// audioCodecEncoders are the FFmpeg encoders of the codecs tracks are
// converted to.
var audioCodecEncoders = map[string]string{
	"aac": "aac", "mp3": "libmp3lame", "opus": "libopus", "flac": "flac", "wav": "pcm_s16le", "pcm_s16le": "pcm_s16le",
	"vorbis": "libvorbis", "ac3": "ac3", "eac3": "eac3", "alac": "alac",
}

// AudioEncoder is the FFmpeg encoder of a codec, empty when none.
func AudioEncoder(codec string) string {
	return audioCodecEncoders[strings.ToLower(codec)]
}

// hlsAudioCodecs are the codecs HLS carries in each segment container, as
// Jellyfin lists them.
var hlsAudioCodecs = map[string][]string{
	"ts":  {"aac", "ac3", "eac3", "mp3"},
	"mp4": {"aac", "ac3", "eac3", "mp3", "alac", "flac", "opus", "dts", "truehd"},
}

// lossless codecs have no bitrate to choose.
func lossless(codec string) bool {
	switch codec {
	case "flac", "alac", "wav", "pcm_s16le":
		return true
	}
	return false
}

// DecideAudio answers like Jellyfin 12.2 whether the app behind profile
// can play a track as it is: a direct-play profile of type Audio must take
// its container and codec, the source must be within the bitrate limit,
// and the Audio codec profiles' conditions must hold. Otherwise it is
// converted with the first Audio transcoding profile for streaming whose
// codec FFmpeg encodes, at the profile's music bitrate, capped by the
// limit and its codec profiles' conditions; or copied, when only the
// container is refused and an HLS profile's segments carry the codec.
func DecideAudio(profile *DeviceProfile, source AudioSource, options AudioOptions) AudioDecision {
	d := AudioDecision{Container: source.Container}
	limit := options.MaxStreamingBitrate
	if limit <= 0 && profile.MaxStreamingBitrate != nil {
		limit = *profile.MaxStreamingBitrate
	}
	audio := source.stream()
	s := subject{audio: &audio}
	var matched, codecOnly *DirectPlayProfile
	containerSupported, codecSupported := false, false
	for i := range profile.DirectPlayProfiles {
		p := &profile.DirectPlayProfiles[i]
		if !strings.EqualFold(p.Type, "Audio") {
			continue
		}
		container, codec := listMeets(p.Container, source.Container), listHas(p.AudioCodec, source.Codec)
		containerSupported, codecSupported = containerSupported || container, codecSupported || codec
		if container && codec && matched == nil {
			matched = p
		}
		// As in Jellyfin, a profile takes the codec copied into another
		// container only when it names it, as its codec or its container.
		if !container && codecOnly == nil && (strings.EqualFold(p.AudioCodec, source.Codec) || strings.EqualFold(p.Container, source.Codec)) {
			codecOnly = p
		}
	}
	var reasons reason
	switch {
	case matched != nil:
	case codecOnly != nil:
		reasons |= containerNotSupported
	default:
		if !containerSupported {
			reasons |= containerNotSupported
		}
		if !codecSupported {
			reasons |= audioCodecNotSupported
		}
	}
	if limit > 0 && source.Bitrate > limit {
		reasons |= containerBitrateExceedsLimit
	}
	if matched != nil && reasons == 0 {
		for i := range profile.CodecProfiles {
			c := &profile.CodecProfiles[i]
			if c.Type == "Audio" && listHas(c.Codec, source.Codec) && listMeets(c.Container, source.Container) && s.holdAll(c.ApplyConditions) {
				reasons |= s.failures(c.Conditions)
			}
		}
		if reasons == 0 {
			if options.EnableDirectPlay {
				d.DirectPlay = true
				return d
			}
			reasons |= directPlayError
		}
	}
	d.Reasons = reasons.names()
	for i := range profile.TranscodingProfiles {
		t := &profile.TranscodingProfiles[i]
		codec := strings.ToLower(cmp.Or(t.AudioCodec, t.Container))
		if strings.EqualFold(t.Type, "Audio") && (t.Context == "" || strings.EqualFold(t.Context, "Streaming")) &&
			slices.Contains(options.Can.Encoders, AudioEncoder(codec)) {
			d.Transcoding = t
			break
		}
	}
	t := d.Transcoding
	if t == nil {
		return d
	}
	d.HLS = strings.EqualFold(t.Protocol, "hls")
	// Only the container refused: the codec is copied into HLS segments
	// that carry it.
	if codecOnly != nil && matched == nil && d.HLS && !options.ConvertAudio && reasons == containerNotSupported &&
		slices.Contains(hlsAudioCodecs[strings.ToLower(t.Container)], source.Codec) {
		return d
	}
	target := &AudioTarget{Codec: strings.ToLower(cmp.Or(t.AudioCodec, t.Container))}
	if strings.Contains(target.Codec, ",") {
		target.Codec, _, _ = strings.Cut(target.Codec, ",")
	}
	if !lossless(target.Codec) {
		bitrate := int64(128_000)
		if profile.MusicStreamingTranscodingBitrate != nil && *profile.MusicStreamingTranscodingBitrate > 0 {
			bitrate = *profile.MusicStreamingTranscodingBitrate
		} else if limit > 0 {
			bitrate = limit
		}
		if limit > 0 {
			bitrate = min(bitrate, limit)
		}
		target.Bitrate = bitrate
	}
	target.Channels = source.Channels
	if n, ok := atoi(t.MaxAudioChannels); ok && n > 0 && (target.Channels == 0 || target.Channels > n) {
		target.Channels = n
	}
	target.SampleRate = source.SampleRate
	for i := range profile.CodecProfiles {
		c := &profile.CodecProfiles[i]
		if c.Type != "Audio" || !listHas(c.Codec, target.Codec) || !listMeets(c.Container, t.Container) {
			continue
		}
		for _, condition := range c.Conditions {
			n, ok := atoi(condition.Value)
			if condition.Condition != "LessThanEqual" || !ok || n <= 0 {
				continue
			}
			switch condition.Property {
			case "AudioChannels":
				if target.Channels == 0 || target.Channels > n {
					target.Channels = n
				}
			case "AudioSampleRate":
				if target.SampleRate == 0 || target.SampleRate > n {
					target.SampleRate = n
				}
			case "AudioBitrate":
				if target.Bitrate > int64(n) {
					target.Bitrate = int64(n)
				}
			}
		}
	}
	// Opus takes 48 kHz, and MP3 at most 48 kHz.
	switch {
	case target.Codec == "opus":
		target.SampleRate = 48_000
	case target.Codec == "mp3" && target.SampleRate > 48_000:
		target.SampleRate = 48_000
	}
	d.Target = target
	return d
}

func atoi(text string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	return n, err == nil
}

// stream is the source's audio as a Jellyfin MediaStream, for codec
// profile conditions.
func (s AudioSource) stream() MediaStream {
	m := MediaStream{Type: "Audio", Codec: s.Codec, Profile: s.Profile}
	if s.Bitrate > 0 {
		m.BitRate = new(s.Bitrate)
	}
	if s.Channels > 0 {
		m.Channels = new(s.Channels)
	}
	if s.SampleRate > 0 {
		m.SampleRate = new(s.SampleRate)
	}
	if s.BitDepth > 0 {
		m.BitDepth = new(s.BitDepth)
	}
	return m
}

// ErrUnsupportedSource reports a track stream FFmpeg may not read for a
// user: a DASH manifest of a source confined to public addresses, whose
// segments FFmpeg would fetch itself.
var ErrUnsupportedSource = errors.New("the track's stream cannot be read within its source's rules")

// audioInput is where FFmpeg reads a track from, through the source rules:
// a file through the shared source cache, an HLS playlist through the
// loopback interface, which rewrites the addresses it lists, and a DASH
// manifest directly, only for a source that may reach any address. release
// frees what it holds.
func (s *Service) audioInput(version library.Version) (input string, options []string, release func(), err error) {
	manifest := ""
	if version.Audio != nil {
		manifest = version.Audio.Manifest
	}
	switch manifest {
	case "hls":
		target, free := s.loopback.registerLive(version)
		return target, media.LiveOptions, free, nil
	case "dash":
		if version.Confined || len(version.Headers) > 0 {
			return "", nil, nil, ErrUnsupportedSource
		}
		return version.URL, nil, func() {}, nil
	}
	src := s.open(version)
	unurge := src.Urge()
	target, free := s.loopback.register(src)
	return target, nil, func() { free(); unurge(); src.Release() }, nil
}

// audio describes a conversion to target, read from version.
func audioConversion(input string, options []string, target *AudioTarget) hls.Audio {
	a := hls.Audio{Input: input, InputOptions: options, Encoder: "copy"}
	if target != nil {
		a.Encoder, a.Bitrate, a.SampleRate, a.Channels = AudioEncoder(target.Codec), target.Bitrate, target.SampleRate, target.Channels
	}
	return a
}

// ConvertAudio streams a track converted to target in format, an FFmpeg
// muxer, from start, to w.
func (s *Service) ConvertAudio(ctx context.Context, version library.Version, target *AudioTarget, format string, start time.Duration, w io.Writer) error {
	input, options, release, err := s.audioInput(version)
	if err != nil {
		return err
	}
	defer release()
	return s.segments.Convert(ctx, audioConversion(input, options, target), format, start, w)
}

// AudioRemux is a track an app plays converted, or copied, into HLS
// segments: its play session, version and duration, and what it is
// converted to (nil to copy it).
type AudioRemux struct {
	Session  string
	Version  library.Version
	Duration time.Duration
	Target   *AudioTarget
	Format   hls.Format
}

func (r AudioRemux) key() hls.AudioKey {
	return hls.AudioKey{Session: r.Session, Format: r.Format}
}

func (s *Service) audioOpener(r AudioRemux) hls.AudioOpener {
	return func(ctx context.Context) (hls.Audio, hls.Plan, func(), error) {
		input, options, release, err := s.audioInput(r.Version)
		if err != nil {
			return hls.Audio{}, hls.Plan{}, nil, err
		}
		return audioConversion(input, options, r.Target), hls.AudioPlan(r.Duration), release, nil
	}
}

// AudioInit opens the initialization segment of a track's HLS segments.
func (s *Service) AudioInit(ctx context.Context, r AudioRemux) (*os.File, error) {
	return s.segments.AudioInit(ctx, r.key(), s.audioOpener(r))
}

// AudioSegment opens segment n of a track's HLS segments.
func (s *Service) AudioSegment(ctx context.Context, r AudioRemux, n int) (*os.File, error) {
	return s.segments.AudioSegment(ctx, r.key(), s.audioOpener(r), n)
}

// AudioDuration is how long a track lasts: as analyzed, else as its addon
// says, else analyzed now.
func (s *Service) AudioDuration(ctx context.Context, version library.Version) (time.Duration, error) {
	if analysis, ok := s.Analyzed(ctx, version.ID); ok && analysis.Duration > 0 {
		return analysis.Duration, nil
	}
	if version.Runtime > 0 {
		return version.Runtime, nil
	}
	analysis, err := s.AnalyzeAudio(ctx, version)
	if err != nil {
		return 0, err
	}
	return analysis.Duration, nil
}
