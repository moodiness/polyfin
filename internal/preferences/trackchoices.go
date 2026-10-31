package preferences

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// TrackChoice is what a user chose of a title's tracks, a series for its
// episodes or a movie, for its next plays: languages, never tracks'
// indices, as versions number their tracks differently.
type TrackChoice struct {
	// AudioLanguage is the language of the audio track chosen, as the
	// track names it; empty when none was chosen.
	AudioLanguage string
	// AudioVariant is the regional variant of that track, one of
	// localization.AudioVariants, kept only when it differs from the
	// user's audio region; empty otherwise, and without AudioLanguage.
	AudioVariant string
	Subtitle     SubtitleChoice
}

// SubtitleChoice is what a user chose of a title's subtitles: none when
// it is the zero value, no subtitles when Off, else a track in Language
// with its flags.
type SubtitleChoice struct {
	Off                     bool
	Language                string
	Forced, HearingImpaired bool
}

// Chosen reports whether a subtitle choice was made.
func (s SubtitleChoice) Chosen() bool {
	return s.Off || s.Language != ""
}

// Empty reports whether nothing was chosen.
func (c TrackChoice) Empty() bool {
	return c.AudioLanguage == "" && !c.Subtitle.Chosen()
}

// TrackChoices returns the track choices user made, by title.
func (s *Store) TrackChoices(ctx context.Context, user accounts.ID) (map[accounts.ID]TrackChoice, error) {
	rows, err := s.db.Query(ctx, `
		SELECT title_id, audio_language, audio_variant, subtitle, subtitle_language, subtitle_forced,
			subtitle_hearing_impaired
		FROM track_choices WHERE user_id = $1`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices := map[accounts.ID]TrackChoice{}
	for rows.Next() {
		var title accounts.ID
		var c TrackChoice
		var subtitle string
		if err := rows.Scan(&title, &c.AudioLanguage, &c.AudioVariant, &subtitle, &c.Subtitle.Language, &c.Subtitle.Forced,
			&c.Subtitle.HearingImpaired); err != nil {
			return nil, err
		}
		c.Subtitle.Off = subtitle == "off"
		choices[title] = c
	}
	return choices, rows.Err()
}

// PutTrackChoice replaces the track choice user made for title, or
// forgets it when choice is empty.
func (s *Store) PutTrackChoice(ctx context.Context, user, title accounts.ID, choice TrackChoice) error {
	if choice.Empty() {
		_, err := s.db.Exec(ctx, "DELETE FROM track_choices WHERE user_id = $1 AND title_id = $2", user, title)
		return err
	}
	subtitle := "none"
	switch {
	case choice.Subtitle.Off:
		subtitle = "off"
		choice.Subtitle = SubtitleChoice{Off: true}
	case choice.Subtitle.Language != "":
		subtitle = "track"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO track_choices (user_id, title_id, audio_language, audio_variant, subtitle,
			subtitle_language, subtitle_forced, subtitle_hearing_impaired)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, title_id) DO UPDATE SET audio_language = excluded.audio_language,
			audio_variant = excluded.audio_variant, subtitle = excluded.subtitle,
			subtitle_language = excluded.subtitle_language,
			subtitle_forced = excluded.subtitle_forced,
			subtitle_hearing_impaired = excluded.subtitle_hearing_impaired, chosen_at = now()`,
		user, title, choice.AudioLanguage, choice.AudioVariant, subtitle, choice.Subtitle.Language,
		choice.Subtitle.Forced, choice.Subtitle.HearingImpaired)
	return err
}

// ForgetTrackChoices forgets every track choice user made.
func (s *Store) ForgetTrackChoices(ctx context.Context, user accounts.ID) error {
	_, err := s.db.Exec(ctx, forgetTrackChoices, user)
	return err
}

// forgetTrackChoices deletes a user's track choices.
const forgetTrackChoices = "DELETE FROM track_choices WHERE user_id = $1"
