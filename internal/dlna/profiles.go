package dlna

import (
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/playback"
)

// Profile is a device profile in the XML format of Jellyfin's DLNA plugin,
// whose profiles can be added as they are: what a family of devices plays
// as it is, what Polyfin converts for it, and how they are recognized.
// Elements Polyfin does not use are read and ignored.
type Profile struct {
	XMLName                          xml.Name             `xml:"Profile"`
	Name                             string               `xml:"Name"`
	Identification                   *Identification      `xml:"Identification"`
	MaxStreamingBitrate              int64                `xml:"MaxStreamingBitrate"`
	MaxStaticBitrate                 int64                `xml:"MaxStaticBitrate"`
	MusicStreamingTranscodingBitrate int64                `xml:"MusicStreamingTranscodingBitrate"`
	DirectPlayProfiles               []DirectPlayProfile  `xml:"DirectPlayProfiles>DirectPlayProfile"`
	TranscodingProfiles              []TranscodingProfile `xml:"TranscodingProfiles>TranscodingProfile"`
	ContainerProfiles                []ContainerProfile   `xml:"ContainerProfiles>ContainerProfile"`
	CodecProfiles                    []CodecProfile       `xml:"CodecProfiles>CodecProfile"`
	ResponseProfiles                 []ResponseProfile    `xml:"ResponseProfiles>ResponseProfile"`
	SubtitleProfiles                 []SubtitleProfile    `xml:"SubtitleProfiles>SubtitleProfile"`
}

// Identification tells the devices a profile is for: by what their
// description announces, each field set a substring or a regular
// expression that must match, or by the headers they send, any of which
// matching is enough.
type Identification struct {
	FriendlyName string           `xml:"FriendlyName"`
	ModelNumber  string           `xml:"ModelNumber"`
	ModelName    string           `xml:"ModelName"`
	Manufacturer string           `xml:"Manufacturer"`
	Headers      []HTTPHeaderInfo `xml:"Headers>HttpHeaderInfo"`
}

// HTTPHeaderInfo is a header a device sends: Match is Equals, Substring or
// Regex, as in Jellyfin's profiles.
type HTTPHeaderInfo struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
	Match string `xml:"match,attr"`
}

type DirectPlayProfile struct {
	Container  string `xml:"container,attr"`
	AudioCodec string `xml:"audioCodec,attr"`
	VideoCodec string `xml:"videoCodec,attr"`
	Type       string `xml:"type,attr"`
}

type TranscodingProfile struct {
	Container        string             `xml:"container,attr"`
	Type             string             `xml:"type,attr"`
	VideoCodec       string             `xml:"videoCodec,attr"`
	AudioCodec       string             `xml:"audioCodec,attr"`
	MaxAudioChannels string             `xml:"maxAudioChannels,attr"`
	Context          string             `xml:"context,attr"`
	Conditions       []ProfileCondition `xml:"Conditions>ProfileCondition"`
}

type ContainerProfile struct {
	Type       string             `xml:"type,attr"`
	Container  string             `xml:"container,attr"`
	Conditions []ProfileCondition `xml:"Conditions>ProfileCondition"`
}

type CodecProfile struct {
	Type            string             `xml:"type,attr"`
	Codec           string             `xml:"codec,attr"`
	Container       string             `xml:"container,attr"`
	Conditions      []ProfileCondition `xml:"Conditions>ProfileCondition"`
	ApplyConditions []ProfileCondition `xml:"ApplyConditions>ProfileCondition"`
}

type ProfileCondition struct {
	Condition  string `xml:"condition,attr"`
	Property   string `xml:"property,attr"`
	Value      string `xml:"value,attr"`
	IsRequired string `xml:"isRequired,attr"`
}

// ResponseProfile gives the MIME type and DLNA profile name sent for a
// container.
type ResponseProfile struct {
	Container  string `xml:"container,attr"`
	AudioCodec string `xml:"audioCodec,attr"`
	VideoCodec string `xml:"videoCodec,attr"`
	Type       string `xml:"type,attr"`
	OrgPn      string `xml:"orgPn,attr"`
	MimeType   string `xml:"mimeType,attr"`
}

// SubtitleProfile is how a device takes subtitles of a format: External,
// through CaptionInfo.sec, or Encode, burned in.
type SubtitleProfile struct {
	Format   string `xml:"format,attr"`
	Method   string `xml:"method,attr"`
	DidlMode string `xml:"didlMode,attr"`
	Language string `xml:"language,attr"`
}

// MaxProfileBytes bounds the XML of a profile.
const MaxProfileBytes = 256 << 10

// ErrInvalidProfile reports XML that is not a usable profile.
var ErrInvalidProfile = errors.New("invalid DLNA profile")

// ParseProfile reads a profile's XML: it must be a Profile with a name, at
// most 128 characters, and say how video plays, as it is or converted.
func ParseProfile(data []byte) (Profile, error) {
	if len(data) > MaxProfileBytes || !utf8.Valid(data) {
		return Profile{}, ErrInvalidProfile
	}
	var p Profile
	if err := xml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("%w: %v", ErrInvalidProfile, err)
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 128 {
		return Profile{}, fmt.Errorf("%w: it needs a Name of 1 to 128 characters", ErrInvalidProfile)
	}
	if len(p.DirectPlayProfiles) == 0 && len(p.TranscodingProfiles) == 0 {
		return Profile{}, fmt.Errorf("%w: it has neither DirectPlayProfiles nor TranscodingProfiles", ErrInvalidProfile)
	}
	if id := p.Identification; id != nil {
		for _, pattern := range []string{id.FriendlyName, id.ModelNumber, id.ModelName, id.Manufacturer} {
			if _, err := regexp.Compile("(?i)" + pattern); err != nil {
				return Profile{}, fmt.Errorf("%w: %v", ErrInvalidProfile, err)
			}
		}
		for _, header := range id.Headers {
			if strings.EqualFold(header.Match, "Regex") {
				if _, err := regexp.Compile("(?i)" + header.Value); err != nil {
					return Profile{}, fmt.Errorf("%w: %v", ErrInvalidProfile, err)
				}
			}
		}
	}
	return p, nil
}

//go:embed profiles/*.xml
var builtinFiles embed.FS

// GenericProfile is the name of the profile of the devices no other
// profile matches.
const GenericProfile = "Generic"

// builtins are the built-in profiles, the generic one first.
var builtins = func() []Profile {
	files, err := fs.Glob(builtinFiles, "profiles/*.xml")
	if err != nil {
		panic(err)
	}
	var profiles []Profile
	for _, file := range files {
		data, err := builtinFiles.ReadFile(file)
		if err != nil {
			panic(err)
		}
		p, err := ParseProfile(data)
		if err != nil {
			panic(file + ": " + err.Error())
		}
		profiles = append(profiles, p)
	}
	slices.SortStableFunc(profiles, func(a, b Profile) int {
		switch {
		case a.Name == GenericProfile:
			return -1
		case b.Name == GenericProfile:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return profiles
}()

// BuiltinXML returns the XML of the built-in profile named name.
func BuiltinXML(name string) ([]byte, bool) {
	files, _ := fs.Glob(builtinFiles, "profiles/*.xml")
	for _, file := range files {
		data, _ := builtinFiles.ReadFile(file)
		if p, err := ParseProfile(data); err == nil && p.Name == name {
			return data, true
		}
	}
	return nil, false
}

// Builtins lists the built-in profiles, the generic one first.
func Builtins() []Profile { return slices.Clone(builtins) }

// Seen is what a device told of itself: its description's names, and the
// headers of its request.
type Seen struct {
	Name, Manufacturer, Model string
	Headers                   http.Header
}

// matches reports whether p is for the device seen: by its description,
// every field the identification sets matching, or by any of the headers
// it names. A profile without identification matches nothing.
func (p Profile) matches(seen Seen) bool {
	id := p.Identification
	if id == nil {
		return false
	}
	for _, header := range id.Headers {
		for _, value := range seen.Headers.Values(header.Name) {
			if headerMatches(value, header) {
				return true
			}
		}
	}
	fields := []struct{ pattern, value string }{{id.FriendlyName, seen.Name}, {id.ModelName, seen.Model}, {id.Manufacturer, seen.Manufacturer}}
	set := false
	for _, f := range fields {
		if f.pattern == "" {
			continue
		}
		set = true
		if f.value == "" || !substringOrRegex(f.value, f.pattern) {
			return false
		}
	}
	return set
}

// headerMatches reports whether a header's value matches info.
func headerMatches(value string, info HTTPHeaderInfo) bool {
	switch {
	case strings.EqualFold(info.Match, "Equals"):
		return strings.EqualFold(value, info.Value)
	case strings.EqualFold(info.Match, "Regex"):
		re, err := regexp.Compile("(?i)" + info.Value)
		return err == nil && re.MatchString(value)
	}
	return info.Value != "" && strings.Contains(strings.ToLower(value), strings.ToLower(info.Value))
}

// substringOrRegex matches as Jellyfin's DLNA plugin does: a substring in
// any letter case, or a regular expression.
func substringOrRegex(value, pattern string) bool {
	if strings.Contains(strings.ToLower(value), strings.ToLower(pattern)) {
		return true
	}
	re, err := regexp.Compile("(?i)" + pattern)
	return err == nil && re.MatchString(value)
}

// Choose returns the profile for a device: the first of profiles, added
// ones first, that matches what it told, else the generic one.
func Choose(profiles []Profile, seen Seen) Profile {
	for _, p := range profiles {
		if p.matches(seen) {
			return p
		}
	}
	return builtins[0]
}

// Playback is the profile as playback decides with it. Polyfin converts
// for DLNA to MPEG-TS only, and audio to MP3: those transcoding profiles
// are kept, marked as the ones Polyfin streams (see playback.Decide), as
// one file, which carries the audio codecs the device takes in MPEG-TS.
func (p Profile) Playback() *playback.DeviceProfile {
	profile := &playback.DeviceProfile{Name: p.Name}
	if p.MaxStreamingBitrate > 0 {
		profile.MaxStreamingBitrate = new(p.MaxStreamingBitrate)
	}
	if p.MaxStaticBitrate > 0 {
		profile.MaxStaticBitrate = new(p.MaxStaticBitrate)
	}
	for _, d := range p.DirectPlayProfiles {
		profile.DirectPlayProfiles = append(profile.DirectPlayProfiles, playback.DirectPlayProfile{
			Container: d.Container, AudioCodec: d.AudioCodec, VideoCodec: d.VideoCodec, Type: d.Type})
	}
	for _, t := range p.TranscodingProfiles {
		if !strings.EqualFold(t.Type, "Video") || !mpegTS(t.Container) {
			continue
		}
		profile.TranscodingProfiles = append(profile.TranscodingProfiles, playback.TranscodingProfile{
			Container: "ts", Type: "Video", VideoCodec: t.VideoCodec, AudioCodec: t.AudioCodec, Protocol: "hls", Context: "Streaming",
			MaxAudioChannels: t.MaxAudioChannels, Conditions: conditions(t.Conditions), OneFile: true})
	}
	for _, c := range p.ContainerProfiles {
		profile.ContainerProfiles = append(profile.ContainerProfiles, playback.ContainerProfile{
			Type: c.Type, Container: c.Container, Conditions: conditions(c.Conditions)})
	}
	for _, c := range p.CodecProfiles {
		profile.CodecProfiles = append(profile.CodecProfiles, playback.CodecProfile{Type: c.Type, Codec: c.Codec, Container: c.Container,
			Conditions: conditions(c.Conditions), ApplyConditions: conditions(c.ApplyConditions)})
	}
	for _, s := range p.SubtitleProfiles {
		if strings.EqualFold(s.Method, "External") || strings.EqualFold(s.Method, "Encode") {
			profile.SubtitleProfiles = append(profile.SubtitleProfiles, playback.SubtitleProfile{Format: s.Format, Method: s.Method,
				Language: s.Language})
		}
	}
	return profile
}

// mpegTS reports whether a container is MPEG-TS, by one of its names.
func mpegTS(container string) bool {
	for name := range strings.SplitSeq(strings.ToLower(container), ",") {
		switch strings.TrimSpace(name) {
		case "ts", "mpegts", "m2ts":
			return true
		}
	}
	return false
}

func conditions(list []ProfileCondition) []playback.ProfileCondition {
	var result []playback.ProfileCondition
	for _, c := range list {
		result = append(result, playback.ProfileCondition{Condition: c.Condition, Property: c.Property, Value: c.Value,
			IsRequired: !strings.EqualFold(c.IsRequired, "false")})
	}
	return result
}

// TakesCaptions reports whether the profile takes text subtitles as files
// beside the video, through CaptionInfo.sec.
func (p Profile) TakesCaptions() bool {
	return slices.ContainsFunc(p.SubtitleProfiles, func(s SubtitleProfile) bool {
		return strings.EqualFold(s.Method, "External") && strings.EqualFold(s.Format, "srt")
	})
}

// TakesAudio reports whether the profile plays a song as it is.
func (p Profile) TakesAudio(container, codec string) bool {
	return slices.ContainsFunc(p.DirectPlayProfiles, func(d DirectPlayProfile) bool {
		return strings.EqualFold(d.Type, "Audio") && listHas(d.Container, container) && (d.AudioCodec == "" || listHas(d.AudioCodec, codec))
	})
}

// MimeType is the MIME type and DLNA profile name sent for a container,
// as the profile's response profiles give them, else fallback.
func (p Profile) MimeType(kind, container, fallback string) (mime, orgPn string) {
	for _, r := range p.ResponseProfiles {
		if strings.EqualFold(r.Type, kind) && listHas(r.Container, container) && r.MimeType != "" {
			return r.MimeType, r.OrgPn
		}
	}
	return fallback, ""
}

// listHas reports whether a comma-separated list holds name, in any
// letter case; an empty list holds every name.
func listHas(list, name string) bool {
	if strings.TrimSpace(list) == "" {
		return true
	}
	for want := range strings.SplitSeq(strings.ToLower(name), ",") {
		for item := range strings.SplitSeq(strings.ToLower(list), ",") {
			if strings.TrimSpace(item) == strings.TrimSpace(want) {
				return true
			}
		}
	}
	return false
}
