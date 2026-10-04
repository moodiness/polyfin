package library

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Subtitle files users add to movies and episodes from their apps. They
// belong to the title, for every version, and are offered first among its
// subtitle files, before those of the addons (see Subtitles).

// MaxUploadedSubtitles bounds the subtitle files added to one title.
const MaxUploadedSubtitles = 20

var (
	// ErrNotUploaded is returned for a subtitle that is not one a user
	// added, which cannot be deleted.
	ErrNotUploaded = errors.New("not an added subtitle")
	// ErrTooManySubtitles is returned when a title has
	// MaxUploadedSubtitles already.
	ErrTooManySubtitles = errors.New("too many added subtitles")
)

// UploadedSubtitle is a subtitle file added to a title: its language as
// the app gave it, its format (srt, vtt, ass or ssa) and its text.
type UploadedSubtitle struct {
	Language        string
	Format          string
	Forced          bool
	HearingImpaired bool
	Data            []byte
}

// uploadTarget checks that the user reaches a movie or an episode, which
// alone take subtitle files.
func (s *Service) uploadTarget(ctx context.Context, user accounts.User, id accounts.ID) error {
	t, _, err := s.target(ctx, user, id)
	if err != nil {
		return err
	}
	if t.kind != KindMovie && t.kind != KindEpisode {
		return ErrNotFound
	}
	return nil
}

// UploadSubtitle adds a subtitle file to a movie or an episode the user
// reaches.
func (s *Service) UploadSubtitle(ctx context.Context, user accounts.User, id accounts.ID, subtitle UploadedSubtitle) error {
	if err := s.uploadTarget(ctx, user, id); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `INSERT INTO uploaded_subtitles (item_id, language, format, forced, hearing_impaired, data)
		SELECT $1, $2, $3, $4, $5, $6 WHERE (SELECT count(*) FROM uploaded_subtitles WHERE item_id = $1) < $7`,
		id, subtitle.Language, subtitle.Format, subtitle.Forced, subtitle.HearingImpaired, subtitle.Data, MaxUploadedSubtitles)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrTooManySubtitles
	}
	return err
}

// DeleteUploadedSubtitle deletes a title's subtitle file at index among
// its subtitle files, which must be one a user added: they come first. It
// returns the identifier of the file deleted.
func (s *Service) DeleteUploadedSubtitle(ctx context.Context, user accounts.User, id accounts.ID, index int) (accounts.ID, error) {
	if err := s.uploadTarget(ctx, user, id); err != nil {
		return accounts.ID{}, err
	}
	uploaded, err := s.uploaded(ctx, id)
	if err != nil {
		return accounts.ID{}, err
	}
	if index < 0 || index >= len(uploaded) {
		return accounts.ID{}, ErrNotUploaded
	}
	_, err = s.db.Exec(ctx, "DELETE FROM uploaded_subtitles WHERE id = $1", uploaded[index].ID)
	return uploaded[index].ID, err
}

// uploaded lists the subtitle files added to a title, oldest first, as
// external subtitles.
func (s *Service) uploaded(ctx context.Context, id accounts.ID) ([]ExternalSubtitle, error) {
	rows, err := s.db.Query(ctx, `SELECT id, language, format, forced, hearing_impaired FROM uploaded_subtitles
		WHERE item_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	var result []ExternalSubtitle
	var subtitle ExternalSubtitle
	_, err = pgx.ForEachRow(rows, []any{&subtitle.ID, &subtitle.Language, &subtitle.Format, &subtitle.Forced, &subtitle.HearingImpaired}, func() error {
		subtitle.Uploaded = true
		result = append(result, subtitle)
		return nil
	})
	return result, err
}

// UploadedSubtitleText returns the text of a subtitle file a user added;
// ErrNotFound once it is deleted.
func (s *Service) UploadedSubtitleText(ctx context.Context, id accounts.ID) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT data FROM uploaded_subtitles WHERE id = $1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}
