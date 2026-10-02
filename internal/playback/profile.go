package playback

import "encoding/json"

// DeviceProfile is what a Jellyfin app says it can play, sent with its
// PlaybackInfo requests. Field names follow Jellyfin's API.
type DeviceProfile struct {
	Name                string
	MaxStreamingBitrate *int64 `json:",omitempty"`
	MaxStaticBitrate    *int64 `json:",omitempty"`
	DirectPlayProfiles  []DirectPlayProfile
	TranscodingProfiles []TranscodingProfile
	ContainerProfiles   []ContainerProfile
	CodecProfiles       []CodecProfile
	SubtitleProfiles    []SubtitleProfile
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
