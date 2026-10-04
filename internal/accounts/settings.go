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
	// CatalogLimit is how many items one read of a film or series catalog
	// (any catalog but a live TV one) fetches at most: some catalogs are
	// nearly endless.
	CatalogLimit int
	// ChannelLimit is how many items one read of a live TV catalog fetches
	// at most: its channels, or one day of its guide.
	ChannelLimit int
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, "SELECT server_name, quick_connect_enabled, legacy_authorization, language, chapters, prepare_ahead, transcoding, downloads, catalog_limit, channel_limit FROM settings").
		Scan(&settings.ServerName, &settings.QuickConnectEnabled, &settings.LegacyAuthorization, &settings.Language,
			&settings.Chapters, &settings.PrepareAhead, &settings.Transcoding, &settings.Downloads, &settings.CatalogLimit, &settings.ChannelLimit)
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
	_, err := s.db.Exec(ctx,
		"UPDATE settings SET server_name = $1, quick_connect_enabled = $2, legacy_authorization = $3, language = $4, chapters = $5, prepare_ahead = $6, transcoding = $7, downloads = $8, catalog_limit = $9, channel_limit = $10",
		settings.ServerName, settings.QuickConnectEnabled, settings.LegacyAuthorization, settings.Language,
		settings.Chapters, settings.PrepareAhead, settings.Transcoding, settings.Downloads, settings.CatalogLimit, settings.ChannelLimit)
	if err != nil {
		return Settings{}, err
	}
	s.settings.Store(&settings)
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
