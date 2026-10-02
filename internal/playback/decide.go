package playback

import (
	"math/bits"
	"strconv"
	"strings"
)

// Options are what a PlaybackInfo request chose.
type Options struct {
	// MaxStreamingBitrate caps the source bitrate for direct play. 0 falls
	// back to the DeviceProfile's MaxStreamingBitrate, and to no limit when
	// the profile has none.
	MaxStreamingBitrate int64
	// AudioStreamIndex is the audio track asked for; nil plays the source's
	// default.
	AudioStreamIndex *int
	// SubtitleStreamIndex is the subtitle track asked for; nil or -1 shows
	// none.
	SubtitleStreamIndex *int
	// EnableDirectPlay false refuses direct play, with DirectPlayError when
	// nothing else would refuse it.
	EnableDirectPlay bool
	// EnableDirectStream changes nothing on its own: Jellyfin 12.1 answers
	// the same whether it is set or not.
	EnableDirectStream bool
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
	// Remux is set when direct play is refused but Transcoding can carry
	// the video and the audio that plays as they are, over HLS.
	Remux bool
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
// Jellyfin 12.1's answers confirm the relative order of the first four,
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

// Decide answers like Jellyfin 12.1 whether the app behind profile can
// direct play source.
//
// The audio track checked against the profile is the one asked for, else
// the one flagged default. A source whose audio tracks are all unflagged
// direct plays whatever their codecs, and gets AudioCodecNotSupported once
// refused for another reason, as Jellyfin does. Codec, container and
// subtitle requirements apply to the whole source; of the direct-play
// profiles, the one with the fewest failures, first in order, gives the
// container and codec reasons.
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
	if limit > 0 && source.Bitrate > limit {
		reasons |= containerBitrateExceedsLimit
	}
	var matched *DirectPlayProfile
	if options.EnableDirectPlay && reasons == 0 {
		var why reason
		matched, why = d.directPlay()
		reasons |= why
	}
	decision.DirectPlay = matched != nil
	if decision.DirectPlay {
		decision.Container = singleContainer(source.Container, matched, nil)
	} else {
		decision.Container = singleContainer(source.Container, nil, profile.DirectPlayProfiles)
		if t := decision.Transcoding; t != nil {
			decision.Remux = reasons&containerBitrateExceedsLimit == 0 && d.remuxable(t)
			reasons |= d.transcodeReasons(t)
			if reasons == 0 {
				reasons = directPlayError
			}
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

// subject is what conditions see of the source.
func (d *decider) subject() subject {
	s := subject{video: d.video, audio: d.checked}
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
// it does not take, and audio that is not taken or not flagged.
func (d *decider) transcodeReasons(t *TranscodingProfile) reason {
	var why reason
	if d.video != nil && !listHas(t.VideoCodec, d.video.Codec) {
		why |= videoCodecNotSupported
	}
	if d.hasAudio && (d.checked == nil || !listHas(t.AudioCodec, d.checked.Codec)) {
		why |= audioCodecNotSupported
	}
	return why
}

// remuxable reports whether transcoding with t can copy the video and the
// audio that plays: t is HLS, takes their codecs and the audio's channels,
// and the codec profiles of its container accept the remux. The remux is
// what they judge: it carries only the audio that plays, so never a
// secondary track, and the codec tag it writes.
func (d *decider) remuxable(t *TranscodingProfile) bool {
	if !strings.EqualFold(t.Protocol, "hls") || d.video == nil || !listHas(t.VideoCodec, d.video.Codec) {
		return false
	}
	audio := d.played
	if audio != nil {
		if !listHas(t.AudioCodec, audio.Codec) {
			return false
		}
		if limit, err := strconv.Atoi(t.MaxAudioChannels); err == nil && audio.Channels != nil && *audio.Channels > limit {
			return false
		}
	}
	s := subject{video: d.video, audio: audio, codecTag: RemuxTag(d.video.Codec, t.Container)}
	for i := range d.profile.CodecProfiles {
		codec := &d.profile.CodecProfiles[i]
		var stream *MediaStream
		switch codec.Type {
		case "Video":
			stream = d.video
		case "VideoAudio":
			stream = audio
		}
		if stream != nil && listHas(codec.Codec, stream.Codec) && listMeets(codec.Container, t.Container) &&
			s.holdAll(codec.ApplyConditions) && s.failures(codec.Conditions) != 0 {
			return false
		}
	}
	return true
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
			case p.Method == "External" && stream.IsTextSubtitleStream == textSubtitle(p.Format):
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
	// codecTag is the video's codec tag when it is known: only a remux's,
	// which writes it.
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
		// Jellyfin 12.1 never knows a source's codec tag: it reports none,
		// even for MP4.
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
