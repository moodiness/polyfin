package accounts

import (
	"context"
	"errors"
	"net/mail"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	ErrInvalidDeinterlaceMethod      = errors.New("invalid deinterlace method")
	ErrInvalidDownmixAlgorithm       = errors.New("invalid downmix algorithm")
	ErrInvalidDownmixBoost           = errors.New("invalid downmix boost")
	ErrInvalidMaxAudioChannels       = errors.New("invalid maximum of audio channels")
	ErrInvalidAudioBitrate           = errors.New("invalid audio bitrate")
	ErrInvalidEncodingThreads        = errors.New("invalid encoding threads")
	ErrInvalidAheadSeconds           = errors.New("invalid seconds ahead")
)

// The choices of the settings tuning conversions, named as Jellyfin's
// encoding options name them where it has them; the first of each is the
// default.
var (
	// EncoderPresets are the values of Settings.EncoderPreset: auto lets
	// Polyfin choose each encoder's, then from the slowest to the fastest.
	EncoderPresets = []string{"auto", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"}
	// HardwareAccelerations are the values of Settings.HardwareAcceleration,
	// those POLYFIN_HWACCEL takes: auto tries NVIDIA, then VAAPI.
	HardwareAccelerations = []string{"auto", "nvenc", "vaapi", "none"}
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

// ErrInvalidPlaybackHistoryDays reports a PlaybackHistoryDays outside [1,
// MaxPlaybackHistoryDays].
var ErrInvalidPlaybackHistoryDays = errors.New("invalid playback history days")

// The bounds and default of Settings.PlaybackHistoryDays: a year by
// default, ten at most.
const (
	MaxPlaybackHistoryDays     = 3650
	DefaultPlaybackHistoryDays = 365
)

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

// ErrInvalidCustomCss, ErrInvalidCustomJs and ErrInvalidLoginDisclaimer
// report a Settings.CustomCss, CustomJs or LoginDisclaimer longer than its
// maximum, or holding a NUL character, which no web page has.
var (
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
var SegmentSources = []string{"theintrodb", "introdb", "publicmetadb"}

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

// ErrInvalidTraktApp, ErrInvalidSimklApp and ErrInvalidLastFMApp report a
// Settings.TraktClientID, TraktClientSecret, SimklClientID, LastFMAPIKey or
// LastFMSecret longer than MaxTrackingAppBytes, or holding anything but
// printable ASCII without spaces, which the services' credentials are made
// of.
var (
	ErrInvalidTraktApp  = errors.New("invalid Trakt app")
	ErrInvalidSimklApp  = errors.New("invalid Simkl app")
	ErrInvalidLastFMApp = errors.New("invalid Last.fm API account")
)

// MaxTrackingAppBytes is the longest Trakt, Simkl or Last.fm credential, in
// bytes.
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

// Languages are the server languages, as ISO 639-1 codes. The first is the
// default.
var Languages = []string{"en", "fr"}

// ValidLanguage reports whether language is one of Languages.
func ValidLanguage(language string) bool {
	return slices.Contains(Languages, language)
}

// Settings are the server-wide options set from the admin interface.
type Settings struct {
	// ServerName is the name Jellyfin apps show for this server.
	ServerName          string
	QuickConnectEnabled bool
	// LegacyAuthorization accepts the X-Emby-* headers, the api_key query
	// parameter and the Emby scheme, which Jellyfin 12.2 refuses by default.
	LegacyAuthorization bool
	// Language is the language of the names Polyfin generates for Jellyfin
	// apps, one of Languages.
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
	// SimilarTitles lists titles close to a movie or series from the
	// addons' catalogs; off, titles have none.
	SimilarTitles bool
	// Lyrics looks music tracks up on LRCLIB and serves their lyrics to
	// apps, synced line by line when LRCLIB has them; off, LRCLIB is never
	// asked and tracks have no lyrics.
	Lyrics bool
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
	// Jellyfin or Emby server, under My account.
	ServerImports bool
	// LoginAttempts is how many wrong passwords in a row block an account
	// for LoginBlock; 0 never blocks.
	LoginAttempts int
	// InactiveDeviceDays is after how many days unused a Jellyfin app is
	// signed out; 0 never signs it out.
	InactiveDeviceDays int
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
	// MaxConversions bounds the playbacks whose video the server converts
	// at once, files and live alike, 0 for no limit.
	MaxConversions int
	// MaxConversionHeight is the height converted video is scaled down to
	// at most, keeping its shape, one of ConversionHeights; 0 keeps the
	// original's.
	MaxConversionHeight int
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
	// DeinterlaceMethod is one of DeinterlaceMethods; DeinterlaceDoubleRate
	// makes a frame of each field, doubling the frame rate up to 30.
	DeinterlaceMethod     string
	DeinterlaceDoubleRate bool
	// DownmixAlgorithm is how audio is mixed down to stereo, one of
	// DownmixAlgorithms, and DownmixBoost the volume it is multiplied by
	// then. MaxAudioChannels caps converted audio, one of
	// AudioChannelLimits, and AudioBitratePerChannel, in kb/s, sets its
	// bitrate; 0 keeps Polyfin's (192 kb/s in stereo, 64 kb/s a channel
	// above).
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
	// LiveTvRefreshHours is how many hours after it was fetched an XMLTV
	// guide or an IPTV source's channel list is fetched again.
	LiveTvRefreshHours int
	// LocalScanHours is how many hours after its last scan a local folder
	// is scanned again, 0 for never on a schedule: folders are still
	// scanned at startup and when an administrator asks.
	LocalScanHours int
	// CustomCss and LoginDisclaimer are Jellyfin's branding: the CSS
	// jellyfin-web applies to every page, unless a user turns it off in
	// their display settings, and the text, Markdown or HTML it shows
	// under its sign-in form. CustomJs is a script Polyfin adds to
	// jellyfin-web's page, which Jellyfin has no setting for. CustomCss and
	// CustomJs load the LumaaGlaass theme by default (see DefaultCustomCss);
	// LoginDisclaimer is empty by default, which shows nothing.
	CustomCss       string
	CustomJs        string
	LoginDisclaimer string
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
	// BackupHour is the hour of the server's time zone, 0 to 23, the
	// database is backed up at every day, and BackupsKept how many of the
	// newest backups are kept, while Backups is on.
	BackupHour  int
	BackupsKept int
	// CollectionReadHour is the hour of the server's time zone, 0 to 23,
	// at which every collection of the server's collection libraries is
	// read each day; -1 never.
	CollectionReadHour int
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

// LastFMAvailable reports whether users can connect Last.fm: its API
// account's key and shared secret are saved.
func (s Settings) LastFMAvailable() bool {
	return s.LastFMAPIKey != "" && s.LastFMSecret != ""
}

// validTrackingApp reports whether credential may be a Trakt, Simkl or
// Last.fm credential: empty, or up to MaxTrackingAppBytes of printable
// ASCII without spaces.
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
	"skip_buttons, publicmetadb_key, theintrodb_key, segment_order, segment_sources_off, similar_titles, lyrics, played_percent, resume_percent, version_list_minutes, catalog_refresh_minutes, " +
	"personal_addons, server_imports, login_attempts, inactive_device_days, detailed_log, " +
	"analysis_timeout, version_attempts, prefer_direct_play, max_conversions, max_conversion_height, " +
	"encoder_preset, h264_quality, hevc_quality, allow_hevc_encoding, hardware_acceleration, hardware_decoding_codecs, " +
	"tone_mapping, tone_mapping_algorithm, tone_mapping_peak, tone_mapping_desat, deinterlace_method, deinterlace_double_rate, " +
	"downmix_algorithm, downmix_boost, max_audio_channels, audio_bitrate_per_channel, encoding_threads, ahead_seconds, " +
	"trickplay, trickplay_interval, trickplay_width, chapter_images, thumbnail_storage_gb, " +
	"recording_pre_padding, recording_post_padding, recording_retention_days, live_tv_refresh_hours, local_scan_hours, " +
	"custom_css, custom_js, login_disclaimer, trakt_client_id, trakt_client_secret, simkl_client_id, lastfm_api_key, lastfm_secret, " +
	"backup_hour, backups_kept, collection_read_hour, remuxdb, remuxdb_url, " +
	"cache_size_gb, vaapi_device, recording, recordings_folder, backups, backup_folder, public_address, " +
	"smtp_host, smtp_port, smtp_security, smtp_user, smtp_password, smtp_from, smtp_from_name, " +
	"playback_history, playback_history_days"

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
		&settings.SkipButtons, &settings.PublicMetaDBKey, &settings.TheIntroDBKey, &settings.SegmentOrder, &settings.SegmentSourcesOff,
		&settings.SimilarTitles, &settings.Lyrics, &settings.PlayedPercent, &settings.ResumePercent, &settings.VersionListMinutes, &settings.CatalogRefreshMinutes,
		&settings.PersonalAddons, &settings.ServerImports, &settings.LoginAttempts, &settings.InactiveDeviceDays, &settings.DetailedLog,
		&settings.AnalysisTimeout, &settings.VersionAttempts, &settings.PreferDirectPlay, &settings.MaxConversions, &settings.MaxConversionHeight,
		&settings.EncoderPreset, &settings.H264Quality, &settings.HevcQuality, &settings.AllowHevcEncoding, &settings.HardwareAcceleration, &settings.HardwareDecodingCodecs,
		&settings.ToneMapping, &settings.ToneMappingAlgorithm, &settings.ToneMappingPeak, &settings.ToneMappingDesat, &settings.DeinterlaceMethod, &settings.DeinterlaceDoubleRate,
		&settings.DownmixAlgorithm, &settings.DownmixBoost, &settings.MaxAudioChannels, &settings.AudioBitratePerChannel, &settings.EncodingThreads, &settings.AheadSeconds,
		&settings.Trickplay, &settings.TrickplayInterval, &settings.TrickplayWidth, &settings.ChapterImages, &settings.ThumbnailStorageGB,
		&settings.RecordingPrePadding, &settings.RecordingPostPadding, &settings.RecordingRetentionDays, &settings.LiveTvRefreshHours, &settings.LocalScanHours,
		&settings.CustomCss, &settings.CustomJs, &settings.LoginDisclaimer, &settings.TraktClientID, &settings.TraktClientSecret, &settings.SimklClientID,
		&settings.LastFMAPIKey, &settings.LastFMSecret,
		&settings.BackupHour, &settings.BackupsKept, &settings.CollectionReadHour, &settings.RemuxDB, &settings.RemuxDBURL,
		&settings.CacheSizeGB, &settings.VAAPIDevice, &settings.Recording, &settings.RecordingsFolder, &settings.Backups, &settings.BackupFolder,
		&settings.PublicAddress,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPSecurity, &settings.SMTPUser, &settings.SMTPPassword, &settings.SMTPFrom, &settings.SMTPFromName,
		&settings.PlaybackHistory, &settings.PlaybackHistoryDays}
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, "SELECT "+settingsColumns+" FROM settings").Scan(settings.fields()...)
	if err != nil {
		return settings, err
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
		{"traktClientSecret", &settings.TraktClientSecret},
		{"lastFmSecret", &settings.LastFMSecret},
		{"smtpPassword", &settings.SMTPPassword},
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
	s.settings.Store(&settings)
	s.applyLogLevel()
	return nil
}

// UpdateSettings replaces the settings.
func (s *Store) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	settings.ServerName = strings.TrimSpace(settings.ServerName)
	if !validServerName(settings.ServerName) {
		return Settings{}, ErrInvalidServerName
	}
	if !ValidLanguage(settings.Language) {
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
	if settings.AnalysisTimeout < MinAnalysisTimeout || settings.AnalysisTimeout > MaxAnalysisTimeout {
		return Settings{}, ErrInvalidAnalysisTimeout
	}
	if settings.VersionAttempts < MinVersionAttempts || settings.VersionAttempts > MaxVersionAttempts {
		return Settings{}, ErrInvalidVersionAttempts
	}
	if settings.MaxConversions < MinMaxConversions || settings.MaxConversions > MaxMaxConversions {
		return Settings{}, ErrInvalidMaxConversions
	}
	if !slices.Contains(ConversionHeights, settings.MaxConversionHeight) {
		return Settings{}, ErrInvalidMaxConversionHeight
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
	if settings.LiveTvRefreshHours < MinLiveTvRefreshHours || settings.LiveTvRefreshHours > MaxLiveTvRefreshHours {
		return Settings{}, ErrInvalidLiveTvRefreshHours
	}
	if settings.LocalScanHours < 0 || settings.LocalScanHours > MaxLocalScanHours {
		return Settings{}, ErrInvalidLocalScanHours
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
	if settings.BackupHour < 0 || settings.BackupHour > 23 {
		return Settings{}, ErrInvalidBackupHour
	}
	if settings.BackupsKept < MinBackupsKept || settings.BackupsKept > MaxBackupsKept {
		return Settings{}, ErrInvalidBackupsKept
	}
	if settings.CollectionReadHour < -1 || settings.CollectionReadHour > 23 {
		return Settings{}, ErrInvalidCollectionReadHour
	}
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
	s.secretsMu.Lock()
	defer s.secretsMu.Unlock()
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
	s.settings.Store(&settings)
	s.applyLogLevel()
	return settings, nil
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
	case !slices.Contains(ToneMappingAlgorithms, settings.ToneMappingAlgorithm):
		return nil, ErrInvalidToneMappingAlgorithm
	case settings.ToneMappingPeak != 0 && (settings.ToneMappingPeak < MinToneMappingPeak || settings.ToneMappingPeak > MaxToneMappingPeak):
		return nil, ErrInvalidToneMappingPeak
	case !(settings.ToneMappingDesat >= 0 && settings.ToneMappingDesat <= MaxToneMappingDesat):
		return nil, ErrInvalidToneMappingDesat
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
