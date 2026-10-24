package accounts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrCustomTextsChanged reports Settings.CustomCss or CustomJs changed
// since they were read: the change made from them is not saved.
var ErrCustomTextsChanged = errors.New("the custom CSS or JavaScript changed")

// ReplaceCustomTexts sets Settings.CustomCss and CustomJs to css and js,
// provided they still are wasCss and wasJs, and runs also, when not nil, in
// the same transaction: what keeps a copy of a theme's files (see
// themecopy) rewrites the texts with the record of the copy, or not at
// all. The other settings are left as they are.
func (s *Store) ReplaceCustomTexts(ctx context.Context, wasCss, wasJs, css, js string, also func(pgx.Tx) error) (Settings, error) {
	if !validWebText(css, MaxCustomCodeBytes) {
		return Settings{}, ErrInvalidCustomCss
	}
	if !validWebText(js, MaxCustomCodeBytes) {
		return Settings{}, ErrInvalidCustomJs
	}
	// secretsMu serializes the writes of the settings (see UpdateSettings).
	s.secretsMu.Lock()
	defer s.secretsMu.Unlock()
	settings := s.Settings()
	if settings.CustomCss != wasCss || settings.CustomJs != wasJs {
		return Settings{}, ErrCustomTextsChanged
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE settings SET custom_css = $1, custom_js = $2", css, js); err != nil {
			return err
		}
		if also != nil {
			return also(tx)
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	settings.CustomCss, settings.CustomJs = css, js
	s.storeSettings(&settings)
	return settings, nil
}
