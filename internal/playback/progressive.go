package playback

import (
	"context"
	"io"
	"time"

	"github.com/moodiness/polyfin/internal/hls"
)

// Progressive streams a version as one file, in remux.Format, hls.FMP4 for
// a fragmented MP4 or hls.TS for MPEG-TS, from start on, to w as FFmpeg
// writes it, at the pace w takes it. Its video and audio are copied or
// converted, and a picture subtitle burned in, as for an HLS remux
// (remux.ConvertVideo, ConvertAudio, Burn and BurnFile); nothing is cut
// into segments, so no keyframe index is needed, and nothing is kept on
// disk. Converted downloads use it, and so may any app taking a single
// stream.
//
// It counts toward the conversions the server runs at once while it runs,
// when it converts the video, on its own even next to the user's other
// plays of the version: one past the settings' limit is refused with
// hls.ErrBusy before anything is read or written (see MayConvertAlone).
// The user's playbacks at once are the caller's to count. StopRemux, for
// remux.Session, ends it with hls.ErrStopped; a source failing, with its
// failure, which marks the version failed as a remux does.
func (s *Service) Progressive(ctx context.Context, remux Remux, start time.Duration, w io.Writer) error {
	return s.segments.Progressive(ctx, remux.key(), s.progressiveOpener(remux), start, w)
}

// MayConvertAlone reports whether a stream as one file converting its
// video may start now: the server converts fewer videos than its settings
// allow, counting the user's own plays (see Progressive).
func (s *Service) MayConvertAlone() bool {
	return s.segments.MayConvertAlone()
}

// progressiveOpener reads the version as a remux does (see remuxOpener),
// without its keyframe index: a version read from its input is read there
// by FFmpeg, any other through the source cache, as a playback, holding
// one of its source's connections when it takes one. An MPEG-TS file or a
// disc's title starts at zero, as their remuxes do, so that a subtitle
// file burned in lines up.
func (s *Service) progressiveOpener(remux Remux) hls.Opener {
	return func(ctx context.Context) (hls.Remux, func(), error) {
		analysis, err := s.Analyze(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		hold, err := s.Hold(ForUser(ctx, remux.User), remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		video, _ := videoOf(analysis)
		r := hls.Remux{Video: video.Index, Audio: audioOf(analysis, remux.Audio), Format: remux.Format}
		if s.ReadsInput(remux.Version) {
			in, err := s.input(ctx, remux.Version)
			if err != nil {
				hold()
				return hls.Remux{}, nil, err
			}
			r.Input, r.InputOptions = in.Path, append(in.Args(), "-start_at_zero")
			remux.convert(&r, video, TuningOf(s.settings()))
			return r, hold, nil
		}
		src := s.open(remux.Version)
		unurge := src.Urge()
		target, unregister := s.loopback.register(src)
		done := make(chan struct{})
		go s.watchSource(remux.Version, src, done)
		release := func() {
			close(done)
			unregister()
			unurge()
			src.Release()
			hold()
		}
		failed := src.Failed()
		r.Input, r.Failed, r.Failure = target, failed, func() error { return failureOf(src, failed) }
		if transportStream(analysis) {
			r.InputOptions = []string{"-start_at_zero"}
			if stream, ok := streamOf(analysis, remux.Audio); ok && stream.Codec == "aac" {
				r.ADTS = true
			}
		}
		remux.convert(&r, video, TuningOf(s.settings()))
		return r, release, nil
	}
}
