package library

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// SearchEpisodes lists up to count episodes whose name contains term,
// without regard to case, among those Polyfin knows from the series users
// opened, most recently seen first. Addons search titles, not episodes:
// this asks no addon to search, only to describe the series of the
// episodes found, as opening them would. Episodes of series the user does
// not reach, or that their parental control or blocked genres hide, are
// left out.
func (s *Service) SearchEpisodes(ctx context.Context, user accounts.User, term string, count int) ([]Item, error) {
	term = strings.TrimSpace(term)
	if term == "" || count <= 0 {
		return nil, nil
	}
	// Twice as many as wanted leaves room for those the user cannot reach.
	rows, err := s.db.Query(ctx, `SELECT id FROM items WHERE kind = $1
		AND strpos(lower(coalesce(nullif(data->'video'->>'name', ''), data->'video'->>'title', '')), lower($2)) > 0
		ORDER BY updated_at DESC, id LIMIT $3`, string(KindEpisode), term, 2*count)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil, err
	}
	items, err := s.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	return s.overridden(items[:min(count, len(items))]), nil
}

// Years lists, in ascending order, the years the user's year pages offer
// titles of the given kinds for: the genre options of their catalogs that
// are years (see Narrowed), those of one library when libraryID is set. A
// library narrowed to a genre offers that one only.
// ErrNotFound is returned for a library the user does not see.
func (s *Service) Years(ctx context.Context, user accounts.User, libraryID *accounts.ID, kinds []Kind) ([]int, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var catalogs []titleCatalog
	if libraryID != nil {
		l, ok := v.library(*libraryID)
		if !ok {
			return nil, ErrNotFound
		}
		catalogs = []titleCatalog{{addon: l.addon, catalog: l.catalog, genre: l.genre}}
	} else if catalogs, err = s.titleCatalogs(ctx, v); err != nil {
		return nil, err
	}
	var years []int
	for _, c := range catalogs {
		if kind, ok := titleKind(c.catalog.Type); !ok || !slices.Contains(kinds, kind) {
			continue
		}
		for _, option := range c.options() {
			option = strings.TrimSpace(option)
			// Only options a year page matches: the year as written.
			if year, err := strconv.Atoi(option); err == nil && year > 0 && strconv.Itoa(year) == option && !slices.Contains(years, year) {
				years = append(years, year)
			}
		}
	}
	slices.Sort(years)
	return years, nil
}
