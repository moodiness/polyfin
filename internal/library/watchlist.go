package library

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// Watchlist is a view of Polyfin's own, beside the libraries: the movies
// and series on the user's watchlists that they have not started, the
// latest added first (see package watchlist). Each user has their own;
// it shows while it holds titles. Its titles are described as they are
// anywhere, and those the user may not see, by parental control or
// blocked genres, are left out, as are those the user hid.

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
	return Item{ID: WatchlistViewID, Kind: KindLibrary, Name: v.words.watchlist}, nil
}

// watchlistIDs lists the titles of the user's watchlist they have not
// started nor hidden, the latest added first; none for a request made
// with an API key.
func (s *Service) watchlistIDs(ctx context.Context, v view) ([]accounts.ID, error) {
	if v.user == (accounts.ID{}) {
		return nil, nil
	}
	ids, err := s.watchlist.Titles(ctx, v.user)
	return slices.DeleteFunc(ids, func(id accounts.ID) bool { return v.hidden[id] }), err
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
	return s.watchlistPage(ctx, v, ids, start, count, first)
}

// watchlistPage describes the titles ids of a watchlist from start, as
// watchlistChildren does.
func (s *Service) watchlistPage(ctx context.Context, v view, ids []accounts.ID, start, count int, first bool) (Page, error) {
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

// WatchlistAvailable lists, from start, the titles of the user's
// watchlist they have not started that the last check found versions for
// (see watchlist.Store.Available), those found the latest first: the Now
// available home row. WatchlistWaiting lists those it found none for, the
// latest added first: the Still waiting row. Both leave out what the
// Watchlist view leaves out, and list nothing for a request made with an
// API key. A home row's, first, describes only as many titles as it shows.
func (s *Service) WatchlistAvailable(ctx context.Context, user accounts.User, start, count int, first bool) (Page, error) {
	return s.watchlistRow(ctx, user, s.watchlist.Available, start, count, first)
}

// WatchlistWaiting lists the Still waiting row (see WatchlistAvailable).
func (s *Service) WatchlistWaiting(ctx context.Context, user accounts.User, start, count int, first bool) (Page, error) {
	return s.watchlistRow(ctx, user, s.watchlist.Waiting, start, count, first)
}

func (s *Service) watchlistRow(ctx context.Context, user accounts.User, list func(context.Context, accounts.ID) ([]accounts.ID, error),
	start, count int, first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil || v.user == (accounts.ID{}) {
		return Page{}, err
	}
	ids, err := list(ctx, v.user)
	if err != nil {
		return Page{}, err
	}
	ids = slices.DeleteFunc(ids, func(id accounts.ID) bool { return v.hidden[id] })
	return s.watchlistPage(ctx, v, ids, start, count, first)
}

// AddToWatchlist puts a movie or a series on user's watchlist, on the list
// of the titles they add themselves (see watchlist.Own), unless a list
// already holds it.
func (s *Service) AddToWatchlist(ctx context.Context, user, title accounts.ID) error {
	return s.watchlist.Add(ctx, user, title)
}

// RemoveFromWatchlist takes a title off user's watchlist, off every list
// holding it.
func (s *Service) RemoveFromWatchlist(ctx context.Context, user, title accounts.ID) error {
	return s.watchlist.Remove(ctx, user, title)
}

// WatchlistHeld lists the titles user's watchlist holds, started or not,
// the latest added first: those started are left out of the Watchlist
// view only.
func (s *Service) WatchlistHeld(ctx context.Context, user accounts.ID) ([]watchlist.Title, error) {
	return s.watchlist.Held(ctx, user)
}
