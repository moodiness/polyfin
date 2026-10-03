package library

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	// similarGenres is how many of a title's genres, its first ones, similar
	// titles are looked for in.
	similarGenres = 3
	// similarSources bounds the catalogs read for similar titles.
	similarSources = 12
	// similarCandidates is how many titles of those catalogs are ranked.
	similarCandidates = 120
)

// Similar lists up to count titles of the same kind as a movie or series,
// closest first. Addons recommend nothing, so the candidates are the titles
// the user's catalogs list for the title's genres, as on genre pages (see
// Narrowed), mostly from their first pages, which genre pages share. They
// are ranked with Jellyfin's weights (see similarity); candidates that rank
// equal keep the catalogs' order. Other items have no similar titles.
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
	catalogs, err := s.titleCatalogs(ctx, v)
	if err != nil {
		return nil, err
	}
	genres := title.Genres[:min(len(title.Genres), similarGenres)]
	var sources []source
	for _, c := range catalogs {
		if kind, _ := titleKind(c.catalog.Type); kind != title.Kind {
			continue
		}
		for _, extra := range c.catalog.Extra {
			if extra.Name != "genre" {
				continue
			}
			for _, genre := range genres {
				i := slices.IndexFunc(extra.Options, func(option string) bool {
					return strings.EqualFold(strings.TrimSpace(option), strings.TrimSpace(genre))
				})
				if i >= 0 && len(sources) < similarSources {
					sources = append(sources, source{addon: c.addon, catalog: c.catalog, genre: extra.Options[i]})
				}
			}
			break
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	metas, _, err := s.merged(ctx, sources, 0, similarCandidates)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		item   Item
		record record
		score  int
	}
	var candidates []candidate
	for _, meta := range metas {
		src := sources[0]
		for _, other := range sources {
			if other.catalog.Type == meta.Type {
				src = other
				break
			}
		}
		item, r, err := titleItem(src.addon.addon.ID, src.catalog, meta, accounts.ID{}, src.addon.confined)
		if err != nil || sameTitle(title, item) {
			continue
		}
		// A title already described compares with its credits too.
		if full, ok := s.cachedMeta(r); ok {
			fromMeta(&item, full)
		}
		// Like a search result, the title keeps the folder it was last
		// listed in.
		r.Parent = nil
		candidates = append(candidates, candidate{item, r, similarity(title, item)})
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
