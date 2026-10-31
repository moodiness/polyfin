package library

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// What the libraries a user leaves out of their home screen sections
// (Jellyfin's LatestItemsExcludes) take from Next Up and Continue
// Watching.

// ListedOnlyIn returns those of titles (movies, series, videos) that only
// the libraries of excluded list for user, as far as Polyfin knows without
// asking an addon. A title is listed by the library it was last listed in
// (the one a parentId finds it under, see ListedUnder), by a local folder
// library holding it, and by a catalog library whose kept pages list it. A
// title last listed in a library not excluded, or in none of the user's
// libraries (a search result), is not returned: Polyfin cannot tell that
// it is only the excluded libraries'.
func (s *Service) ListedOnlyIn(ctx context.Context, user accounts.User, titles, excluded []accounts.ID) (map[accounts.ID]bool, error) {
	if len(titles) == 0 || len(excluded) == 0 {
		return nil, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	left := map[accounts.ID]bool{}
	for _, id := range excluded {
		if _, ok := v.library(id); ok {
			left[id] = true
		}
	}
	if len(left) == 0 {
		return nil, nil
	}
	records, err := s.loadAll(ctx, titles)
	if err != nil {
		return nil, err
	}
	only := map[accounts.ID]bool{}
	for _, r := range records {
		listed, err := s.listedIn(ctx, v, r)
		if err != nil {
			return nil, err
		}
		if !left[listed] {
			continue
		}
		elsewhere, err := s.listedElsewhere(ctx, v, r, left)
		if err != nil {
			return nil, err
		}
		if !elsewhere {
			only[r.ID] = true
		}
	}
	return only, nil
}

// listedIn returns the library of v that r was last listed in, climbing
// the addon collections between them, zero when it is none of v's.
func (s *Service) listedIn(ctx context.Context, v view, r record) (accounts.ID, error) {
	seen := map[accounts.ID]bool{}
	for range maxListingDepth {
		parent := deref(r.Parent)
		if parent == (accounts.ID{}) || seen[parent] {
			return accounts.ID{}, nil
		}
		if _, ok := v.library(parent); ok {
			return parent, nil
		}
		seen[parent] = true
		next, err := s.load(ctx, parent)
		if errors.Is(err, ErrNotFound) {
			return accounts.ID{}, nil
		}
		if err != nil {
			return accounts.ID{}, err
		}
		r = next
	}
	return accounts.ID{}, nil
}

// listedElsewhere reports whether a library of v not in left lists the
// title of r: a local folder that holds it, or a catalog whose kept pages
// list it.
func (s *Service) listedElsewhere(ctx context.Context, v view, r record, left map[accounts.ID]bool) (bool, error) {
	_, stremioID, ok := strings.Cut(r.Key, "|")
	if !ok || stremioID == "" {
		return false, nil
	}
	listed, err := json.Marshal([]map[string]string{{"id": stremioID}})
	if err != nil {
		return false, err
	}
	for _, l := range v.libraries {
		if left[l.item.ID] || l.addon.addon.HasMusic() {
			continue
		}
		if l.addon.addon.Local() {
			if s.local == nil {
				continue
			}
			_, err := s.local.Meta(ctx, l.addon.addon.ID, stremioID)
			if err == nil {
				return true, nil
			}
			if !errors.Is(err, stremio.ErrNotFound) {
				return false, err
			}
			continue
		}
		var found bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM catalog_pages
			WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4 AND metas @> $5::jsonb)`,
			l.addon.addon.ID, l.catalog.Type, l.catalog.ID, l.genre, string(listed)).Scan(&found); err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}
