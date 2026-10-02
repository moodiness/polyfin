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
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, "SELECT server_name, quick_connect_enabled, legacy_authorization, language FROM settings").
		Scan(&settings.ServerName, &settings.QuickConnectEnabled, &settings.LegacyAuthorization, &settings.Language)
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
	_, err := s.db.Exec(ctx,
		"UPDATE settings SET server_name = $1, quick_connect_enabled = $2, legacy_authorization = $3, language = $4",
		settings.ServerName, settings.QuickConnectEnabled, settings.LegacyAuthorization, settings.Language)
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
