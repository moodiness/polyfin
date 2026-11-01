package playback

import (
	"math/bits"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/hls"
)

// Options are what a PlaybackInfo request chose.
type Options struct {
	// MaxStreamingBitrate caps the source bitrate for direct play. 0 falls
	// back to the DeviceProfile's MaxStreamingBitrate, and to no limit when
	// the profile has none.
	MaxStreamingBitrate int64
	// UserBitrate is the user's MaxBitrate, 0 for none: the bitrate limit
	// is theirs, rather than the app's, when it is not lower.
	UserBitrate int64
	// AudioStreamIndex is the audio track asked for; nil plays the source's
	// default.
	AudioStreamIndex *int
	// SubtitleStreamIndex is the subtitle track asked for; nil or -1 shows
	// none.
	SubtitleStreamIndex *int
	// EnableDirectPlay false refuses direct play, with DirectPlayError when
	// nothing else would refuse it.
	EnableDirectPlay bool
	// DirectPlayOff tells that the app itself asked for no direct play,
	// as the Jellyfin app for Android does after playback errors, with
	// EnableDirectPlay false. The direct-play check still runs, as in
	// Jellyfin 12.4 (StreamBuilder.GetVideoDirectPlayProfile, L1392-L1426):
	// a source that would have played as it is gets DirectPlayError beside
	// the reasons its conversion has, and one that would not gets the
	// reasons the check found. Polyfin's own refusals, such as a disc's,
	// leave it false and skip the check.
	DirectPlayOff bool
	// EnableDirectStream changes nothing on its own: Jellyfin 12.2 answers
	// the same whether it is set or not.
	EnableDirectStream bool
	// ConvertAudio converts the audio of a remux even when the app could
	// take it as it is, as apps ask with AllowAudioStreamCopy false;
	// ConvertVideo, the video, as with AllowVideoStreamCopy false.
	ConvertAudio, ConvertVideo bool
	// MaxHeight is the tallest video the app is sent as it is, 0 for no
	// limit: taller video only plays converted, refused direct play with
	// VideoResolutionNotSupported, as a profile's height limit refuses it.
	// It is the user's quality group. Converted video is no taller than
	// either height that is positive: ConversionHeight, the settings' cap,
	// caps it whatever the source.
	MaxHeight, ConversionHeight int
	// Progressive lets the source be streamed as one file, copied or
	// converted, to an app whose video TranscodingProfile asks for it
	// (Protocol http), as apps saving a title for offline viewing do: the
	// Converted downloads setting.
	Progressive bool
	// Can is what the installed FFmpeg converts with.
	Can Capabilities
}

// MediaSource is a version as the decision sees it.
type MediaSource struct {
	// Container is Jellyfin's container value: "mkv", "mkv,webm",
	// "mov,mp4,m4a,3gp,3g2,mj2", …
	Container string
	Bitrate   int64
	// Streams are Jellyfin MediaStreams: external subtitles first, then the
	// container's tracks.
	Streams []MediaStream
}

// Decision is whether the app can play the source as it is.
type Decision struct {
	DirectPlay bool
	// Container is the value PlaybackInfo reports: a single name out of a
	// source listing several, picked with the direct-play profiles.
	Container string
	// Reasons are Jellyfin's TranscodeReasons, in Jellyfin's order, when
	// direct play is refused. They are empty when the app cannot transcode
	// either, since Jellyfin reports them nowhere then.
	Reasons []string
	// DirectPlayOff is set when the app turned direct play off for a
	// source that would have played as it is (see Options.DirectPlayOff):
	// DirectPlayError is then among the Reasons.
	DirectPlayOff bool
	// AudioStreamIndex is the chosen audio track, -1 if none.
	AudioStreamIndex int
	// SubtitleStreamIndex is the chosen subtitle track, -1 if none.
	SubtitleStreamIndex int
	// Subtitles gives each subtitle stream's delivery, keyed by stream
	// index.
	Subtitles map[int]SubtitleDelivery
	// Transcoding is the profile Jellyfin transcodes with when direct play
	// is refused; nil when the DeviceProfile has none for video.
	Transcoding *TranscodingProfile
	// HLS is set when direct play is refused but Polyfin can stream the
	// source over HLS with Transcoding; Progressive, when it can stream it
	// as one file instead, which Options.Progressive allows. Video and
	// Audio then tell how the video and the audio that plays reach the
	// app: copied when nil, else converted.
	HLS         bool
	Progressive bool
	Video       *VideoConversion
	Audio       *AudioConversion
	// Caps are what the app's profile allows the video converted, which
	// the TranscodingUrl carries (see AppCaps); Limit, when the profile's
	// bitrate cap is below what the bitrate limit leaves the video, the
	// limit the conversion counts with: that cap with the audio sent, as
	// HLS requests read the two back. Both are zero when the video is not
	// converted.
	Caps  AppCaps
	Limit int64
}

// Streamed reports whether Polyfin streams the source, copied or
// converted, over HLS or as one file.
func (d Decision) Streamed() bool {
	return d.HLS || d.Progressive
}

// SubtitleDelivery is how a subtitle stream reaches the app.
type SubtitleDelivery struct {
	// Method is "Embed", "External", "Hls" or "Encode".
	Method string
	// Format is the SubtitleProfile's Format verbatim, the extension of an
	// External stream's DeliveryUrl; the stream's codec for Encode.
	Format string
}

// reason is a set of TranscodeReasons, one bit each in Jellyfin's enum
// order:
//
//	ContainerNotSupported, VideoCodecNotSupported, AudioCodecNotSupported,
//	SubtitleCodecNotSupported, AudioIsExternal, SecondaryAudioNotSupported,
//	VideoProfileNotSupported, VideoLevelNotSupported,
//	VideoResolutionNotSupported, VideoBitDepthNotSupported,
//	VideoFramerateNotSupported, RefFramesNotSupported,
//	AnamorphicVideoNotSupported, InterlacedVideoNotSupported,
//	VideoCodecTagNotSupported, VideoBitrateNotSupported,
//	AudioChannelsNotSupported, AudioProfileNotSupported,
//	AudioSampleRateNotSupported, AudioBitDepthNotSupported,
//	AudioBitrateNotSupported, ContainerBitrateExceedsLimit,
//	UnknownVideoStreamInfo, UnknownAudioStreamInfo, DirectPlayError,
//	VideoRangeTypeNotSupported, StreamCountExceedsLimit.
//
// Jellyfin 12.2's answers confirm the relative order of the first four,
// of AudioCodec before SecondaryAudio and VideoCodecTag, of VideoProfile
// before VideoResolution, and of AudioCodec before
// ContainerBitrateExceedsLimit.
type reason uint32

const (
	containerNotSupported reason = 1 << iota
	videoCodecNotSupported
	audioCodecNotSupported
	subtitleCodecNotSupported
	audioIsExternal
	secondaryAudioNotSupported
	videoProfileNotSupported
	videoLevelNotSupported
	videoResolutionNotSupported
	videoBitDepthNotSupported
	videoFramerateNotSupported
	refFramesNotSupported
	anamorphicVideoNotSupported
	interlacedVideoNotSupported
	videoCodecTagNotSupported
	videoBitrateNotSupported
	audioChannelsNotSupported
	audioProfileNotSupported
	audioSampleRateNotSupported
	audioBitDepthNotSupported
	audioBitrateNotSupported
	containerBitrateExceedsLimit
	unknownVideoStreamInfo
	unknownAudioStreamInfo
	directPlayError
	videoRangeTypeNotSupported
	streamCountExceedsLimit
)

var reasonNames = [...]string{
	"ContainerNotSupported", "VideoCodecNotSupported", "AudioCodecNotSupported",
	"SubtitleCodecNotSupported", "AudioIsExternal", "SecondaryAudioNotSupported",
	"VideoProfileNotSupported", "VideoLevelNotSupported",
	"VideoResolutionNotSupported", "VideoBitDepthNotSupported",
	"VideoFramerateNotSupported", "RefFramesNotSupported",
	"AnamorphicVideoNotSupported", "InterlacedVideoNotSupported",
	"VideoCodecTagNotSupported", "VideoBitrateNotSupported",
	"AudioChannelsNotSupported", "AudioProfileNotSupported",
	"AudioSampleRateNotSupported", "AudioBitDepthNotSupported",
	"AudioBitrateNotSupported", "ContainerBitrateExceedsLimit",
	"UnknownVideoStreamInfo", "UnknownAudioStreamInfo", "DirectPlayError",
	"VideoRangeTypeNotSupported", "StreamCountExceedsLimit",
}

// names lists the reasons in enum order.
func (r reason) names() []string {
	if r == 0 {
		return nil
	}
	names := make([]string, 0, bits.OnesCount32(uint32(r)))
	for r != 0 {
		bit := bits.TrailingZeros32(uint32(r))
		names = append(names, reasonNames[bit])
		r &^= 1 << bit
	}
	return names
}

// Decide answers like Jellyfin 12.2 whether the app behind profile can
// direct play source.
//
// The audio track checked against the profile is the one asked for, else
// the one flagged default. A source whose audio tracks are all unflagged
// direct plays whatever their codecs, and gets AudioCodecNotSupported once
// refused for another reason, as Jellyfin does. Codec, container and
// subtitle requirements apply to the whole source; of the direct-play
// profiles, the one with the fewest failures, first in order, gives the
// container and codec reasons. A source whose bitrate is not known, as a
// live HLS stream's, counts against the bitrate limit as the usual
// bitrate of its height (see countedBitrate).
func Decide(profile *DeviceProfile, source MediaSource, options Options) Decision {
	d := decider{profile: profile, source: source}
	d.locate(options)
	decision := Decision{AudioStreamIndex: -1, SubtitleStreamIndex: -1, Transcoding: d.transcoding()}
	if d.played != nil {
		decision.AudioStreamIndex = d.played.Index
	}
	if options.AudioStreamIndex != nil {
		decision.AudioStreamIndex = *options.AudioStreamIndex
	}
	if options.SubtitleStreamIndex != nil {
		decision.SubtitleStreamIndex = *options.SubtitleStreamIndex
	}

	var reasons reason
	limit := options.MaxStreamingBitrate
	if limit <= 0 && profile.MaxStreamingBitrate != nil {
		limit = *profile.MaxStreamingBitrate
	}
	if limit > 0 && d.countedBitrate() > limit {
		reasons |= containerBitrateExceedsLimit
	}
	var matched *DirectPlayProfile
	// refused is set when the app turned direct play off for a source that
	// would have played as it is.
	refused := false
	if (options.EnableDirectPlay || options.DirectPlayOff) && reasons == 0 {
		var why reason
		matched, why = d.directPlay()
		reasons |= why
		if !options.EnableDirectPlay {
			refused, matched = matched != nil, nil
		}
	}
	tall := d.taller(options.MaxHeight)
	if tall {
		matched, refused, reasons = nil, false, reasons|videoResolutionNotSupported
	}
	decision.DirectPlay = matched != nil
	if decision.DirectPlay {
		decision.Container = singleContainer(source.Container, matched, nil)
	} else {
		decision.Container = singleContainer(source.Container, nil, profile.DirectPlayProfiles)
		if t := decision.Transcoding; t != nil {
			reasons |= d.transcodeReasons(t)
			caps := d.appCaps(t, reasons)
			streamed, video, audio, played := d.stream(t, options, limit, reasons&containerBitrateExceedsLimit != 0 || tall, caps)
			decision.HLS, decision.Progressive = streamed && strings.EqualFold(t.Protocol, "hls"), streamed && !strings.EqualFold(t.Protocol, "hls")
			decision.Video, decision.Audio = video, audio
			if video != nil {
				decision.Caps = caps
				if played != limit {
					decision.Limit = played
				}
			}
			if reasons == 0 || refused {
				reasons |= directPlayError
			}
			decision.DirectPlayOff = refused
			decision.Reasons = reasons.names()
		}
	}

	for i := range source.Streams {
		stream := &source.Streams[i]
		if stream.Type != "Subtitle" {
			continue
		}
		if decision.Subtitles == nil {
			decision.Subtitles = map[int]SubtitleDelivery{}
		}
		if t := decision.Transcoding; !decision.DirectPlay && t != nil {
			decision.Subtitles[stream.Index] = subtitleDelivery(profile.SubtitleProfiles, stream, true, t.Container, t.Protocol)
		} else {
			decision.Subtitles[stream.Index] = subtitleDelivery(profile.SubtitleProfiles, stream, false, source.Container, "")
		}
	}
	return decision
}

// countedBitrate is the bitrate the source counts as against a bitrate
// limit: its own, else the usual bitrate of its height, which converted
// video gets when its source's is not known (see rungs): 10 Mb/s for
// 1080p. Jellyfin 12.4's StreamBuilder.IsBitrateLimitExceeded (lines
// 1655-1679) never limits a remote source, and counts a local one whose
// bitrate it does not know as 40 Mb/s: Polyfin limits every source, so
// that a lower quality picked during a play converts a live stream too.
func (d *decider) countedBitrate() int64 {
	if d.source.Bitrate > 0 || d.video == nil {
		return d.source.Bitrate
	}
	class := sizeClass(*d.video)
	for _, r := range rungs {
		if r.height == class {
			return r.usual
		}
	}
	return 0
}

// decider holds what a decision looks at.
type decider struct {
	profile *DeviceProfile
	source  MediaSource
	video   *MediaStream
	// played is the audio track that plays: asked for, flagged default or
	// first.
	played *MediaStream
	// checked is the audio track checked against the profile: asked for or
	// flagged default.
	checked    *MediaStream
	firstAudio int
	hasAudio   bool
	subtitle   *MediaStream
}

// locate finds the streams the decision is about.
func (d *decider) locate(options Options) {
	var first, flagged, asked *MediaStream
	for i := range d.source.Streams {
		stream := &d.source.Streams[i]
		switch stream.Type {
		case "Video":
			if d.video == nil {
				d.video = stream
			}
		case "Audio":
			if first == nil {
				first = stream
			}
			if flagged == nil && stream.IsDefault {
				flagged = stream
			}
			if options.AudioStreamIndex != nil && stream.Index == *options.AudioStreamIndex {
				asked = stream
			}
		case "Subtitle":
			if options.SubtitleStreamIndex != nil && stream.Index == *options.SubtitleStreamIndex {
				d.subtitle = stream
			}
		}
	}
	d.hasAudio = first != nil
	if first != nil {
		d.firstAudio = first.Index
	}
	d.checked = flagged
	if asked != nil {
		d.checked = asked
	}
	d.played = d.checked
	if d.played == nil {
		d.played = first
	}
}

// taller reports whether any of the source's video is taller than height,
// when it is positive: live sources list each variant they offer.
func (d *decider) taller(height int) bool {
	if height <= 0 {
		return false
	}
	for _, stream := range d.source.Streams {
		if stream.Type == "Video" && stream.Height != nil && *stream.Height > height {
			return true
		}
	}
	return false
}

// directPlay returns the direct-play profile the source plays with, or the
// reasons none does.
func (d *decider) directPlay() (*DirectPlayProfile, reason) {
	shared := d.codecReasons(d.source.Container) | d.containerReasons()
	if d.subtitle != nil {
		switch subtitleDelivery(d.profile.SubtitleProfiles, d.subtitle, false, d.source.Container, "").Method {
		case "Embed", "External":
		default:
			shared |= subtitleCodecNotSupported
		}
	}
	var best *DirectPlayProfile
	var least reason
	for i := range d.profile.DirectPlayProfiles {
		candidate := &d.profile.DirectPlayProfiles[i]
		if !strings.EqualFold(candidate.Type, "Video") {
			continue
		}
		var why reason
		if !listMeets(candidate.Container, d.source.Container) {
			why |= containerNotSupported
		}
		if d.video != nil && !listHas(candidate.VideoCodec, d.video.Codec) {
			why |= videoCodecNotSupported
		}
		if d.checked != nil && !listHas(candidate.AudioCodec, d.checked.Codec) {
			why |= audioCodecNotSupported
		}
		if best == nil || bits.OnesCount32(uint32(why)) < bits.OnesCount32(uint32(least)) {
			best, least = candidate, why
		}
	}
	if best == nil {
		return nil, shared
	}
	if why := least | shared; why != 0 {
		return nil, why
	}
	return best, 0
}

// codecReasons checks the profile's Video and VideoAudio codec profiles
// that apply to the source in container.
func (d *decider) codecReasons(container string) reason {
	return d.codecProfileReasons("Video", d.video, container) | d.codecProfileReasons("VideoAudio", d.checked, container)
}

// codecProfileReasons checks the codec profiles of a type that apply to
// stream in container.
func (d *decider) codecProfileReasons(kind string, stream *MediaStream, container string) reason {
	if stream == nil {
		return 0
	}
	s := d.subject()
	var why reason
	for i := range d.profile.CodecProfiles {
		codec := &d.profile.CodecProfiles[i]
		if codec.Type == kind && listHas(codec.Codec, stream.Codec) && listMeets(codec.Container, container) && s.holdAll(codec.ApplyConditions) {
			why |= s.failures(codec.Conditions)
		}
	}
	return why
}

// subject is what conditions see of the source: its tracks, and its
// video's codec tag, which Jellyfin 12.2 reads from ffprobe (Jellyfin 12.1
// read none).
func (d *decider) subject() subject {
	s := subject{video: d.video, audio: d.checked}
	if d.video != nil {
		s.codecTag = d.video.CodecTag
	}
	if d.checked != nil {
		s.secondary = d.checked.Index != d.firstAudio
	}
	return s
}

// containerReasons checks the profile's Video container profiles that
// apply to the source.
func (d *decider) containerReasons() reason {
	s := d.subject()
	var why reason
	for i := range d.profile.ContainerProfiles {
		container := &d.profile.ContainerProfiles[i]
		if strings.EqualFold(container.Type, "Video") && listMeets(container.Container, d.source.Container) {
			why |= s.failures(container.Conditions)
		}
	}
	return why
}

// transcoding picks the video TranscodingProfile for streaming: the first
// that keeps the video, preferably meeting its codec profiles, then the
// first that keeps the audio that plays.
func (d *decider) transcoding() *TranscodingProfile {
	var best *TranscodingProfile
	bestRank := 0
	for i := range d.profile.TranscodingProfiles {
		candidate := &d.profile.TranscodingProfiles[i]
		if !strings.EqualFold(candidate.Type, "Video") || (candidate.Context != "" && !strings.EqualFold(candidate.Context, "Streaming")) {
			continue
		}
		rank := 0
		if d.video != nil {
			switch {
			case !listHas(candidate.VideoCodec, d.video.Codec):
				rank += 4
			case d.codecProfileReasons("Video", d.video, candidate.Container) != 0:
				rank += 2
			}
		}
		if d.played != nil && !listHas(candidate.AudioCodec, d.played.Codec) {
			rank++
		}
		if best == nil || rank < bestRank {
			best, bestRank = candidate, rank
		}
	}
	return best
}

// transcodeReasons are what transcoding with t cannot keep: a video codec
// it does not take, and audio that is not taken, not carried by its HLS
// segments, or not flagged.
func (d *decider) transcodeReasons(t *TranscodingProfile) reason {
	var why reason
	if d.video != nil && !listHas(t.VideoCodec, d.video.Codec) {
		why |= videoCodecNotSupported
	}
	if d.hasAudio && (d.checked == nil || !streamCarries(t, d.checked.Codec)) {
		why |= audioCodecNotSupported
	}
	return why
}

// hlsVideoCodecs are the video codecs Jellyfin streams over HLS.
var hlsVideoCodecs = []string{"h264", "hevc", "vp9", "av1"}

// videoReasonBits are Jellyfin's VideoReasons: those about the video.
const videoReasonBits = videoCodecNotSupported | videoResolutionNotSupported | anamorphicVideoNotSupported | interlacedVideoNotSupported |
	videoBitDepthNotSupported | videoBitrateNotSupported | videoFramerateNotSupported | videoLevelNotSupported | refFramesNotSupported |
	videoRangeTypeNotSupported | videoProfileNotSupported

// appCaps are what the app's profile allows video converted with t, as
// Jellyfin's StreamBuilder applies its conditions to what it converts to
// (BuildStreamVideoItem, ApplyTranscodingConditions): the conditions of
// the Video codec profiles naming one of the codecs t converts to, over
// HLS those Jellyfin streams so, in t's container, whose ApplyConditions
// the source meets, the first profile prevailing; then, when the video
// does not play as it is or the source is above the bitrate limit (in
// reasons), those of t itself. Width, Height, VideoBitrate and
// VideoFramerate cap whatever the codec; VideoLevel and VideoProfile
// apply to the codec of the profile naming them, never in t's own. Equals
// sets the value and LessThanEqual lowers it, whether the condition is
// required or not; other comparisons, and values that are not numbers
// where numbers are read, are left out, as Jellyfin leaves them.
func (d *decider) appCaps(t *TranscodingProfile, reasons reason) AppCaps {
	var caps AppCaps
	hlsStream := strings.EqualFold(t.Protocol, "hls")
	var codecs []string
	for codec := range strings.SplitSeq(t.VideoCodec, ",") {
		if codec = strings.ToLower(strings.TrimSpace(codec)); codec != "" && (!hlsStream || slices.Contains(hlsVideoCodecs, codec)) {
			codecs = append(codecs, codec)
		}
	}
	s := d.subject()
	for i := len(d.profile.CodecProfiles) - 1; i >= 0; i-- {
		profile := &d.profile.CodecProfiles[i]
		container := profile.Container
		if hlsStream && strings.EqualFold(container, "hls") {
			container = profile.SubContainer
		}
		if profile.Type != "Video" || !listMeets(container, t.Container) || !s.holdAll(profile.ApplyConditions) {
			continue
		}
		for _, codec := range codecs {
			if listHas(profile.Codec, codec) {
				caps.apply(profile.Conditions, codec)
			}
		}
	}
	if reasons&(videoReasonBits|containerBitrateExceedsLimit) != 0 {
		caps.apply(t.Conditions, "")
	}
	return caps
}

// apply applies conditions to caps, those about a codec's own properties
// only for codec, when there is one.
func (c *AppCaps) apply(conditions []ProfileCondition, codec string) {
	for _, condition := range conditions {
		value := strings.TrimSpace(condition.Value)
		if value == "" || condition.Condition != "Equals" && condition.Condition != "LessThanEqual" && condition.Condition != "EqualsAny" {
			continue
		}
		lower := func(current, n int64) int64 {
			if condition.Condition == "Equals" || current <= 0 {
				return n
			}
			return min(current, n)
		}
		n, err := strconv.ParseInt(value, 10, 64)
		number := err == nil && condition.Condition != "EqualsAny"
		switch condition.Property {
		case "Width":
			if number {
				c.Width = int(lower(int64(c.Width), n))
			}
		case "Height":
			if number {
				c.Height = int(lower(int64(c.Height), n))
			}
		case "VideoBitrate":
			if number {
				c.Bitrate = lower(c.Bitrate, n)
			}
		case "VideoFramerate":
			if rate, err := strconv.ParseFloat(value, 64); err == nil && condition.Condition != "EqualsAny" {
				if condition.Condition == "Equals" || c.FrameRate <= 0 {
					c.FrameRate = rate
				} else {
					c.FrameRate = min(c.FrameRate, rate)
				}
			}
		case "VideoLevel":
			if number && codec != "" {
				if c.Levels == nil {
					c.Levels = map[string]int{}
				}
				c.Levels[codec] = int(lower(int64(c.Levels[codec]), n))
			}
		case "VideoProfile":
			if codec != "h264" || condition.Condition == "LessThanEqual" {
				continue
			}
			var allowed []string
			for choice := range strings.SplitSeq(value, "|") {
				if choice = strings.ToLower(strings.ReplaceAll(choice, " ", "")); choice != "" {
					allowed = append(allowed, choice)
				}
			}
			if c.H264Profiles != nil {
				allowed = slices.DeleteFunc(allowed, func(p string) bool { return !slices.Contains(c.H264Profiles, p) })
			}
			c.H264Profiles = allowed
		}
	}
}

// stream decides whether Polyfin can stream the source with t, over HLS,
// or as one file when options allow it and t asks for it (Protocol http),
// and how the video and the audio that plays then reach the app. The video
// is copied when t takes its codec, the codec profiles of its container
// accept it with the codec tag a remux writes, and the source fits the
// bitrate limit; else it is converted, within what the limit leaves once
// the audio sent is taken off (see SentAudioBitrate). The audio is copied
// when t takes its codec and channels, its HLS segments carry the codec,
// and the codec profiles accept it, as the only audio, so never a
// secondary track, and, when the video is converted under a limit, when
// it fits the audio's share of the limit (see streamAudio); else it is
// converted to a codec t takes and its segments carry. Converted
// video keeps within caps, what the app's profile allows (see appCaps);
// a bitrate cap below what the limit leaves the video becomes the limit,
// with the audio sent, which the audio is then decided under too: the
// HLS requests, which see only the bitrates the TranscodingUrl gives, so
// decide the same. played is the limit the conversion counts with.
func (d *decider) stream(t *TranscodingProfile, options Options, limit int64, mustConvert bool, caps AppCaps) (bool, *VideoConversion, *AudioConversion, int64) {
	// Streamed as one file, the video goes out as fragmented MP4 or
	// MPEG-TS, the containers hls.ParseFormat reads.
	_, oneFile := hls.ParseFormat(t.Container)
	hlsStream, progressive := strings.EqualFold(t.Protocol, "hls"), strings.EqualFold(t.Protocol, "http") && options.Progressive && oneFile
	if !hlsStream && !progressive || d.video == nil {
		return false, nil, nil, limit
	}
	s := subject{video: d.video, codecTag: RemuxTag(d.video.Codec, t.Container)}
	convertVideo := options.ConvertVideo || mustConvert || !listHas(t.VideoCodec, d.video.Codec) || d.remuxFails("Video", d.video, t.Container, s)
	audio, audioStreams := d.streamAudio(t, options, limit, convertVideo, s)
	played := limit
	var video *VideoConversion
	if convertVideo {
		// Each round can only lower the audio sent: two settle it.
		for range 3 {
			sent := SentAudioBitrate(d.played, audio)
			if caps.Bitrate <= 0 || played > 0 && VideoLimit(played, sent) <= caps.Bitrate {
				break
			}
			played = caps.Bitrate + sent
			audio, audioStreams = d.streamAudio(t, options, played, true, s)
		}
		if played != limit {
			played = caps.Bitrate + SentAudioBitrate(d.played, audio)
		}
		// Converted video is scaled down to the lower of the heights it is
		// capped at, which never prevents a conversion.
		limits := Limits{Bitrate: played, Video: VideoLimit(played, SentAudioBitrate(d.played, audio)), Group: options.MaxHeight,
			Server: options.ConversionHeight, App: caps}
		if caps.Bitrate > 0 {
			limits.Video = min(limits.Video, caps.Bitrate)
		}
		limits.ByUser = options.UserBitrate > 0 && played > 0 && played >= options.UserBitrate
		if video = ConvertVideo(t.VideoCodec, limits, *d.video, options.Can); video == nil {
			return false, nil, nil, limit
		}
	}
	return audioStreams, video, audio, played
}

// streamAudio decides how the audio that plays reaches the app streamed
// with t under a bitrate limit, 0 for none: copied (nil, true), converted
// (its conversion, true), or not at all (nil, false) when t takes no codec
// Polyfin encodes. Over HLS, only a codec the segments carry is copied or
// converted to (see StreamAudioCodecs). With the video converted under a
// limit, a track whose bitrate is above the audio's share of the limit
// (see audioShare) is converted, as Jellyfin's encoder refuses to copy a
// track above the audio bitrate it counts, that share at most: copied, it
// would leave the video too little. It is copied all the same when t
// takes no codec Polyfin encodes. A track copied with the video fits the
// limit with it.
func (d *decider) streamAudio(t *TranscodingProfile, options Options, limit int64, convertVideo bool, s subject) (*AudioConversion, bool) {
	audio := d.played
	if audio == nil {
		return nil, true
	}
	s.audio = audio
	channelsTaken := true
	if most, err := strconv.Atoi(t.MaxAudioChannels); err == nil && audio.Channels != nil && *audio.Channels > most {
		channelsTaken = false
	}
	copyable := !options.ConvertAudio && channelsTaken && streamCarries(t, audio.Codec) && !d.remuxFails("VideoAudio", audio, t.Container, s)
	heavy := convertVideo && limit > 0 && audio.BitRate != nil && *audio.BitRate > audioShare(limit)
	if copyable && !heavy {
		return nil, true
	}
	source := AudioSource{Codec: audio.Codec, ChannelLayout: audio.ChannelLayout}
	if audio.Channels != nil {
		source.Channels = *audio.Channels
	}
	if audio.SampleRate != nil {
		source.SampleRate = *audio.SampleRate
	}
	conversion := ConvertAudio(StreamAudioCodecs(t), t.MaxAudioChannels, source, limit, options.Can.Tuning)
	if conversion == nil && copyable {
		return nil, true
	}
	return conversion, conversion != nil
}

// SentAudioBitrate is the bitrate of the audio sent for played, the audio
// track that plays: its conversion's when it is converted to a lossy
// codec, else the track's own, copied or converted without loss. A track
// whose bitrate is not known counts as Jellyfin's StreamBuilder counts
// it, as lossy audio of its channels (see audioBitrateFor). Zero means no
// audio.
func SentAudioBitrate(played *MediaStream, conversion *AudioConversion) int64 {
	switch {
	case conversion != nil && conversion.Bitrate > 0:
		return conversion.Bitrate
	case played == nil:
		return 0
	case played.BitRate != nil && *played.BitRate > 0:
		return *played.BitRate
	}
	channels := 2
	if played.Channels != nil && *played.Channels > 0 {
		channels = *played.Channels
	}
	return audioBitrateFor(channels, 0)
}

// VideoLimit is what a bitrate limit leaves the video, as Jellyfin writes
// it in a TranscodingUrl: the limit less the bitrate of the audio sent
// (see SentAudioBitrate). Zero means no limit.
func VideoLimit(limit, audioBitrate int64) int64 {
	if limit <= 0 {
		return 0
	}
	return max(limit-audioBitrate, 1)
}

// remuxFails reports whether a codec profile of a type refuses stream in a
// remux into container.
func (d *decider) remuxFails(kind string, stream *MediaStream, container string, s subject) bool {
	for i := range d.profile.CodecProfiles {
		codec := &d.profile.CodecProfiles[i]
		if codec.Type == kind && listHas(codec.Codec, stream.Codec) && listMeets(codec.Container, container) &&
			s.holdAll(codec.ApplyConditions) && s.failures(codec.Conditions) != 0 {
			return true
		}
	}
	return false
}

// RemuxTag is the sample entry a remux into container writes for a video
// codec, empty when it writes none: in MP4, hvc1 for HEVC, which Apple
// players require, and the usual tags of the others.
func RemuxTag(codec, container string) string {
	if !strings.EqualFold(container, "mp4") {
		return ""
	}
	switch codec {
	case "hevc":
		return "hvc1"
	case "h264":
		return "avc1"
	case "av1":
		return "av01"
	}
	return ""
}

// singleContainer is the container PlaybackInfo reports for a source
// listing several: the first of them that matched supports, or when
// matched is nil any of the video profiles. A source no profile supports
// keeps its list.
func singleContainer(source string, matched *DirectPlayProfile, profiles []DirectPlayProfile) string {
	if !strings.Contains(source, ",") {
		return source
	}
	for name := range strings.SplitSeq(source, ",") {
		if matched != nil && listHas(matched.Container, name) {
			return name
		}
		for i := range profiles {
			if strings.EqualFold(profiles[i].Type, "Video") && listHas(profiles[i].Container, name) {
				return name
			}
		}
	}
	return source
}

// subtitleDelivery is how stream reaches the app, when direct playing
// container or when transcoding to container over protocol:
//   - embedded in the stream, in its own format, then in another text
//     format, unless the stream is a sidecar or the output cannot carry it;
//   - as a file or in the HLS playlist (HLS only when transcoding), in its
//     own format, then converted to another text format;
//   - otherwise burned into the video.
//
// A VobSub file is a pair, an .idx and a .sub file, which no app takes as
// one file: Jellyfin 12.2 gives VobSub as a file only extracted as
// Matroska (.mks), and burns such a pair in.
func subtitleDelivery(profiles []SubtitleProfile, stream *MediaStream, transcode bool, container, protocol string) SubtitleDelivery {
	language := stream.Language
	if language == "" {
		language = "und"
	}
	embeddable := !stream.IsExternal && (!transcode || (!strings.EqualFold(protocol, "hls") && embedsSubtitles(container)))
	for convert := range 2 {
		if !embeddable {
			break
		}
		for i := range profiles {
			p := &profiles[i]
			if p.Method != "Embed" || !listHas(p.Language, language) || !listMeets(p.Container, container) || stream.IsTextSubtitleStream != textSubtitle(p.Format) {
				continue
			}
			if strings.EqualFold(p.Format, stream.Codec) || (convert == 1 && stream.IsTextSubtitleStream && convertible(stream.Codec, p.Format)) {
				return SubtitleDelivery{Method: p.Method, Format: p.Format}
			}
		}
	}
	for convert := range 2 {
		for i := range profiles {
			p := &profiles[i]
			switch {
			case p.Method == "External" && stream.IsTextSubtitleStream == textSubtitle(p.Format) && !(stream.IsExternal && strings.EqualFold(stream.Codec, "DVDSUB")):
			case p.Method == "Hls" && transcode && stream.IsTextSubtitleStream:
			default:
				continue
			}
			if !listHas(p.Language, language) {
				continue
			}
			if strings.EqualFold(p.Format, stream.Codec) || (convert == 1 && stream.IsTextSubtitleStream && stream.SupportsExternalStream && convertible(stream.Codec, p.Format)) {
				return SubtitleDelivery{Method: p.Method, Format: p.Format}
			}
		}
	}
	return SubtitleDelivery{Method: "Encode", Format: stream.Codec}
}

// embedsSubtitles reports whether a transcode to container keeps subtitle
// tracks.
func embedsSubtitles(container string) bool {
	return listMeets("mkv,matroska", container)
}

// textSubtitle reports whether a subtitle format is text, not images.
func textSubtitle(format string) bool {
	for _, image := range [...]string{"pgs", "dvd", "dvbsub"} {
		if containsFold(format, image) {
			return false
		}
	}
	return !strings.EqualFold(format, "sub") && !strings.EqualFold(format, "sup") && !strings.EqualFold(format, "dvb_subtitle")
}

// convertible reports whether a text subtitle converts from one format to
// another; ASS and SSA styling is never converted.
func convertible(from, to string) bool {
	styled := func(format string) bool {
		return strings.EqualFold(format, "ass") || strings.EqualFold(format, "ssa")
	}
	return !styled(from) && !styled(to)
}

func containsFold(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// listHas reports whether a comma-separated list holds name, ignoring
// case. An empty list holds anything.
func listHas(list, name string) bool {
	if list == "" {
		return true
	}
	for item := range strings.SplitSeq(list, ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return true
		}
	}
	return false
}

// listIndex is the position of name in a comma-separated list, ignoring
// case; a name the list does not hold comes after all.
func listIndex(list, name string) int {
	i := 0
	for item := range strings.SplitSeq(list, ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return i
		}
		i++
	}
	return i
}

// listMeets reports whether a comma-separated list holds any of the
// comma-separated names. An empty list holds anything.
func listMeets(list, names string) bool {
	if list == "" {
		return true
	}
	for name := range strings.SplitSeq(names, ",") {
		if listHas(list, strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// subject is what profile conditions are evaluated against.
type subject struct {
	video, audio *MediaStream
	// secondary is set when audio is not the source's first audio track.
	secondary bool
	// codecTag is the video's codec tag when it is known: the source's,
	// or for a remux the one it writes.
	codecTag string
}

// holdAll reports whether every condition holds.
func (s subject) holdAll(conditions []ProfileCondition) bool {
	for _, condition := range conditions {
		if v, why := s.property(condition.Property); why != 0 && !condition.holds(v) {
			return false
		}
	}
	return true
}

// failures returns the reasons of the conditions that do not hold.
func (s subject) failures(conditions []ProfileCondition) reason {
	var failed reason
	for _, condition := range conditions {
		if v, why := s.property(condition.Property); why != 0 && !condition.holds(v) {
			failed |= why
		}
	}
	return failed
}

// noStream stands for a missing stream, whose properties are all unknown.
var noStream MediaStream

// valueKind is how a property compares.
type valueKind uint8

const (
	textValue valueKind = iota
	numberValue
	flagValue
)

// value is a property of the source; known is false when the source does
// not say.
type value struct {
	kind   valueKind
	known  bool
	text   string
	number float64
	flag   bool
}

func text(s string) value { return value{kind: textValue, known: s != "", text: s} }

func number[T int | int64 | float64](n *T) value {
	if n == nil {
		return value{kind: numberValue}
	}
	return value{kind: numberValue, known: true, number: float64(*n)}
}

func flag(b *bool) value {
	if b == nil {
		return value{kind: flagValue}
	}
	return value{kind: flagValue, known: true, flag: *b}
}

// property returns the value of a condition property and the reason its
// failure gives. Properties Polyfin does not evaluate give no reason and
// are ignored.
func (s subject) property(name string) (value, reason) {
	video, audio := s.video, s.audio
	if video == nil {
		video = &noStream
	}
	if audio == nil {
		audio = &noStream
	}
	switch name {
	case "VideoProfile":
		return text(video.Profile), videoProfileNotSupported
	case "VideoLevel":
		return number(video.Level), videoLevelNotSupported
	case "VideoRangeType":
		return text(video.VideoRangeType), videoRangeTypeNotSupported
	case "VideoCodecTag":
		if s.codecTag != "" {
			return text(s.codecTag), videoCodecTagNotSupported
		}
		return value{kind: textValue}, videoCodecTagNotSupported
	case "Width":
		return number(video.Width), videoResolutionNotSupported
	case "Height":
		return number(video.Height), videoResolutionNotSupported
	case "VideoBitDepth":
		return number(video.BitDepth), videoBitDepthNotSupported
	case "VideoBitrate":
		return number(video.BitRate), videoBitrateNotSupported
	case "VideoFramerate":
		rate := video.ReferenceFrameRate
		if rate == nil {
			rate = video.AverageFrameRate
		}
		if rate == nil {
			rate = video.RealFrameRate
		}
		return number(rate), videoFramerateNotSupported
	case "RefFrames":
		return number(video.RefFrames), refFramesNotSupported
	case "IsAnamorphic":
		return flag(video.IsAnamorphic), anamorphicVideoNotSupported
	case "IsInterlaced":
		if s.video == nil {
			return flag(nil), interlacedVideoNotSupported
		}
		return flag(&video.IsInterlaced), interlacedVideoNotSupported
	case "IsSecondaryAudio":
		if s.audio == nil {
			return flag(nil), secondaryAudioNotSupported
		}
		return flag(&s.secondary), secondaryAudioNotSupported
	case "AudioChannels":
		return number(audio.Channels), audioChannelsNotSupported
	case "AudioProfile":
		return text(audio.Profile), audioProfileNotSupported
	case "AudioSampleRate":
		return number(audio.SampleRate), audioSampleRateNotSupported
	case "AudioBitDepth":
		return number(audio.BitDepth), audioBitDepthNotSupported
	case "AudioBitrate":
		return number(audio.BitRate), audioBitrateNotSupported
	}
	return value{}, 0
}

// holds reports whether the condition holds for v. An unknown value fails
// a required condition and passes an optional one. Text compares ignoring
// case; NotEquals and EqualsAny take choices separated by "|".
func (c ProfileCondition) holds(v value) bool {
	if !v.known {
		return !c.IsRequired
	}
	switch v.kind {
	case textValue:
		switch c.Condition {
		case "Equals":
			return strings.EqualFold(c.Value, v.text)
		case "NotEquals":
			return !choicesHave(c.Value, v.text)
		case "EqualsAny":
			return choicesHave(c.Value, v.text)
		}
	case numberValue:
		if c.Condition == "EqualsAny" {
			for choice := range strings.SplitSeq(c.Value, "|") {
				if n, err := strconv.ParseFloat(strings.TrimSpace(choice), 64); err == nil && n == v.number {
					return true
				}
			}
			return false
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(c.Value), 64)
		if err != nil {
			return false
		}
		switch c.Condition {
		case "Equals":
			return v.number == n
		case "NotEquals":
			return v.number != n
		case "LessThanEqual":
			return v.number <= n
		case "GreaterThanEqual":
			return v.number >= n
		}
	case flagValue:
		b, err := strconv.ParseBool(strings.TrimSpace(c.Value))
		if err != nil {
			return false
		}
		switch c.Condition {
		case "Equals":
			return v.flag == b
		case "NotEquals":
			return v.flag != b
		}
	}
	return false
}

// choicesHave reports whether choices separated by "|" hold s, ignoring
// case.
func choicesHave(choices, s string) bool {
	for choice := range strings.SplitSeq(choices, "|") {
		if strings.EqualFold(strings.TrimSpace(choice), s) {
			return true
		}
	}
	return false
}
