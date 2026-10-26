package accounts

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/torrent"
)

// ErrInvalidServerName reports an empty, too long or unprintable server name.
var ErrInvalidServerName = errors.New("invalid server name")

// ErrInvalidLanguage reports a server language Polyfin does not speak.
var ErrInvalidLanguage = errors.New("invalid server language")

// ErrInvalidCatalogLimit reports a CatalogLimit outside [MinCatalogLimit,
// MaxCatalogLimit].
var ErrInvalidCatalogLimit = errors.New("invalid catalog limit")

// ErrInvalidChannelLimit reports a ChannelLimit outside [MinChannelLimit,
// MaxChannelLimit].
var ErrInvalidChannelLimit = errors.New("invalid channel limit")

// The bounds and defaults of Settings.CatalogLimit and ChannelLimit.
const (
	MinCatalogLimit     = 100
	MaxCatalogLimit     = 20000
	DefaultCatalogLimit = 2000
	MinChannelLimit     = 100
	MaxChannelLimit     = 50000
	DefaultChannelLimit = 10000
)

// ErrInvalidPlayedPercent reports a PlayedPercent outside [MinPlayedPercent,
// MaxPlayedPercent].
var ErrInvalidPlayedPercent = errors.New("invalid played percent")

// ErrInvalidResumePercent reports a ResumePercent outside [MinResumePercent,
// MaxResumePercent].
var ErrInvalidResumePercent = errors.New("invalid resume percent")

// ErrResumeNotBelowPlayed reports a ResumePercent that is not below
// PlayedPercent: no position would then keep a resume point.
var ErrResumeNotBelowPlayed = errors.New("resume percent not below played percent")

// ErrInvalidVersionListMinutes reports a VersionListMinutes outside
// [MinVersionListMinutes, MaxVersionListMinutes].
var ErrInvalidVersionListMinutes = errors.New("invalid version list minutes")

// ErrInvalidCatalogRefreshMinutes reports a CatalogRefreshMinutes outside
// [MinCatalogRefreshMinutes, MaxCatalogRefreshMinutes].
var ErrInvalidCatalogRefreshMinutes = errors.New("invalid catalog refresh minutes")

// The bounds and defaults of Settings.PlayedPercent, ResumePercent,
// VersionListMinutes and CatalogRefreshMinutes. The percents default to
// those of a Jellyfin server (MaxResumePct and MinResumePct); catalogs are
// read again every hour by default.
const (
	MinPlayedPercent             = 50
	MaxPlayedPercent             = 100
	DefaultPlayedPercent         = 90
	MinResumePercent             = 0
	MaxResumePercent             = 50
	DefaultResumePercent         = 5
	MinVersionListMinutes        = 1
	MaxVersionListMinutes        = 360
	DefaultVersionListMinutes    = 10
	MinCatalogRefreshMinutes     = 1
	MaxCatalogRefreshMinutes     = 1440
	DefaultCatalogRefreshMinutes = 60
)

// ErrInvalidLoginAttempts reports a LoginAttempts other than 0 outside
// [MinLoginAttempts, MaxLoginAttempts].
var ErrInvalidLoginAttempts = errors.New("invalid login attempts")

// ErrInvalidInactiveDeviceDays reports an InactiveDeviceDays outside [0,
// MaxInactiveDeviceDays].
var ErrInvalidInactiveDeviceDays = errors.New("invalid inactive device days")

// The bounds and defaults of Settings.LoginAttempts and InactiveDeviceDays;
// 0, their default, turns them off.
const (
	MinLoginAttempts          = 3
	MaxLoginAttempts          = 20
	DefaultLoginAttempts      = 0
	MaxInactiveDeviceDays     = 365
	DefaultInactiveDeviceDays = 0
)

// ErrInvalidKnownProxies reports a Known proxies entry that is not an IP
// address, a network nor a host name, or more than MaxKnownProxies entries
// between Settings.KnownProxies and Settings.KnownProxyNames.
var ErrInvalidKnownProxies = errors.New("invalid known proxies")

// MaxKnownProxies is how many addresses, networks and host names Known
// proxies holds at most, together.
const MaxKnownProxies = 64

// ParseKnownProxies reads the entries of Known proxies: IPv4 and IPv6
// addresses, and networks in CIDR notation, such as 172.18.0.0/16 (see
// canonicalProxies), then host names, such as a container's name on a
// Docker network (see canonicalProxyNames).
func ParseKnownProxies(entries []string) ([]netip.Prefix, []string, error) {
	if len(entries) > MaxKnownProxies {
		return nil, nil, ErrInvalidKnownProxies
	}
	proxies := make([]netip.Prefix, 0, len(entries))
	names := []string{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		network, err := netip.ParsePrefix(entry)
		if err != nil {
			address, err := netip.ParseAddr(entry)
			if err != nil && validHostName(entry) {
				names = append(names, entry)
				continue
			}
			if err != nil || address.Zone() != "" {
				return nil, nil, ErrInvalidKnownProxies
			}
			network = netip.PrefixFrom(address, address.BitLen())
		}
		proxies = append(proxies, network)
	}
	proxies, err := canonicalProxies(proxies)
	if err != nil {
		return nil, nil, err
	}
	names, err = canonicalProxyNames(names)
	if err != nil {
		return nil, nil, err
	}
	return proxies, names, nil
}

// FormatKnownProxies writes known proxies as ParseKnownProxies reads them:
// the addresses and networks, a single address without its prefix length,
// then the host names.
func FormatKnownProxies(proxies []netip.Prefix, names []string) []string {
	entries := make([]string, len(proxies), len(proxies)+len(names))
	for i, network := range proxies {
		if network.IsSingleIP() {
			entries[i] = network.Addr().String()
		} else {
			entries[i] = network.String()
		}
	}
	return append(entries, names...)
}

// canonicalProxies checks known proxies and returns them as they are
// compared with addresses: a network without its host bits, an
// IPv4-mapped IPv6 network as IPv4, each once, in the order given; empty,
// not nil, for none, as the column holds no NULL.
func canonicalProxies(proxies []netip.Prefix) ([]netip.Prefix, error) {
	if len(proxies) > MaxKnownProxies {
		return nil, ErrInvalidKnownProxies
	}
	canonical := make([]netip.Prefix, 0, len(proxies))
	for _, network := range proxies {
		if !network.IsValid() {
			return nil, ErrInvalidKnownProxies
		}
		if address := network.Addr(); address.Is4In6() {
			if network.Bits() < 96 {
				return nil, ErrInvalidKnownProxies
			}
			network = netip.PrefixFrom(address.Unmap(), network.Bits()-96)
		}
		if network = network.Masked(); !slices.Contains(canonical, network) {
			canonical = append(canonical, network)
		}
	}
	return canonical, nil
}

// canonicalProxyNames checks the host names of known proxies and returns
// them in lower case, each once, in the order given; empty, not nil, for
// none, as the column holds no NULL.
func canonicalProxyNames(names []string) ([]string, error) {
	if len(names) > MaxKnownProxies {
		return nil, ErrInvalidKnownProxies
	}
	canonical := make([]string, 0, len(names))
	for _, name := range names {
		if !validHostName(name) {
			return nil, ErrInvalidKnownProxies
		}
		if name = strings.ToLower(name); !slices.Contains(canonical, name) {
			canonical = append(canonical, name)
		}
	}
	return canonical, nil
}

// validHostName reports whether name is a host name the resolver can look
// up: dot-separated labels of 1 to 63 letters, digits, hyphens and
// underscores, as Docker allows in a container's name, none starting or
// ending with a hyphen, 253 characters at most, with an optional final
// dot. Its last label is not all digits, so that a mistyped IPv4 address,
// such as 192.168.1.300, is no name.
func validHostName(name string) bool {
	name = strings.TrimSuffix(name, ".")
	if name == "" || len(name) > 253 {
		return false
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range []byte(label) {
			if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

// ErrInvalidAnalysisTimeout reports an AnalysisTimeout outside
// [MinAnalysisTimeout, MaxAnalysisTimeout].
var ErrInvalidAnalysisTimeout = errors.New("invalid analysis timeout")

// ErrInvalidVersionAttempts reports a VersionAttempts outside
// [MinVersionAttempts, MaxVersionAttempts].
var ErrInvalidVersionAttempts = errors.New("invalid version attempts")

// ErrInvalidMaxConversions reports a MaxConversions outside
// [MinMaxConversions, MaxMaxConversions].
var ErrInvalidMaxConversions = errors.New("invalid maximum of conversions")

// ErrInvalidMaxConversionHeight reports a MaxConversionHeight that is not
// one of ConversionHeights.
var ErrInvalidMaxConversionHeight = errors.New("invalid maximum height of converted video")

// ErrInvalidSubtitleOCR reports a SubtitleOCR not among SubtitleOCRModes,
// and ErrInvalidSubtitleOCRTracks a SubtitleOCRTracks outside
// [MinSubtitleOCRTracks, MaxSubtitleOCRTracks].
var (
	ErrInvalidSubtitleOCR       = errors.New("invalid choice of picture subtitles read into text")
	ErrInvalidSubtitleOCRTracks = errors.New("invalid number of picture subtitle tracks read at once")
)

// SubtitleOCRModes are the values Settings.SubtitleOCR takes, the first by
// default: picture subtitles never read into text, read when an app that
// cannot draw them plays a version, or also after the scans of local
// folders.
var SubtitleOCRModes = []string{"off", "played", "scans"}

// The bounds and default of Settings.SubtitleOCRTracks.
const (
	MinSubtitleOCRTracks     = 1
	MaxSubtitleOCRTracks     = 4
	DefaultSubtitleOCRTracks = 1
)

// ErrInvalidDefaultLanguage reports a DefaultAudioLanguage or a
// DefaultSubtitleLanguage not among TrackLanguages.
var ErrInvalidDefaultLanguage = errors.New("invalid default track language")

// TrackLanguages are the values DefaultAudioLanguage and
// DefaultSubtitleLanguage take: "" for none, the default, then the ISO
// 639-2/B code of each of localization.Languages, the codes jellyfin-web
// saves as a user's own preferences (from /Localization/Cultures).
var TrackLanguages = sync.OnceValue(func() []string {
	codes := []string{""}
	for _, language := range localization.Languages() {
		codes = append(codes, language.Bibliographic)
	}
	return codes
})

// validTrackLanguage reports whether code is one of TrackLanguages.
func validTrackLanguage(code string) bool {
	return slices.Contains(TrackLanguages(), code)
}

// The bounds and defaults of Settings.AnalysisTimeout, in seconds,
// VersionAttempts and MaxConversions. A source that does not answer within
// 20 seconds is given up by default, which still lets a slow 2160p file be
// analyzed; the others are what Polyfin did before they were settings.
const (
	MinAnalysisTimeout     = 5
	MaxAnalysisTimeout     = 120
	DefaultAnalysisTimeout = 20
	MinVersionAttempts     = 1
	MaxVersionAttempts     = 10
	DefaultVersionAttempts = 3
	MinMaxConversions      = 0
	MaxMaxConversions      = 32
	DefaultMaxConversions  = 0
)

// ConversionHeights are the values Settings.MaxConversionHeight takes: 0,
// the default, keeps the height of the original, the others are the
// heights of usual video.
var ConversionHeights = []int{0, 480, 720, 1080, 1440, 2160}

// VersionCriteria are the values Settings.VersionOrder holds, each once
// at most and one of each kind: the user's preferred audio language, the
// height of the video, highest or lowest first, the size, largest or
// smallest first, the video codec, in the order of
// Settings.VersionCodecOrder, HDR or Dolby Vision, first or last, and the
// source, in the order of Settings.VersionSourceOrder.
var VersionCriteria = []string{"language", "resolution-high", "resolution-low", "size-large", "size-small", "codec", "hdr-first", "hdr-last",
	"source"}

// VersionCodecs are the video codecs the version rules know, by FFmpeg's
// names, in the default order of Settings.VersionCodecOrder: the most
// efficient first.
var VersionCodecs = []string{"av1", "hevc", "vp9", "h264", "vc1", "mpeg4", "mpeg2video"}

// The bounds of the version rules: MaxVersionCriteria criteria order the
// versions at most, MaxVersionSources sources are placed at most, versions
// are hidden above MaxHideVersionsLargerGB at most, and
// MaxHiddenVersionWords words, of MaxHiddenVersionWordLength characters at
// most, hide versions.
const (
	MaxVersionCriteria         = 4
	MaxVersionSources          = 500
	MaxHideVersionsLargerGB    = 1000
	MaxHiddenVersionWords      = 50
	MaxHiddenVersionWordLength = 100
)

// The errors of the version rules: ErrInvalidVersionOrder reports a
// Settings.VersionOrder with a value that is not one of VersionCriteria,
// two of one kind, or more than MaxVersionCriteria;
// ErrInvalidVersionCodecOrder a VersionCodecOrder that is not every one of
// VersionCodecs once; ErrInvalidVersionSourceOrder a VersionSourceOrder
// holding a source twice, or more than MaxVersionSources, and, from the
// admin API, a source the server does not have;
// ErrInvalidHideVersionsLarger a HideVersionsLargerGB outside [0,
// MaxHideVersionsLargerGB]; ErrInvalidHiddenVersionCodecs a
// HiddenVersionCodecs holding another codec, or one twice; and
// ErrInvalidHiddenVersionWords a HiddenVersionWords with more than
// MaxHiddenVersionWords words, or one longer than
// MaxHiddenVersionWordLength characters or holding a control character.
var (
	ErrInvalidVersionOrder        = errors.New("invalid version order")
	ErrInvalidVersionCodecOrder   = errors.New("invalid version codec order")
	ErrInvalidVersionSourceOrder  = errors.New("invalid version source order")
	ErrInvalidHideVersionsLarger  = errors.New("invalid size above which versions are hidden")
	ErrInvalidHiddenVersionCodecs = errors.New("invalid hidden video codecs")
	ErrInvalidHiddenVersionWords  = errors.New("invalid words hiding versions")
)

// VersionCriterionKind is what a criterion of VersionCriteria looks at,
// whichever way it sorts: "resolution" for resolution-high and
// resolution-low.
func VersionCriterionKind(criterion string) string {
	kind, _, _ := strings.Cut(criterion, "-")
	return kind
}

// validVersionOrder reports whether order holds criteria of
// VersionCriteria, one of each kind at most, MaxVersionCriteria at most.
func validVersionOrder(order []string) bool {
	if len(order) > MaxVersionCriteria {
		return false
	}
	for i, criterion := range order {
		if !slices.Contains(VersionCriteria, criterion) ||
			slices.ContainsFunc(order[:i], func(before string) bool { return VersionCriterionKind(before) == VersionCriterionKind(criterion) }) {
			return false
		}
	}
	return true
}

// validVersionCodecs reports whether codecs are some of VersionCodecs,
// each once.
func validVersionCodecs(codecs []string) bool {
	for i, codec := range codecs {
		if !slices.Contains(VersionCodecs, codec) || slices.Contains(codecs[:i], codec) {
			return false
		}
	}
	return true
}

// validVersionSources reports whether sources hold each source once,
// MaxVersionSources at most.
func validVersionSources(sources []ID) bool {
	if len(sources) > MaxVersionSources {
		return false
	}
	for i, source := range sources {
		if slices.Contains(sources[:i], source) {
			return false
		}
	}
	return true
}

// canonicalHiddenVersionWords checks the words hiding versions and returns
// them trimmed, each once whatever its letter case, with the spelling it
// first has, in the order given, empty lines left out; empty, not nil, for
// none, as the column holds no NULL.
func canonicalHiddenVersionWords(words []string) ([]string, error) {
	canonical := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if !utf8.ValidString(word) || utf8.RuneCountInString(word) > MaxHiddenVersionWordLength || strings.ContainsFunc(word, unicode.IsControl) {
			return nil, ErrInvalidHiddenVersionWords
		}
		if !slices.ContainsFunc(canonical, func(kept string) bool { return strings.EqualFold(kept, word) }) {
			canonical = append(canonical, word)
		}
	}
	if len(canonical) > MaxHiddenVersionWords {
		return nil, ErrInvalidHiddenVersionWords
	}
	return canonical, nil
}

// The errors of the settings tuning conversions, reporting a value
// outside the bounds or choices below.
var (
	ErrInvalidEncoderPreset          = errors.New("invalid encoder preset")
	ErrInvalidVideoQuality           = errors.New("invalid video quality")
	ErrInvalidHardwareAcceleration   = errors.New("invalid hardware acceleration")
	ErrInvalidHardwareDecodingCodecs = errors.New("invalid hardware decoding codecs")
	ErrInvalidToneMappingAlgorithm   = errors.New("invalid tone mapping algorithm")
	ErrInvalidToneMappingPeak        = errors.New("invalid tone mapping peak")
	ErrInvalidToneMappingDesat       = errors.New("invalid tone mapping desaturation")
	ErrInvalidToneMappingHeight      = errors.New("invalid height of HDR tone mapped on the processor")
	ErrInvalidDeinterlaceMethod      = errors.New("invalid deinterlace method")
	ErrInvalidDownmixAlgorithm       = errors.New("invalid downmix algorithm")
	ErrInvalidDownmixBoost           = errors.New("invalid downmix boost")
	ErrInvalidMaxAudioChannels       = errors.New("invalid maximum of audio channels")
	ErrInvalidAudioBitrate           = errors.New("invalid audio bitrate")
	ErrInvalidEncodingThreads        = errors.New("invalid encoding threads")
	ErrInvalidAheadSeconds           = errors.New("invalid seconds ahead")
	ErrInvalidAdaptiveQuality        = errors.New("invalid adaptive quality")
)

// The choices of the settings tuning conversions, named as Jellyfin's
// encoding options name them where it has them; the first of each is the
// default.
var (
	// EncoderPresets are the values of Settings.EncoderPreset: auto lets
	// Polyfin choose each encoder's, then from the slowest to the fastest.
	EncoderPresets = []string{"auto", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"}
	// HardwareAccelerations are the values of Settings.HardwareAcceleration
	// on this system, those POLYFIN_HWACCEL takes (see
	// HardwareAccelerationsOn).
	HardwareAccelerations = HardwareAccelerationsOn(runtime.GOOS)
	// HardwareDecodingCodecs are the values Settings.HardwareDecodingCodecs
	// holds, all by default: FFmpeg's codec names, and hevc_10bit for HEVC
	// in 10 bits, which HEVC needs too.
	HardwareDecodingCodecs = []string{"h264", "hevc", "hevc_10bit", "vp9", "av1", "mpeg2video", "vc1"}
	// ToneMappingAlgorithms are the values of Settings.ToneMappingAlgorithm:
	// auto is BT.2390 on the GPU and Hable on the processor.
	ToneMappingAlgorithms = []string{"auto", "bt2390", "hable", "reinhard", "mobius", "clip", "linear"}
	// DeinterlaceMethods are the values of Settings.DeinterlaceMethod.
	DeinterlaceMethods = []string{"yadif", "bwdif"}
	// DownmixAlgorithms are the values of Settings.DownmixAlgorithm: None
	// leaves FFmpeg's own downmix.
	DownmixAlgorithms = []string{"None", "Dave750", "NightmodeDialogue", "Rfc7845", "Ac4"}
	// AudioChannelLimits are the values of Settings.MaxAudioChannels: 0
	// leaves the app's limit alone.
	AudioChannelLimits = []int{0, 1, 2, 6}
	// ToneMappingHeights are the values of
	// Settings.ProcessorToneMappingHeight: 0, Automatic, is what a timing
	// at startup chose, then heights of usual video.
	ToneMappingHeights = []int{0, 720, 1080, 1440, 2160}
	// AdaptiveQualities are the values of Settings.AdaptiveQuality.
	AdaptiveQualities = []string{AdaptiveQualityOff, AdaptiveQualityRemote, AdaptiveQualityAlways}
)

// HardwareAccelerationsOn are the GPU choices of system, a GOOS: auto
// tries the system's GPU methods by preference, NVIDIA (nvenc) then VAAPI
// on Linux, NVIDIA then Intel (qsv) then AMD (amf) on Windows, and
// VideoToolbox on macOS; none converts on the processor. Other systems
// offer Linux's.
func HardwareAccelerationsOn(system string) []string {
	switch system {
	case "darwin":
		return []string{"auto", "videotoolbox", "none"}
	case "windows":
		return []string{"auto", "nvenc", "qsv", "amf", "none"}
	}
	return []string{"auto", "nvenc", "vaapi", "none"}
}

// The values of Settings.AdaptiveQuality.
const (
	// AdaptiveQualityOff offers one quality, the one chosen.
	AdaptiveQualityOff = "off"
	// AdaptiveQualityRemote offers lower ones to apps outside the local
	// network, as sign-in PINs judge it.
	AdaptiveQualityRemote = "remote"
	// AdaptiveQualityAlways offers lower ones to every app.
	AdaptiveQualityAlways = "always"
)

// The bounds and defaults of the numbers tuning conversions. The defaults
// are what Polyfin did before they were settings: 0 quality aims for the
// bitrate alone, 0 peak keeps the video's, a boost of 1 changes nothing,
// 0 audio bitrate keeps Polyfin's per channel count, and 0 threads lets
// FFmpeg choose.
const (
	MinVideoQuality           = 1
	MaxVideoQuality           = 51
	MinToneMappingPeak        = 100
	MaxToneMappingPeak        = 10000
	MaxToneMappingDesat       = 10
	MinDownmixBoost           = 0.5
	MaxDownmixBoost           = 3
	DefaultDownmixBoost       = 1
	MinAudioBitratePerChannel = 32
	MaxAudioBitratePerChannel = 320
	MaxEncodingThreads        = 64
)

// The bounds and default of Settings.AheadSeconds: a player buffers a
// minute or two ahead, and FFmpeg working further keeps the source's
// connection busy, absorbing its slowdowns.
const (
	MinAheadSeconds     = 30
	MaxAheadSeconds     = 600
	DefaultAheadSeconds = 120
)

// ErrInvalidTrickplayInterval reports a TrickplayInterval outside
// [MinTrickplayInterval, MaxTrickplayInterval].
var ErrInvalidTrickplayInterval = errors.New("invalid trickplay interval")

// ErrInvalidTrickplayWidth reports a TrickplayWidth that is not one of
// TrickplayWidths.
var ErrInvalidTrickplayWidth = errors.New("invalid trickplay width")

// ErrInvalidThumbnailStorage reports a ThumbnailStorageGB outside
// [MinThumbnailStorageGB, MaxThumbnailStorageGB].
var ErrInvalidThumbnailStorage = errors.New("invalid thumbnail storage")

// The bounds and defaults of Settings.TrickplayInterval, in seconds,
// TrickplayWidth, in pixels, and ThumbnailStorageGB. The interval and
// width default to Jellyfin's.
const (
	MinTrickplayInterval      = 5
	MaxTrickplayInterval      = 60
	DefaultTrickplayInterval  = 10
	DefaultTrickplayWidth     = 320
	MinThumbnailStorageGB     = 1
	MaxThumbnailStorageGB     = 50
	DefaultThumbnailStorageGB = 2
)

// TrickplayWidths are the widths Settings.TrickplayWidth takes.
var TrickplayWidths = []int{240, 320, 480}

// ErrInvalidRecordingPadding reports a RecordingPrePadding or
// RecordingPostPadding outside [0, MaxRecordingPadding].
var ErrInvalidRecordingPadding = errors.New("invalid recording padding")

// ErrInvalidRecordingRetentionDays reports a RecordingRetentionDays outside
// [0, MaxRecordingRetentionDays].
var ErrInvalidRecordingRetentionDays = errors.New("invalid recording retention days")

// The bounds and defaults of Settings.RecordingPrePadding and
// RecordingPostPadding, in seconds, and RecordingRetentionDays. The
// paddings default to Jellyfin's, none; 0 days keeps recordings until they
// are deleted.
const (
	MaxRecordingPadding           = 3600
	DefaultRecordingPrePadding    = 0
	DefaultRecordingPostPadding   = 0
	MaxRecordingRetentionDays     = 3650
	DefaultRecordingRetentionDays = 0
)

// ErrInvalidConvertRecordingsHours reports a ConvertRecordingsFrom or
// ConvertRecordingsUntil outside [0, 23], or both the same hour.
var ErrInvalidConvertRecordingsHours = errors.New("invalid hours of the conversion of recordings")

// The defaults of Settings.ConvertRecordingsFrom and ConvertRecordingsUntil:
// from 1:00 to 6:00, in the server's time zone.
const (
	DefaultConvertRecordingsFrom  = 1
	DefaultConvertRecordingsUntil = 6
)

// ErrInvalidPlaybackHistoryDays reports a PlaybackHistoryDays outside [1,
// MaxPlaybackHistoryDays].
var ErrInvalidPlaybackHistoryDays = errors.New("invalid playback history days")

// The bounds and default of Settings.PlaybackHistoryDays: a year by
// default, ten at most.
const (
	MaxPlaybackHistoryDays     = 3650
	DefaultPlaybackHistoryDays = 365
)

// ErrInvalidWeeklySummaryDay reports a WeeklySummaryDay outside [0, 6], and
// ErrInvalidWeeklySummaryHour a WeeklySummaryHour outside [0, 23].
var (
	ErrInvalidWeeklySummaryDay  = errors.New("invalid weekly summary day")
	ErrInvalidWeeklySummaryHour = errors.New("invalid weekly summary hour")
)

// MaxMetricsTokenBytes bounds Settings.MetricsToken, which Polyfin makes
// of 32 hexadecimal characters; ErrInvalidMetricsToken reports one longer,
// or holding control characters.
const MaxMetricsTokenBytes = 32

var ErrInvalidMetricsToken = errors.New("invalid metrics token")

// The defaults of Settings.WeeklySummaryDay and WeeklySummaryHour: Monday
// at 9:00.
const (
	DefaultWeeklySummaryDay  = 1
	DefaultWeeklySummaryHour = 9
)

// ErrInvalidImportEveryMinutes reports an ImportEveryMinutes that is not
// one of ImportEveryChoices.
var ErrInvalidImportEveryMinutes = errors.New("invalid import interval")

// ImportEveryChoices are the values Settings.ImportEveryMinutes takes, from
// 15 minutes to 24 hours; DefaultImportEveryMinutes is 6 hours.
var ImportEveryChoices = []int{15, 30, 60, 120, 180, 360, 720, 1440}

const DefaultImportEveryMinutes = 360

// ErrInvalidLiveTvRefreshHours reports a LiveTvRefreshHours outside
// [MinLiveTvRefreshHours, MaxLiveTvRefreshHours].
var ErrInvalidLiveTvRefreshHours = errors.New("invalid Live TV refresh hours")

// The bounds and default of Settings.LiveTvRefreshHours: by default, the
// XMLTV guides and IPTV channel lists are fetched again every 12 hours.
const (
	MinLiveTvRefreshHours     = 1
	MaxLiveTvRefreshHours     = 168
	DefaultLiveTvRefreshHours = 12
)

// ErrInvalidLocalScanHours reports a LocalScanHours outside
// [0, MaxLocalScanHours].
var ErrInvalidLocalScanHours = errors.New("invalid local scan hours")

// The bound and default of Settings.LocalScanHours: by default, local
// folders are scanned again every 6 hours.
const (
	MaxLocalScanHours     = 168
	DefaultLocalScanHours = 6
)

// The web player's looks, Settings.WebTheme: the LumaaGlaass theme Polyfin
// bundles (LumaaGlaassCss and LumaaGlaassJs), jellyfin-web's own look, with
// no custom CSS or script, and the administrator's CustomCss and CustomJs.
const (
	WebThemeLumaaGlaass = "lumaaglaass"
	WebThemeNone        = "none"
	WebThemeCustom      = "custom"
)

// WebThemes are the values of Settings.WebTheme, the default first.
var WebThemes = []string{WebThemeLumaaGlaass, WebThemeNone, WebThemeCustom}

// ServedCss is the custom CSS the web player gets for WebTheme: Jellyfin's
// branding CSS.
func (settings Settings) ServedCss() string {
	switch settings.WebTheme {
	case WebThemeLumaaGlaass:
		return LumaaGlaassCss
	case WebThemeCustom:
		return settings.CustomCss
	}
	return ""
}

// ServedJs is the script Polyfin adds to the web player's page for
// WebTheme.
func (settings Settings) ServedJs() string {
	switch settings.WebTheme {
	case WebThemeLumaaGlaass:
		return LumaaGlaassJs
	case WebThemeCustom:
		return settings.CustomJs
	}
	return ""
}

// ErrInvalidWebTheme reports a Settings.WebTheme not in WebThemes.
// ErrInvalidCustomCss, ErrInvalidCustomJs and ErrInvalidLoginDisclaimer
// report a Settings.CustomCss, CustomJs or LoginDisclaimer longer than its
// maximum, or holding a NUL character, which no web page has.
var (
	ErrInvalidWebTheme        = errors.New("invalid web player theme")
	ErrInvalidCustomCss       = errors.New("invalid custom CSS")
	ErrInvalidCustomJs        = errors.New("invalid custom JavaScript")
	ErrInvalidLoginDisclaimer = errors.New("invalid login disclaimer")
)

// The largest Settings.CustomCss and CustomJs, and LoginDisclaimer, in
// bytes. 2 MiB holds a whole theme pasted in, tens of thousands of lines.
const (
	MaxCustomCodeBytes      = 2 << 20
	MaxLoginDisclaimerBytes = 8 << 10
)

// ErrInvalidPublicMetaDBKey and ErrInvalidTheIntroDBKey report a
// Settings.PublicMetaDBKey or TheIntroDBKey that is not a valid key (see
// ValidSegmentKey).
var (
	ErrInvalidPublicMetaDBKey = errors.New("invalid PublicMetaDB key")
	ErrInvalidTheIntroDBKey   = errors.New("invalid TheIntroDB key")
)

// MaxSegmentKeyBytes is the longest Settings.PublicMetaDBKey and
// TheIntroDBKey.
const MaxSegmentKeyBytes = 256

// ValidSegmentKey reports whether key may be the API key of a segment
// database: at most MaxSegmentKeyBytes of printable ASCII. Empty is valid,
// for no key.
func ValidSegmentKey(key string) bool {
	if len(key) > MaxSegmentKeyBytes {
		return false
	}
	for i := range len(key) {
		if key[i] < ' ' || key[i] > '~' {
			return false
		}
	}
	return true
}

// SegmentSources are the segment databases, by the names POLYFIN_SEGMENTS
// gives them, in the default order of preference.
var SegmentSources = []string{"theintrodb", "introdb", "publicmetadb", "skipdb"}

// ErrInvalidSegmentOrder reports a Settings.SegmentOrder that is not every
// one of SegmentSources once, and ErrInvalidSegmentSourcesOff a
// SegmentSourcesOff holding another name, or one twice.
var (
	ErrInvalidSegmentOrder      = errors.New("invalid segment order")
	ErrInvalidSegmentSourcesOff = errors.New("invalid segment sources turned off")
)

// validSegmentOrder reports whether order is SegmentSources in any order.
func validSegmentOrder(order []string) bool {
	return len(order) == len(SegmentSources) && validSegmentSources(order)
}

// validSegmentSources reports whether names are some of SegmentSources,
// each once.
func validSegmentSources(names []string) bool {
	for i, name := range names {
		if !slices.Contains(SegmentSources, name) || slices.Contains(names[:i], name) {
			return false
		}
	}
	return true
}

// ErrInvalidMDBListRatingsKey reports a Settings.MDBListRatingsKey that is
// not a valid key (see ValidRatingsKey), and ErrInvalidCriticRatingShows a
// CriticRatingShows not among CriticRatingChoices.
var (
	ErrInvalidMDBListRatingsKey = errors.New("invalid MDBList ratings key")
	ErrInvalidCriticRatingShows = errors.New("invalid critic rating choice")
)

// MaxRatingsKeyBytes is the longest Settings.MDBListRatingsKey.
const MaxRatingsKeyBytes = 256

// ValidRatingsKey reports whether key may be an MDBList API key: at most
// MaxRatingsKeyBytes of printable ASCII without spaces. Empty is valid,
// for no key.
func ValidRatingsKey(key string) bool {
	if len(key) > MaxRatingsKeyBytes {
		return false
	}
	for i := range len(key) {
		if key[i] <= ' ' || key[i] > '~' {
			return false
		}
	}
	return true
}

// CriticRatingChoices are the scores CriticRating may carry (see
// Settings.CriticRatingShows), the first by default: the critics' score
// or the audience score.
var CriticRatingChoices = []string{"critics", "audience"}

// ErrInvalidTMDBKey reports a Settings.TMDBKey that is not a valid key
// (see ValidTMDBKey).
var ErrInvalidTMDBKey = errors.New("invalid TMDB key")

// MaxTMDBKeyBytes is the longest Settings.TMDBKey: a read access token is
// much longer than an API key.
const MaxTMDBKeyBytes = 512

// ValidTMDBKey reports whether key may be a TMDB API key or read access
// token: at most MaxTMDBKeyBytes of printable ASCII without spaces. Empty
// is valid, for no key.
func ValidTMDBKey(key string) bool {
	if len(key) > MaxTMDBKeyBytes {
		return false
	}
	for i := range len(key) {
		if key[i] <= ' ' || key[i] > '~' {
			return false
		}
	}
	return true
}

// ErrInvalidMeasureLoudness reports a Settings.MeasureLoudness not among
// MeasureLoudnessChoices.
var ErrInvalidMeasureLoudness = errors.New("invalid loudness measure choice")

// MeasureLoudnessChoices are the songs without a gain whose loudness is
// measured (see Settings.MeasureLoudness), the first by default: none,
// those of local music folders, or those of music addons too.
var MeasureLoudnessChoices = []string{"off", "local", "all"}

// ErrInvalidTraktApp, ErrInvalidSimklApp, ErrInvalidLastFMApp,
// ErrInvalidAniListApp and ErrInvalidMyAnimeListApp report a
// Settings.TraktClientID, TraktClientSecret, SimklClientID, LastFMAPIKey,
// LastFMSecret, AniListClientID, AniListClientSecret, MyAnimeListClientID or
// MyAnimeListClientSecret longer than MaxTrackingAppBytes, or holding
// anything but printable ASCII without spaces, which the services'
// credentials are made of.
var (
	ErrInvalidTraktApp       = errors.New("invalid Trakt app")
	ErrInvalidSimklApp       = errors.New("invalid Simkl app")
	ErrInvalidLastFMApp      = errors.New("invalid Last.fm API account")
	ErrInvalidAniListApp     = errors.New("invalid AniList app")
	ErrInvalidMyAnimeListApp = errors.New("invalid MyAnimeList app")
)

// MaxTrackingAppBytes is the longest credential of a tracking service's
// app, in bytes.
const MaxTrackingAppBytes = 256

// ErrInvalidBackupHour reports a BackupHour outside [0, 23],
// ErrInvalidBackupsKept a BackupsKept outside [MinBackupsKept,
// MaxBackupsKept], and ErrInvalidCollectionReadHour a CollectionReadHour
// outside [-1, 23].
var (
	ErrInvalidBackupHour         = errors.New("invalid backup hour")
	ErrInvalidBackupsKept        = errors.New("invalid number of backups kept")
	ErrInvalidCollectionReadHour = errors.New("invalid collection read hour")
)

// The bounds and defaults of Settings.BackupHour, BackupsKept and
// CollectionReadHour: by default, the database is backed up at 4 in the
// morning, the last week of backups is kept, and collections are never
// all read.
const (
	DefaultBackupHour         = 4
	MinBackupsKept            = 1
	MaxBackupsKept            = 90
	DefaultBackupsKept        = 7
	DefaultCollectionReadHour = -1
)

// ErrInvalidCollectionLibraryApps reports a Settings.CollectionLibraryApps
// entry that is empty once trimmed, longer than MaxCollectionLibraryAppLength
// characters or holding a control character, or more than
// MaxCollectionLibraryApps entries.
var ErrInvalidCollectionLibraryApps = errors.New("invalid collection library apps")

// MaxCollectionLibraryApps is how many apps Settings.CollectionLibraryApps
// names at most, and MaxCollectionLibraryAppLength the longest name, in
// characters.
const (
	MaxCollectionLibraryApps      = 32
	MaxCollectionLibraryAppLength = 100
)

// canonicalCollectionLibraryApps checks the apps told collection libraries
// hold collections and returns them trimmed, each once whatever its letter
// case, with the spelling it first has, in the order given; empty, not
// nil, for none, as the column holds no NULL.
func canonicalCollectionLibraryApps(apps []string) ([]string, error) {
	if len(apps) > MaxCollectionLibraryApps {
		return nil, ErrInvalidCollectionLibraryApps
	}
	canonical := make([]string, 0, len(apps))
	for _, app := range apps {
		app = strings.TrimSpace(app)
		if app == "" || !utf8.ValidString(app) || utf8.RuneCountInString(app) > MaxCollectionLibraryAppLength ||
			strings.ContainsFunc(app, unicode.IsControl) {
			return nil, ErrInvalidCollectionLibraryApps
		}
		if !slices.ContainsFunc(canonical, func(kept string) bool { return strings.EqualFold(kept, app) }) {
			canonical = append(canonical, app)
		}
	}
	return canonical, nil
}

// ErrInvalidRemuxDBURL reports a Settings.RemuxDBURL that is not a valid
// address (see ValidRemuxDBURL).
var ErrInvalidRemuxDBURL = errors.New("invalid RemuxDB address")

// DefaultRemuxDBURL is the RemuxDB server asked by default, and
// MaxRemuxDBURLBytes the longest Settings.RemuxDBURL.
const (
	DefaultRemuxDBURL  = "https://remuxdb.1632022.xyz"
	MaxRemuxDBURLBytes = 512
)

// ValidRemuxDBURL reports whether address may be the address of a RemuxDB
// server: an absolute http or https URL with a host, without user info,
// query or fragment, of at most MaxRemuxDBURLBytes, holding no spaces or
// control characters. Empty is invalid.
func ValidRemuxDBURL(address string) bool {
	// The scheme is matched as written, in lower case, as the database's
	// constraint matches it.
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") || len(address) > MaxRemuxDBURLBytes {
		return false
	}
	for _, r := range address {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	// A question mark or a number sign would start a query or a fragment.
	if strings.ContainsAny(address, "?#") {
		return false
	}
	parsed, err := url.Parse(address)
	return err == nil && parsed.Hostname() != "" && parsed.User == nil
}

// ErrInvalidUpdateToolURL reports a Settings.UpdateToolURL that is not
// empty nor a valid address (see ValidUpdateToolURL), and
// ErrInvalidUpdateToolToken a Settings.UpdateToolToken longer than
// MaxUpdateToolTokenBytes or holding spaces or control characters.
var (
	ErrInvalidUpdateToolURL   = errors.New("invalid update tool address")
	ErrInvalidUpdateToolToken = errors.New("invalid update tool token")
)

// MaxUpdateToolURLBytes is the longest Settings.UpdateToolURL, and
// MaxUpdateToolTokenBytes the longest Settings.UpdateToolToken.
const (
	MaxUpdateToolURLBytes   = 1024
	MaxUpdateToolTokenBytes = 1024
)

// ValidUpdateToolURL reports whether address may be the update API of an
// update tool: empty, or an absolute http or https URL with a host, a path
// and a query, without user info or fragment, of at most
// MaxUpdateToolURLBytes, holding no spaces or control characters.
func ValidUpdateToolURL(address string) bool {
	if address == "" {
		return true
	}
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") || len(address) > MaxUpdateToolURLBytes ||
		strings.ContainsFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) || strings.Contains(address, "#") {
		return false
	}
	parsed, err := url.Parse(address)
	return err == nil && parsed.Hostname() != "" && parsed.User == nil
}

// ErrInvalidPublicAddress reports a Settings.PublicAddress that is not
// empty nor a valid address (see ValidPublicAddress).
var ErrInvalidPublicAddress = errors.New("invalid public address")

// MaxPublicAddressBytes is the longest Settings.PublicAddress.
const MaxPublicAddressBytes = 512

// ValidPublicAddress reports whether address may be the address people
// open Polyfin at: empty, or an absolute http or https URL with a host,
// without user info, query or fragment, of at most MaxPublicAddressBytes,
// holding no spaces or control characters.
func ValidPublicAddress(address string) bool {
	return address == "" || len(address) <= MaxPublicAddressBytes && ValidRemuxDBURL(address)
}

// ErrInvalidLocalAddress reports a Settings.LocalAddress that is not empty
// nor a valid address (see ValidLocalAddress).
var ErrInvalidLocalAddress = errors.New("invalid local address")

// MaxLocalAddressBytes is the longest Settings.LocalAddress.
const MaxLocalAddressBytes = 512

// ValidLocalAddress reports whether address may be the address Polyfin is
// reached at on the local network: empty, or an absolute http or https URL
// with a host, without user info, query or fragment, of at most
// MaxLocalAddressBytes, holding no spaces or control characters.
func ValidLocalAddress(address string) bool {
	return address == "" || len(address) <= MaxLocalAddressBytes && ValidRemuxDBURL(address)
}

// MaxDLNANameBytes bounds Settings.DLNAName; ErrInvalidDLNAName reports
// one longer, or holding NUL.
const MaxDLNANameBytes = 256

var ErrInvalidDLNAName = errors.New("invalid DLNA name")

// The SMTP server email notifications go through: Settings.SMTPSecurity is
// one of SMTPSecurities, the first by default, which upgrades a plain
// connection with STARTTLS; "tls" is TLS from the start, "none" neither.
var SMTPSecurities = []string{"starttls", "tls", "none"}

// The bounds of the SMTP settings, and the default port, the submission
// port.
const (
	DefaultSMTPPort      = 587
	MaxSMTPHostBytes     = 253
	MaxSMTPUserBytes     = 256
	MaxSMTPPasswordBytes = 256
	MaxEmailBytes        = 254
	MaxSMTPFromNameBytes = 128
)

// ErrInvalidSMTPHost reports a Settings.SMTPHost longer than
// MaxSMTPHostBytes or holding anything but printable ASCII without spaces;
// ErrInvalidSMTPPort an SMTPPort outside [1, 65535]; ErrInvalidSMTPSecurity
// an SMTPSecurity not among SMTPSecurities; ErrInvalidSMTPAccount an
// SMTPUser or SMTPPassword too long or holding control characters; and
// ErrInvalidSMTPSender an SMTPFrom that is not empty nor an email address
// (see ValidEmail), or an SMTPFromName too long or holding control
// characters.
var (
	ErrInvalidSMTPHost     = errors.New("invalid SMTP host")
	ErrInvalidSMTPPort     = errors.New("invalid SMTP port")
	ErrInvalidSMTPSecurity = errors.New("invalid SMTP security")
	ErrInvalidSMTPAccount  = errors.New("invalid SMTP user or password")
	ErrInvalidSMTPSender   = errors.New("invalid SMTP sender")
)

// ValidEmail reports whether address is an email address alone, such as
// polyfin@example.org, without a name or angle brackets, of at most
// MaxEmailBytes.
func ValidEmail(address string) bool {
	if len(address) > MaxEmailBytes || strings.ContainsFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return false
	}
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Name == "" && parsed.Address == address
}

// validLine reports whether text fits in max bytes and holds no control
// character.
func validLine(text string, max int) bool {
	return len(text) <= max && utf8.ValidString(text) && !strings.ContainsFunc(text, unicode.IsControl)
}

// ErrInvalidCacheSize reports a CacheSizeGB outside [MinCacheSizeGB,
// MaxCacheSizeGB], and ErrInvalidVAAPIDevice a VAAPIDevice that is not a
// render node (see ValidVAAPIDevice).
var (
	ErrInvalidCacheSize   = errors.New("invalid cache size")
	ErrInvalidVAAPIDevice = errors.New("invalid VAAPI render node")
)

// The bounds and default of Settings.CacheSizeGB, in gigabytes (10^9
// bytes): by default, 10 GB keep a few movies, read again for seeks and
// restarts without downloading them again.
const (
	MinCacheSizeGB     = 1
	MaxCacheSizeGB     = 2000
	DefaultCacheSizeGB = 10
)

// vaapiDevice matches the render nodes VAAPI may open.
var vaapiDevice = regexp.MustCompile(`^/dev/dri/renderD[0-9]{1,4}$`)

// ValidVAAPIDevice reports whether device may be Settings.VAAPIDevice:
// empty, for each render node in turn, or a render node such as
// /dev/dri/renderD128.
func ValidVAAPIDevice(device string) bool {
	return device == "" || vaapiDevice.MatchString(device)
}

// ErrInvalidRecordingsFolder and ErrInvalidBackupFolder report a
// RecordingsFolder or a BackupFolder that is not a folder's path (see
// CleanFolder), or, from the admin API, a folder Polyfin cannot write into.
var (
	ErrInvalidRecordingsFolder = errors.New("invalid recordings folder")
	ErrInvalidBackupFolder     = errors.New("invalid backup folder")
)

// MaxFolderBytes is the longest RecordingsFolder or BackupFolder, in bytes.
const MaxFolderBytes = 512

// CleanFolder returns folder as Settings.RecordingsFolder and BackupFolder
// keep it, cleaned (see filepath.Clean), and whether it may be one: empty,
// for the default folder, or an absolute path of at most MaxFolderBytes
// holding no control characters. Nothing is looked up on disk.
func CleanFolder(folder string) (string, bool) {
	if folder == "" {
		return "", true
	}
	if !filepath.IsAbs(folder) || strings.ContainsFunc(folder, unicode.IsControl) {
		return "", false
	}
	folder = filepath.Clean(folder)
	return folder, len(folder) <= MaxFolderBytes
}

// DefaultRecordingsFolder is the folder recordings are written to when the
// settings name none, on a server keeping its files in dataDir.
func DefaultRecordingsFolder(dataDir string) string {
	return filepath.Join(dataDir, "recordings")
}

// DefaultBackupFolder is the folder the database is backed up into when
// the settings name none, on a server keeping its files in dataDir.
func DefaultBackupFolder(dataDir string) string {
	return filepath.Join(dataDir, "backups")
}

// RecordingsDir is the folder recordings are written to on a server
// keeping its files in dataDir: RecordingsFolder, or the default folder,
// while recording is on; empty while it is off.
func (s Settings) RecordingsDir(dataDir string) string {
	switch {
	case !s.Recording:
		return ""
	case s.RecordingsFolder != "":
		return s.RecordingsFolder
	}
	return DefaultRecordingsFolder(dataDir)
}

// BackupDir is the folder the database is backed up into on a server
// keeping its files in dataDir: BackupFolder, or the default folder, while
// backups are on; empty while they are off.
func (s Settings) BackupDir(dataDir string) string {
	switch {
	case !s.Backups:
		return ""
	case s.BackupFolder != "":
		return s.BackupFolder
	}
	return DefaultBackupFolder(dataDir)
}

// ErrInvalidConversionFolder, ErrInvalidReadingFolder and
// ErrInvalidCacheFolder report a ConversionFolder, a ReadingFolder or a
// CacheFolder that is not a folder's path (see CleanFolder), or, from the
// admin API, a folder Polyfin cannot write into.
var (
	ErrInvalidConversionFolder = errors.New("invalid conversion folder")
	ErrInvalidReadingFolder    = errors.New("invalid folder for files being read")
	ErrInvalidCacheFolder      = errors.New("invalid cache folder")
)

// DefaultCacheFolder is the cache in the data folder dataDir: the folder
// the segments of conversions, the files being read and the cache are
// kept in when the settings name no ConversionFolder, ReadingFolder or
// CacheFolder.
func DefaultCacheFolder(dataDir string) string {
	return filepath.Join(dataDir, "cache")
}

// Settings are the server-wide options set from the admin interface.
type Settings struct {
	// ServerName is the name Jellyfin apps show for this server.
	ServerName          string
	QuickConnectEnabled bool
	// LegacyAuthorization accepts the X-Emby-* headers, the api_key query
	// parameter and the Emby scheme, which Jellyfin 12.3 refuses by default.
	LegacyAuthorization bool
	// UpdateCheck asks GitHub once a day for Polyfin's latest release, to
	// tell administrators about a new version (see package updates).
	UpdateCheck bool
	// RestartConfirmed tells that an administrator confirmed that Polyfin
	// is started again when it exits, by its container's restart policy
	// or its service manager: Restart in the admin app then asks no more
	// than a confirmation. Polyfin cannot see its container's restart
	// policy without the Docker socket.
	RestartConfirmed bool
	// UpdateToolURL is the address Update now sends its request to, the
	// update API of a tool the administrator runs beside Polyfin, empty
	// for none (see ValidUpdateToolURL and package updatetool), and
	// UpdateToolToken the bearer token sent with it, empty for none. The
	// token is a secret, like PublicMetaDBKey.
	UpdateToolURL   string
	UpdateToolToken string
	// LocalDiscovery answers the discovery messages Jellyfin apps
	// broadcast on the local network (see package discovery).
	LocalDiscovery bool
	// LocalAddress is the address Polyfin is reached at on the local
	// network, such as http://192.168.1.10:8096, without a trailing slash,
	// as Jellyfin's PublishedServerUrl: discovery answers announce it, and
	// /System/Info/Public gives it as LocalAddress. Empty, discovery
	// announces the address of the interface that reaches the app.
	LocalAddress string
	// DLNA shares the libraries over DLNA with the devices of the local
	// network (see package dlna).
	DLNA bool
	// DLNAName is the name DLNA devices show for the server, at most
	// MaxDLNANameBytes; empty, they show ServerName.
	DLNAName string
	// DLNANewDevicesUser is the user whose libraries a DLNA device seen for
	// the first time shows; nil, no one's, which gives it an empty server
	// until an administrator chooses a user for it.
	DLNANewDevicesUser *ID
	// DLNAMarkPlayed marks played a title a DLNA device reads past
	// PlayedPercent of: devices report no position, so the bytes or the
	// time they ask for stand for it.
	DLNAMarkPlayed bool
	// Language is the language of the names Polyfin generates for Jellyfin
	// apps, one of i18n.Languages.
	Language string
	// PrepareAhead analyzes the version a title would play as soon as its
	// details open, and readies the next episode near the end of the one
	// playing, so that playback starts at once.
	PrepareAhead bool
	// Transcoding lets the server convert (re-encode) video and audio, for
	// the users allowed to have them converted; files still play as they
	// are or repackaged without it.
	Transcoding bool
	// CatalogLimit is how many items one read of a movie or series catalog
	// (any catalog but a live TV one) fetches at most: some catalogs are
	// nearly endless.
	CatalogLimit int
	// ChannelLimit is how many items one read of a live TV catalog fetches
	// at most: its channels, or one day of its guide.
	ChannelLimit int
	// SkipButtons finds the parts of titles apps offer to skip (intro,
	// recap, credits) in the segment databases; off, titles have none.
	SkipButtons bool
	// PublicMetaDBKey is the API key the server asks PublicMetaDB for
	// segments with; empty, PublicMetaDB is not asked. It is a secret:
	// never shown, never logged.
	PublicMetaDBKey string
	// TheIntroDBKey is the API key TheIntroDB is asked with, which raises
	// its daily limit and adds its owner's own submissions; empty,
	// TheIntroDB is asked without one. A secret, like PublicMetaDBKey.
	TheIntroDBKey string
	// SegmentOrder is the order of preference of the segment databases,
	// every one of SegmentSources once, and SegmentSourcesOff those never
	// asked, wherever they are in it. Both come from POLYFIN_SEGMENTS at
	// the first start that read it (see AdoptEnvironment).
	SegmentOrder      []string
	SegmentSourcesOff []string
	// AudioIntros finds the intros and the credits of the episodes of
	// local folders in their audio, where neither their chapters nor the
	// segment databases give them (see package audiosegments). It counts
	// only with SkipButtons on.
	AudioIntros bool
	// SimilarTitles lists titles close to a movie or series from the
	// addons' catalogs; off, titles have none.
	SimilarTitles bool
	// GenerateImages gives the addons' collections without a poster, and
	// the smart lists shown as collections, an image Polyfin draws from
	// the posters of their first titles (see library.GeneratedImage); off,
	// they show none. Libraries set to a generated image keep theirs.
	GenerateImages bool
	// MDBListRatingsKey is the server's MDBList API key, which movies and
	// series have their IMDb rating, critics' score and audience score
	// looked up with (see package ratings); empty, MDBList is never asked
	// about them. Users' own tracking keys are never used for this. A
	// secret, like PublicMetaDBKey.
	MDBListRatingsKey string
	// CriticRatingShows is the score CriticRating carries, one of
	// CriticRatingChoices: the critics' score or the audience score.
	CriticRatingShows string
	// TMDBKey is the server's TMDB API key or read access token, which
	// Edit images searches TMDB's images with (see package tmdb); empty,
	// only the addons' images are listed and TMDB is never asked. A
	// secret, like PublicMetaDBKey.
	TMDBKey string
	// Lyrics looks music tracks up on LRCLIB and serves their lyrics to
	// apps, synced line by line when LRCLIB has them; off, LRCLIB is never
	// asked and tracks have no lyrics.
	Lyrics bool
	// EvenVolume gives apps the gains of songs and albums known, which
	// even the volume from one song to the next (see package loudness);
	// off, apps get none and nothing is measured. MeasureLoudness, one of
	// MeasureLoudnessChoices, tells which songs without a gain have their
	// loudness measured.
	EvenVolume      bool
	MeasureLoudness string
	// PlayedPercent is how far into a title, in percent of its runtime, a
	// reported position marks it played; ResumePercent is how far it must
	// be to keep a resume point. ResumePercent is below PlayedPercent.
	PlayedPercent int
	ResumePercent int
	// VersionListMinutes is how long a title's version and subtitle lists
	// from the addons are kept.
	VersionListMinutes int
	// CatalogRefreshMinutes is how long catalog pages are kept.
	CatalogRefreshMinutes int
	// PersonalAddons lets users add and use their own addons, those their
	// own permission allows to (see PersonalAddonsAllowed).
	PersonalAddons bool
	// ServerImports lets users import their own watch history from another
	// Jellyfin or Emby server, or from an app's account (see appimport),
	// under My account.
	ServerImports bool
	// ShareLinks lets the users allowed to share give a movie or an episode
	// to someone without an account, through a link that expires (see
	// ShareLink). Off, no link is made and every link is refused, without
	// being deleted.
	ShareLinks bool
	// LoginAttempts is how many wrong passwords in a row block an account
	// for LoginBlock; 0 never blocks.
	LoginAttempts int
	// TwoStepAdministrators requires two-step sign-in of administrators:
	// one without it sets it up right after their password, before their
	// session opens, and their password alone signs in to no Jellyfin app.
	TwoStepAdministrators bool
	// InactiveDeviceDays is after how many days unused a Jellyfin app is
	// signed out; 0 never signs it out.
	InactiveDeviceDays int
	// KnownProxies are the addresses and networks of the reverse proxies
	// whose forwarded headers tell the client of their requests (see
	// forwarded.Resolve). Empty by default: no forwarded header is
	// trusted.
	KnownProxies []netip.Prefix
	// KnownProxyNames are the host names of reverse proxies, in lower
	// case, trusted at the addresses they resolve to (see
	// forwarded.Proxies.Lookup). Empty by default.
	KnownProxyNames []string
	// HTTPSMode is how Polyfin serves HTTPS, one of HTTPSModes: off by
	// default, with the certificate and private key files HTTPSCertificate
	// and HTTPSPrivateKey name, or with a certificate it obtains from
	// Let's Encrypt for ACMEHosts. HTTPSPort is the TCP port it listens on,
	// HTTP keeping POLYFIN_LISTEN; HTTPSRedirect sends the requests of
	// clients on the internet that come over HTTP to HTTPS.
	HTTPSMode        string
	HTTPSPort        int
	HTTPSRedirect    bool
	HTTPSCertificate string
	HTTPSPrivateKey  string
	// ACMEHosts are the public host names, in lower case, the certificate
	// of Let's Encrypt is for; ACMEEmail the account's contact, empty for
	// none; ACMETerms whether the administrator agreed to Let's Encrypt's
	// terms; ACMEChallenge how Let's Encrypt checks the names, one of
	// ACMEChallenges.
	ACMEHosts     []string
	ACMEEmail     string
	ACMETerms     bool
	ACMEChallenge string
	// DetailedLog logs at the debug level rather than at the info level
	// (see FollowLogLevel).
	DetailedLog bool
	// AnalysisTimeout bounds each ffprobe analysis, of a file or of a live
	// stream, in seconds: a source that does not answer in time is given up
	// for a while, and PlaybackInfo moves on to the next version.
	AnalysisTimeout int
	// VersionAttempts is how many versions PlaybackInfo analyzes at most
	// when the app did not choose one, files and channels alike.
	VersionAttempts int
	// PreferDirectPlay makes PlaybackInfo, when the app did not choose a
	// version, pick the first version the app plays without conversion (as
	// it is, or repackaged with its tracks copied) rather than the first
	// that plays at all.
	PreferDirectPlay bool
	// VersionOrder are the criteria ordering a title's versions, of
	// VersionCriteria, each grouping them and the next sorting within
	// them; ties keep the addons' order, as does none, the default.
	// VersionCodecOrder is the order of the video codecs for the codec
	// criterion, every one of VersionCodecs once. VersionSourceOrder is
	// the order of the server's sources, addons and folders, for the
	// source criterion: the sources it lists first, in its order, then the
	// others in the addons' order. It may still hold a source removed
	// since, which no version comes from; empty by default.
	VersionOrder       []string
	VersionCodecOrder  []string
	VersionSourceOrder []ID
	// HideVersionsLargerGB hides the versions known to be larger, in
	// gigabytes (10^9 bytes), 0 for no limit; HiddenVersionCodecs, those
	// known to be in these video codecs, of VersionCodecs; and
	// HiddenVersionWords, those whose labels hold one of these words,
	// matched without case. When they would hide every version of a title,
	// none is hidden.
	HideVersionsLargerGB int
	HiddenVersionCodecs  []string
	HiddenVersionWords   []string
	// NameHowVersionsPlay ends the names of a movie's or an episode's
	// versions with how the app asking plays each, as the device profile of
	// its last PlaybackInfo decides it.
	NameHowVersionsPlay bool
	// SyncAddonSubtitles times the addons' subtitles to the version played,
	// against when the version has speech (see playback.Service.FindSpeech).
	SyncAddonSubtitles bool
	// MaxConversions bounds the playbacks whose video the server converts
	// at once, files and live alike, 0 for no limit.
	MaxConversions int
	// MaxConversionHeight is the height converted video is scaled down to
	// at most, keeping its shape, one of ConversionHeights; 0 keeps the
	// original's.
	MaxConversionHeight int
	// ConvertedDownloads streams a title as one file, copied or converted
	// within the user's quality, to an app whose device profile asks for
	// video so (Protocol http), as apps saving titles for offline viewing
	// do, for users who may download. Off, such apps download the version
	// as it is.
	ConvertedDownloads bool
	// SubtitleOCR reads picture subtitles into text with Tesseract, one of
	// SubtitleOCRModes; SubtitleOCRTracks bounds the tracks read at once.
	SubtitleOCR       string
	SubtitleOCRTracks int
	// DefaultAudioLanguage and DefaultSubtitleLanguage are the audio and
	// subtitle languages of the users who chose none, ISO 639-2/B codes
	// as their own preferences take them, empty for none (see
	// jellyfin.trackPreferences). A user's own choice always wins.
	DefaultAudioLanguage    string
	DefaultSubtitleLanguage string
	// The settings below tune the conversions of files and Live TV alike,
	// as Jellyfin's Transcoding page does (recordings copy the stream).
	// EncoderPreset trades encoding speed for quality, one of
	// EncoderPresets. H264Quality and HevcQuality are the encoders' quality
	// factors, CRF in software and CQ or QVBR's on GPUs, the bitrate then
	// being a cap; 0 aims for the bitrate alone.
	EncoderPreset string
	H264Quality   int
	HevcQuality   int
	// AllowHevcEncoding converts video to HEVC for the apps that list it
	// before H.264; off, HEVC is only for apps taking no H.264, as with
	// Jellyfin's option of the same name.
	AllowHevcEncoding bool
	// AdaptiveQuality has the master playlist of converted video offer up
	// to two lower qualities, which apps step down to on a weak
	// connection: one of AdaptiveQualities.
	AdaptiveQuality string
	// HardwareAcceleration chooses the GPU, one of HardwareAccelerations,
	// from POLYFIN_HWACCEL at the first start. HardwareDecodingCodecs are the
	// codecs, of HardwareDecodingCodecs, the GPU decodes; the others are
	// decoded in software.
	HardwareAcceleration   string
	HardwareDecodingCodecs []string
	// ToneMapping converts HDR video to SDR when it is converted, with
	// ToneMappingAlgorithm, one of ToneMappingAlgorithms; off, it keeps
	// HDR's washed-out colors. ToneMappingPeak, in nits, overrides the
	// video's peak, 0 keeping it, and ToneMappingDesat desaturates the
	// highlights: both on the processor only.
	ToneMapping          bool
	ToneMappingAlgorithm string
	ToneMappingPeak      int
	ToneMappingDesat     float64
	// GPUToneMapping tone maps HDR on a GPU that can, NVIDIA's or Intel's;
	// off, the processor tone maps, a way out when a GPU's colors look
	// wrong. ProcessorToneMappingHeight caps video the processor tone maps,
	// one of ToneMappingHeights; 0, Automatic, is the height a timing at
	// startup chose.
	GPUToneMapping             bool
	ProcessorToneMappingHeight int
	// DeinterlaceMethod is one of DeinterlaceMethods; DeinterlaceDoubleRate
	// makes a frame of each field, doubling the frame rate up to 30.
	DeinterlaceMethod     string
	DeinterlaceDoubleRate bool
	// DownmixAlgorithm is how audio is mixed down to stereo, one of
	// DownmixAlgorithms, and DownmixBoost the volume it is multiplied by
	// then. MaxAudioChannels caps converted audio, one of
	// AudioChannelLimits, and AudioBitratePerChannel, in kb/s, sets its
	// bitrate; 0 keeps Jellyfin's (128 kb/s in mono, 384 kb/s in stereo,
	// 640 kb/s in 5.1).
	DownmixAlgorithm       string
	DownmixBoost           float64
	MaxAudioChannels       int
	AudioBitratePerChannel int
	// EncodingThreads is how many threads FFmpeg converts with, 0 letting
	// it choose.
	EncodingThreads int
	// AheadSeconds is how many seconds of picture a remux or conversion of
	// a file makes past the end of the last segment the app asked for
	// before it waits.
	AheadSeconds int
	// KeepConvertedVideo converts the video of a playback apart from its
	// audio, and keeps it when an app changes the audio track: the new
	// play session takes over the video converted, and only the new track
	// is made.
	KeepConvertedVideo bool
	// Trickplay makes scrubbing thumbnails of the versions played, one
	// every TrickplayInterval seconds, TrickplayWidth pixels wide, and
	// ChapterImages an image of each of their chapters. Both read keyframes
	// from the sources, in the background, so both start turned off.
	Trickplay         bool
	TrickplayInterval int
	TrickplayWidth    int
	ChapterImages     bool
	// ThumbnailStorageGB bounds the space the thumbnails and chapter
	// images of every version take: past it, the versions used longest ago
	// lose theirs.
	ThumbnailStorageGB int
	// RecordingPrePadding and RecordingPostPadding are how many seconds
	// new Live TV recordings start before their programme and go on after
	// it, Jellyfin's PrePaddingSeconds and PostPaddingSeconds.
	RecordingPrePadding  int
	RecordingPostPadding int
	// RecordingRetentionDays is after how many days a Live TV recording is
	// deleted, 0 for never.
	RecordingRetentionDays int
	// RecordingCommercials finds the commercial breaks of finished Live TV
	// recordings, which apps may skip while SkipButtons is on (see package
	// commercials).
	RecordingCommercials bool
	// ConvertRecordings converts finished recordings to HEVC, one at a
	// time, from the hour ConvertRecordingsFrom to the hour
	// ConvertRecordingsUntil, in the server's time zone, on the GPU
	// conversions use, or on the processor when there is none and
	// ConvertRecordingsOnProcessor is set.
	ConvertRecordings            bool
	ConvertRecordingsFrom        int
	ConvertRecordingsUntil       int
	ConvertRecordingsOnProcessor bool
	// LiveTvRefreshHours is how many hours after it was fetched an XMLTV
	// guide or an IPTV source's channel list is fetched again.
	LiveTvRefreshHours int
	// LocalScanHours is how many hours after its last scan a local folder
	// is scanned again, 0 for never on a schedule: folders are still
	// scanned at startup, when an administrator asks and, when watched,
	// when they change.
	LocalScanHours int
	// WatchLocalFolders watches the local folders for changes: those
	// mounted in the container, each scanned again a few seconds after its
	// files stop changing, and network shares, scanned again a few seconds
	// after an SMB server tells of a change, or once a listing made every
	// few minutes changed.
	WatchLocalFolders bool
	// WebTheme is the web player's look, one of WebThemes: the LumaaGlaass
	// theme Polyfin bundles, the default, jellyfin-web's own look, or the
	// administrator's CustomCss and CustomJs. ServedCss and ServedJs are
	// what the web player gets for it.
	WebTheme string
	// CustomCss and CustomJs are the administrator's own theme, served
	// while WebTheme is WebThemeCustom and kept while another is chosen:
	// the CSS jellyfin-web applies to every page, unless a user turns it
	// off in their display settings, which is Jellyfin's branding, and a
	// script Polyfin adds to jellyfin-web's page, which Jellyfin has no
	// setting for. Both are empty by default. LoginDisclaimer, Jellyfin's
	// branding too, is the text, Markdown or HTML jellyfin-web shows under
	// its sign-in form, empty by default, which shows nothing.
	CustomCss       string
	CustomJs        string
	LoginDisclaimer string
	// AddVersionsToOpenPage has the web player add the versions the
	// addons find to a title's page while it is open (see
	// /Polyfin/Items/{id}/Versions). When off, it only replaces the
	// placeholder with the versions, and the later ones show when the page
	// is opened again.
	AddVersionsToOpenPage bool
	// TraktClientID and TraktClientSecret identify the app an
	// administrator registered with Trakt, and SimklClientID the one
	// registered with Simkl, through which users connect their accounts
	// to have what they watch sent there. Empty by default, which offers
	// neither service.
	TraktClientID     string
	TraktClientSecret string
	SimklClientID     string
	// LastFMAPIKey and LastFMSecret are the API key and shared secret of
	// the API account an administrator created on Last.fm, through which
	// users connect their Last.fm accounts to have the songs they play
	// sent there. Empty by default, which offers Last.fm to nobody.
	LastFMAPIKey string
	LastFMSecret string
	// AniListClientID and AniListClientSecret identify the app an
	// administrator registered with AniList, MyAnimeListClientID and
	// MyAnimeListClientSecret the one registered with MyAnimeList, through
	// which users connect their lists. Empty by default, which offers
	// neither service.
	AniListClientID         string
	AniListClientSecret     string
	MyAnimeListClientID     string
	MyAnimeListClientSecret string
	// BackupHour is the hour of the server's time zone, 0 to 23, the
	// database is backed up at every day, and BackupsKept how many of the
	// newest backups are kept, while Backups is on.
	BackupHour  int
	BackupsKept int
	// CollectionReadHour is the hour of the server's time zone, 0 to 23,
	// at which every collection of the server's collection libraries is
	// read each day; -1 never.
	CollectionReadHour int
	// CollectionLibraryApps are the apps, by the name they send in the
	// Client field of their authorization, told an addon's collection
	// library holds collections, as Jellyfin's Collections view does;
	// other apps are told it holds mixed content (see
	// Store.CollectionLibraryApp). None by default.
	CollectionLibraryApps []string
	// RemuxDB makes item details describe the versions of a movie or an
	// episode not analyzed yet, their audio, subtitle and video tracks, as
	// RemuxDB found them in the same file; off, RemuxDB is never asked.
	RemuxDB bool
	// RemuxDBURL is the address of the RemuxDB server asked: an http or
	// https URL, without a trailing slash (see ValidRemuxDBURL).
	RemuxDBURL string
	// CacheSizeGB bounds the space the blocks of the sources being played
	// take, in gigabytes (10^9 bytes); lowered, the cache evicts down to it
	// at its next write.
	CacheSizeGB int
	// VAAPIDevice is the render node VAAPI opens, such as
	// /dev/dri/renderD128; empty tries each in turn (see ValidVAAPIDevice).
	VAAPIDevice string
	// Recording turns Live TV recording on. RecordingsFolder is the folder
	// recordings are written to, empty for the default one under the data
	// folder (see RecordingsDir).
	Recording        bool
	RecordingsFolder string
	// Backups turns the daily database backups on. BackupFolder is the
	// folder they are written to, empty for the default one under the data
	// folder (see BackupDir).
	Backups      bool
	BackupFolder string
	// PublicAddress is the address people open Polyfin at, such as
	// https://media.example.org, without a trailing slash: links in
	// notifications start with it. Empty, messages carry no link.
	PublicAddress string
	// SMTPHost and SMTPPort are the SMTP server email notifications go
	// through, empty for none, which offers no email target; SMTPSecurity
	// how the connection is secured (see SMTPSecurities); SMTPUser and
	// SMTPPassword what it is signed in with, empty for no sign-in; and
	// SMTPFrom and SMTPFromName the sender's address and name.
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	// PlaybackHistory tells whether Polyfin keeps a history of the videos
	// played, for the statistics; PlaybackHistoryDays is how many days a
	// playback stays in it.
	PlaybackHistory     bool
	PlaybackHistoryDays int
	// WeeklySummaryDay is the day of the week the weekly summary is sent,
	// 0 for Sunday to 6 for Saturday, as time.Weekday counts them, and
	// WeeklySummaryHour its hour, 0 to 23, both in the server's time zone.
	WeeklySummaryDay  int
	WeeklySummaryHour int
	// ImportEveryMinutes is how often, in minutes, connected accounts are
	// imported again: the app connections (see appimport) and the watch
	// histories of tracking services (see trackers). One of
	// ImportEveryChoices.
	ImportEveryMinutes int
	// Metrics serves Polyfin's metrics, for Prometheus, at /metrics (see
	// package metrics), to requests that send MetricsToken as a bearer
	// token. Polyfin makes the token when metrics are first turned on, and
	// again when asked (see NewMetricsToken). A secret, like
	// PublicMetaDBKey.
	Metrics      bool
	MetricsToken string
	// TorrentStreams plays the torrents addons list, to the users allowed
	// to (see TorrentsAllowed), over one route alone: TorrentInterface, the
	// network interface every torrent socket is bound to (Linux only),
	// else TorrentProxy, a SOCKS5 proxy (host:port) with TorrentProxyUser
	// and TorrentProxyPassword when it asks for them, else
	// TorrentWireGuard, the configuration of a WireGuard tunnel Polyfin
	// runs (see torrent.ParseWireGuardConfig), stored as Marshal writes
	// it. TorrentProxyPassword and TorrentWireGuard, which holds the
	// tunnel's private key, are secrets, like PublicMetaDBKey.
	// TorrentDownloadMbps bounds the download, 0 for no limit;
	// TorrentUploadMbps the upload, 0 for no sharing. TorrentsAtOnce
	// bounds the torrents running at once, server-wide, 0 for no limit
	// (see torrent.ErrAtCap).
	TorrentStreams       bool
	TorrentInterface     string
	TorrentProxy         string
	TorrentProxyUser     string
	TorrentProxyPassword string
	TorrentWireGuard     string
	TorrentDownloadMbps  int
	TorrentUploadMbps    int
	TorrentsAtOnce       int
	// ConversionFolder keeps the segments of conversions and
	// repackagings; ReadingFolder the files being read, the blocks of
	// addons' files and torrents' pieces; CacheFolder the cache, relayed
	// images, guides and picture subtitles being read. Each is empty for
	// the cache in the data folder (see DefaultCacheFolder), and applies
	// at the next start (see config.ChooseFolders).
	ConversionFolder string
	ReadingFolder    string
	CacheFolder      string
}

// TorrentsAllowed reports whether user is offered torrents' files: the
// server plays them and the user may, administrators included.
func (s Settings) TorrentsAllowed(user User) bool {
	return s.TorrentStreams && user.TorrentStreams
}

// InterfaceRoutes tells whether this system can bind sockets to a network
// interface, which only Linux does: elsewhere, torrents use a proxy alone.
const InterfaceRoutes = runtime.GOOS == "linux"

// The bounds of Settings.TorrentDownloadMbps and TorrentUploadMbps, and
// of the fields of the torrents' route.
const (
	MaxTorrentMbps          = 10000
	MaxTorrentsAtOnce       = 100
	MaxTorrentInterfaceName = 15
	MaxTorrentProxyBytes    = 255
)

// ErrInvalidTorrentRoute reports torrent settings without a route while
// torrents play, with more than one, with an interface name or a proxy
// address not valid, or with an interface on a system other than Linux;
// ErrInvalidTorrentLimit a limit outside [0, MaxTorrentMbps], or torrents
// at once outside [0, MaxTorrentsAtOnce]; and
// ErrInvalidWireGuardConfig a WireGuard configuration refused, wrapped
// with the *torrent.WireGuardConfigError that tells why.
var (
	ErrInvalidTorrentRoute    = errors.New("invalid route for torrents")
	ErrInvalidTorrentLimit    = errors.New("invalid limit for torrents")
	ErrInvalidWireGuardConfig = errors.New("invalid WireGuard configuration")
)

// validTorrentRoute checks the torrent settings' route.
func validTorrentRoute(s Settings) bool {
	routes := 0
	for _, route := range []string{s.TorrentInterface, s.TorrentProxy, s.TorrentWireGuard} {
		if route != "" {
			routes++
		}
	}
	if routes > 1 {
		return false
	}
	if s.TorrentInterface != "" && (!InterfaceRoutes || !ValidInterfaceName(s.TorrentInterface)) {
		return false
	}
	if s.TorrentProxy != "" && !validProxyAddress(s.TorrentProxy) {
		return false
	}
	if len(s.TorrentProxyUser) > MaxTorrentProxyBytes || len(s.TorrentProxyPassword) > MaxTorrentProxyBytes ||
		strings.ContainsRune(s.TorrentProxyUser, 0) || strings.ContainsRune(s.TorrentProxyPassword, 0) {
		return false
	}
	return !s.TorrentStreams || routes == 1
}

// ValidInterfaceName reports whether name may name a Linux network
// interface: 1 to 15 bytes, none of them a slash, a colon or a space.
func ValidInterfaceName(name string) bool {
	if name == "" || len(name) > MaxTorrentInterfaceName || name == "." || name == ".." {
		return false
	}
	for i := range len(name) {
		if c := name[i]; c <= ' ' || c > '~' || c == '/' || c == ':' {
			return false
		}
	}
	return true
}

// validProxyAddress reports whether address is a host and a port.
func validProxyAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || len(address) > MaxTorrentProxyBytes {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	_, err = netip.ParseAddr(host)
	return err == nil || validHostName(host)
}

// SMTPAvailable reports whether email notifications can be sent: an SMTP
// server and a sender address are saved.
func (s Settings) SMTPAvailable() bool {
	return s.SMTPHost != "" && s.SMTPFrom != ""
}

// TraktAvailable reports whether users can connect Trakt: its app's ID and
// secret are saved.
func (s Settings) TraktAvailable() bool {
	return s.TraktClientID != "" && s.TraktClientSecret != ""
}

// SimklAvailable reports whether users can connect Simkl: its app's ID is
// saved.
func (s Settings) SimklAvailable() bool {
	return s.SimklClientID != ""
}

// ImportEvery is how long after an import of a connected account the next
// one runs (see ImportEveryMinutes).
func (s Settings) ImportEvery() time.Duration {
	return time.Duration(s.ImportEveryMinutes) * time.Minute
}

// LastFMAvailable reports whether users can connect Last.fm: its API
// account's key and shared secret are saved.
func (s Settings) LastFMAvailable() bool {
	return s.LastFMAPIKey != "" && s.LastFMSecret != ""
}

// AniListAvailable reports whether users can connect AniList: its app's ID
// and secret are saved.
func (s Settings) AniListAvailable() bool {
	return s.AniListClientID != "" && s.AniListClientSecret != ""
}

// MyAnimeListAvailable reports whether users can connect MyAnimeList: its
// app's ID and secret are saved.
func (s Settings) MyAnimeListAvailable() bool {
	return s.MyAnimeListClientID != "" && s.MyAnimeListClientSecret != ""
}

// validTrackingApp reports whether credential may be a tracking service's
// app credential: empty, or up to MaxTrackingAppBytes of printable ASCII
// without spaces.
func validTrackingApp(credential string) bool {
	if len(credential) > MaxTrackingAppBytes {
		return false
	}
	for i := range len(credential) {
		if credential[i] <= ' ' || credential[i] > '~' {
			return false
		}
	}
	return true
}

// validWebText reports whether text fits in max bytes and holds no NUL.
func validWebText(text string, max int) bool {
	return len(text) <= max && !strings.ContainsRune(text, 0)
}

// settingsColumns are the columns of the settings, in the order of
// Settings.fields.
const settingsColumns = "server_name, quick_connect_enabled, legacy_authorization, language, prepare_ahead, transcoding, catalog_limit, channel_limit, " +
	"update_check, local_discovery, local_address, " +
	"dlna, dlna_name, dlna_new_devices_user, dlna_mark_played, " +
	"skip_buttons, publicmetadb_key, theintrodb_key, segment_order, segment_sources_off, audio_intros, similar_titles, generate_images, lyrics, even_volume, measure_loudness, played_percent, resume_percent, version_list_minutes, catalog_refresh_minutes, " +
	"mdblist_ratings_key, critic_rating_shows, tmdb_key, " +
	"personal_addons, server_imports, share_links, login_attempts, inactive_device_days, known_proxies, known_proxy_names, " +
	"two_step_administrators, " +
	"https_mode, https_port, https_redirect, https_certificate, https_private_key, acme_hosts, acme_email, acme_terms, acme_challenge, detailed_log, " +
	"analysis_timeout, version_attempts, prefer_direct_play, " +
	"version_order, version_codec_order, version_source_order, hide_versions_larger_gb, hidden_version_codecs, hidden_version_words, name_how_versions_play, " +
	"sync_addon_subtitles, max_conversions, max_conversion_height, converted_downloads, subtitle_ocr, subtitle_ocr_tracks, " +
	"default_audio_language, default_subtitle_language, " +
	"encoder_preset, h264_quality, hevc_quality, allow_hevc_encoding, hardware_acceleration, hardware_decoding_codecs, " +
	"adaptive_quality, " +
	"tone_mapping, tone_mapping_algorithm, tone_mapping_peak, tone_mapping_desat, gpu_tone_mapping, processor_tone_mapping_height, deinterlace_method, deinterlace_double_rate, " +
	"downmix_algorithm, downmix_boost, max_audio_channels, audio_bitrate_per_channel, encoding_threads, ahead_seconds, keep_converted_video, " +
	"trickplay, trickplay_interval, trickplay_width, chapter_images, thumbnail_storage_gb, " +
	"recording_pre_padding, recording_post_padding, recording_retention_days, " +
	"recording_commercials, " +
	"live_tv_refresh_hours, local_scan_hours, watch_local_folders, " +
	"convert_recordings, convert_recordings_from, convert_recordings_until, convert_recordings_on_processor, " +
	"web_theme, custom_css, custom_js, login_disclaimer, add_versions_to_open_page, trakt_client_id, trakt_client_secret, simkl_client_id, lastfm_api_key, lastfm_secret, " +
	"anilist_client_id, anilist_client_secret, myanimelist_client_id, myanimelist_client_secret, " +
	"backup_hour, backups_kept, collection_read_hour, collection_library_apps, remuxdb, remuxdb_url, " +
	"cache_size_gb, vaapi_device, recording, recordings_folder, backups, backup_folder, public_address, " +
	"smtp_host, smtp_port, smtp_security, smtp_user, smtp_password, smtp_from, smtp_from_name, " +
	"playback_history, playback_history_days, weekly_summary_day, weekly_summary_hour, import_every_minutes, " +
	"metrics, metrics_token, " +
	"torrent_streams, torrent_interface, torrent_proxy, torrent_proxy_user, torrent_proxy_password, torrent_wireguard, torrent_download_mbps, torrent_upload_mbps, torrents_at_once, " +
	"restart_confirmed, update_tool_url, update_tool_token, " +
	"conversion_folder, reading_folder, cache_folder"

// updateSettingsQuery sets every column of settingsColumns, in order.
var updateSettingsQuery = func() string {
	columns := strings.Split(settingsColumns, ", ")
	for i, column := range columns {
		columns[i] = column + " = $" + strconv.Itoa(i+1)
	}
	return "UPDATE settings SET " + strings.Join(columns, ", ")
}()

// fields points to the fields of settings, in the order of
// settingsColumns: what a row scans into, and what an update writes.
func (settings *Settings) fields() []any {
	return []any{&settings.ServerName, &settings.QuickConnectEnabled, &settings.LegacyAuthorization, &settings.Language,
		&settings.PrepareAhead, &settings.Transcoding, &settings.CatalogLimit, &settings.ChannelLimit,
		&settings.UpdateCheck, &settings.LocalDiscovery, &settings.LocalAddress,
		&settings.DLNA, &settings.DLNAName, &settings.DLNANewDevicesUser, &settings.DLNAMarkPlayed,
		&settings.SkipButtons, &settings.PublicMetaDBKey, &settings.TheIntroDBKey, &settings.SegmentOrder, &settings.SegmentSourcesOff, &settings.AudioIntros,
		&settings.SimilarTitles, &settings.GenerateImages, &settings.Lyrics, &settings.EvenVolume, &settings.MeasureLoudness, &settings.PlayedPercent, &settings.ResumePercent,
		&settings.VersionListMinutes, &settings.CatalogRefreshMinutes,
		&settings.MDBListRatingsKey, &settings.CriticRatingShows, &settings.TMDBKey,
		&settings.PersonalAddons, &settings.ServerImports, &settings.ShareLinks, &settings.LoginAttempts, &settings.InactiveDeviceDays, &settings.KnownProxies,
		&settings.KnownProxyNames,
		&settings.TwoStepAdministrators,
		&settings.HTTPSMode, &settings.HTTPSPort, &settings.HTTPSRedirect, &settings.HTTPSCertificate, &settings.HTTPSPrivateKey,
		&settings.ACMEHosts, &settings.ACMEEmail, &settings.ACMETerms, &settings.ACMEChallenge, &settings.DetailedLog,
		&settings.AnalysisTimeout, &settings.VersionAttempts, &settings.PreferDirectPlay,
		&settings.VersionOrder, &settings.VersionCodecOrder, &settings.VersionSourceOrder, &settings.HideVersionsLargerGB, &settings.HiddenVersionCodecs,
		&settings.HiddenVersionWords,
		&settings.NameHowVersionsPlay,
		&settings.SyncAddonSubtitles, &settings.MaxConversions, &settings.MaxConversionHeight, &settings.ConvertedDownloads, &settings.SubtitleOCR, &settings.SubtitleOCRTracks,
		&settings.DefaultAudioLanguage, &settings.DefaultSubtitleLanguage,
		&settings.EncoderPreset, &settings.H264Quality, &settings.HevcQuality, &settings.AllowHevcEncoding, &settings.HardwareAcceleration, &settings.HardwareDecodingCodecs,
		&settings.AdaptiveQuality,
		&settings.ToneMapping, &settings.ToneMappingAlgorithm, &settings.ToneMappingPeak, &settings.ToneMappingDesat, &settings.GPUToneMapping, &settings.ProcessorToneMappingHeight,
		&settings.DeinterlaceMethod, &settings.DeinterlaceDoubleRate,
		&settings.DownmixAlgorithm, &settings.DownmixBoost, &settings.MaxAudioChannels, &settings.AudioBitratePerChannel, &settings.EncodingThreads, &settings.AheadSeconds,
		&settings.KeepConvertedVideo,
		&settings.Trickplay, &settings.TrickplayInterval, &settings.TrickplayWidth, &settings.ChapterImages, &settings.ThumbnailStorageGB,
		&settings.RecordingPrePadding, &settings.RecordingPostPadding, &settings.RecordingRetentionDays,
		&settings.RecordingCommercials,
		&settings.LiveTvRefreshHours, &settings.LocalScanHours,
		&settings.WatchLocalFolders,
		&settings.ConvertRecordings, &settings.ConvertRecordingsFrom, &settings.ConvertRecordingsUntil, &settings.ConvertRecordingsOnProcessor,
		&settings.WebTheme, &settings.CustomCss, &settings.CustomJs, &settings.LoginDisclaimer, &settings.AddVersionsToOpenPage,
		&settings.TraktClientID, &settings.TraktClientSecret, &settings.SimklClientID,
		&settings.LastFMAPIKey, &settings.LastFMSecret,
		&settings.AniListClientID, &settings.AniListClientSecret, &settings.MyAnimeListClientID, &settings.MyAnimeListClientSecret,
		&settings.BackupHour, &settings.BackupsKept, &settings.CollectionReadHour, &settings.CollectionLibraryApps, &settings.RemuxDB, &settings.RemuxDBURL,
		&settings.CacheSizeGB, &settings.VAAPIDevice, &settings.Recording, &settings.RecordingsFolder, &settings.Backups, &settings.BackupFolder,
		&settings.PublicAddress,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPSecurity, &settings.SMTPUser, &settings.SMTPPassword, &settings.SMTPFrom, &settings.SMTPFromName,
		&settings.PlaybackHistory, &settings.PlaybackHistoryDays, &settings.WeeklySummaryDay, &settings.WeeklySummaryHour,
		&settings.ImportEveryMinutes, &settings.Metrics, &settings.MetricsToken,
		&settings.TorrentStreams, &settings.TorrentInterface, &settings.TorrentProxy, &settings.TorrentProxyUser, &settings.TorrentProxyPassword,
		&settings.TorrentWireGuard,
		&settings.TorrentDownloadMbps, &settings.TorrentUploadMbps, &settings.TorrentsAtOnce,
		&settings.RestartConfirmed, &settings.UpdateToolURL, &settings.UpdateToolToken,
		&settings.ConversionFolder, &settings.ReadingFolder, &settings.CacheFolder}
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, "SELECT "+settingsColumns+" FROM settings").Scan(settings.fields()...)
	if err != nil {
		return settings, err
	}
	// A backup of a server on another system may name a GPU method this
	// one does not offer: it reads as auto, which tries this system's.
	if !slices.Contains(HardwareAccelerations, settings.HardwareAcceleration) {
		settings.HardwareAcceleration = HardwareAccelerations[0]
	}
	s.openSecrets(&settings)
	return settings, nil
}

// secrets points to the secrets of settings, by the names the secrets
// package and the admin API give them.
func (settings *Settings) secrets() []struct {
	name  string
	value *string
} {
	return []struct {
		name  string
		value *string
	}{
		{"publicMetaDbKey", &settings.PublicMetaDBKey},
		{"theIntroDbKey", &settings.TheIntroDBKey},
		{"mdblistRatingsKey", &settings.MDBListRatingsKey},
		{"tmdbKey", &settings.TMDBKey},
		{"traktClientSecret", &settings.TraktClientSecret},
		{"lastFmSecret", &settings.LastFMSecret},
		{"aniListClientSecret", &settings.AniListClientSecret},
		{"myAnimeListClientSecret", &settings.MyAnimeListClientSecret},
		{"smtpPassword", &settings.SMTPPassword},
		{"metricsToken", &settings.MetricsToken},
		{"torrentProxyPassword", &settings.TorrentProxyPassword},
		{"torrentWireGuard", &settings.TorrentWireGuard},
		{"updateToolToken", &settings.UpdateToolToken},
	}
}

// openSecrets opens the secrets of settings as read from the database. One
// the key cannot open reads as not set, and is kept as it is stored until
// another value replaces it: a corrected key opens it again.
func (s *Store) openSecrets(settings *Settings) {
	s.secretsMu.Lock()
	defer s.secretsMu.Unlock()
	s.unreadable = map[string]string{}
	for _, secret := range settings.secrets() {
		value, err := s.box.Open(*secret.value)
		if err != nil {
			s.unreadable[secret.name] = *secret.value
		}
		*secret.value = value
	}
}

// sealSecrets returns settings as they are stored: their secrets sealed,
// and those that read as not set because the key could not open them kept
// as they were stored. It is called with secretsMu held.
func (s *Store) sealSecrets(settings Settings) Settings {
	for _, secret := range settings.secrets() {
		if stored, ok := s.unreadable[secret.name]; ok && *secret.value == "" {
			*secret.value = stored
			continue
		}
		*secret.value = s.box.Seal(*secret.value)
	}
	return settings
}

// Settings returns the current settings without querying the database. This
// process is the only writer, so the cached copy is always current.
func (s *Store) Settings() Settings {
	return *s.settings.Load()
}

// KnownProxies returns Settings().KnownProxies without copying the
// settings: every request reads it.
func (s *Store) KnownProxies() []netip.Prefix {
	return s.settings.Load().KnownProxies
}

// KnownProxyNames returns Settings().KnownProxyNames without copying the
// settings.
func (s *Store) KnownProxyNames() []string {
	return s.settings.Load().KnownProxyNames
}

// CollectionLibraryApp reports whether an app, by the name it sends in the
// Client field of its authorization, is one of CollectionLibraryApps,
// whatever its letter case, without copying the settings: every listing
// of libraries asks.
func (s *Store) CollectionLibraryApp(client string) bool {
	return slices.ContainsFunc(s.settings.Load().CollectionLibraryApps, func(app string) bool { return strings.EqualFold(app, client) })
}

// Environment is what the environment variables that were options before
// they were settings give, each zero when its variable is not set: the GPU
// POLYFIN_HWACCEL names, the segment databases POLYFIN_SEGMENTS asks, the
// preferred first (empty, not nil, for none), POLYFIN_CACHE_SIZE in
// gigabytes, DetailedLog for POLYFIN_LOG_LEVEL=debug, the render node
// POLYFIN_VAAPI_DEVICE names, and the folders of POLYFIN_RECORDINGS_DIR
// and POLYFIN_BACKUP_DIR, which turned recording and backups on.
type Environment struct {
	Hardware         string
	Segments         []string
	CacheSizeGB      int
	DetailedLog      bool
	VAAPIDevice      string
	RecordingsFolder string
	BackupFolder     string
}

// AdoptEnvironment copies env into the settings, once. The database lists
// the settings still to be copied, those of a new server or of one that
// left them to the variables before: each takes the value its variable
// gives, and keeps its own when the variable is not set. A recordings or
// backup folder turns recording or backups on with it. Once they are
// copied, the variables are not read again for them, and what an
// administrator chooses stays chosen.
func (s *Store) AdoptEnvironment(ctx context.Context, env Environment) error {
	if env.Hardware != "" && !slices.Contains(HardwareAccelerations, env.Hardware) {
		return ErrInvalidHardwareAcceleration
	}
	if env.Segments != nil && !validSegmentSources(env.Segments) {
		return ErrInvalidSegmentSourcesOff
	}
	if env.CacheSizeGB != 0 && (env.CacheSizeGB < MinCacheSizeGB || env.CacheSizeGB > MaxCacheSizeGB) {
		return ErrInvalidCacheSize
	}
	if !ValidVAAPIDevice(env.VAAPIDevice) {
		return ErrInvalidVAAPIDevice
	}
	recordings, ok := CleanFolder(env.RecordingsFolder)
	if !ok {
		return ErrInvalidRecordingsFolder
	}
	backups, ok := CleanFolder(env.BackupFolder)
	if !ok {
		return ErrInvalidBackupFolder
	}
	// The databases asked, in the order given, then the others, turned
	// off; nil when POLYFIN_SEGMENTS is not set.
	var order, off []string
	if env.Segments != nil {
		order, off = slices.Clone(env.Segments), []string{}
		for _, name := range SegmentSources {
			if !slices.Contains(env.Segments, name) {
				order, off = append(order, name), append(off, name)
			}
		}
	}
	adopted, err := s.db.Exec(ctx, `UPDATE settings SET
		hardware_acceleration = CASE WHEN $1::text <> '' AND 'hardware_acceleration' = ANY(environment_pending)
			THEN $1::text ELSE hardware_acceleration END,
		segment_order = CASE WHEN $2::text[] IS NOT NULL AND 'segment_order' = ANY(environment_pending)
			THEN $2::text[] ELSE segment_order END,
		segment_sources_off = CASE WHEN $3::text[] IS NOT NULL AND 'segment_sources_off' = ANY(environment_pending)
			THEN $3::text[] ELSE segment_sources_off END,
		cache_size_gb = CASE WHEN $4::integer > 0 AND 'cache_size_gb' = ANY(environment_pending)
			THEN $4::integer ELSE cache_size_gb END,
		detailed_log = CASE WHEN $5::boolean AND 'detailed_log' = ANY(environment_pending) THEN true ELSE detailed_log END,
		vaapi_device = CASE WHEN $6::text <> '' AND 'vaapi_device' = ANY(environment_pending)
			THEN $6::text ELSE vaapi_device END,
		recording = CASE WHEN $7::text <> '' AND 'recording' = ANY(environment_pending) THEN true ELSE recording END,
		recordings_folder = CASE WHEN $7::text <> '' AND 'recording' = ANY(environment_pending)
			THEN $7::text ELSE recordings_folder END,
		backups = CASE WHEN $8::text <> '' AND 'backups' = ANY(environment_pending) THEN true ELSE backups END,
		backup_folder = CASE WHEN $8::text <> '' AND 'backups' = ANY(environment_pending)
			THEN $8::text ELSE backup_folder END,
		environment_pending = '{}'
		WHERE environment_pending <> '{}'`,
		env.Hardware, order, off, env.CacheSizeGB, env.DetailedLog, env.VAAPIDevice, recordings, backups)
	if err != nil || adopted.RowsAffected() == 0 {
		return err
	}
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return err
	}
	s.storeSettings(&settings)
	return nil
}

// UpdateSettings replaces the settings.
func (s *Store) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	settings.ServerName = strings.TrimSpace(settings.ServerName)
	if !validServerName(settings.ServerName) {
		return Settings{}, ErrInvalidServerName
	}
	if !i18n.Valid(settings.Language) {
		return Settings{}, ErrInvalidLanguage
	}
	if settings.CatalogLimit < MinCatalogLimit || settings.CatalogLimit > MaxCatalogLimit {
		return Settings{}, ErrInvalidCatalogLimit
	}
	if settings.ChannelLimit < MinChannelLimit || settings.ChannelLimit > MaxChannelLimit {
		return Settings{}, ErrInvalidChannelLimit
	}
	if settings.PlayedPercent < MinPlayedPercent || settings.PlayedPercent > MaxPlayedPercent {
		return Settings{}, ErrInvalidPlayedPercent
	}
	if settings.ResumePercent < MinResumePercent || settings.ResumePercent > MaxResumePercent {
		return Settings{}, ErrInvalidResumePercent
	}
	if settings.ResumePercent >= settings.PlayedPercent {
		return Settings{}, ErrResumeNotBelowPlayed
	}
	if settings.VersionListMinutes < MinVersionListMinutes || settings.VersionListMinutes > MaxVersionListMinutes {
		return Settings{}, ErrInvalidVersionListMinutes
	}
	if settings.CatalogRefreshMinutes < MinCatalogRefreshMinutes || settings.CatalogRefreshMinutes > MaxCatalogRefreshMinutes {
		return Settings{}, ErrInvalidCatalogRefreshMinutes
	}
	if !ValidSegmentKey(settings.PublicMetaDBKey) {
		return Settings{}, ErrInvalidPublicMetaDBKey
	}
	if !ValidSegmentKey(settings.TheIntroDBKey) {
		return Settings{}, ErrInvalidTheIntroDBKey
	}
	if !ValidRatingsKey(settings.MDBListRatingsKey) {
		return Settings{}, ErrInvalidMDBListRatingsKey
	}
	if !ValidTMDBKey(settings.TMDBKey) {
		return Settings{}, ErrInvalidTMDBKey
	}
	if !slices.Contains(CriticRatingChoices, settings.CriticRatingShows) {
		return Settings{}, ErrInvalidCriticRatingShows
	}
	if !slices.Contains(MeasureLoudnessChoices, settings.MeasureLoudness) {
		return Settings{}, ErrInvalidMeasureLoudness
	}
	if !validSegmentOrder(settings.SegmentOrder) {
		return Settings{}, ErrInvalidSegmentOrder
	}
	if !validSegmentSources(settings.SegmentSourcesOff) {
		return Settings{}, ErrInvalidSegmentSourcesOff
	}
	// The databases turned off are kept in the order of SegmentSources,
	// none being an empty list: the column holds no NULL.
	settings.SegmentOrder = slices.Clone(settings.SegmentOrder)
	off := []string{}
	for _, name := range SegmentSources {
		if slices.Contains(settings.SegmentSourcesOff, name) {
			off = append(off, name)
		}
	}
	settings.SegmentSourcesOff = off
	if settings.LoginAttempts != 0 && (settings.LoginAttempts < MinLoginAttempts || settings.LoginAttempts > MaxLoginAttempts) {
		return Settings{}, ErrInvalidLoginAttempts
	}
	if settings.InactiveDeviceDays < 0 || settings.InactiveDeviceDays > MaxInactiveDeviceDays {
		return Settings{}, ErrInvalidInactiveDeviceDays
	}
	proxies, err := canonicalProxies(settings.KnownProxies)
	if err != nil {
		return Settings{}, err
	}
	settings.KnownProxies = proxies
	names, err := canonicalProxyNames(settings.KnownProxyNames)
	if err != nil {
		return Settings{}, err
	}
	if len(proxies)+len(names) > MaxKnownProxies {
		return Settings{}, ErrInvalidKnownProxies
	}
	settings.KnownProxyNames = names
	if err := validHTTPS(&settings); err != nil {
		return Settings{}, err
	}
	if settings.AnalysisTimeout < MinAnalysisTimeout || settings.AnalysisTimeout > MaxAnalysisTimeout {
		return Settings{}, ErrInvalidAnalysisTimeout
	}
	if settings.VersionAttempts < MinVersionAttempts || settings.VersionAttempts > MaxVersionAttempts {
		return Settings{}, ErrInvalidVersionAttempts
	}
	if !validVersionOrder(settings.VersionOrder) {
		return Settings{}, ErrInvalidVersionOrder
	}
	if len(settings.VersionCodecOrder) != len(VersionCodecs) || !validVersionCodecs(settings.VersionCodecOrder) {
		return Settings{}, ErrInvalidVersionCodecOrder
	}
	if !validVersionSources(settings.VersionSourceOrder) {
		return Settings{}, ErrInvalidVersionSourceOrder
	}
	if settings.HideVersionsLargerGB < 0 || settings.HideVersionsLargerGB > MaxHideVersionsLargerGB {
		return Settings{}, ErrInvalidHideVersionsLarger
	}
	if !validVersionCodecs(settings.HiddenVersionCodecs) {
		return Settings{}, ErrInvalidHiddenVersionCodecs
	}
	words, err := canonicalHiddenVersionWords(settings.HiddenVersionWords)
	if err != nil {
		return Settings{}, err
	}
	// The lists are kept apart from the caller's, none being an empty
	// list: the columns hold no NULL. The codecs hidden are kept in the
	// order of VersionCodecs.
	settings.VersionOrder = append([]string{}, settings.VersionOrder...)
	settings.VersionCodecOrder = slices.Clone(settings.VersionCodecOrder)
	settings.VersionSourceOrder = append([]ID{}, settings.VersionSourceOrder...)
	hidden := []string{}
	for _, codec := range VersionCodecs {
		if slices.Contains(settings.HiddenVersionCodecs, codec) {
			hidden = append(hidden, codec)
		}
	}
	settings.HiddenVersionCodecs, settings.HiddenVersionWords = hidden, words
	if settings.MaxConversions < MinMaxConversions || settings.MaxConversions > MaxMaxConversions {
		return Settings{}, ErrInvalidMaxConversions
	}
	if !slices.Contains(ConversionHeights, settings.MaxConversionHeight) {
		return Settings{}, ErrInvalidMaxConversionHeight
	}
	if !slices.Contains(SubtitleOCRModes, settings.SubtitleOCR) {
		return Settings{}, ErrInvalidSubtitleOCR
	}
	if settings.SubtitleOCRTracks < MinSubtitleOCRTracks || settings.SubtitleOCRTracks > MaxSubtitleOCRTracks {
		return Settings{}, ErrInvalidSubtitleOCRTracks
	}
	if !validTrackLanguage(settings.DefaultAudioLanguage) || !validTrackLanguage(settings.DefaultSubtitleLanguage) {
		return Settings{}, ErrInvalidDefaultLanguage
	}
	codecs, err := validConversion(settings)
	if err != nil {
		return Settings{}, err
	}
	settings.HardwareDecodingCodecs = codecs
	if settings.TrickplayInterval < MinTrickplayInterval || settings.TrickplayInterval > MaxTrickplayInterval {
		return Settings{}, ErrInvalidTrickplayInterval
	}
	if !slices.Contains(TrickplayWidths, settings.TrickplayWidth) {
		return Settings{}, ErrInvalidTrickplayWidth
	}
	if settings.ThumbnailStorageGB < MinThumbnailStorageGB || settings.ThumbnailStorageGB > MaxThumbnailStorageGB {
		return Settings{}, ErrInvalidThumbnailStorage
	}
	if settings.RecordingPrePadding < 0 || settings.RecordingPrePadding > MaxRecordingPadding ||
		settings.RecordingPostPadding < 0 || settings.RecordingPostPadding > MaxRecordingPadding {
		return Settings{}, ErrInvalidRecordingPadding
	}
	if settings.RecordingRetentionDays < 0 || settings.RecordingRetentionDays > MaxRecordingRetentionDays {
		return Settings{}, ErrInvalidRecordingRetentionDays
	}
	if settings.ConvertRecordingsFrom < 0 || settings.ConvertRecordingsFrom > 23 || settings.ConvertRecordingsUntil < 0 ||
		settings.ConvertRecordingsUntil > 23 || settings.ConvertRecordingsFrom == settings.ConvertRecordingsUntil {
		return Settings{}, ErrInvalidConvertRecordingsHours
	}
	if settings.LiveTvRefreshHours < MinLiveTvRefreshHours || settings.LiveTvRefreshHours > MaxLiveTvRefreshHours {
		return Settings{}, ErrInvalidLiveTvRefreshHours
	}
	if settings.LocalScanHours < 0 || settings.LocalScanHours > MaxLocalScanHours {
		return Settings{}, ErrInvalidLocalScanHours
	}
	if !slices.Contains(WebThemes, settings.WebTheme) {
		return Settings{}, ErrInvalidWebTheme
	}
	if !validWebText(settings.CustomCss, MaxCustomCodeBytes) {
		return Settings{}, ErrInvalidCustomCss
	}
	if !validWebText(settings.CustomJs, MaxCustomCodeBytes) {
		return Settings{}, ErrInvalidCustomJs
	}
	if !validWebText(settings.LoginDisclaimer, MaxLoginDisclaimerBytes) {
		return Settings{}, ErrInvalidLoginDisclaimer
	}
	if !validTrackingApp(settings.TraktClientID) || !validTrackingApp(settings.TraktClientSecret) {
		return Settings{}, ErrInvalidTraktApp
	}
	if !validTrackingApp(settings.SimklClientID) {
		return Settings{}, ErrInvalidSimklApp
	}
	if !validTrackingApp(settings.LastFMAPIKey) || !validTrackingApp(settings.LastFMSecret) {
		return Settings{}, ErrInvalidLastFMApp
	}
	if !validTrackingApp(settings.AniListClientID) || !validTrackingApp(settings.AniListClientSecret) {
		return Settings{}, ErrInvalidAniListApp
	}
	if !validTrackingApp(settings.MyAnimeListClientID) || !validTrackingApp(settings.MyAnimeListClientSecret) {
		return Settings{}, ErrInvalidMyAnimeListApp
	}
	if settings.BackupHour < 0 || settings.BackupHour > 23 {
		return Settings{}, ErrInvalidBackupHour
	}
	if settings.BackupsKept < MinBackupsKept || settings.BackupsKept > MaxBackupsKept {
		return Settings{}, ErrInvalidBackupsKept
	}
	if settings.CollectionReadHour < -1 || settings.CollectionReadHour > 23 {
		return Settings{}, ErrInvalidCollectionReadHour
	}
	apps, err := canonicalCollectionLibraryApps(settings.CollectionLibraryApps)
	if err != nil {
		return Settings{}, err
	}
	settings.CollectionLibraryApps = apps
	if settings.CacheSizeGB < MinCacheSizeGB || settings.CacheSizeGB > MaxCacheSizeGB {
		return Settings{}, ErrInvalidCacheSize
	}
	if !ValidVAAPIDevice(settings.VAAPIDevice) {
		return Settings{}, ErrInvalidVAAPIDevice
	}
	var ok bool
	if settings.RecordingsFolder, ok = CleanFolder(settings.RecordingsFolder); !ok {
		return Settings{}, ErrInvalidRecordingsFolder
	}
	if settings.BackupFolder, ok = CleanFolder(settings.BackupFolder); !ok {
		return Settings{}, ErrInvalidBackupFolder
	}
	if settings.ConversionFolder, ok = CleanFolder(settings.ConversionFolder); !ok {
		return Settings{}, ErrInvalidConversionFolder
	}
	if settings.ReadingFolder, ok = CleanFolder(settings.ReadingFolder); !ok {
		return Settings{}, ErrInvalidReadingFolder
	}
	if settings.CacheFolder, ok = CleanFolder(settings.CacheFolder); !ok {
		return Settings{}, ErrInvalidCacheFolder
	}
	// The address is kept without surrounding spaces or trailing slashes:
	// the paths asked are added after it.
	settings.RemuxDBURL = strings.TrimRight(strings.TrimSpace(settings.RemuxDBURL), "/")
	if !ValidRemuxDBURL(settings.RemuxDBURL) {
		return Settings{}, ErrInvalidRemuxDBURL
	}
	settings.PublicAddress = strings.TrimRight(strings.TrimSpace(settings.PublicAddress), "/")
	if !ValidPublicAddress(settings.PublicAddress) {
		return Settings{}, ErrInvalidPublicAddress
	}
	settings.LocalAddress = strings.TrimRight(strings.TrimSpace(settings.LocalAddress), "/")
	if !ValidLocalAddress(settings.LocalAddress) {
		return Settings{}, ErrInvalidLocalAddress
	}
	settings.DLNAName = strings.TrimSpace(settings.DLNAName)
	if !validWebText(settings.DLNAName, MaxDLNANameBytes) {
		return Settings{}, ErrInvalidDLNAName
	}
	settings.SMTPHost, settings.SMTPFrom = strings.TrimSpace(settings.SMTPHost), strings.TrimSpace(settings.SMTPFrom)
	settings.SMTPFromName = strings.TrimSpace(settings.SMTPFromName)
	switch {
	case len(settings.SMTPHost) > MaxSMTPHostBytes || !validTrackingApp(settings.SMTPHost):
		return Settings{}, ErrInvalidSMTPHost
	case settings.SMTPPort < 1 || settings.SMTPPort > 65535:
		return Settings{}, ErrInvalidSMTPPort
	case !slices.Contains(SMTPSecurities, settings.SMTPSecurity):
		return Settings{}, ErrInvalidSMTPSecurity
	case !validLine(settings.SMTPUser, MaxSMTPUserBytes) || !validLine(settings.SMTPPassword, MaxSMTPPasswordBytes):
		return Settings{}, ErrInvalidSMTPAccount
	case settings.SMTPFrom != "" && !ValidEmail(settings.SMTPFrom), !validLine(settings.SMTPFromName, MaxSMTPFromNameBytes):
		return Settings{}, ErrInvalidSMTPSender
	}
	if settings.PlaybackHistoryDays < 1 || settings.PlaybackHistoryDays > MaxPlaybackHistoryDays {
		return Settings{}, ErrInvalidPlaybackHistoryDays
	}
	if settings.WeeklySummaryDay < 0 || settings.WeeklySummaryDay > 6 {
		return Settings{}, ErrInvalidWeeklySummaryDay
	}
	if settings.WeeklySummaryHour < 0 || settings.WeeklySummaryHour > 23 {
		return Settings{}, ErrInvalidWeeklySummaryHour
	}
	if !slices.Contains(ImportEveryChoices, settings.ImportEveryMinutes) {
		return Settings{}, ErrInvalidImportEveryMinutes
	}
	if !validLine(settings.MetricsToken, MaxMetricsTokenBytes) {
		return Settings{}, ErrInvalidMetricsToken
	}
	settings.UpdateToolURL = strings.TrimSpace(settings.UpdateToolURL)
	if !ValidUpdateToolURL(settings.UpdateToolURL) {
		return Settings{}, ErrInvalidUpdateToolURL
	}
	if !validLine(settings.UpdateToolToken, MaxUpdateToolTokenBytes) || strings.ContainsFunc(settings.UpdateToolToken, unicode.IsSpace) {
		return Settings{}, ErrInvalidUpdateToolToken
	}
	if settings.LoginAttempts == 0 {
		// Without a limit, no account stays blocked, nor keeps counting.
		unblocked, err := s.db.Exec(ctx, "UPDATE users SET invalid_login_attempts = 0, blocked_until = NULL "+
			"WHERE invalid_login_attempts <> 0 OR blocked_until IS NOT NULL")
		if err != nil {
			return Settings{}, err
		}
		if unblocked.RowsAffected() > 0 {
			s.forgetSignIns()
		}
	}
	settings.TorrentInterface = strings.TrimSpace(settings.TorrentInterface)
	settings.TorrentProxy = strings.TrimSpace(settings.TorrentProxy)
	if strings.TrimSpace(settings.TorrentWireGuard) == "" {
		settings.TorrentWireGuard = ""
	} else {
		config, err := torrent.ParseWireGuardConfig(settings.TorrentWireGuard)
		if err != nil {
			return Settings{}, fmt.Errorf("%w: %w", ErrInvalidWireGuardConfig, err)
		}
		settings.TorrentWireGuard = config.Marshal()
	}
	if !validTorrentRoute(settings) {
		return Settings{}, ErrInvalidTorrentRoute
	}
	if settings.TorrentDownloadMbps < 0 || settings.TorrentDownloadMbps > MaxTorrentMbps ||
		settings.TorrentUploadMbps < 0 || settings.TorrentUploadMbps > MaxTorrentMbps ||
		settings.TorrentsAtOnce < 0 || settings.TorrentsAtOnce > MaxTorrentsAtOnce {
		return Settings{}, ErrInvalidTorrentLimit
	}
	// Metrics turned on with no token, the first time or when the key
	// cannot read the one stored, get one.
	if settings.Metrics && settings.MetricsToken == "" {
		settings.MetricsToken, _ = newToken()
	}
	s.secretsMu.Lock()
	defer s.secretsMu.Unlock()
	// Another route replaces a WireGuard configuration the key cannot
	// open: it is not kept as it was stored.
	if settings.TorrentInterface != "" || settings.TorrentProxy != "" {
		delete(s.unreadable, "torrentWireGuard")
	}
	stored := s.sealSecrets(settings)
	_, err = s.db.Exec(ctx, updateSettingsQuery, stored.fields()...)
	if err != nil {
		return Settings{}, err
	}
	for _, secret := range settings.secrets() {
		if *secret.value != "" {
			delete(s.unreadable, secret.name)
		}
	}
	s.storeSettings(&settings)
	return settings, nil
}

// NewMetricsToken makes a new metrics token, which ends the former one,
// and returns the settings with it.
func (s *Store) NewMetricsToken(ctx context.Context) (Settings, error) {
	settings := s.Settings()
	settings.MetricsToken, _ = newToken()
	return s.UpdateSettings(ctx, settings)
}

func validServerName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// validConversion checks the settings tuning conversions, and returns the
// codecs decoded on the GPU in the order of HardwareDecodingCodecs.
func validConversion(settings Settings) ([]string, error) {
	validQuality := func(quality int) bool {
		return quality == 0 || quality >= MinVideoQuality && quality <= MaxVideoQuality
	}
	switch {
	case !slices.Contains(EncoderPresets, settings.EncoderPreset):
		return nil, ErrInvalidEncoderPreset
	case !validQuality(settings.H264Quality) || !validQuality(settings.HevcQuality):
		return nil, ErrInvalidVideoQuality
	case !slices.Contains(HardwareAccelerations, settings.HardwareAcceleration):
		return nil, ErrInvalidHardwareAcceleration
	case !slices.Contains(AdaptiveQualities, settings.AdaptiveQuality):
		return nil, ErrInvalidAdaptiveQuality
	case !slices.Contains(ToneMappingAlgorithms, settings.ToneMappingAlgorithm):
		return nil, ErrInvalidToneMappingAlgorithm
	case settings.ToneMappingPeak != 0 && (settings.ToneMappingPeak < MinToneMappingPeak || settings.ToneMappingPeak > MaxToneMappingPeak):
		return nil, ErrInvalidToneMappingPeak
	case !(settings.ToneMappingDesat >= 0 && settings.ToneMappingDesat <= MaxToneMappingDesat):
		return nil, ErrInvalidToneMappingDesat
	case !slices.Contains(ToneMappingHeights, settings.ProcessorToneMappingHeight):
		return nil, ErrInvalidToneMappingHeight
	case !slices.Contains(DeinterlaceMethods, settings.DeinterlaceMethod):
		return nil, ErrInvalidDeinterlaceMethod
	case !slices.Contains(DownmixAlgorithms, settings.DownmixAlgorithm):
		return nil, ErrInvalidDownmixAlgorithm
	case !(settings.DownmixBoost >= MinDownmixBoost && settings.DownmixBoost <= MaxDownmixBoost):
		return nil, ErrInvalidDownmixBoost
	case !slices.Contains(AudioChannelLimits, settings.MaxAudioChannels):
		return nil, ErrInvalidMaxAudioChannels
	case settings.AudioBitratePerChannel != 0 &&
		(settings.AudioBitratePerChannel < MinAudioBitratePerChannel || settings.AudioBitratePerChannel > MaxAudioBitratePerChannel):
		return nil, ErrInvalidAudioBitrate
	case settings.EncodingThreads < 0 || settings.EncodingThreads > MaxEncodingThreads:
		return nil, ErrInvalidEncodingThreads
	case settings.AheadSeconds < MinAheadSeconds || settings.AheadSeconds > MaxAheadSeconds:
		return nil, ErrInvalidAheadSeconds
	}
	codecs := []string{}
	for _, codec := range HardwareDecodingCodecs {
		if slices.Contains(settings.HardwareDecodingCodecs, codec) {
			codecs = append(codecs, codec)
		}
	}
	// A codec unknown, or listed twice, leaves the counts apart.
	if len(codecs) != len(settings.HardwareDecodingCodecs) {
		return nil, ErrInvalidHardwareDecodingCodecs
	}
	return codecs, nil
}
