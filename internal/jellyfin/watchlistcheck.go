package jellyfin

import (
	"context"
	"errors"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// FindWatchlistVersions checks a title of user's watchlist for versions,
// for the watchlist checks (see watchlist.Checker). A title the user may
// not see, by parental control, blocked genres, the libraries hidden from
// them or their hidden titles, is watchlist.Unseen: no addon is asked. A
// movie whose release date is to come, and a series none of whose regular
// episodes is out, are watchlist.Unreleased. Otherwise the movie, or the
// series' first regular episode out, is asked of the user's stream
// addons, torrents' files included for a user who may play them (see
// library.WatchlistVersions): it is available when one of its versions is
// neither hidden by the version rules nor taller than the user's quality
// group. Playback still offers these versions when no other is left, but
// a title found only in them is not told of as available: they are what
// the user chose not to be offered.
func (h *Handler) FindWatchlistVersions(ctx context.Context, user accounts.User, title accounts.ID) (watchlist.Found, error) {
	item, err := h.Library.Item(ctx, user, title)
	if errors.Is(err, library.ErrNotFound) {
		return watchlist.Unseen, nil
	}
	if err != nil {
		return watchlist.Unknown, err
	}
	hidden, err := h.Library.HiddenTitles(ctx, user)
	if err != nil {
		return watchlist.Unknown, err
	}
	if hidden[title] {
		return watchlist.Unseen, nil
	}
	asked := title
	switch item.Kind {
	case library.KindMovie:
		if item.PremiereDate != nil && item.PremiereDate.After(h.now()) {
			return watchlist.Unreleased, nil
		}
	case library.KindSeries:
		episodes, err := h.Library.Episodes(ctx, user, title, nil)
		if err != nil {
			return watchlist.Unknown, err
		}
		i := slices.IndexFunc(episodes, func(e library.Item) bool { return e.ParentIndexNumber > 0 && e.Available })
		if i < 0 {
			return watchlist.Unreleased, nil
		}
		asked = episodes[i].ID
	default:
		return watchlist.Unseen, nil
	}
	versions, complete, err := h.Library.WatchlistVersions(ctx, user, asked)
	if errors.Is(err, library.ErrNotFound) {
		return watchlist.Unseen, nil
	}
	if err != nil {
		return watchlist.Unknown, err
	}
	rules := newVersionRules(h.Accounts.Settings())
	if slices.ContainsFunc(versions, func(v library.Version) bool {
		facts := h.versionFacts(ctx, v)
		return !(rules.hides() && rules.hidden(v, facts)) && user.FitsGroup(facts.height)
	}) {
		return watchlist.Available, nil
	}
	if !complete {
		return watchlist.Unknown, nil
	}
	return watchlist.Missing, nil
}
