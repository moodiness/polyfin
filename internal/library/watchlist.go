package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Watchlist is a view of Polyfin's own, beside the libraries: the movies
// and series on the user's watchlists that they have not started, the
// latest added first (see package watchlist). Each user has their own;
// it shows while it holds titles. Its titles are described as they are
// anywhere, and those the user may not see, by parental control or
// blocked genres, are left out.

// WatchlistViewID identifies the Watchlist view, the same on every server.
var WatchlistViewID = itemID("view|watchlist")

// watchlistKinds are what the view lists.
var watchlistKinds = []Kind{KindMovie, KindSeries}

// watchlistView describes the Watchlist view, ErrNotFound for a user whose
// watchlist holds no title they have not started. It has no collection
// type, as a library of mixed content: apps give it a row of latest items
// on their home screen.
func (s *Service) watchlistView(ctx context.Context, v view) (Item, error) {
	ids, err := s.watchlistIDs(ctx, v)
	if err != nil {
		return Item{}, err
	}
	if len(ids) == 0 {
		return Item{}, ErrNotFound
	}
	return Item{ID: WatchlistViewID, Kind: KindLibrary, Name: s.words().watchlist}, nil
}

// watchlistIDs lists the titles of the user's watchlist they have not
// started, the latest added first; none for a request made with an API
// key.
func (s *Service) watchlistIDs(ctx context.Context, v view) ([]accounts.ID, error) {
	if v.user == (accounts.ID{}) {
		return nil, nil
	}
	return s.watchlist.Titles(ctx, v.user)
}

// watchlistChildren lists the titles of the Watchlist view from start. A
// home row's, first, describes only as many as it shows; a listing
// describes them all, to count those the user may see. A view emptied
// since the app showed it lists nothing.
func (s *Service) watchlistChildren(ctx context.Context, v view, start, count int, first bool) (Page, error) {
	if v.user == (accounts.ID{}) {
		return Page{}, ErrNotFound
	}
	ids, err := s.watchlistIDs(ctx, v)
	if err != nil {
		return Page{}, err
	}
	if !first {
		items, err := s.itemsOf(ctx, v, ids, watchlistKinds)
		return slicePage(items, start, count), err
	}
	want := start + count
	var items []Item
	from := 0
	for from < len(ids) && len(items) < want {
		to := min(from+want-len(items), len(ids))
		described, err := s.itemsOf(ctx, v, ids[from:to], watchlistKinds)
		if err != nil {
			return Page{}, err
		}
		items, from = append(items, described...), to
	}
	page := slicePage(items, start, count)
	page.More = from < len(ids)
	return page, nil
}
