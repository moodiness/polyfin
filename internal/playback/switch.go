package playback

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// When the source of the version a play session repackages or converts
// over HLS fails for good, past its mirrors (see open), the play goes on
// from another version of the title, from the segment the player waits
// for (see hls.Remux.Switch). The player keeps its playlist, and, in
// fragmented MP4, its initialization segment: the other version is made
// into what the player had, the same codec, size and frame rate, its
// video converted, and its audio track of the same language in the same
// codec, converted when it must be. Its times are those of the first
// version, moved by how much later its speech comes when both versions'
// speech is known (see FindSpeech). The playlist served after the switch
// marks it with a discontinuity, for players that load it again.
const (
	// maxSwitches bounds the versions a play session goes on from.
	maxSwitches = 3
	// alikeDuration is how far, as a share of it, another version's
	// duration may be from the first's, and alikeShape its frame's shape:
	// another cut, or a picture cropped otherwise, is no version to go on
	// from at the same time.
	alikeDuration = 0.01
	alikeShape    = 0.02
	// switchesKept is how long a play session's switches are remembered,
	// for its encodings opened again, as after a pause.
	switchesKept = 6 * time.Hour
)

// errNotAlike reports another version a play cannot go on from.
var errNotAlike = errors.New("the version cannot carry the play on")

// errNoAlternate reports a play no other version can carry on.
var errNoAlternate = errors.New("no other version can carry the play on")

// switchKey is a play session's switch from a version.
type switchKey struct {
	session string
	from    accounts.ID
}

// versionSwitch is what a play session went on from once a version
// failed: the version, its analysis, how much later its times are than
// the first version's (see hls.Remux.Shift), and the segment it went on
// from.
type versionSwitch struct {
	to       library.Version
	analysis media.Analysis
	shift    time.Duration
	segment  int
}

// switchState keeps the play sessions' switches: by the version each
// left, and the version each plays now.
type switchState struct {
	by     *cache.Cache[switchKey, versionSwitch]
	latest *cache.Cache[string, library.Version]
}

func newSwitchState() switchState {
	return switchState{by: cache.New[switchKey, versionSwitch](2000, switchesKept),
		latest: cache.New[string, library.Version](2000, switchesKept)}
}

// SwitchedTo returns the version a play session went on from once the
// one it played failed, the last when it switched several times.
func (s *Service) SwitchedTo(session string) (library.Version, bool) {
	return s.switches.latest.Get(session)
}

// Discontinuities are the segments of a play session of version where
// it went on from another version, in order.
func (s *Service) Discontinuities(session string, version accounts.ID) []int {
	var segments []int
	for range maxSwitches {
		sw, ok := s.switches.by.Get(switchKey{session, version})
		if !ok {
			break
		}
		segments = append(segments, sw.segment)
		version = sw.to.ID
	}
	return segments
}

// carry is what a playback is carried on into: what the app plays of the
// first version, which every version it goes on from is made into.
type carry struct {
	s        *Service
	remux    Remux
	analysis media.Analysis
	plan     hls.Plan
}

// carries reports whether a playback of remux, of analysis and plan, may
// go on from another version: one the app asked to be offered others,
// cut on its keyframes, as an MPEG-TS file is not, and whose video it
// takes converted, or copied as 8-bit SDR in MPEG-TS segments, which
// carry what decodes them.
func carries(remux Remux, analysis media.Analysis, plan hls.Plan) bool {
	if remux.Alternates == nil || plan.Grid() || plan.IsSearched() {
		return false
	}
	if remux.ConvertVideo != nil {
		return true
	}
	video, ok := videoOf(analysis)
	return ok && remux.Format == hls.TS && video.BitDepth <= 8 && hlsRange(video) == "SDR" && (video.Codec == "h264" || video.Codec == "hevc")
}

// switchFrom is the hls.Remux.Switch of an input of version from: it
// goes on from the version the session switched to from it, chosen once.
func (c *carry) switchFrom(from library.Version, switched int) func(ctx context.Context, n int) (hls.Remux, func(), error) {
	if switched >= maxSwitches {
		return nil
	}
	return func(ctx context.Context, n int) (hls.Remux, func(), error) {
		sw, err := c.choose(ctx, from, n)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		return c.input(ctx, sw, switched+1)
	}
}

// choose returns the version the play session goes on from once from
// failed at segment n: the one it chose before, else the first version
// after from in the order the user is offered them, then those before it,
// that is no torrent, did not fail lately, plays as the app's stream
// does, and was not played before in the session. The search is shared
// by the variants of the playback.
func (c *carry) choose(ctx context.Context, from library.Version, n int) (versionSwitch, error) {
	key := switchKey{c.remux.Session, from.ID}
	if sw, ok := c.s.switches.by.Get(key); ok {
		return sw, nil
	}
	v, err := c.s.shared(ctx, "switch\x00"+c.remux.Session+"\x00"+from.ID.String(), func() (any, error) {
		if sw, ok := c.s.switches.by.Get(key); ok {
			return sw, nil
		}
		played := map[accounts.ID]bool{c.remux.Version.ID: true}
		for version := c.remux.Version.ID; ; {
			sw, ok := c.s.switches.by.Get(switchKey{c.remux.Session, version})
			if !ok {
				break
			}
			played[sw.to.ID], version = true, sw.to.ID
		}
		for _, candidate := range c.candidates(ctx, from) {
			if played[candidate.ID] || candidate.Torrent != nil || c.s.Failed(candidate.ID) || c.s.ReadsInput(candidate) {
				continue
			}
			sw, err := c.alike(ctx, candidate)
			if err != nil {
				c.s.logger.Debug("A version cannot carry a play on", "addon", candidate.Addon, "error", err)
				continue
			}
			sw.segment = n
			c.s.switches.by.Put(key, sw)
			c.s.switches.latest.Put(c.remux.Session, sw.to)
			c.s.logger.Info("A play went on from another version: its source failed", "from", from.Addon, "to", sw.to.Addon,
				"segment", n, "shift", sw.shift)
			return sw, nil
		}
		return nil, errNoAlternate
	})
	if err != nil {
		return versionSwitch{}, err
	}
	return v.(versionSwitch), nil
}

// candidates are the versions the user is offered after from, then those
// before it, from left out.
func (c *carry) candidates(ctx context.Context, from library.Version) []library.Version {
	versions := c.remux.Alternates(ctx)
	i := slices.IndexFunc(versions, func(v library.Version) bool { return v.ID == from.ID })
	if i < 0 {
		return versions
	}
	return slices.Concat(versions[i+1:], versions[:i])
}

// alike analyzes a version, and tells how the play goes on from it, or
// errNotAlike: another length or frame shape, tracks the play cannot make
// into the app's, or a file of a container whose index Polyfin does not
// read (see unindexedFormat): it is searched from before the time asked,
// and may start late, where the play's plan, cut on keyframes, reads the
// file it goes on from at the time asked, from zero.
func (c *carry) alike(ctx context.Context, version library.Version) (versionSwitch, error) {
	analysis, err := c.s.Analyze(ctx, version)
	if err != nil {
		return versionSwitch{}, err
	}
	if unindexedFormat(analysis) {
		return versionSwitch{}, fmt.Errorf("%w: a file without an index Polyfin reads", errNotAlike)
	}
	sw := versionSwitch{to: version, analysis: analysis}
	if _, err := c.target(sw); err != nil {
		return versionSwitch{}, err
	}
	sw.shift = c.s.speechShift(ctx, c.remux.Version.ID, version.ID, analysis)
	return sw, nil
}

// speechShift is how much later the speech of version to comes than that
// of version first, when both are known and the one is the other at the
// same speed; zero otherwise.
func (s *Service) speechShift(ctx context.Context, first, to accounts.ID, analysis media.Analysis) time.Duration {
	reference, ok := s.Speech(ctx, first)
	if !ok {
		return 0
	}
	other, ok := s.Speech(ctx, to)
	if !ok {
		return 0
	}
	cues := make([]subtitles.Cue, len(reference.Speech))
	for i, span := range reference.Speech {
		cues[i] = subtitles.Cue{Start: span.Start, End: span.End}
	}
	timing := speech.Align(cues, other, videoFrameRate(analysis))
	if timing.Ratio != 1 {
		return 0
	}
	return timing.Offset
}

// reopen opens the input the play session went on from last, when it
// switched versions, and reports whether it did: an encoding opened again,
// as after a pause, does not wait for the version that failed to fail
// again.
func (c *carry) reopen(ctx context.Context) (hls.Remux, func(), bool) {
	var last versionSwitch
	switched := 0
	for version := c.remux.Version.ID; switched < maxSwitches; switched++ {
		sw, ok := c.s.switches.by.Get(switchKey{c.remux.Session, version})
		if !ok {
			break
		}
		last, version = sw, sw.to.ID
	}
	if switched == 0 {
		return hls.Remux{}, nil, false
	}
	r, release, err := c.input(ctx, last, switched)
	return r, release, err == nil
}

// input opens the input a play session goes on from, as remuxOpener opens
// a version's: through the source cache, kept as failed once its source
// fails, and switched in turn for the next.
func (c *carry) input(ctx context.Context, sw versionSwitch, switched int) (hls.Remux, func(), error) {
	r, err := c.target(sw)
	if err != nil {
		return hls.Remux{}, nil, err
	}
	hold, err := c.s.Hold(ForUser(ctx, c.remux.User), sw.to)
	if err != nil {
		return hls.Remux{}, nil, err
	}
	src := c.s.open(sw.to)
	unurge := src.Urge()
	target, unregister := c.s.loopback.register(src)
	done := make(chan struct{})
	go c.s.watchSource(sw.to, src, done)
	failed := src.Failed()
	r.Input, r.Failed, r.Failure = target, failed, func() error { return failureOf(src, failed) }
	r.Shift = sw.shift
	r.Switch = c.switchFrom(sw.to, switched)
	return r, func() {
		close(done)
		unregister()
		unurge()
		src.Release()
		hold()
	}, nil
}

// target is what an encoding of the version sw names makes, its input
// left to set: the app's stream of the first version, its plan and
// format, the version's video converted into the codec, size and frame
// rate the app plays, and its audio track of the language the app plays,
// copied when it is alike, else converted into the codec, channels and
// sample rate the app plays.
func (c *carry) target(sw versionSwitch) (hls.Remux, error) {
	first, ok := videoOf(c.analysis)
	video, other := videoOf(sw.analysis)
	if !ok || !other {
		return hls.Remux{}, fmt.Errorf("%w: no video", errNotAlike)
	}
	if d := c.analysis.Duration; d <= 0 || math.Abs(float64(sw.analysis.Duration-d)) > alikeDuration*float64(d) {
		return hls.Remux{}, fmt.Errorf("%w: another length", errNotAlike)
	}
	if a, b := frameShape(first), frameShape(video); a <= 0 || b <= 0 || math.Abs(a-b) > alikeShape*a {
		return hls.Remux{}, fmt.Errorf("%w: another frame shape", errNotAlike)
	}
	tuning := TuningOf(c.s.settings())
	var played hls.Remux
	c.remux.convert(&played, first, tuning)
	r := hls.Remux{Video: video.Index, Audio: -1, Format: c.remux.Format, Plan: c.plan, Threads: tuning.Threads}
	encode, err := c.video(first, video, sw.analysis, played.Encode, tuning)
	if err != nil {
		return hls.Remux{}, err
	}
	r.Encode, r.VideoTag = encode, played.VideoTag
	// The plan's fast segments are converted as the app's first ones were,
	// from this video.
	if c.plan.HasFast() {
		c.remux.fast(&played, first, c.s.Capabilities())
		if played.Fast != nil {
			fast := *played.Fast
			fast.ToneMap, fast.ToneMapOnCPU, fast.Deinterlace, fast.DoubleRate = encode.ToneMap, encode.ToneMapOnCPU, encode.Deinterlace, false
			fast.Burn, fast.DecodeOnCPU, fast.FrameRate, fast.DropFrames = nil, encode.DecodeOnCPU, encode.FrameRate, encode.DropFrames
			r.Fast, r.FastTag = &fast, played.FastTag
		}
	}
	if err := c.audio(&r, sw.analysis, played, tuning); err != nil {
		return hls.Remux{}, err
	}
	return r, nil
}

// video is the conversion of video, of analysis, into the video the app
// plays of first: converted is the app's conversion of it, nil when it
// is copied. A conversion is the same, but the new video's colors, tone
// mapping and deinterlacing; a picture subtitle of the first version's
// own file is not burned in. Copied video is converted into its codec,
// size, profile and frame rate.
func (c *carry) video(first, video media.Stream, analysis media.Analysis, converted *hls.VideoEncoding, t Tuning) (*hls.VideoEncoding, error) {
	codec := first.Codec
	if converted != nil {
		codec = c.remux.ConvertVideo.Codec
	}
	conversion := ConvertVideo(codec, Limits{}, videoStream(video, video.Index, analysis.Remote), c.s.Capabilities())
	if conversion == nil || conversion.Codec != codec {
		return nil, fmt.Errorf("%w: its video cannot be converted to %s", errNotAlike, codec)
	}
	rate := cmp0(first.AverageRate, first.FrameRate)
	if converted == nil {
		conversion.Width, conversion.Height = first.Width&^1, first.Height&^1
		conversion.Bitrate = max(int64(cmp0(float64(first.Bitrate), float64(c.analysis.Bitrate))), 300_000)
		conversion.Peak, conversion.FrameRate, conversion.Size = 0, 0, ConversionSize{}
		if profile := strings.ToLower(first.Profile); codec == "h264" && (profile == "main" || profile == "constrained baseline" || profile == "baseline") {
			conversion.Profile = strings.TrimPrefix(profile, "constrained ")
		}
		alone := c.remux
		alone.ConvertVideo, alone.Variant, alone.Burn, alone.BurnFile, alone.ConvertAudio = conversion, 0, nil, "", nil
		var r hls.Remux
		alone.convert(&r, video, t)
		converted = r.Encode
		converted.Level = conversion.Level(rate)
	} else {
		e := *converted
		e.ToneMap, e.ToneMapOnCPU, e.Deinterlace, e.DoubleRate = conversion.ToneMap, conversion.ToneMapOnCPU, conversion.Deinterlace, false
		e.Burn = nil
		e.DecodeOnCPU = !t.DecodesOnGPU(video.Codec, video.BitDepth)
		converted = &e
		rate = e.FrameRate
	}
	// The frame rate the app plays, which sets the timescale of fragmented
	// MP4 and the frames of each segment.
	if own := cmp0(video.AverageRate, video.FrameRate); rate > 0 {
		converted.FrameRate = rate
		converted.DropFrames = own <= 0 || math.Abs(own-rate) > rate/1000
	}
	return converted, nil
}

// audio sets the audio track r plays of analysis: the first of the
// language of the track the app plays of the first version, else the
// default one; as played, the app's stream, has it: converted into its
// codec, channels and sample rate, or copied when alike, else converted
// into what was copied.
func (c *carry) audio(r *hls.Remux, analysis media.Analysis, played hls.Remux, t Tuning) error {
	first, ok := streamOf(c.analysis, c.remux.Audio)
	if !ok || first.Type != "audio" {
		return nil
	}
	audio, ok := sameLanguage(analysis, first.Language)
	if !ok {
		return fmt.Errorf("%w: no audio", errNotAlike)
	}
	r.Audio = audio.Index
	rate := 0
	if audio.SampleRate != first.SampleRate {
		rate = first.SampleRate
	}
	if played.AudioCodec != "" {
		r.AudioCodec, r.AudioChannels, r.AudioBitrate, r.AudioRate = played.AudioCodec, played.AudioChannels, played.AudioBitrate, rate
		if played.AudioChannels == 2 && audio.Channels > 2 {
			r.AudioFilter = t.downmix(audio.Channels, audio.ChannelLayout)
		}
		return nil
	}
	if audio.Codec == first.Codec && audio.Channels == first.Channels && rate == 0 && strings.EqualFold(audio.Profile, first.Profile) {
		return nil
	}
	if !slices.Contains(audioEncoders, first.Codec) || c.remux.Format == hls.FMP4 && first.Codec != "aac" {
		return fmt.Errorf("%w: its audio cannot be converted to %s", errNotAlike, first.Codec)
	}
	r.AudioCodec, r.AudioChannels, r.AudioRate = first.Codec, max(first.Channels, 1), rate
	if first.Codec != "flac" {
		r.AudioBitrate = cmp.Or(audioBitrate(first), audioBitrateFor(r.AudioChannels, 0))
	}
	if r.AudioChannels == 2 && audio.Channels > 2 {
		r.AudioFilter = t.downmix(audio.Channels, audio.ChannelLayout)
	}
	return nil
}

// sameLanguage is the first audio track of analysis in language, else its
// default one, else its first.
func sameLanguage(analysis media.Analysis, language string) (media.Stream, bool) {
	var fallback media.Stream
	found := false
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		if language != "" && strings.EqualFold(stream.Language, language) {
			return stream, true
		}
		if !found || stream.Default && !fallback.Default {
			fallback, found = stream, true
		}
	}
	return fallback, found
}

// frameShape is a video's width over its height, its pixels' shape
// counted, 0 when not known.
func frameShape(video media.Stream) float64 {
	if video.Width <= 0 || video.Height <= 0 {
		return 0
	}
	shape := float64(video.Width) / float64(video.Height)
	if w, h, ok := strings.Cut(video.SampleAspect, ":"); ok {
		x, errX := strconv.ParseFloat(w, 64)
		y, errY := strconv.ParseFloat(h, 64)
		if errX == nil && errY == nil && x > 0 && y > 0 {
			shape *= x / y
		}
	}
	return shape
}

// cmp0 is a, or b when a is not above zero.
func cmp0(a, b float64) float64 {
	if a > 0 {
		return a
	}
	return b
}
