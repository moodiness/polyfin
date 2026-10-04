package playback

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// ErrNotRemuxable reports a version that cannot be cut into segments on
// its keyframes: its container has no index, or no video.
var ErrNotRemuxable = errors.New("the version cannot be remuxed")

// Remux is a remux an app plays.
type Remux struct {
	// Session is the play session it belongs to.
	Session string
	Version library.Version
	// Audio is the audio track, as an index among the version's streams
	// (ffprobe's); -1 plays none.
	Audio  int
	Format hls.Format
	// ConvertVideo and ConvertAudio are what the video and the audio are
	// converted to; nil copies them.
	ConvertVideo *VideoConversion
	ConvertAudio *AudioConversion
	// Burn is the FFmpeg index of the image subtitle burned into converted
	// video, nil for none.
	Burn *int
}

// Plan returns how a version is cut into segments, reading its keyframe
// index on first use.
func (s *Service) Plan(ctx context.Context, version library.Version) (hls.Plan, error) {
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return hls.Plan{}, err
	}
	if _, ok := videoOf(analysis); !ok || analysis.Duration <= 0 {
		return hls.Plan{}, ErrNotRemuxable
	}
	times, err := s.keyframes(ctx, version, analysis)
	if err != nil {
		return hls.Plan{}, err
	}
	return hls.NewPlan(times, analysis.Duration), nil
}

// keyframes returns a version's keyframe times: remembered, saved, or read
// from its container's index through the source cache.
func (s *Service) keyframes(ctx context.Context, version library.Version, analysis media.Analysis) ([]time.Duration, error) {
	if times, ok := s.indexes.Get(version.ID); ok {
		return times, nil
	}
	if err, failed := s.unindexed.Get(version.ID); failed {
		return nil, err
	}
	result, err, _ := s.flight.Do("keyframes "+version.ID.String(), func() (any, error) {
		// Shared and kept like an analysis: read to the end, and saved, even
		// once the request that started it is canceled.
		ctx := context.WithoutCancel(ctx)
		var data []byte
		err := s.db.QueryRow(ctx, "SELECT keyframes FROM media_keyframes WHERE version_id = $1", version.ID).Scan(&data)
		if err == nil {
			if times, err := decodeKeyframes(data); err == nil {
				s.indexes.Put(version.ID, times)
				return times, nil
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("Reading a keyframe index failed", "error", err)
		}
		src := s.open(version)
		defer src.Release()
		size := analysis.Size
		if size <= 0 {
			if size, err = src.Size(ctx); err != nil {
				return nil, err
			}
		}
		started := time.Now()
		times, err := container.Keyframes(ctx, src, size)
		if errors.Is(err, container.ErrNoIndex) {
			err = fmt.Errorf("%w: %w", ErrNotRemuxable, err)
		}
		if err != nil || len(times) == 0 {
			if err == nil {
				err = fmt.Errorf("%w: the index lists no keyframe", ErrNotRemuxable)
			}
			s.unindexed.Put(version.ID, err)
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)
			ON CONFLICT (version_id) DO UPDATE SET keyframes = excluded.keyframes, indexed_at = now()`, version.ID, encodeKeyframes(times)); err != nil {
			s.logger.Warn("Saving a keyframe index failed", "error", err)
		}
		s.indexes.Put(version.ID, times)
		s.logger.Debug("Read a keyframe index", "addon", version.Addon, "keyframes", len(times), "duration", time.Since(started))
		return times, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]time.Duration), nil
}

// encodeKeyframes packs ascending times as varints of the microseconds
// between them: a few bytes per keyframe.
func encodeKeyframes(times []time.Duration) []byte {
	data := make([]byte, 0, len(times)*3)
	previous := int64(0)
	for _, t := range times {
		micros := t.Microseconds()
		data = binary.AppendVarint(data, micros-previous)
		previous = micros
	}
	return data
}

func decodeKeyframes(data []byte) ([]time.Duration, error) {
	var times []time.Duration
	micros := int64(0)
	for len(data) > 0 {
		delta, n := binary.Varint(data)
		if n <= 0 {
			return nil, errors.New("a keyframe index is corrupt")
		}
		micros += delta
		times = append(times, time.Duration(micros)*time.Microsecond)
		data = data[n:]
	}
	return times, nil
}

// videoOf returns a version's video track: the first that is not a cover.
func videoOf(analysis media.Analysis) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			return stream, true
		}
	}
	return media.Stream{}, false
}

func streamOf(analysis media.Analysis, index int) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Index == index {
			return stream, true
		}
	}
	return media.Stream{}, false
}

// Variant describes a remux in a master playlist.
func (s *Service) Variant(ctx context.Context, remux Remux) (hls.Variant, error) {
	analysis, err := s.Analyze(ctx, remux.Version)
	if err != nil {
		return hls.Variant{}, err
	}
	video, ok := videoOf(analysis)
	if !ok {
		return hls.Variant{}, ErrNotRemuxable
	}
	bandwidth := analysis.Bitrate
	if bandwidth <= 0 && analysis.Duration > 0 {
		bandwidth = int64(float64(analysis.Size*8) / analysis.Duration.Seconds())
	}
	return variant(analysis, video, bandwidth, remux), nil
}

// variant describes in a master playlist what remux makes of video, the
// source being bandwidth bits per second.
func variant(analysis media.Analysis, video media.Stream, bandwidth int64, remux Remux) hls.Variant {
	v := hls.Variant{Bandwidth: bandwidth, Width: video.Width, Height: video.Height, Range: hlsRange(video)}
	if rate := video.AverageRate; rate > 0 {
		v.FrameRate = math.Round(rate*1000) / 1000
	} else if video.FrameRate > 0 {
		v.FrameRate = math.Round(video.FrameRate*1000) / 1000
	}
	codecs := []string{videoCodecString(video)}
	if c := remux.ConvertVideo; c != nil {
		// What the encoder writes, at most its peak rate with the audio.
		codecs[0] = c.CodecString(v.FrameRate)
		v.Width, v.Height, v.Range = c.Width, c.Height, "SDR"
		v.Bandwidth = c.Bitrate*3/2 + 640_000
	}
	if audio, ok := streamOf(analysis, remux.Audio); ok {
		if remux.ConvertAudio != nil {
			// What the encoder writes: AAC-LC from FFmpeg's.
			audio = media.Stream{Codec: remux.ConvertAudio.Codec}
		}
		codecs = append(codecs, audioCodecString(audio))
	}
	// A partial list would make players refuse codecs they could play.
	if !strings.Contains(","+strings.Join(codecs, ",")+",", ",,") {
		v.Codecs = strings.Join(codecs, ",")
	}
	return v
}

// hlsRange is a video's VIDEO-RANGE: PQ and HLG name the transfer of HDR
// video, SDR the rest.
func hlsRange(video media.Stream) string {
	switch video.ColorTransfer {
	case "smpte2084":
		return "PQ"
	case "arib-std-b67":
		return "HLG"
	}
	return "SDR"
}

// videoCodecString is a video's RFC 6381 codec string, or empty when
// ffprobe's description is not enough to write it.
func videoCodecString(video media.Stream) string {
	switch video.Codec {
	case "h264":
		profiles := map[string]string{
			"constrained baseline": "42E0", "baseline": "4200", "main": "4D40", "extended": "5800",
			"high": "6400", "high 10": "6E00", "high 4:2:2": "7A00", "high 4:4:4 predictive": "F400",
		}
		prefix, ok := profiles[strings.ToLower(video.Profile)]
		if !ok || video.Level <= 0 || video.Level > 0xff {
			return ""
		}
		return fmt.Sprintf("avc1.%s%02X", prefix, video.Level)
	case "hevc":
		var profile string
		switch strings.ToLower(video.Profile) {
		case "main":
			profile = "1.6"
		case "main 10":
			profile = "2.4"
		default:
			return ""
		}
		if video.Level <= 0 {
			return ""
		}
		// The tier is not known from ffprobe: the main tier covers
		// nearly every file.
		return "hvc1." + profile + ".L" + strconv.Itoa(video.Level) + ".B0"
	case "av1":
		profiles := map[string]string{"main": "0", "high": "1", "professional": "2"}
		profile, ok := profiles[strings.ToLower(video.Profile)]
		depth := video.BitDepth
		if !ok || video.Level < 0 || depth == 0 {
			return ""
		}
		return fmt.Sprintf("av01.%s.%02dM.%02d", profile, video.Level, depth)
	}
	return ""
}

// audioCodecString is an audio track's RFC 6381 codec string, or empty.
func audioCodecString(audio media.Stream) string {
	switch audio.Codec {
	case "aac":
		switch strings.ToLower(audio.Profile) {
		case "he-aac":
			return "mp4a.40.5"
		case "he-aacv2":
			return "mp4a.40.29"
		}
		return "mp4a.40.2"
	case "mp3":
		return "mp4a.40.34"
	case "ac3":
		return "ac-3"
	case "eac3":
		return "ec-3"
	case "opus":
		return "Opus"
	case "flac":
		return "fLaC"
	case "alac":
		return "alac"
	}
	return ""
}

// RemuxInit opens the initialization segment of a fragmented MP4 remux.
func (s *Service) RemuxInit(ctx context.Context, remux Remux) (*os.File, error) {
	return s.segments.Init(ctx, remux.key(), s.remuxOpener(remux))
}

// RemuxSegment opens segment n of a remux.
func (s *Service) RemuxSegment(ctx context.Context, remux Remux, n int) (*os.File, error) {
	return s.segments.Segment(ctx, remux.key(), s.remuxOpener(remux), n)
}

// RemuxSubtitle returns segment n of an embedded text subtitle track of a
// remux, stream being its FFmpeg index: cut from the track when it was read
// whole through the version's index, else once FFmpeg has extracted it. A
// track the index locates is read whole meanwhile, for the segments after,
// unless remuxes have extracted the version's tracks whole already.
func (s *Service) RemuxSubtitle(ctx context.Context, remux Remux, stream, n int) ([]byte, error) {
	plan, err := s.Plan(ctx, remux.Version)
	if err != nil {
		return nil, err
	}
	if cues, ok := s.keptCues(ctx, trackKey{remux.Version.ID, stream}); ok {
		return hls.SubtitleSegment(cues, plan, n), nil
	}
	if analysis, err := s.Analyze(ctx, remux.Version); err == nil && !s.SubtitlesExtracted(ctx, remux.Version.ID, analysis.Duration) &&
		s.SubtitlesLocated(ctx, remux.Version, analysis)[stream] {
		s.PrefetchSubtitle(remux.Version, analysis, stream)
	}
	if err := s.segments.Subtitles(ctx, remux.key(), s.remuxOpener(remux), n); err != nil {
		return nil, err
	}
	x := s.extractedOf(ctx, remux.Version.ID)
	return hls.SubtitleSegment(x.cues(stream, plan.Start(n), plan.End(n)), plan, n), nil
}

// StopRemux stops the remuxes of a play session.
func (s *Service) StopRemux(session string) {
	s.segments.Stop(session)
}

func (r Remux) key() hls.Key {
	return hls.Key{Session: r.Session, Audio: r.Audio, Format: r.Format}
}

// remuxOpener reads the version through the source cache, which keeps
// what FFmpeg reads for the next seek. FFmpeg extracts the version's text
// subtitles at the same time, until the version's are all extracted.
func (s *Service) remuxOpener(remux Remux) hls.Opener {
	return func(ctx context.Context) (hls.Remux, func(), error) {
		plan, err := s.Plan(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		analysis, err := s.Analyze(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		video, _ := videoOf(analysis)
		audio := audioOf(analysis, remux.Audio)
		x := s.extractedOf(ctx, remux.Version.ID)
		var streams []int
		if !x.Covers(0, analysis.Duration) {
			streams = textSubtitles(analysis)
		}
		src := s.open(remux.Version)
		target, unregister := s.loopback.register(src)
		release := func() {
			unregister()
			src.Release()
			s.saveExtracted(context.Background(), remux.Version.ID, x)
		}
		r := hls.Remux{Input: target, Video: video.Index, Audio: audio, Format: remux.Format, Plan: plan,
			Subtitles: streams, Extracted: x}
		remux.convert(&r, video)
		return r, release, nil
	}
}

// audioOf is the FFmpeg index of the audio track index names, -1 for none.
func audioOf(analysis media.Analysis, index int) int {
	if stream, ok := streamOf(analysis, index); ok && stream.Type == "audio" {
		return stream.Index
	}
	return -1
}

// convert sets what an encoding of video tags and converts its video and
// audio to.
func (remux Remux) convert(r *hls.Remux, video media.Stream) {
	codec := video.Codec
	if remux.ConvertVideo != nil {
		codec = remux.ConvertVideo.Codec
	}
	if remux.Format == hls.FMP4 {
		r.VideoTag = RemuxTag(codec, "mp4")
	}
	if c := remux.ConvertVideo; c != nil {
		rate := video.AverageRate
		if rate <= 0 {
			rate = video.FrameRate
		}
		r.Encode = &hls.VideoEncoding{Encoder: c.Encoder, Level: c.Level(rate), Width: c.Width, Height: c.Height, Bitrate: c.Bitrate,
			FrameRate: rate, ToneMap: c.ToneMap, Deinterlace: c.Deinterlace, Burn: remux.Burn, Hardware: c.Hardware}
	}
	if c := remux.ConvertAudio; c != nil && r.Audio >= 0 {
		r.AudioCodec, r.AudioChannels, r.AudioBitrate = c.Codec, c.Channels, c.Bitrate
	}
}
