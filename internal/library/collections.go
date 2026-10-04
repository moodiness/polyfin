package library

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
)

// What users' collections need of the titles they hold. A collection holds
// titles from any library; these read what Polyfin remembers of them, and
// ask no addon.

// maxListingDepth bounds the folders ListedUnder climbs: an episode's
// series, the collections of an addon above it, then a library.
const maxListingDepth = 6

// ListedUnder reports whether any of the items was last listed under
// folder: in it, or in a series or addon collection listed there.
func (s *Service) ListedUnder(ctx context.Context, ids []accounts.ID, folder accounts.ID) (bool, error) {
	seen := map[accounts.ID]bool{}
	for depth := 0; len(ids) > 0 && depth < maxListingDepth; depth++ {
		records, err := s.loadAll(ctx, ids)
		if err != nil {
			return false, err
		}
		ids = nil
		for _, r := range records {
			parent := deref(r.Parent)
			if r.Kind == KindSeason || r.Kind == KindEpisode {
				parent = r.seriesItemID()
			}
			if parent == folder {
				return true, nil
			}
			if parent != (accounts.ID{}) && !seen[parent] {
				seen[parent] = true
				ids = append(ids, parent)
			}
		}
	}
	return false, nil
}

// Poster returns the URL of the poster an item shows in a collection, and
// whether downloading it is confined to public addresses: an episode shows
// its series' poster, or its own image when the series has none; anything
// else, its Primary image. Like Artwork, it needs no user.
func (s *Service) Poster(ctx context.Context, id accounts.ID) (string, bool, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return "", false, err
	}
	if r.Kind == KindEpisode {
		url, confined, err := s.Artwork(ctx, r.seriesItemID(), "Primary")
		if !errors.Is(err, ErrNotFound) {
			return url, confined, err
		}
	}
	return s.Artwork(ctx, id, "Primary")
}

// CollectionPoster is the poster item shows in a collection, as Poster
// returns it: an episode's series poster, else its Primary image.
func CollectionPoster(item Item) string {
	if item.Kind == KindEpisode && item.SeriesPoster != "" {
		return item.SeriesPoster
	}
	return item.Images.Primary
}
