package accounts

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Conversions are the conversions a user's apps may have: Video converts
// the video, which burning subtitles in also takes; Audio, the audio.
// Playing a file as it is, or repackaged with its tracks copied, needs
// neither.
type Conversions struct {
	Video, Audio bool
}

// Conversions returns the conversions user may have: those their own
// permissions allow, while the server converts at all.
func (s *Store) Conversions(user User) Conversions {
	on := s.Settings().Transcoding
	return Conversions{Video: on && user.VideoTranscoding, Audio: on && user.AudioTranscoding}
}

// MayDownload reports whether user may download titles: their own
// permission alone decides (see TurnOffDownloads).
func (s *Store) MayDownload(user User) bool {
	return user.ContentDownloading
}

// TurnOffDownloads takes the permission to download away from every user
// who has it, and returns them as they are now. Their apps lose it from
// their next request.
func (s *Store) TurnOffDownloads(ctx context.Context) ([]User, error) {
	rows, _ := s.db.Query(ctx, "UPDATE users SET content_downloading = false WHERE content_downloading "+
		"RETURNING "+userColumns)
	changed, err := pgx.AppendRows([]User(nil), rows, func(row pgx.CollectableRow) (User, error) {
		var user User
		err := row.Scan(user.fields()...)
		return user, err
	})
	if err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		s.forgetSignIns()
	}
	return changed, nil
}

// PersonalAddonsAllowed reports whether user may add and use their own
// addons: their own permission allows it, and the server allows users' own
// addons. Otherwise their addons are kept, but not used.
func (settings Settings) PersonalAddonsAllowed(user User) bool {
	return settings.PersonalAddons && user.PersonalAddons
}
