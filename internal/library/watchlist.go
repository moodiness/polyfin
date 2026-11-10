package library

import (
	"context"
	"errors"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// Each user's watchlist is a playlist of theirs (see package watchlist),
// which Jellyfin apps list with their playlists. The library lists its
// titles in the Now available and Still waiting rows, names it in the
// language of whoever sees it while it keeps the default name, and draws
// its image (see personalimages.go). Changing it through these methods
// has its image compared with its titles again as it next shows.

// watchlistKinds are what a watchlist holds.
var watchlistKinds = []Kind{KindMovie, KindSeries, KindEpisode}

// watchlistPage describes the titles ids of a watchlist from start,
// without those the user may not see, by parental control or blocked
// genres, nor those they hid. A home row's, first, describes only as many
// as it shows; a listing describes them all, to count those the user may
// see.
func (s *Service) watchlistPage(ctx context.Context, v view, ids []accounts.ID, start, count int, first bool) (Page, error) {
	if !first {
		items, err := s.itemsOf(ctx, v, ids, watchlistKinds)
		return slicePage(v.hidden.Without(items), start, count), err
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
		items, from = append(items, v.hidden.Without(described)...), to
	}
	page := slicePage(items, start, count)
	page.More = from < len(ids)
	return page, nil
}

// WatchlistAvailable lists, from start, the titles of the user's
// watchlist they have not started that the last check found versions for
// (see watchlist.Store.Available), those found the latest first: the Now
// available home row. WatchlistWaiting lists those it found none for, in
// the watchlist's order: the Still waiting row. Both leave out the titles
// the user may not see or hid, and list nothing for a request made with an
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

// WatchlistName is the default name of a watchlist, in the language of
// user, who sees it: theirs when Polyfin speaks it, else the server's.
func (s *Service) WatchlistName(ctx context.Context, user accounts.User) (string, error) {
	v, err := s.view(ctx, user)
	return v.words.watchlist, err
}

// WatchlistTitles describes the titles of user's watchlist, started or
// not, in its order, without those the user may not see or hid; none for
// a request made with an API key.
func (s *Service) WatchlistTitles(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil || v.user == (accounts.ID{}) {
		return nil, err
	}
	ids, err := s.watchlist.Titles(ctx, v.user)
	if err != nil {
		return nil, err
	}
	page, err := s.watchlistPage(ctx, v, ids, 0, len(ids), false)
	return page.Items, err
}

// Watchlist returns the identifier of user's watchlist, and false when
// they deleted it and nothing was added since.
func (s *Service) Watchlist(ctx context.Context, user accounts.ID) (accounts.ID, bool, error) {
	return s.watchlist.Playlist(ctx, user)
}

// PlaylistEpisodes lists the episodes a playlist takes for a series, or
// for one of its seasons when seasonID is set, as Jellyfin's playlists
// take them: those aired, in the series' order; for a whole series, those
// of its regular seasons only, without the specials.
func (s *Service) PlaylistEpisodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]Item, error) {
	episodes, err := s.Episodes(ctx, user, seriesID, seasonID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(episodes, func(e Item) bool { return !e.Available || seasonID == nil && e.ParentIndexNumber <= 0 }), nil
}

// WatchlistEntries tells what title, of user's watchlist, makes there
// (see watchlist.Entries): only what is left to watch, as the titles
// lists and imports hold make entries through it, never those added by
// hand. A movie makes itself unless user started or played it; an
// episode itself unless played; a series or a season those of the
// episodes PlaylistEpisodes lists, once one aired, that user has not
// played, keeping one in progress. A title with nothing left to watch is
// Known without Items. A title the user cannot reach waits; one no
// playlist holds makes nothing.
func (s *Service) WatchlistEntries(ctx context.Context, user accounts.User, title accounts.ID) (watchlist.Made, error) {
	item, err := s.Item(ctx, user, title)
	if errors.Is(err, ErrNotFound) {
		return watchlist.Made{}, nil
	}
	if err != nil {
		return watchlist.Made{}, err
	}
	var episodes []Item
	switch item.Kind {
	case KindMovie, KindEpisode:
		data, err := s.userData.Get(ctx, user.ID, []accounts.ID{item.ID})
		if err != nil {
			return watchlist.Made{}, err
		}
		made := watchlist.Made{Known: true}
		if item.Kind == KindEpisode {
			made.Series = item.SeriesID
		}
		if d := data[item.ID]; !d.Played && (item.Kind == KindEpisode || d.Position == 0) {
			made.Items = []accounts.ID{item.ID}
		}
		return made, nil
	case KindSeries:
		episodes, err = s.PlaylistEpisodes(ctx, user, item.ID, nil)
	case KindSeason:
		episodes, err = s.PlaylistEpisodes(ctx, user, item.SeriesID, &item.ID)
	default:
		return watchlist.Made{Known: true}, nil
	}
	if err != nil || len(episodes) == 0 {
		return watchlist.Made{}, err
	}
	ids := make([]accounts.ID, len(episodes))
	for i, e := range episodes {
		ids[i] = e.ID
	}
	data, err := s.userData.Get(ctx, user.ID, ids)
	if err != nil {
		return watchlist.Made{}, err
	}
	made := watchlist.Made{Series: episodes[0].SeriesID, Known: true}
	for _, id := range ids {
		if !data[id].Played {
			made.Items = append(made.Items, id)
		}
	}
	return made, nil
}

// watchlistChanged has the images of user's watchlist compared with its
// titles again as they next show.
func (s *Service) watchlistChanged(ctx context.Context, user accounts.ID) {
	if playlist, ok, err := s.watchlist.Playlist(ctx, user); err == nil && ok {
		s.recheckPersonal(playlist)
	}
}

// WatchlistChange is what a change of a user's watchlist made of its
// titles: the movies and series it newly lists, and those it no longer
// lists, neither an entry nor waiting for its entries.
type WatchlistChange struct {
	Added, Removed []accounts.ID
}

// listedTitles lists the movies and series user's watchlist holds an
// entry of or waits for.
func (s *Service) listedTitles(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	held, err := s.watchlist.Held(ctx, user)
	if err != nil {
		return nil, err
	}
	pending, err := s.watchlist.Pending(ctx, user)
	if err != nil {
		return nil, err
	}
	titles := make(map[accounts.ID]bool, len(held)+len(pending))
	for _, h := range held {
		if h.Series != (accounts.ID{}) {
			titles[h.Series] = true
		} else {
			titles[h.ID] = true
		}
	}
	for _, p := range pending {
		titles[p.ID] = true
	}
	return titles, nil
}

// watchlistEdit makes edit to user's watchlist, and tells what it made of
// its titles.
func (s *Service) watchlistEdit(ctx context.Context, user accounts.ID, edit func() error) (WatchlistChange, error) {
	defer s.watchlistChanged(ctx, user)
	before, err := s.listedTitles(ctx, user)
	if err != nil {
		return WatchlistChange{}, err
	}
	if err := edit(); err != nil {
		return WatchlistChange{}, err
	}
	after, err := s.listedTitles(ctx, user)
	if err != nil {
		// The edit is made: only what it made is unknown.
		return WatchlistChange{}, nil
	}
	var c WatchlistChange
	for id := range after {
		if !before[id] {
			c.Added = append(c.Added, id)
		}
	}
	for id := range before {
		if !after[id] {
			c.Removed = append(c.Removed, id)
		}
	}
	return c, nil
}

// AddToWatchlist puts groups on user's watchlist, on the list of the
// titles they add themselves (see watchlist.Store.Add): the entries it
// does not hold at position, the first being 0.
func (s *Service) AddToWatchlist(ctx context.Context, user accounts.ID, groups []watchlist.Group, position int) (WatchlistChange, error) {
	return s.watchlistEdit(ctx, user, func() error { return s.watchlist.Add(ctx, user, groups, position) })
}

// RemoveFromWatchlist takes items and the episodes of series off user's
// watchlist (see watchlist.Store.Remove).
func (s *Service) RemoveFromWatchlist(ctx context.Context, user accounts.ID, items, series []accounts.ID) (WatchlistChange, error) {
	return s.watchlistEdit(ctx, user, func() error { return s.watchlist.Remove(ctx, user, items, series) })
}

// RemoveWatchlistEntries takes entries off user's watchlist, as
// RemoveFromWatchlist takes their items off.
func (s *Service) RemoveWatchlistEntries(ctx context.Context, user accounts.ID, entries []accounts.ID) (WatchlistChange, error) {
	return s.watchlistEdit(ctx, user, func() error { return s.watchlist.RemoveEntries(ctx, user, entries) })
}

// MoveInWatchlist places entry of user's watchlist just before the entry
// before, or last (see watchlist.Store.Move).
func (s *Service) MoveInWatchlist(ctx context.Context, user, entry, before accounts.ID) error {
	defer s.watchlistChanged(ctx, user)
	return s.watchlist.Move(ctx, user, entry, before)
}

// ReplaceWatchlist makes groups all user's watchlist holds, in order (see
// watchlist.Store.Replace).
func (s *Service) ReplaceWatchlist(ctx context.Context, user accounts.ID, groups []watchlist.Group) (WatchlistChange, error) {
	return s.watchlistEdit(ctx, user, func() error { return s.watchlist.Replace(ctx, user, groups) })
}

// DeleteWatchlist deletes user's watchlist (see watchlist.Store.Delete).
func (s *Service) DeleteWatchlist(ctx context.Context, user accounts.ID) error {
	return s.watchlist.Delete(ctx, user)
}

// WatchlistHeld lists the entries of user's watchlist, started or not, in
// its order.
func (s *Service) WatchlistHeld(ctx context.Context, user accounts.ID) ([]watchlist.Held, error) {
	return s.watchlist.Held(ctx, user)
}

// WatchlistPending lists the titles of user's watchlist that wait for
// their entries, the latest first (see watchlist.Store.Pending).
func (s *Service) WatchlistPending(ctx context.Context, user accounts.ID) ([]watchlist.Held, error) {
	return s.watchlist.Pending(ctx, user)
}
