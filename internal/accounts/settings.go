package accounts

import (
	"context"
	"errors"
	"slices"
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
// those of a Jellyfin server (MaxResumePct and MinResumePct).
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
	DefaultCatalogRefreshMinutes = 10
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
// VersionAttempts and MaxConversions. The defaults are what Polyfin did
// before they were settings.
const (
	MinAnalysisTimeout     = 5
	MaxAnalysisTimeout     = 120
	DefaultAnalysisTimeout = 45
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
	// parameter and the Emby scheme, which Jellyfin 12.1 refuses by default.
	LegacyAuthorization bool
	// Language is the language of the names Polyfin generates for Jellyfin
	// apps, one of Languages.
	Language string
	// Chapters sends apps the chapters of the versions Polyfin analyzed.
	// They are read with the analysis every first play needs, so they cost
	// nothing: turning them off only hides them.
	Chapters bool
	// PrepareAhead analyzes the version a title would play as soon as its
	// details open, and readies the next episode near the end of the one
	// playing, so that playback starts at once.
	PrepareAhead bool
	// Transcoding lets the server convert (re-encode) video and audio, for
	// the users allowed to have them converted; files still play as they
	// are or repackaged without it.
	Transcoding bool
	// Downloads lets the users allowed to download titles do so.
	Downloads bool
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
	// SimilarTitles lists titles close to a movie or series from the
	// addons' catalogs; off, titles have none.
	SimilarTitles bool
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
	// LoginAttempts is how many wrong passwords in a row block an account
	// for LoginBlock; 0 never blocks.
	LoginAttempts int
	// InactiveDeviceDays is after how many days unused a Jellyfin app is
	// signed out; 0 never signs it out.
	InactiveDeviceDays int
	// DetailedLog logs at the debug level, whatever the configured level
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
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, "SELECT server_name, quick_connect_enabled, legacy_authorization, language, chapters, prepare_ahead, transcoding, downloads, catalog_limit, channel_limit, skip_buttons, similar_titles, played_percent, resume_percent, version_list_minutes, catalog_refresh_minutes, personal_addons, login_attempts, inactive_device_days, detailed_log, analysis_timeout, version_attempts, prefer_direct_play, max_conversions, max_conversion_height FROM settings").
		Scan(&settings.ServerName, &settings.QuickConnectEnabled, &settings.LegacyAuthorization, &settings.Language,
			&settings.Chapters, &settings.PrepareAhead, &settings.Transcoding, &settings.Downloads, &settings.CatalogLimit, &settings.ChannelLimit,
			&settings.SkipButtons, &settings.SimilarTitles, &settings.PlayedPercent, &settings.ResumePercent, &settings.VersionListMinutes, &settings.CatalogRefreshMinutes,
			&settings.PersonalAddons, &settings.LoginAttempts, &settings.InactiveDeviceDays, &settings.DetailedLog,
			&settings.AnalysisTimeout, &settings.VersionAttempts, &settings.PreferDirectPlay, &settings.MaxConversions, &settings.MaxConversionHeight)
	return settings, err
}

// Settings returns the current settings without querying the database. This
// process is the only writer, so the cached copy is always current.
func (s *Store) Settings() Settings {
	return *s.settings.Load()
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
	if settings.LoginAttempts == 0 {
		// Without a limit, no account stays blocked, nor keeps counting.
		if _, err := s.db.Exec(ctx, "UPDATE users SET invalid_login_attempts = 0, blocked_until = NULL "+
			"WHERE invalid_login_attempts <> 0 OR blocked_until IS NOT NULL"); err != nil {
			return Settings{}, err
		}
	}
	_, err := s.db.Exec(ctx,
		"UPDATE settings SET server_name = $1, quick_connect_enabled = $2, legacy_authorization = $3, language = $4, chapters = $5, prepare_ahead = $6, transcoding = $7, downloads = $8, catalog_limit = $9, channel_limit = $10, skip_buttons = $11, similar_titles = $12, played_percent = $13, resume_percent = $14, version_list_minutes = $15, catalog_refresh_minutes = $16, personal_addons = $17, login_attempts = $18, inactive_device_days = $19, detailed_log = $20, analysis_timeout = $21, version_attempts = $22, prefer_direct_play = $23, max_conversions = $24, max_conversion_height = $25",
		settings.ServerName, settings.QuickConnectEnabled, settings.LegacyAuthorization, settings.Language,
		settings.Chapters, settings.PrepareAhead, settings.Transcoding, settings.Downloads, settings.CatalogLimit, settings.ChannelLimit,
		settings.SkipButtons, settings.SimilarTitles, settings.PlayedPercent, settings.ResumePercent, settings.VersionListMinutes, settings.CatalogRefreshMinutes,
		settings.PersonalAddons, settings.LoginAttempts, settings.InactiveDeviceDays, settings.DetailedLog,
		settings.AnalysisTimeout, settings.VersionAttempts, settings.PreferDirectPlay, settings.MaxConversions, settings.MaxConversionHeight)
	if err != nil {
		return Settings{}, err
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
