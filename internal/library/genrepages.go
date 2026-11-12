package library

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Genres' pages.
//
// A genre's page lists (see GenreTitles):
//   - the titles the user's catalogs list for an option of its name (see
//     Narrowed);
//   - then the movies and series Polyfin keeps of that genre that the user
//     reaches, keptMost at most, the most recently seen, by name: those
//     whose genres, as their catalog's preview or the complete description
//     by one of the server's addons gave them, or as an administrator set
//     them, name it, whatever their letter case. An administrator's genres
//     win over the addons'.
//
// Many catalogs offer no genre filter, or name genres otherwise than the
// descriptions their titles' pages show ("Drame" and "Drama"): a genre's
// page then lists the titles Polyfin kept of it. Each title is listed
// once. Parental control, blocked genres, the user's hidden titles and the
// movies waiting for their digital release apply as on every listing.
//
// What it costs: one read of the items table by the genres an index keeps
// of each movie and series (polyfin_title_genres, migration 0242), and
// the kept records of the titles found, no request to any addon.

// genreNamesMost bounds the names GenreNames returns.
const genreNamesMost = 50_000

// GenreTitles lists the titles of the given kinds of the genres named
// names (see above), [start, start+count): those of the user's catalogs
// for a genre option matches matches, as Narrowed lists them, then the
// titles kept of those genres (see namedTitles).
func (s *Service) GenreTitles(ctx context.Context, user accounts.User, names []string, matches func(option string) bool, kinds []Kind,
	start, count int) (Page, error) {
	return s.namedTitles(ctx, user, matches, kinds, start, count, func(v view, listed []Item) []Item {
		return s.genreExtras(ctx, v, names, kinds, listed)
	})
}

// genreExtras returns the titles of the given kinds kept of the genres
// named names that listed, the catalogs' titles, do not hold, by name.
func (s *Service) genreExtras(ctx context.Context, v view, names []string, kinds []Kind, listed []Item) []Item {
	names = slices.DeleteFunc(slices.Clone(names), func(name string) bool { return strings.TrimSpace(name) == "" })
	if len(names) == 0 || !slices.Contains(kinds, KindMovie) && !slices.Contains(kinds, KindSeries) {
		return nil
	}
	shown := newShownTitles(listed)
	extras := shown.byName(s.titleItems(ctx, v, s.genreKept(ctx, v, names, kinds), kinds))
	s.translate(ctx, v, extras)
	return s.overridden(extras)
}

// genreKept returns the records of the titles of the given kinds whose
// kept genres name one of names, the most recently seen first, or whose
// genres an administrator set to one of them, as keptTitles keeps them.
// The names fold as the index folds the titles' genres.
func (s *Service) genreKept(ctx context.Context, v view, names []string, kinds []Kind) []record {
	var ids []accounts.ID
	for id, o := range s.overridesNow().fields {
		if o.Genres != nil && namedAny(o.Genres, names) {
			ids = append(ids, id)
		}
	}
	kindNames := make([]string, len(kinds))
	for i, kind := range kinds {
		kindNames[i] = string(kind)
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM items
		WHERE kind IN ('movie', 'series') AND kind = ANY($2::text[])
			AND polyfin_title_genres(data) && polyfin_title_genres(jsonb_build_object('genres', to_jsonb($1::text[])))
		ORDER BY updated_at DESC LIMIT $3`, names, kindNames, keptMost)
	if err == nil {
		var kept []accounts.ID
		kept, err = pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		ids = append(ids, kept...)
	}
	if err != nil {
		s.logger.Debug("The titles of a genre could not be read", "error", err)
	}
	return s.keptTitles(ctx, v, ids, kinds, func(o Overrides) bool { return o.Genres != nil && !namedAny(o.Genres, names) })
}

// GenreNames returns the names of the genres of the movies and series
// Polyfin keeps, and those an administrator set, each once whatever its
// case as most of its titles spell it, those of the most titles first,
// genreNamesMost at most. It reads every title.
func (s *Service) GenreNames(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT genre FROM (
			SELECT btrim(genre) AS genre, count(*) AS titles
			FROM items, jsonb_array_elements_text(
				CASE WHEN jsonb_typeof(data->'genres') = 'array' THEN data->'genres' ELSE '[]' END
				|| CASE WHEN jsonb_typeof(data->'meta'->'genres') = 'array' THEN data->'meta'->'genres' ELSE '[]' END) AS genre
			WHERE kind IN ('movie', 'series')
			GROUP BY 1) AS kept
		WHERE genre <> ''
		ORDER BY titles DESC, genre COLLATE "C" LIMIT $1`, genreNamesMost)
	if err != nil {
		return nil, err
	}
	kept, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if name = strings.TrimSpace(name); name != "" && !seen[strings.ToLower(name)] {
			seen[strings.ToLower(name)] = true
			names = append(names, name)
		}
	}
	for _, name := range kept {
		add(name)
	}
	for _, o := range s.overridesNow().fields {
		for _, genre := range o.Genres {
			add(genre)
		}
	}
	return names, nil
}
