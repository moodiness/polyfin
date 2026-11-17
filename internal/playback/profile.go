package playback

import "encoding/json"

// DeviceProfile is what a Jellyfin app says it can play, sent with its
// PlaybackInfo requests. Field names follow Jellyfin's API, whose request
// bodies are read leniently: a string field also takes a number, and a
// number field a string holding one, as apps differ (see the jellyfin
// package's bodyOptions).
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

// The bounds of a device profile's lists, which a profile kept for its
// device stays within: MaxProfileEntries for its lists of profiles,
// MaxProfileConditions for each list of conditions. They stand well
// above what apps send: Jellyfin for Android's integrated player lists 19
// direct play, 19 container and 31 codec profiles, jellyfin-web 11
// transcoding profiles, Swiftfin 38 subtitle profiles, and no app more
// than 7 conditions in a list.
const (
	MaxProfileEntries    = 256
	MaxProfileConditions = 64
)

// Bounded reports whether every list of p is within its bound (see
// MaxProfileEntries).
func (p *DeviceProfile) Bounded() bool {
	if len(p.DirectPlayProfiles) > MaxProfileEntries || len(p.TranscodingProfiles) > MaxProfileEntries ||
		len(p.ContainerProfiles) > MaxProfileEntries || len(p.CodecProfiles) > MaxProfileEntries ||
		len(p.SubtitleProfiles) > MaxProfileEntries {
		return false
	}
	for _, profile := range p.TranscodingProfiles {
		if len(profile.Conditions) > MaxProfileConditions {
			return false
		}
	}
	for _, profile := range p.ContainerProfiles {
		if len(profile.Conditions) > MaxProfileConditions {
			return false
		}
	}
	for _, profile := range p.CodecProfiles {
		if len(profile.Conditions) > MaxProfileConditions || len(profile.ApplyConditions) > MaxProfileConditions {
			return false
		}
	}
	return true
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
	Protocol string
	Context  string
	// MaxAudioChannels is a string, as in Jellyfin: jellyfin-web sends
	// "2", an app that sends numbers 2.
	MaxAudioChannels string `json:",omitempty"`
	// MinSegments is a number, which jellyfin-web sends as a string.
	MinSegments         json.Number        `json:",omitempty"`
	BreakOnNonKeyFrames bool               `json:",omitempty"`
	Conditions          []ProfileCondition `json:",omitempty"`
	// OneFile marks a profile whose conversion Polyfin sends to the
	// device as one MPEG-TS stream rather than as HLS segments, as it
	// does for DLNA devices: what HLS segments carry does not restrict
	// its audio (see StreamAudioCodecs).
	OneFile bool `json:"-"`
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
