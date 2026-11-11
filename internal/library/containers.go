package library

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The collections of addons and the smart lists shown as collections
// whose titles are all out of reach.
//
// Jellyfin hides a collection none of whose titles the user may see, as
// Polyfin hides the collections and playlists users make (see the
// jellyfin package). Addons' collections and smart lists shown as
// collections are hidden the same way, whatever hides their titles: the
// user's parental control, unrated titles blocked, blocked genres, or the
// highest rating a listing asks, the only part of its rating filter a
// container's titles are judged by. Their titles cannot be read whole for
// each listing, so:
//   - An addon's collection shows when the first page of a catalog it
//     groups, or that a collection within it groups, holds a title the
//     user may see, as its home row would show it, and its own rating,
//     when its description gives one, is within those limits. One that
//     groups no catalog shows. A collection library's catalog judges its
//     collections as it reads them (see visibleMetas), so that its
//     listings count and page what they show; a collection whose titles
//     wait for their rating is left out of that answer, which tells that
//     more may follow, as such titles are (see window).
//   - A smart list shown as a collection shows when the user's own read of
//     it keeps a title (see smartMatches). A smart list shown as a library
//     is a library, which is never hidden.
//
// A hidden one's own pages answer ErrHidden. Only the listings that leave
// titles out this way judge containers; others pay nothing.

// ErrHidden reports a collection the user may not see, as none of its
// titles is within their reach (see above). It is an ErrNotFound.
var ErrHidden = fmt.Errorf("%w: none of its titles may be seen", ErrNotFound)

const (
	// collectionJudgements bounds the addons' collections judged at once,
	// and smartJudgements the smart lists; collectionProbe is how many of
	// a page's first titles a collection's judgement judges before the
	// others (see firstPageShows).
	collectionJudgements = 8
	smartJudgements      = 4
	collectionProbe      = 4
)

// forMembers returns v as it judges the titles of a container: narrowed by
// the highest rating its rating filter asks only (see WithMembersFilter),
// with a held mark of its own. Collections among those titles are not
// judged in turn.
func (v view) forMembers() view {
	q := ratingQuery{max: v.query.max}
	q.active = q.max != nil
	v.query, v.held, v.members = q, new(atomic.Bool), true
	return v
}

// allowsOwnRating reports whether a container rated rating is within the
// user's parental control and the highest rating the listing asks, scored
// under the server's rating country. One rated as Polyfin cannot score, or
// not rated, is within them, as Jellyfin's are.
func (v view) allowsOwnRating(rating string) bool {
	score, rated := localization.RatingScore(rating, v.country)
	if !rated {
		return true
	}
	return v.parental.Allows("Movie", rating, v.country) && (v.query.max == nil || compareScores(score, *v.query.max) <= 0)
}

// collectionShown judges the collection meta addon lists, in a library of
// at most most titles (0 for the catalog limit), for the user of v (see
// above). decided is false while titles that may show wait for their
// rating, or for a read that asks no addon, on pages not kept.
func (s *Service) collectionShown(ctx context.Context, v view, addon installed, meta stremio.Meta, most int) (shown, decided bool) {
	mv := v.forMembers()
	if !mv.narrows() {
		return true, true
	}
	full, err := s.meta(ctx, addon, "collection", meta.ID)
	if err != nil {
		if ctx.Err() != nil {
			return false, false
		}
		// One that cannot be described opens on nothing.
		s.logger.Debug("A collection could not be described", "collection", meta.ID, "error", err)
		return false, true
	}
	rating := certification(full, v.country)
	if rating == "" {
		rating = certification(meta, v.country)
	}
	own := s.overriddenTraits(itemID(collectionKey(addon.addon.ID, meta.ID)), traits{rating: rating, ratingKnown: true})
	if !mv.allowsOwnRating(own.rating) {
		return false, true
	}
	sources, _ := s.collectionSources(ctx, addon, full, most, true)
	if len(sources) == 0 {
		return true, true
	}
	var found, waiting atomic.Bool
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for _, src := range sources {
		group.Go(func() error {
			if found.Load() {
				return nil
			}
			switch shown, decided := s.firstPageShows(ctx, mv, src); {
			case shown:
				found.Store(true)
			case !decided:
				waiting.Store(true)
			}
			return nil
		})
	}
	_ = group.Wait()
	if ctx.Err() != nil {
		return false, false
	}
	return found.Load(), found.Load() || !waiting.Load()
}

// firstPageShows reports whether the first page of src holds a title the
// user of v may see, and whether that is decided: not while titles on it
// wait for their rating, nor when a read that asks no addon does not keep
// the page. A page the addon cannot give holds none. Its first titles are
// judged first, then the others, so that a collection one of whose first
// titles shows does not look up the ratings of the rest.
func (s *Service) firstPageShows(ctx context.Context, v view, src source) (shown, decided bool) {
	metas, err := s.page(ctx, src, 0)
	switch {
	case errors.Is(err, errNotKept):
		return false, false
	case err != nil:
		return false, ctx.Err() == nil
	}
	held := false
	first := min(collectionProbe, len(metas))
	for _, part := range [][]stremio.Meta{metas[:first], metas[first:]} {
		if len(part) == 0 {
			continue
		}
		visible, waiting := s.visibleMetas(ctx, v, src, part)
		held = held || waiting
		for _, meta := range visible {
			if _, _, err := s.titleItem(src.addon, src.catalog, meta, accounts.ID{}, v.country); err == nil {
				return true, true
			}
		}
	}
	return false, !held
}

// shownCollections marks in allowed the entries at indexes of a page of
// the collection catalog src, its collections, the user of v is not shown,
// or not yet (see collectionShown), and reports whether any was left out
// only as its titles wait for their rating.
func (s *Service) shownCollections(ctx context.Context, v view, src source, metas []stremio.Meta, indexes []int, allowed []bool) (held bool) {
	if v.members || len(indexes) == 0 || !v.forMembers().narrows() {
		return false
	}
	var waiting atomic.Bool
	var group errgroup.Group
	group.SetLimit(collectionJudgements)
	for _, i := range indexes {
		group.Go(func() error {
			shown, decided := s.collectionShown(ctx, v, src.addon, metas[i], src.max)
			allowed[i] = shown && decided
			if !decided {
				waiting.Store(true)
			}
			return nil
		})
	}
	_ = group.Wait()
	return waiting.Load()
}

// shownCollection answers ErrHidden for an addon's collection, recorded as
// r, the user of v is not shown (see collectionShown).
func (s *Service) shownCollection(ctx context.Context, v view, addon installed, r record) error {
	if v.members || !v.forMembers().narrows() {
		return nil
	}
	if shown, decided := s.collectionShown(ctx, v, addon, *r.Meta, s.collectionMax(ctx, v, r)); !shown || !decided {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrHidden
	}
	return nil
}

// smartShown reports whether the user of v is shown the smart list l: one
// shown as a library always, and one shown as a collection when their own
// read of it keeps a title (see above). The collection of a search in
// plain words is not judged: it is a search.
func (s *Service) smartShown(ctx context.Context, v view, l smartList) (bool, error) {
	if l.item.Kind != KindCollection || l.search {
		return true, nil
	}
	mv := v.forMembers()
	if !mv.narrows() {
		return true, nil
	}
	result, err := s.smartMatches(ctx, mv, l.list)
	if err != nil {
		return false, err
	}
	return len(result.titles) > 0, nil
}

// ShownSmartCollections lists the smart lists user sees as collections,
// in order, as SmartCollections does, but those they are not shown (see
// above). A list that cannot be read is left out.
func (s *Service) ShownSmartCollections(ctx context.Context, user accounts.User) ([]Item, error) {
	items, err := s.SmartCollections(ctx, user)
	if err != nil || len(items) == 0 {
		return items, err
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	if !v.forMembers().narrows() {
		return items, nil
	}
	shown := make([]bool, len(items))
	var group errgroup.Group
	group.SetLimit(smartJudgements)
	for i, item := range items {
		l, ok := v.smartList(item.ID)
		if !ok {
			continue
		}
		group.Go(func() error {
			var err error
			if shown[i], err = s.smartShown(ctx, v, l); err != nil && ctx.Err() == nil {
				s.logger.Debug("A smart list could not be judged", "list", l.list.ID, "error", err)
			}
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kept := items[:0:0]
	for i, item := range items {
		if shown[i] {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

// shownSmartList answers ErrHidden for the smart list l the user of v is
// not shown (see smartShown).
func (s *Service) shownSmartList(ctx context.Context, v view, l smartList) error {
	shown, err := s.smartShown(ctx, v, l)
	if err == nil && !shown {
		err = ErrHidden
	}
	return err
}
