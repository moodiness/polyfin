// Package watchlisttest gives tests a watchlist store that makes the
// entries of the titles lists newly hold without a library: each title
// becomes one entry of itself, as a movie does, unless the user started
// or played it, as an import adds only what is left to watch.
package watchlisttest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// Store returns a store backed by db whose titles are entries of their
// own, those the user started or played none.
func Store(db *pgxpool.Pool) *watchlist.Store {
	data := userdata.New(db)
	return watchlist.New(db).WithEntries(func(ctx context.Context, user, title accounts.ID) (watchlist.Made, error) {
		got, err := data.Get(ctx, user, []accounts.ID{title})
		if err != nil {
			return watchlist.Made{}, err
		}
		if d := got[title]; d.Played || d.Position > 0 {
			return watchlist.Made{Known: true}, nil
		}
		return watchlist.Made{Items: []accounts.ID{title}, Known: true}, nil
	})
}
