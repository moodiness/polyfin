package library

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// The titles of a genre's or studio's page: those of the user's catalogs
// for an option of its name (see Narrowed), then, once the catalogs are
// read through, those its stages add (see GenreTitles and StudioTitles),
// each title listed once.

// keptMost bounds the titles a genre's or studio's page lists from those
// Polyfin keeps.
const keptMost = 1000

// namedTitles lists the titles of the given kinds of a page, [start,
// start+count): those of the user's catalogs for a genre option matches
// matches, as Narrowed lists them, then those extras returns that listed,
// the catalogs' titles, do not hold. Total counts them all once the
// catalogs are read through; while they have more, it counts those read,
// plus one.
func (s *Service) namedTitles(ctx context.Context, user accounts.User, matches func(option string) bool, kinds []Kind, start, count int,
	extras func(v view, listed []Item) []Item) (Page, error) {
	start = max(start, 0)
	need := start + count
	catalogs, err := s.Narrowed(ctx, user, matches, kinds, 0, need)
	if err != nil {
		return Page{}, err
	}
	if catalogs.More {
		return Page{Items: window(catalogs.Items, start, need), Total: catalogs.Total, More: true}, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	all := slices.Concat(catalogs.Items, extras(v, catalogs.Items))
	return Page{Items: window(all, start, need), Total: len(all)}, nil
}

// window returns items [start, end), within items.
func window(items []Item, start, end int) []Item {
	start, end = min(start, len(items)), min(end, len(items))
	return items[start:end]
}

// shownTitles are the titles a page listed: by identifier, by the keys of
// their IMDb and TMDB identifiers (see listedKeys), and by their TMDB
// identifiers.
type shownTitles struct {
	ids  map[accounts.ID]bool
	keys map[string]bool
	tmdb map[tmdbTitle]bool
}

// newShownTitles returns the titles of listed as shown.
func newShownTitles(listed []Item) shownTitles {
	shown := shownTitles{ids: map[accounts.ID]bool{}, keys: map[string]bool{}, tmdb: map[tmdbTitle]bool{}}
	for _, item := range listed {
		shown.add(item)
	}
	return shown
}

// add notes item as shown, and reports whether it was not yet.
func (shown shownTitles) add(item Item) bool {
	keys := listedKeys(item)
	if shown.ids[item.ID] || slices.ContainsFunc(keys, func(key string) bool { return shown.keys[key] }) {
		return false
	}
	shown.ids[item.ID] = true
	for _, key := range keys {
		shown.keys[key] = true
	}
	kind := tmdb.Movie
	if item.Kind == KindSeries {
		kind = tmdb.TV
	}
	if id, err := strconv.Atoi(item.ProviderIDs["Tmdb"]); err == nil && id > 0 {
		shown.tmdb[tmdbTitle{kind, id}] = true
	}
	return true
}

// byName returns the titles of items not shown yet, by name, and notes
// them as shown.
func (shown shownTitles) byName(items []Item) []Item {
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return slices.DeleteFunc(items, func(item Item) bool { return !shown.add(item) })
}

// namedAny reports whether one of list is one of names, whatever their
// letter case and the spaces around them.
func namedAny(list, names []string) bool {
	return slices.ContainsFunc(list, func(listed string) bool {
		return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(strings.TrimSpace(listed), strings.TrimSpace(name)) })
	})
}

// keptTitles returns the records of the movies and series of ids of the
// given kinds that the user reaches, has not hidden and may see, keptMost
// at most, but those whose edits by an administrator edited leaves out:
// parental control and blocked genres, then the wait for a movie's
// digital release, apply as on every listing.
func (s *Service) keptTitles(ctx context.Context, v view, ids []accounts.ID, kinds []Kind, edited func(o Overrides) bool) []record {
	slices.SortFunc(ids, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
	ids = slices.Compact(ids)
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		s.logger.Debug("The titles kept could not be read", "error", err)
		return nil
	}
	set := s.overridesNow()
	records = slices.DeleteFunc(records, func(r record) bool {
		if r.Meta == nil || r.Kind != KindMovie && r.Kind != KindSeries || !slices.Contains(kinds, r.Kind) || v.hidden[r.ID] || !v.reaches(r) {
			return true
		}
		o, ok := set.fields[r.ID]
		return ok && edited(o)
	})
	records = records[:min(len(records), keptMost)]
	records, _ = s.visible(ctx, v, records)
	records, _ = s.released(ctx, v, records)
	return records
}
