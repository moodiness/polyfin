package tmdb

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strings"
)

// TitleArtwork is the main poster and backdrop of a movie or series: their
// paths on the image server, each empty when TMDB has none.
type TitleArtwork struct {
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
}

// TitleArtwork returns the main poster and backdrop of the movie or series
// (kind) TMDB identifies as id, for language (an ISO 639-1 code, with a
// region or not). In English, or without a language, they are those of
// the title's details, which Studios and MovieCollection read too: one
// answer serves them all. In another language, the details come with the
// title's images in that language, in English and without text, in the
// same request, and each image is chosen as Jellyfin 12.4 chooses one
// (EnumerableExtensions.cs, OrderByLanguageDescending): the poster in
// that language first, then in English, then without text, the best rated
// and then the most voted of each; the backdrop without text first, as
// Jellyfin downloads backdrops (ItemImageProvider.cs, line 333). A title
// without such an image keeps the one of its details.
func (c *Client) TitleArtwork(ctx context.Context, kind Kind, id, language string) (TitleArtwork, error) {
	base := strings.ToLower(strings.TrimSpace(language))
	base, _, _ = strings.Cut(base, "-")
	path := "/" + string(kind) + "/" + url.PathEscape(id)
	if base == "" || base == "en" {
		var artwork TitleArtwork
		err := c.get(ctx, path, nil, &artwork)
		return artwork, err
	}
	query := url.Values{"language": {language}, "append_to_response": {"images"}, "include_image_language": {base + ",en,null"}}
	var details struct {
		TitleArtwork
		Images TitleImages `json:"images"`
	}
	if err := c.get(ctx, path, query, &details); err != nil {
		return TitleArtwork{}, err
	}
	artwork := details.TitleArtwork
	if best, ok := bestImage(details.Images.Posters, base, "en", ""); ok {
		artwork.PosterPath = best
	}
	if best, ok := bestImage(details.Images.Backdrops, "", base, "en"); ok {
		artwork.BackdropPath = best
	}
	return artwork, nil
}

// bestImage returns the path of the best of images in the first of
// languages ("" for none) any is in: the best rated, then the most voted.
func bestImage(images []Image, languages ...string) (string, bool) {
	for _, language := range languages {
		var in []Image
		for _, image := range images {
			if strings.EqualFold(image.Language, language) && image.Path != "" {
				in = append(in, image)
			}
		}
		if len(in) == 0 {
			continue
		}
		best := slices.MaxFunc(in, func(a, b Image) int {
			return cmp.Or(cmp.Compare(a.Rating, b.Rating), cmp.Compare(a.Votes, b.Votes))
		})
		return best.Path, true
	}
	return "", false
}

// ForgetTitleArtwork forgets the answers kept of the movie or series
// (kind) TMDB identifies as id that TitleArtwork reads, in every language,
// so that it asks TMDB again, as a refresh of the title asks.
func (c *Client) ForgetTitleArtwork(kind Kind, id string) {
	if c == nil {
		return
	}
	path := "/" + string(kind) + "/" + url.PathEscape(id)
	c.answers.DeleteFunc(func(key string, _ answer) bool {
		kept, query, _ := strings.Cut(key, "?")
		return kept == path && (query == "" || strings.Contains(query, "append_to_response=images"))
	})
}
