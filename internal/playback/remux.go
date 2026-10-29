package playback

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// ErrNotRemuxable reports a version that cannot be cut into segments on
// its keyframes: its container has no index, or no video.
var ErrNotRemuxable = errors.New("the version cannot be remuxed")

// Remux is a remux an app plays.
type Remux struct {
	// Session is the play session it belongs to.
	Session string
	// User is who plays it, and Device the app's device, as its URL tells
	// it.
	User    accounts.ID
	Device  string
	Version library.Version
	// Audio is the audio track, as an index among the version's streams
	// (ffprobe's); -1 plays none.
	Audio  int
	Format hls.Format
	// ConvertVideo and ConvertAudio are what the video and the audio are
	// converted to; nil copies them.
	ConvertVideo *VideoConversion
	ConvertAudio *AudioConversion
	// Variant is the quality played of converted video: 0 for
	// ConvertVideo, else its lower step of that number (see
	// VideoConversion.Steps), which the master playlist offers when apps
	// may step down.
	Variant int
	// Burn is the FFmpeg index of the image subtitle burned into converted
	// video, nil for none; BurnFile, the address of an image subtitle file
	// burned in instead, which FFmpeg reads beside the version.
	Burn     *int
	BurnFile string
	// MinSegments is how many segments the app asks the first playlist of
	// a live stream to list (its TranscodingUrl's MinSegments), 0 when it
	// asks none (see hls.Manager.LivePlaylist).
	MinSegments int
	// Channel names the channel, or the video of an addon, a live stream
	// plays, in the log lines of its encoding (see hls.Remux.Channel).
	Channel string
}

// Plan returns how a version is cut into segments, reading its keyframe
// index on first use. An MPEG-TS file, which has no index, is cut every
// few seconds (see hls.NewGridPlan): it plays over HLS only converted, as
// Jellyfin plays such files, with FFmpeg searching it from before the
// time asked (see hls.Plan.Searched). So is a version read from its input
// (see LocalInputs) whose keyframes the disc does not tell. The bytes of
// its first segment are warmed meanwhile, unless a warm of the version ran
// lately (see Warm), in the background when ctx is.
func (s *Service) Plan(ctx context.Context, version library.Version) (hls.Plan, error) {
	plan, err := s.plan(ctx, version)
	if err == nil && !plan.Grid() {
		s.warm(version, 0, false, !background(ctx))
	}
	return plan, err
}

// Segmented reports whether a version is known to be cut into HLS
// segments (see Plan) from what is kept alone, reading nothing from its
// source: a version read from its input, or an MPEG-TS file, always is;
// another once its keyframe index was read, as preparing a play or a
// play over HLS reads it. For a version played as it is, whose
// conversion only a later request may ask for: its play does not wait
// for its index, nor reads a torrent's pieces for it.
func (s *Service) Segmented(ctx context.Context, version library.Version) bool {
	analysis, ok := s.Analyzed(ctx, version.ID)
	if _, video := videoOf(analysis); !ok || !video || analysis.Duration <= 0 {
		return false
	}
	if s.ReadsInput(version) || transportStream(analysis) {
		return true
	}
	if _, ok := s.indexes.Get(version.ID); ok {
		return true
	}
	_, ok = s.savedKeyframes(ctx, version.ID)
	return ok
}

// plan is Plan, without warming.
func (s *Service) plan(ctx context.Context, version library.Version) (hls.Plan, error) {
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return hls.Plan{}, err
	}
	if _, ok := videoOf(analysis); !ok || analysis.Duration <= 0 {
		return hls.Plan{}, ErrNotRemuxable
	}
	if s.ReadsInput(version) {
		plan := hls.NewGridPlan(analysis.Duration)
		if times, err := s.inputKeyframes(ctx, version); err == nil {
			plan = hls.NewPlan(times, analysis.Duration)
		}
		// FFmpeg reads a Blu-ray disc as an MPEG-TS stream, and a DVD on a
		// share as an MPEG program stream: it seeks in both by searching
		// their timestamps.
		if transportStream(analysis) || slices.Contains(strings.Split(analysis.Format, ","), "mpeg") {
			plan = plan.Searched()
		}
		return plan, nil
	}
	times, err := s.keyframes(ctx, version, analysis)
	if errors.Is(err, container.ErrNoIndex) && transportStream(analysis) {
		return hls.NewGridPlan(analysis.Duration).Searched(), nil
	}
	if err != nil {
		return hls.Plan{}, err
	}
	return hls.NewPlan(times, analysis.Duration), nil
}

// transportStream reports whether an analysis is of an MPEG-TS file.
func transportStream(analysis media.Analysis) bool {
	return slices.Contains(strings.Split(analysis.Format, ","), "mpegts")
}

// keyframesTime bounds a keyframe index read, which reads the file's head
// and its index, a few blocks of it, through the source cache. Its
// connections fail once their source sends nothing for their stall
// timeout (a piece's wait for a torrent) past their tries, so a read that
// goes on ends well within it; a torrent's is waited for as a play waits
// for it, at most as long (see torrentPatience). It ends a read that would
// wait for good, as for a block no connection fetches, which then fails
// and is tried again at the next play.
const keyframesTime = 3 * time.Minute

// keyframes returns a version's keyframe times: remembered, saved, or read
// from its container's index through the source cache, with where each
// keyframe starts in the file, kept for Warm. A version whose index cannot
// be read, for what its file holds, is not read again for a while; one
// whose host failed is read again at the next play, and one whose host
// held the reads back is not kept as failed. A read for a playback, ctx
// not marked Background, reads the source as a playback.
func (s *Service) keyframes(ctx context.Context, version library.Version, analysis media.Analysis) ([]time.Duration, error) {
	if times, ok := s.indexes.Get(version.ID); ok {
		return times, nil
	}
	if err, failed := s.unindexed.Get(version.ID); failed {
		return nil, err
	}
	if s.idleTorrent(ctx, version) {
		return nil, ErrTorrentIdle
	}
	if !background(ctx) {
		src := s.open(version)
		defer src.Release()
		defer src.Urge()()
	}
	result, err := s.shared(ctx, "keyframes "+version.ID.String(), func() (any, error) {
		// Shared and kept like an analysis: read to the end, and saved, even
		// once the request that started it is canceled, within
		// keyframesTime. Started for a playback, it reads its source as a
		// playback (see analyze).
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyframesTime)
		defer cancel()
		if !background(ctx) {
			src := s.open(version)
			defer src.Release()
			defer src.Urge()()
		}
		if times, ok := s.savedKeyframes(ctx, version.ID); ok {
			return times, nil
		}
		src := s.open(version)
		defer src.Release()
		started := time.Now()
		// A torrent's file is waited for as a play waits for it.
		reading := ctx
		if version.Torrent != nil {
			var patient func()
			reading, patient = s.torrentPatience(ctx, version)
			defer patient()
		}
		var times []time.Duration
		err := s.readSized(reading, version, analysis, src, func(size int64) error {
			var err error
			if times, err = container.Keyframes(reading, src, size); err == nil {
				// From the blocks just read: no request more.
				if offsets, err := container.KeyframeOffsets(reading, src, size); err == nil {
					s.offsets.Put(version.ID, offsets)
				}
			}
			return err
		})
		if cause := context.Cause(reading); err != nil && errors.Is(cause, errTorrentSlow) {
			err = cause
		}
		if errors.Is(err, container.ErrNoIndex) {
			err = fmt.Errorf("%w: %w", ErrNotRemuxable, err)
		}
		if err == nil && len(times) == 0 {
			err = fmt.Errorf("%w: the index lists no keyframe", ErrNotRemuxable)
		}
		if err != nil {
			switch failure := src.Failure(); {
			case errors.Is(err, ErrNotRemuxable) || errors.Is(err, container.ErrUnreadable):
				s.unindexed.Put(version.ID, err)
			case failure != nil && !errors.Is(failure, source.ErrSlowDown):
				// The source failed, as when an analysis meets it: the
				// version is not offered for a while. A host asking to slow
				// down tells nothing of the file.
				s.fail(version, failure)
			}
			return nil, err
		}
		s.saveKeyframes(ctx, version.ID, times)
		s.logger.Debug("Read a keyframe index", "addon", version.Addon, "keyframes", len(times), "duration", time.Since(started))
		return times, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]time.Duration), nil
}

// savedKeyframes returns a version's saved keyframe times, then
// remembered.
func (s *Service) savedKeyframes(ctx context.Context, version accounts.ID) ([]time.Duration, bool) {
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT keyframes FROM media_keyframes WHERE version_id = $1", version).Scan(&data)
	if err == nil {
		if times, err := decodeKeyframes(data); err == nil {
			s.indexes.Put(version, times)
			return times, true
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Warn("Reading a keyframe index failed", "error", err)
	}
	return nil, false
}

// saveKeyframes saves and remembers a version's keyframe times.
func (s *Service) saveKeyframes(ctx context.Context, version accounts.ID, times []time.Duration) {
	if _, err := s.db.Exec(ctx, `INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)
		ON CONFLICT (version_id) DO UPDATE SET keyframes = excluded.keyframes, indexed_at = now()`, version, encodeKeyframes(times)); err != nil {
		s.logger.Warn("Saving a keyframe index failed", "error", err)
	}
	s.indexes.Put(version, times)
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

// Variants describe a remux in a master playlist: the quality it plays,
// then, when lower is set and it converts the video, the lower ones apps
// may step down to (see VideoConversion.Steps), each the remux with that
// Variant.
func (s *Service) Variants(ctx context.Context, remux Remux, lower bool) ([]hls.Variant, error) {
	analysis, err := s.Analyze(ctx, remux.Version)
	if err != nil {
		return nil, err
	}
	video, ok := videoOf(analysis)
	if !ok {
		return nil, ErrNotRemuxable
	}
	bandwidth := analysis.Bitrate
	if bandwidth <= 0 && analysis.Duration > 0 {
		bandwidth = int64(float64(analysis.Size*8) / analysis.Duration.Seconds())
	}
	variants := []hls.Variant{variant(analysis, video, bandwidth, remux)}
	if lower && remux.ConvertVideo != nil {
		for i := range remux.ConvertVideo.Steps(video.Width, video.Height) {
			step := remux
			step.Variant = i + 1
			variants = append(variants, variant(analysis, video, bandwidth, step))
		}
	}
	return variants, nil
}

// conversion is what the remux converts video to, at its Variant: nil
// when it copies it, or when the conversion has no such variant.
func (r Remux) conversion(video media.Stream) *VideoConversion {
	c := r.ConvertVideo
	if c == nil || r.Variant == 0 {
		return c
	}
	steps := c.Steps(video.Width, video.Height)
	if r.Variant < 0 || r.Variant > len(steps) {
		return nil
	}
	return &steps[r.Variant-1]
}

// HasVariant reports whether the remux's Variant exists: the first, or a
// lower step of converted video, whose number the source's size leaves
// as it is.
func (r Remux) HasVariant() bool {
	return r.Variant == 0 || r.conversion(media.Stream{}) != nil
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
	if c := remux.conversion(video); c != nil {
		// What the encoder writes, at most its peak rate with the audio
		// sent, on average its bitrate with it, at the frame rate the
		// app's profile allows.
		if c.FrameRate > 0 && (v.FrameRate <= 0 || c.FrameRate < v.FrameRate) {
			v.FrameRate = math.Round(c.FrameRate*1000) / 1000
		}
		codecs[0] = c.CodecString(v.FrameRate)
		v.Width, v.Height, v.Range = c.Width, c.Height, "SDR"
		audio := sentAudio(analysis, remux)
		v.Bandwidth, v.AverageBandwidth = c.peak()+audio, c.Bitrate+audio
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

// sentAudio is the bitrate of the audio remux sends, as SentAudioBitrate
// counts it off a limit: its conversion's, else the track's own or, when
// not known, its estimate; 0 without audio.
func sentAudio(analysis media.Analysis, remux Remux) int64 {
	s, ok := streamOf(analysis, remux.Audio)
	if !ok || s.Type != "audio" {
		return 0
	}
	played := &MediaStream{Channels: new(s.Channels)}
	if bitrate := audioBitrate(s); bitrate > 0 {
		played.BitRate = new(bitrate)
	}
	return SentAudioBitrate(played, remux.ConvertAudio)
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
	if !remux.HasVariant() {
		return nil, hls.ErrNotFound
	}
	return s.segments.Init(ctx, remux.key(), s.remuxOpener(remux))
}

// RemuxSegment opens segment n of a remux, at its Variant.
func (s *Service) RemuxSegment(ctx context.Context, remux Remux, n int) (*os.File, error) {
	if !remux.HasVariant() {
		return nil, hls.ErrNotFound
	}
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

// Encodings lists the remuxes and conversions under way, of files and
// channels.
func (s *Service) Encodings() []hls.Running {
	return s.segments.Running()
}

// MayConvert reports whether user may have the video of version converted
// now: the server converts that of fewer playbacks than its settings
// allow, or already converts it for this user's playback of version.
func (s *Service) MayConvert(user, version accounts.ID) bool {
	return s.segments.MayConvert(user.String(), version.String())
}

func (r Remux) key() hls.Key {
	return hls.Key{Session: r.Session, Audio: r.Audio, Format: r.Format, User: r.User.String(), Device: r.Device,
		Version: r.Version.ID.String(), Converts: r.ConvertVideo != nil, Video: r.videoConversion(), Variant: r.Variant}
}

// videoConversion describes what the remux converts the video to, as
// hls.Key.Video tells it: the conversion of its first variant, which sets
// the others', and the subtitle burned in, hashed, as a subtitle file's
// address may hold a token. Empty when the video is copied.
func (r Remux) videoConversion() string {
	c := r.ConvertVideo
	if c == nil {
		return ""
	}
	burn := r.BurnFile
	if r.Burn != nil {
		burn = "stream " + strconv.Itoa(*r.Burn)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%dx%d\x00%d\x00%d\x00%t\x00%t\x00%t\x00%s", c.Codec, c.Encoder, c.Width, c.Height, c.Bitrate,
		c.Peak, c.ToneMap, c.ToneMapOnCPU, c.Deinterlace, burn)))
	return hex.EncodeToString(sum[:12])
}

// remuxOpener reads the version through the source cache, which keeps
// what FFmpeg reads for the next seek. FFmpeg extracts the version's text
// subtitles at the same time, until the version's are all extracted. A
// source failing meanwhile has the version kept as failed, so that the
// next PlaybackInfo plays another. A version that holds one of its
// source's connections takes it while FFmpeg reads (see Hold). A version
// read from its input is read there by FFmpeg itself (see LocalInputs).
// Seeks in a searched version start as far back as playbacks of its file
// learned they must (see useLookback).
func (s *Service) remuxOpener(remux Remux) hls.Opener {
	return func(ctx context.Context) (hls.Remux, func(), error) {
		plan, err := s.Plan(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		if plan.Grid() && remux.ConvertVideo == nil {
			return hls.Remux{}, nil, fmt.Errorf("%w: its video can only be cut converted", ErrNotRemuxable)
		}
		analysis, err := s.Analyze(ctx, remux.Version)
		if err != nil {
			return hls.Remux{}, nil, err
		}
		hold, err := s.Hold(ForUser(ctx, remux.User), remux.Version)
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
		r := hls.Remux{Video: video.Index, Audio: audio, Format: remux.Format, Plan: plan, Subtitles: streams, Extracted: x}
		if plan.IsSearched() {
			s.useLookback(ctx, &r, remux.Version)
		}
		if s.ReadsInput(remux.Version) {
			in, err := s.input(ctx, remux.Version)
			if err != nil {
				hold()
				return hls.Remux{}, nil, err
			}
			// A disc's title may start late, as a Blu-ray disc's clips start
			// at 600 seconds: its timestamps are shifted to start at zero, as
			// the plan's times do.
			r.Input, r.InputOptions = in.Path, append(in.Args(), "-start_at_zero")
			remux.convert(&r, video, TuningOf(s.settings()))
			return r, func() {
				hold()
				s.saveExtracted(context.Background(), remux.Version.ID, x)
			}, nil
		}
		// A remux is a playback: its reads never wait behind background
		// ones of the host.
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
			s.saveExtracted(context.Background(), remux.Version.ID, x)
		}
		// The encoding stops as soon as the source fails, rather than
		// taking a body cut short for the end of the file.
		failed := src.Failed()
		r.Input, r.Failed, r.Failure = target, failed, func() error { return failureOf(src, failed) }
		// An MPEG-TS file starts at the time its first packet tells, such
		// as 1.4 seconds, or hours into a broadcast: its timestamps are
		// shifted to start at zero, as a grid's times do.
		if plan.Grid() {
			r.InputOptions = []string{"-start_at_zero"}
		}
		if stream, ok := streamOf(analysis, remux.Audio); ok && stream.Codec == "aac" && transportStream(analysis) {
			r.ADTS = true
		}
		remux.convert(&r, video, TuningOf(s.settings()))
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
// audio to, at the remux's Variant, tuned by t.
func (remux Remux) convert(r *hls.Remux, video media.Stream, t Tuning) {
	codec := video.Codec
	if c := remux.conversion(video); c != nil {
		codec = c.Codec
	}
	if remux.Format == hls.FMP4 {
		r.VideoTag = RemuxTag(codec, "mp4")
	}
	if c := remux.conversion(video); c != nil {
		rate := video.AverageRate
		if rate <= 0 {
			rate = video.FrameRate
		}
		// Like Jellyfin, frames are doubled up to 30 a second, within the
		// frame rate the app's profile allows, which drops the frames
		// beyond it.
		double := c.Deinterlace && t.DoubleRate && rate > 0 && rate <= 30 && (c.FrameRate <= 0 || rate*2 <= c.FrameRate)
		if double {
			rate *= 2
		}
		drop := c.FrameRate > 0 && rate > c.FrameRate
		if drop {
			rate = c.FrameRate
		}
		// The source's colors, which video converted without tone mapping
		// keeps (see hls.VideoEncoding.Colors).
		colors := hls.Colors{Range: video.ColorRange, Matrix: video.ColorSpace, Transfer: video.ColorTransfer, Primaries: video.ColorPrimaries}
		r.Encode = &hls.VideoEncoding{Encoder: c.Encoder, Level: c.Level(rate), Width: c.Width, Height: c.Height, Bitrate: c.Bitrate, MaxRate: c.Peak,
			FrameRate: rate, DropFrames: drop, Profile: c.Profile, ToneMap: c.ToneMap, ToneMapOnCPU: c.ToneMapOnCPU, Deinterlace: c.Deinterlace,
			Burn: remux.Burn, BurnFile: remux.BurnFile, Colors: colors, Hardware: c.Hardware, Preset: t.Preset, Quality: t.quality(c.Codec),
			ToneMapCurve: t.ToneMapCurve, ToneMapPeak: t.ToneMapPeak, ToneMapDesat: t.ToneMapDesat, Deinterlacer: t.Deinterlacer, DoubleRate: double,
			DecodeOnCPU: !t.DecodesOnGPU(video.Codec, video.BitDepth)}
	}
	if c := remux.ConvertAudio; c != nil && r.Audio >= 0 {
		r.AudioCodec, r.AudioChannels, r.AudioBitrate, r.AudioFilter = c.Codec, c.Channels, c.Bitrate, c.Filter
	}
	if r.Encode != nil || r.AudioCodec != "" {
		r.Threads = t.Threads
	}
}
