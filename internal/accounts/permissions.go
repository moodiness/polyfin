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
// who has it, and from every group that gives it, so that their members
// who follow them keep following them. It returns the users and the groups
// changed, as they are now. Their apps lose it from their next request.
func (s *Store) TurnOffDownloads(ctx context.Context) ([]User, []Group, error) {
	var changed []User
	var groups []Group
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		rows, _ := tx.Query(ctx, "UPDATE user_groups SET content_downloading = false WHERE id IN ("+
			"SELECT id FROM user_groups WHERE content_downloading ORDER BY id FOR UPDATE) RETURNING id")
		ids, err := pgx.CollectRows(rows, pgx.RowTo[ID])
		if err != nil {
			return err
		}
		for _, id := range ids {
			group, err := scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1", id))
			if err != nil {
				return err
			}
			groups = append(groups, group)
		}
		rows, _ = tx.Query(ctx, "UPDATE users SET content_downloading = false WHERE id IN ("+
			"SELECT id FROM users WHERE content_downloading ORDER BY id FOR UPDATE) RETURNING "+userColumns)
		changed, err = pgx.AppendRows([]User(nil), rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if len(changed) > 0 {
		s.forgetSignIns()
	}
	return changed, groups, nil
}

// PersonalAddonsAllowed reports whether user may add and use their own
// addons: their own permission allows it, and the server allows users' own
// addons. Otherwise their addons are kept, but not used.
func (settings Settings) PersonalAddonsAllowed(user User) bool {
	return settings.PersonalAddons && user.PersonalAddons
}
