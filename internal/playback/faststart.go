package playback

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A faster start for remote sources.
//
// A repackaged or converted HLS play shows its first picture once its
// first segment is made: the version's bytes up to the keyframe after
// 2 seconds, read from the remote source, which with a keyframe every
// 10 seconds is 10 seconds of the version, at its bitrate. With Start
// remote plays faster, a play whose segment where it starts is longer
// than two fast segments has that segment cut into 2-second ones (see
// hls.Plan.FastFrom), converted quickly at 720p at most in the codec the
// app takes; the play then goes on as it would, from the keyframe the next
// segment starts on, after a discontinuity.

// fastHeight caps the fast segments' video.
const fastHeight = 720

// remote reports whether a version's bytes come from elsewhere than the
// server: an addon's link or a torrent's file, not a file of a local
// folder or a recording, which the server serves on its loopback (see
// LocalFiles and FileVersion), nor a version FFmpeg reads from its input
// (see LocalInputs).
func (s *Service) remote(version library.Version) bool {
	return !strings.HasPrefix(version.URL, "http://"+s.loopback.listener.Addr().String()+"/")
}

// FastStarts reports whether a play over HLS of version from start,
// its video converted as conversion says, or copied when nil, starts with
// fast segments: the settings start remote plays faster, the version is
// remote, the segment playing at start is long (see hls.Plan.FastFrom),
// and copied video is 8-bit SDR H.264 or HEVC, which the fast segments
// are converted into, as the GPU or the processor encodes it. Converted
// video kept when the audio track changes (see hls.Manager.KeepVideo)
// starts without them: a new session taking it over would play another
// plan.
func (s *Service) FastStarts(ctx context.Context, version library.Version, conversion *VideoConversion, start time.Duration) bool {
	settings := s.settings()
	if !settings.FastStart || conversion != nil && settings.KeepConvertedVideo {
		return false
	}
	plan, err := s.fastPlan(ctx, version, conversion == nil, start)
	return err == nil && plan.HasFast()
}

// RemuxPlan is how a remux is cut: its version's plan (see Plan), with
// fast segments where it starts when its play asked for them (see
// FastStarts) and they can be made. The same request gives the same
// plan, from the version's keyframes, whatever the settings say since.
func (s *Service) RemuxPlan(ctx context.Context, remux Remux) (hls.Plan, error) {
	if !remux.FastStart {
		return s.Plan(ctx, remux.Version)
	}
	return s.fastPlan(ctx, remux.Version, remux.ConvertVideo == nil, remux.Start)
}

// fastPlan is the plan of a play of version from start, copied or not,
// with fast segments where they can be made (see FastStarts), else as it
// is.
func (s *Service) fastPlan(ctx context.Context, version library.Version, copied bool, start time.Duration) (hls.Plan, error) {
	plan, err := s.Plan(ctx, version)
	if err != nil || !s.remote(version) {
		return plan, err
	}
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return plan, err
	}
	video, ok := videoOf(analysis)
	if !ok || copied && ownConversion(video, s.Capabilities()) == nil {
		return plan, nil
	}
	fast, _ := plan.FastFrom(start)
	return fast, nil
}

// fast sets how an encoding of video makes the fast segments of its plan
// (see hls.Remux.Fast): converted as the remux converts the video, at its
// Variant, or into its own codec when it copies it, at most fastHeight
// tall and at that height's bitrate, with the fastest preset, and by
// bitrate rather than a quality factor.
func (remux Remux) fast(r *hls.Remux, video media.Stream, can Capabilities) {
	c := remux.conversion(video)
	if remux.ConvertVideo == nil {
		c = ownConversion(video, can)
	}
	if c == nil {
		return
	}
	fast := fastConversion(*c)
	e := remux.encoding(&fast, video, can.Tuning)
	e.Preset, e.Quality = "ultrafast", 0
	r.Fast = e
	if remux.Format == hls.FMP4 {
		r.FastTag = RemuxTag(fast.Codec, "mp4")
	}
}

// fastConversion is c at most fastHeight tall, framed as a conversion to
// that rung, at that rung's usual bitrate at most.
func fastConversion(c VideoConversion) VideoConversion {
	if c.Height > fastHeight {
		c.Width, c.Height = frame(fastHeight, c.Width, c.Height)
	}
	usual := rungs[0].usual
	for _, r := range rungs {
		if r.height >= c.Height {
			usual = r.usual
		}
	}
	c.Bitrate = min(c.Bitrate, usual)
	// A peak a bitrate limit set stays, as long as it is below the one the
	// encoder takes by default.
	if c.Peak >= c.Bitrate*3/2 {
		c.Peak = 0
	}
	return c
}

// ownConversion is a conversion of video into its own codec, at its own
// size, profile and bitrate: of 8-bit SDR H.264 or HEVC, on the GPU when
// it encodes that codec, else on the processor; nil for other video, and
// when nothing encodes its codec.
func ownConversion(video media.Stream, can Capabilities) *VideoConversion {
	if video.Codec != "h264" && video.Codec != "hevc" || video.BitDepth > 8 || hlsRange(video) != "SDR" || video.Width <= 0 || video.Height <= 0 {
		return nil
	}
	c := &VideoConversion{Codec: video.Codec, Width: video.Width &^ 1, Height: video.Height &^ 1, Bitrate: max(video.Bitrate, 300_000),
		Deinterlace: video.FieldOrder != "" && video.FieldOrder != "progressive" && video.FieldOrder != "unknown"}
	if video.Bitrate <= 0 {
		c.Bitrate = rungs[0].usual
	}
	if hw := can.Hardware; hw != nil {
		if i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, video.Codec+"_") }); i >= 0 {
			c.Encoder, c.Hardware = hw.Encoders[i], hw
		}
	}
	if c.Encoder == "" {
		for _, e := range videoEncoders {
			if e.codec == video.Codec && slices.Contains(can.Encoders, e.encoder) {
				c.Encoder = e.encoder
			}
		}
	}
	if c.Encoder == "" {
		return nil
	}
	if profile := strings.ToLower(video.Profile); video.Codec == "h264" && (profile == "main" || profile == "constrained baseline" || profile == "baseline") {
		c.Profile = strings.TrimPrefix(profile, "constrained ")
	}
	return c
}
