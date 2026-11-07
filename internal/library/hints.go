package library

import (
	"context"
	"encoding/json"
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
// not reach, that their parental control or blocked genres hide, or that
// they hid, are left out.
func (s *Service) SearchEpisodes(ctx context.Context, user accounts.User, term string, count int) ([]Item, error) {
	term = strings.TrimSpace(term)
	if term == "" || count <= 0 {
		return nil, nil
	}
	// Twice as many as wanted leaves room for those the user cannot reach.
	rows, err := s.db.Query(ctx, `SELECT id, data->>'seriesId' FROM items WHERE kind = 'episode'
		AND `+episodeName+` LIKE lower($1)
		ORDER BY updated_at DESC, id LIMIT $2`, containing(term), 2*count)
	if err != nil {
		return nil, err
	}
	var ids, series []accounts.ID
	var id accounts.ID
	var seriesID *string
	if _, err := pgx.ForEachRow(rows, []any{&id, &seriesID}, func() error {
		ids = append(ids, id)
		if seriesID != nil {
			series = append(series, itemID(titleKey(KindSeries, *seriesID)))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Each episode is described from its series: the series are read
	// together rather than one by one.
	if ctx, err = s.withPreloaded(ctx, series); err != nil {
		return nil, err
	}
	items, err := s.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	hidden, err := s.HiddenTitles(ctx, user)
	if err != nil {
		return nil, err
	}
	items = hidden.Without(items)
	return s.overridden(items[:min(count, len(items))]), nil
}

// episodeName is the name episodes are searched by.
const episodeName = `lower(coalesce(nullif(data->'video'->>'name', ''), data->'video'->>'title', ''))`

type preloadedKey struct{}

// preloadedRow is an items row read ahead: its key and data.
type preloadedRow struct {
	key  string
	data []byte
}

// withPreloaded reads the rows of ids in one query, for load to find in
// the context it returns rather than reading them one by one. Each load
// decodes its own record, so callers may change it.
func (s *Service) withPreloaded(ctx context.Context, ids []accounts.ID) (context.Context, error) {
	if len(ids) == 0 {
		return ctx, nil
	}
	rows, err := s.db.Query(ctx, "SELECT id, key, data FROM items WHERE id = ANY($1::uuid[])", ids)
	if err != nil {
		return ctx, err
	}
	found := make(map[accounts.ID]preloadedRow, len(ids))
	var id accounts.ID
	var row preloadedRow
	if _, err := pgx.ForEachRow(rows, []any{&id, &row.key, &row.data}, func() error {
		found[id] = preloadedRow{key: row.key, data: slices.Clone(row.data)}
		return nil
	}); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, preloadedKey{}, found), nil
}

// preloaded returns the record of id withPreloaded read ahead in ctx, and
// whether it did.
func preloaded(ctx context.Context, id accounts.ID) (record, bool, error) {
	found, _ := ctx.Value(preloadedKey{}).(map[accounts.ID]preloadedRow)
	row, ok := found[id]
	if !ok {
		return record{}, false, nil
	}
	r := record{Key: row.key}
	if err := json.Unmarshal(row.data, &r); err != nil {
		return record{}, true, err
	}
	r.ID = id
	return r, true, nil
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
