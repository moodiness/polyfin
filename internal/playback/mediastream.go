package playback

// MediaStream is a track of a version as Jellyfin describes it, in its
// MediaSources and in decisions. Field names and order follow Jellyfin's
// API; fields Jellyfin leaves out when unknown are pointers or omitted when
// empty.
type MediaStream struct {
	Codec string `json:",omitempty"`
	// CodecTag is ffprobe's tag of the track, such as avc1 in MP4. Tags
	// made of zero bytes, Matroska's, are not reported.
	CodecTag                  string `json:",omitempty"`
	Language                  string `json:",omitempty"`
	ColorRange                string `json:",omitempty"`
	ColorSpace                string `json:",omitempty"`
	ColorTransfer             string `json:",omitempty"`
	ColorPrimaries            string `json:",omitempty"`
	DvVersionMajor            *int   `json:",omitempty"`
	DvVersionMinor            *int   `json:",omitempty"`
	DvProfile                 *int   `json:",omitempty"`
	DvLevel                   *int   `json:",omitempty"`
	RpuPresentFlag            *int   `json:",omitempty"`
	ElPresentFlag             *int   `json:",omitempty"`
	BlPresentFlag             *int   `json:",omitempty"`
	DvBlSignalCompatibilityId *int   `json:",omitempty"`
	Hdr10PlusPresentFlag      *bool  `json:",omitempty"`
	TimeBase                  string `json:",omitempty"`
	Title                     string `json:",omitempty"`
	VideoRange                string // "SDR", "HDR" or "Unknown"
	VideoRangeType            string // "SDR", "HDR10", "HLG", "DOVI", "DOVIWithHDR10", … or "Unknown"
	VideoDoViTitle            string `json:",omitempty"`
	AudioSpatialFormat        string // "None", "DolbyAtmos" or "DTSX"
	LocalizedUndefined        string `json:",omitempty"`
	LocalizedDefault          string `json:",omitempty"`
	LocalizedForced           string `json:",omitempty"`
	LocalizedExternal         string `json:",omitempty"`
	LocalizedHearingImpaired  string `json:",omitempty"`
	LocalizedLanguage         string `json:",omitempty"`
	LocalizedOriginal         string `json:",omitempty"`
	DisplayTitle              string `json:",omitempty"`
	NalLengthSize             string `json:",omitempty"`
	IsInterlaced              bool
	IsAVC                     *bool  `json:",omitempty"`
	ChannelLayout             string `json:",omitempty"`
	BitRate                   *int64 `json:",omitempty"`
	BitDepth                  *int   `json:",omitempty"`
	RefFrames                 *int   `json:",omitempty"`
	Channels                  *int   `json:",omitempty"`
	SampleRate                *int   `json:",omitempty"`
	IsDefault                 bool
	IsForced                  bool
	IsHearingImpaired         bool
	IsOriginal                bool
	Height                    *int     `json:",omitempty"`
	Width                     *int     `json:",omitempty"`
	AverageFrameRate          *float64 `json:",omitempty"`
	RealFrameRate             *float64 `json:",omitempty"`
	ReferenceFrameRate        *float64 `json:",omitempty"`
	Profile                   string   `json:",omitempty"`
	// Type is "Video", "Audio", "Subtitle", "EmbeddedImage" or "Data".
	Type        string
	AspectRatio string `json:",omitempty"`
	Index       int
	Score       *int `json:",omitempty"`
	IsExternal  bool
	// DeliveryMethod is how a subtitle reaches the player: "Embed",
	// "External", "Hls", "Encode" or "Drop". It is decided per request.
	DeliveryMethod         string `json:",omitempty"`
	DeliveryUrl            string `json:",omitempty"`
	IsExternalUrl          *bool  `json:",omitempty"`
	IsTextSubtitleStream   bool
	SupportsExternalStream bool
	Path                   string   `json:",omitempty"`
	PixelFormat            string   `json:",omitempty"`
	Level                  *float64 `json:",omitempty"`
	IsAnamorphic           *bool    `json:",omitempty"`
}
