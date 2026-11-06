package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotShareable reports a title a share link cannot open: neither a
// movie nor an episode.
var ErrNotShareable = errors.New("only a movie or an episode can be shared")

// Shareable returns the title id names when user may give it through a
// share link: a movie or an episode they may see, parental control,
// blocked genres and the titles they hid applying. ErrNotFound reports a
// title they may not see, ErrNotShareable one of another kind.
func (s *Service) Shareable(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	item, err := s.Item(ctx, user, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindMovie && item.Kind != KindEpisode {
		return Item{}, ErrNotShareable
	}
	hidden, err := s.HiddenTitles(ctx, user)
	if err != nil {
		return Item{}, err
	}
	if hidden.Hides(item) {
		return Item{}, ErrNotFound
	}
	return item, nil
}

// ShareTitles finds up to limit movies and series whose name contains
// term, for user to share: those the search catalogs of their addons
// find (see Search), then those Polyfin already knows from the catalogs
// of their addons, as their libraries and home rows listed them. These
// are read from what Polyfin recorded, asking no addon: a catalog without
// a search extra is found so, as far as it was listed. Titles their
// parental control, blocked genres or the addons they do not use (see
// addons.UsedBy) keep from them are left out, and so are the known titles
// they hid; the caller leaves out those Search found that they hid.
func (s *Service) ShareTitles(ctx context.Context, user accounts.User, term string, limit int) ([]Item, error) {
	term = strings.TrimSpace(term)
	if term == "" || limit <= 0 {
		return nil, nil
	}
	kinds := []Kind{KindMovie, KindSeries}
	items, err := s.Search(ctx, user, term, kinds, limit)
	if err != nil || len(items) >= limit {
		return items, err
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	known, err := s.recordedTitles(ctx, v, term, kinds, items, limit-len(items))
	if err != nil {
		return nil, err
	}
	return append(items, s.overridden(known)...), nil
}

// recordedTitles lists up to count titles of the given kinds whose name
// contains term, without regard to case, among those Polyfin recorded
// from a catalog of one of the user's addons, and not among found: those
// whose name starts so first, then the most recently listed. Titles the
// user does not reach are left out (see described).
func (s *Service) recordedTitles(ctx context.Context, v view, term string, kinds []Kind, found []Item, count int) ([]Item, error) {
	if count <= 0 || len(v.addons) == 0 {
		return nil, nil
	}
	// A record keeps its addon as JSON (see record).
	addonIDs := make([]string, 0, len(v.addons))
	for _, entry := range v.addons {
		data, err := json.Marshal(entry.addon.ID)
		if err != nil {
			return nil, err
		}
		addonIDs = append(addonIDs, string(data))
	}
	kindNames := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		kindNames = append(kindNames, string(kind))
	}
	skipped := make([]accounts.ID, 0, len(found))
	for _, item := range found {
		skipped = append(skipped, item.ID)
	}
	// Twice as many as wanted leaves room for those the user cannot reach.
	rows, err := s.db.Query(ctx, `SELECT id FROM items WHERE kind = ANY($1::text[]) AND data->'addon' = ANY($2::text[]::jsonb[])
		AND NOT id = ANY($3::uuid[]) AND strpos(lower(data->'meta'->>'name'), lower($4::text)) > 0
		ORDER BY strpos(lower(data->'meta'->>'name'), lower($4::text)) = 1 DESC, updated_at DESC, id LIMIT $5`,
		kindNames, addonIDs, skipped, term, 2*count)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil, err
	}
	items, err := s.itemsOf(ctx, v, ids, kinds)
	if err != nil {
		return nil, err
	}
	items = slices.DeleteFunc(items, func(item Item) bool { return v.hidden[item.ID] })
	return items[:min(count, len(items))], nil
}

// ShareTitle names a title as share links list it: a movie with its year,
// an episode with its series, season and number.
func ShareTitle(item Item) string {
	switch {
	case item.Kind == KindEpisode:
		name := fmt.Sprintf("%s S%02dE%02d", item.SeriesName, item.ParentIndexNumber, item.IndexNumber)
		if item.Name != "" {
			name += " · " + item.Name
		}
		return name
	case item.ProductionYear > 0:
		return item.Name + " (" + strconv.Itoa(item.ProductionYear) + ")"
	}
	return item.Name
}
