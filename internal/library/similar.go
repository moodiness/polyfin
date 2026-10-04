package library

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// similarSources bounds the catalogs, hence the catalog requests, one list
// of similar titles reads. Apps ask for it each time a title's page opens.
const similarSources = 8

// Similar lists up to count titles of the same kind as a movie or series,
// closest first. Addons recommend nothing, so the candidates are the titles
// on the first page of catalogs narrowed to one of the title's genres
// through their genre filter, as on genre pages: each catalog once, with
// the first of the title's genres it offers, those of the user's libraries
// first, then the other catalogs of the user's addons. They are ranked with
// Jellyfin's weights (see similarity); candidates that rank equal keep the
// catalogs' order. Other items have no similar titles.
func (s *Service) Similar(ctx context.Context, user accounts.User, id accounts.ID, count int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	title, err := s.item(ctx, v, id)
	if err != nil {
		return nil, err
	}
	if title.Kind != KindMovie && title.Kind != KindSeries || len(title.Genres) == 0 || count <= 0 {
		return nil, nil
	}
	sources := similarSourcesOf(v, title)
	pages := make([][]stremio.Meta, len(sources))
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range sources {
		group.Go(func() error {
			metas, err := s.page(ctx, src, 0)
			// An app that stops waiting cancels ctx: nothing failed.
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("A catalog could not be listed for similar titles", "catalog", src.catalog.ID, "error", err)
			}
			// Titles hidden from the user are no candidates.
			pages[i], _ = s.visibleMetas(ctx, v, src, metas)
			return nil
		})
	}
	_ = group.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	type candidate struct {
		item   Item
		record record
		score  int
	}
	var candidates []candidate
	seen := map[accounts.ID]bool{}
	// One title of each catalog in turn, so that ties keep the catalogs
	// interleaved.
	for rank := 0; ; rank++ {
		added := false
		for i, metas := range pages {
			if rank >= len(metas) {
				continue
			}
			added = true
			item, r, err := listed{metas[rank], sources[i]}.title(accounts.ID{})
			if err != nil || seen[item.ID] || sameTitle(title, item) {
				continue
			}
			seen[item.ID] = true
			// A title already described compares with its credits too.
			if full, ok := s.cachedMeta(r); ok {
				fromMeta(&item, full)
			}
			// Like a search result, the title keeps the folder it was last
			// listed in.
			r.Parent = nil
			candidates = append(candidates, candidate{item, r, similarity(title, item)})
		}
		if !added {
			break
		}
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int { return cmp.Compare(b.score, a.score) })
	candidates = candidates[:min(count, len(candidates))]
	items := make([]Item, 0, len(candidates))
	records := make([]record, 0, len(candidates))
	for _, c := range candidates {
		items, records = append(items, c.item), append(records, c.record)
	}
	return items, s.save(ctx, records)
}

// similarSourcesOf picks the catalogs similar titles are read from: up to
// similarSources catalogs of the title's kind that can be listed without
// user input and offer one of its genres, each narrowed to the first of
// its genres it offers. The catalogs of the user's libraries come first;
// the other catalogs of the user's addons follow, which also covers those
// that collection libraries group, without reading the collections.
func similarSourcesOf(v view, title Item) []source {
	var sources []source
	seen := map[catalogKey]bool{}
	add := func(addon installed, catalog stremio.Catalog) {
		key := catalogKey{addon.addon.ID, catalog.Type, catalog.ID}
		if kind, _ := titleKind(catalog.Type); kind != title.Kind || seen[key] || !catalog.Browsable() || len(sources) >= similarSources {
			return
		}
		seen[key] = true
		for _, extra := range catalog.Extra {
			if extra.Name != "genre" {
				continue
			}
			for _, genre := range title.Genres {
				i := slices.IndexFunc(extra.Options, func(option string) bool {
					return strings.EqualFold(strings.TrimSpace(option), strings.TrimSpace(genre))
				})
				if i >= 0 {
					sources = append(sources, source{addon: addon, catalog: catalog, genre: extra.Options[i]})
					return
				}
			}
			return
		}
	}
	for _, l := range v.libraries {
		add(l.addon, l.catalog)
	}
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			add(entry, catalog)
		}
	}
	return sources
}

// sameTitle reports whether a candidate is the title itself, possibly
// listed under another Stremio identifier.
func sameTitle(title, candidate Item) bool {
	imdb := title.ProviderIDs["Imdb"]
	return candidate.ID == title.ID || imdb != "" && candidate.ProviderIDs["Imdb"] == imdb
}

// similarity scores how close a candidate is to a title, with the weights
// of Jellyfin's own similar titles: 10 per shared genre, 50 per shared
// director and 15 per shared actor. Release years five years apart or less
// add up to 5 more, the closer the more.
func similarity(title, candidate Item) int {
	score := 0
	for _, genre := range candidate.Genres {
		if slices.ContainsFunc(title.Genres, func(other string) bool { return strings.EqualFold(other, genre) }) {
			score += 10
		}
	}
	weights := map[string]int{"Director": 50, "Actor": 15}
	for _, person := range candidate.People {
		if weight := weights[person.Type]; weight > 0 &&
			slices.ContainsFunc(title.People, func(other Person) bool { return other.ID == person.ID && other.Type == person.Type }) {
			score += weight
		}
	}
	if title.ProductionYear > 0 && candidate.ProductionYear > 0 {
		gap := title.ProductionYear - candidate.ProductionYear
		score += max(0, 5-max(gap, -gap))
	}
	return score
}
