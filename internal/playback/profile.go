package playback

import (
	"encoding/json"
	"slices"
	"strings"
)

// DeviceProfile is what a Jellyfin app says it can play, sent with its
// PlaybackInfo requests. Field names follow Jellyfin's API.
type DeviceProfile struct {
	Name                string
	MaxStreamingBitrate *int64 `json:",omitempty"`
	MaxStaticBitrate    *int64 `json:",omitempty"`
	// MusicStreamingTranscodingBitrate is the bitrate tracks are converted
	// to when streamed.
	MusicStreamingTranscodingBitrate *int64 `json:",omitempty"`
	DirectPlayProfiles               []DirectPlayProfile
	TranscodingProfiles              []TranscodingProfile
	ContainerProfiles                []ContainerProfile
	CodecProfiles                    []CodecProfile
	SubtitleProfiles                 []SubtitleProfile
}

// DirectPlayProfile is a combination of container and codecs the app plays
// as is. Lists are comma-separated; an empty list allows anything.
type DirectPlayProfile struct {
	Container  string
	AudioCodec string
	VideoCodec string
	// Type is "Video", "Audio" or "Photo".
	Type string
}

// TranscodingProfile is a format the app accepts when the server converts
// a source.
type TranscodingProfile struct {
	Container  string
	Type       string
	VideoCodec string
	AudioCodec string
	// Protocol is "http" for a progressive stream or "hls".
	Protocol         string
	Context          string
	MaxAudioChannels string `json:",omitempty"`
	// MinSegments is a number, which jellyfin-web sends as a string.
	MinSegments         json.Number        `json:",omitempty"`
	BreakOnNonKeyFrames bool               `json:",omitempty"`
	Conditions          []ProfileCondition `json:",omitempty"`
}

// ContainerProfile restricts a container.
type ContainerProfile struct {
	Type       string
	Container  string
	Conditions []ProfileCondition
}

// CodecProfile restricts a codec: Conditions must hold for the app to play
// it, once ApplyConditions select the profile.
type CodecProfile struct {
	// Type is "Video", "VideoAudio" (audio in a video) or "Audio".
	Type            string
	Codec           string
	Container       string
	SubContainer    string `json:",omitempty"`
	Conditions      []ProfileCondition
	ApplyConditions []ProfileCondition
}

// ProfileCondition compares a property of the source with a value.
type ProfileCondition struct {
	// Condition is "Equals", "NotEquals", "LessThanEqual",
	// "GreaterThanEqual" or "EqualsAny" (Value lists choices separated by
	// "|").
	Condition string
	Property  string
	Value     string
	// IsRequired makes the condition fail when the property is unknown.
	IsRequired bool
}

// SubtitleProfile is a subtitle format the app shows, and how it receives
// it: "Embed" in the stream, "External" as a file, "Hls" in a playlist, or
// "Encode" burned into the video.
type SubtitleProfile struct {
	Format    string
	Method    string
	Language  string `json:",omitempty"`
	Container string `json:",omitempty"`
}

// riskyVideoCodecs are the video codecs browsers say they play and then
// often show black, the sound and the clock running: HEVC (and Dolby
// Vision, carried in HEVC or AV1) without a hardware decoder, AV1 on older
// machines.
var riskyVideoCodecs = []string{"hevc", "h265", "av1", "dvhe", "dvh1", "dav1"}

// WithoutRiskyVideo returns a copy of profile in which the app takes none of
// riskyVideoCodecs, as it is or converted: a direct-play profile left with
// no video codec is dropped (an empty list would allow every codec), and a
// video conversion goes to the codecs left, H.264 when none is.
func WithoutRiskyVideo(profile *DeviceProfile) *DeviceProfile {
	safe := *profile
	strip := func(list string) (string, bool) {
		var kept []string
		removed := false
		for _, codec := range strings.Split(list, ",") {
			codec = strings.TrimSpace(codec)
			if codec == "" {
				continue
			}
			if slices.ContainsFunc(riskyVideoCodecs, func(r string) bool { return strings.EqualFold(r, codec) }) {
				removed = true
				continue
			}
			kept = append(kept, codec)
		}
		return strings.Join(kept, ","), removed
	}
	safe.DirectPlayProfiles = nil
	for _, p := range profile.DirectPlayProfiles {
		if !strings.EqualFold(p.Type, "Video") {
			safe.DirectPlayProfiles = append(safe.DirectPlayProfiles, p)
			continue
		}
		if p.VideoCodec == "" {
			// Any codec: H.264 and VP8/VP9 alone.
			p.VideoCodec = "h264,vp8,vp9"
		} else if codecs, removed := strip(p.VideoCodec); removed {
			if codecs == "" {
				continue
			}
			p.VideoCodec = codecs
		}
		safe.DirectPlayProfiles = append(safe.DirectPlayProfiles, p)
	}
	safe.TranscodingProfiles = slices.Clone(profile.TranscodingProfiles)
	for i := range safe.TranscodingProfiles {
		p := &safe.TranscodingProfiles[i]
		if !strings.EqualFold(p.Type, "Video") {
			continue
		}
		if codecs, removed := strip(p.VideoCodec); removed || p.VideoCodec == "" {
			if codecs == "" {
				codecs = "h264"
			}
			p.VideoCodec = codecs
		}
	}
	return &safe
}

// burnedFormats are the subtitle formats a profile that burns every
// subtitle names.
var burnedFormats = []string{"srt", "subrip", "ass", "ssa", "vtt", "webvtt", "mov_text", "ttml", "sub", "smi", "pgs", "pgssub", "dvdsub", "dvbsub", "vobsub"}

// BurningSubtitles returns a copy of profile whose subtitles are all
// burned into the picture (Encode), none sent beside the video.
func BurningSubtitles(profile *DeviceProfile) *DeviceProfile {
	if profile == nil {
		return nil
	}
	out := *profile
	out.SubtitleProfiles = make([]SubtitleProfile, 0, len(burnedFormats))
	for _, format := range burnedFormats {
		out.SubtitleProfiles = append(out.SubtitleProfiles, SubtitleProfile{Format: format, Method: "Encode"})
	}
	return &out
}
