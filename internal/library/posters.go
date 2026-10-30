package library

import (
	"net/url"
	"strings"
)

// The poster address template.
//
// With Settings.PosterTemplate set, a movie's or a series' Primary image
// is the address the template gives with its IMDb or TMDB identifier and
// its type, in place of its addon's poster: {imdb_id}, {tmdb_id}, {type},
// "movie" or "series", and {tmdb_type}, TMDB's "movie" or "tv", are
// filled in. A title lacking an
// identifier the template names keeps its addon's poster, and so does a
// title whose templated poster cannot be downloaded (see
// Artwork.Fallback). A local folder's artwork and an administrator's
// upload still come first.

// PosterAddress fills template for a title of kind, by its provider
// identifiers; ok is false when kind is neither a movie nor a series, or
// the title lacks an identifier the template names.
func PosterAddress(template string, kind Kind, ids map[string]string) (string, bool) {
	if template == "" || kind != KindMovie && kind != KindSeries {
		return "", false
	}
	kindName, tmdbKind := "movie", "movie"
	if kind == KindSeries {
		kindName, tmdbKind = "series", "tv"
	}
	address := template
	for _, field := range []struct{ name, value string }{{"{imdb_id}", ids["Imdb"]}, {"{tmdb_id}", ids["Tmdb"]}, {"{type}", kindName},
		{"{tmdb_type}", tmdbKind}} {
		if !strings.Contains(address, field.name) {
			continue
		}
		if field.value == "" {
			return "", false
		}
		address = strings.ReplaceAll(address, field.name, url.PathEscape(field.value))
	}
	return address, true
}

// templatePoster sets an item's Primary image to the poster template
// gives it, if any; it returns the poster it replaced, and whether it
// replaced it.
func templatePoster(item *Item, template string) (replaced string, ok bool) {
	address, ok := PosterAddress(template, item.Kind, item.ProviderIDs)
	if !ok {
		return "", false
	}
	replaced = item.Images.Primary
	item.Images.Primary = address
	item.PosterShape = ""
	return replaced, true
}
