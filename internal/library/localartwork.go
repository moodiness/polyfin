package library

import (
	"context"
	"io"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Local artwork.
//
// Local folders of movies and shows may hold artwork beside their files:
// posters, backdrops, logos and the like, of a movie, a show or a season,
// and an episode's image (see package localfiles). An image of a type
// replaces the addon's image of that type for its title wherever the title
// shows, the other types still coming from the addon, and an episode's
// image its still; artwork an administrator uploaded wins over both.
// Seasons and episodes show their series' local poster and backdrop as
// they show its addon's, and episodes their season's local poster. In
// Images, a local image is a URL of its own (see LocalArtworkURL), which
// Polyfin serves by opening the image in its folder (see
// OpenLocalArtwork): no path nor address of the server's reaches apps.

// localScheme starts the URL standing for a local folder's image in
// Images.
const localScheme = "polyfin-local:"

// LocalArtworkURL stands for a local folder's image in Images: key opens
// it (see Local.Open), and version changes with the image, which changes
// its ImageTag.
func LocalArtworkURL(key, version string) string { return localScheme + key + "?v=" + version }

// LocalArtworkKey reports whether an artwork URL stands for a local
// folder's image, and the key that opens it.
func LocalArtworkKey(url string) (string, bool) {
	rest, ok := strings.CutPrefix(url, localScheme)
	if !ok {
		return "", false
	}
	key, _, _ := strings.Cut(rest, "?")
	return key, true
}

// OpenLocalArtwork opens a local folder's image by its key (see
// LocalArtworkKey), from its folder or share; ErrNotFound without local
// folders.
func (s *Service) OpenLocalArtwork(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.local == nil {
		return nil, ErrNotFound
	}
	file, _, err := s.local.Open(ctx, key)
	return file, err
}

// TitleItemID is the item of a title of kind, KindMovie or KindSeries, by
// its Stremio identifier.
func TitleItemID(kind Kind, stremioID string) accounts.ID { return itemID(titleKey(kind, stremioID)) }

// SeasonItemID is the item of a season of the series of a Stremio
// identifier.
func SeasonItemID(seriesStremioID string, season int) accounts.ID {
	return itemID(seasonKey(seriesStremioID, season))
}

// EpisodeItemID is the item of the episode of a video identifier, as the
// series' metadata addon lists it.
func EpisodeItemID(videoID string) accounts.ID { return itemID(episodeKey(videoID)) }

// localArtwork returns the artwork of local folders, by item; none
// without local folders.
func (s *Service) localArtwork() map[accounts.ID]Images {
	if s.local == nil {
		return nil
	}
	return s.local.Artwork()
}

// withLocal returns images with those of local in their place, for each
// type local has.
func withLocal(images, local Images) Images {
	for _, pair := range []struct{ from, to *string }{{&local.Primary, &images.Primary}, {&local.Backdrop, &images.Backdrop},
		{&local.Logo, &images.Logo}, {&local.Thumb, &images.Thumb}, {&local.Banner, &images.Banner}, {&local.Art, &images.Art},
		{&local.Disc, &images.Disc}} {
		if *pair.from != "" {
			*pair.to = *pair.from
		}
	}
	return images
}

// applyLocal sets on item the local artwork of it and of the items it
// shows: its series' poster and backdrop, as a season without a poster of
// its own shows them, and its season's poster.
func applyLocal(item *Item, local map[accounts.ID]Images) {
	own := local[item.ID]
	seriesPoster := item.SeriesPoster
	item.Images = withLocal(item.Images, own)
	if item.SeriesID != (accounts.ID{}) {
		series := local[item.SeriesID]
		if series.Primary != "" {
			if item.Kind == KindSeason && own.Primary == "" && item.Images.Primary == seriesPoster {
				item.Images.Primary = series.Primary
			}
			item.SeriesPoster = series.Primary
		}
		if series.Backdrop != "" && own.Backdrop == "" {
			item.Images.Backdrop = series.Backdrop
		}
	}
	if item.SeasonID != (accounts.ID{}) {
		if poster := local[item.SeasonID].Primary; poster != "" {
			item.SeasonPoster = poster
		}
	}
}
