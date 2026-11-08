package library

import (
	"cmp"
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/tmdb"
)

// ContradictedIMDb reports which of candidates, IMDb identifiers a source
// gives for the movie or series ref names by its TMDB identifier,
// Polyfin's own data says are another title's:
//   - a title listed under the candidate with another TMDB identifier;
//   - a TMDB identifier learnt for the candidate (TMDB's external
//     identifiers kept, the credits and box sets looked up) that is
//     another one, or an IMDb identifier learnt for ref's TMDB identifier
//     that is another one;
//   - with a TMDB key, TMDB finding the candidate under another TMDB
//     identifier.
//
// A candidate Polyfin knows nothing of is not contradicted. TMDB failing
// fails the whole, so that nothing is decided without it.
func (s *Service) ContradictedIMDb(ctx context.Context, ref TitleRef, candidates []string) (map[string]bool, error) {
	contradicted := make(map[string]bool, len(candidates))
	if ref.TMDB <= 0 || len(candidates) == 0 {
		return contradicted, nil
	}
	kind, tmdbKind := KindMovie, tmdb.Movie
	if ref.kind() == KindSeries {
		kind, tmdbKind = KindSeries, tmdb.TV
	}
	own := strconv.Itoa(ref.TMDB)
	refs := make([]TitleRef, len(candidates))
	for i, imdb := range candidates {
		refs[i] = TitleRef{Series: kind == KindSeries, IMDb: imdb}
	}
	listed, err := s.knownTitles(ctx, refs)
	if err != nil {
		return nil, err
	}
	for _, title := range listed {
		tmdbID := cmp.Or(title.tmdb, strings.TrimPrefix(title.stremioID, "tmdb:"))
		if title.kind != kind || tmdbID == title.stremioID || tmdbID == own {
			continue
		}
		for _, imdb := range candidates {
			if title.stremioID == imdb || title.imdb == imdb {
				contradicted[imdb] = true
			}
		}
	}
	rows, err := s.db.Query(ctx, `SELECT imdb_id, tmdb_id FROM title_imdb_ids
			WHERE kind = $1 AND imdb_id IS NOT NULL AND (imdb_id = ANY($2::text[]) OR tmdb_id = $3)
		UNION ALL SELECT id, tmdb_id FROM title_credits
			WHERE kind = $1 AND provider = 'imdb' AND tmdb_id IS NOT NULL AND (id = ANY($2::text[]) OR tmdb_id = $3)
		UNION ALL SELECT id, tmdb_id FROM movie_collections
			WHERE $1 = 'movie' AND provider = 'imdb' AND tmdb_id IS NOT NULL AND (id = ANY($2::text[]) OR tmdb_id = $3)`,
		string(tmdbKind), candidates, ref.TMDB)
	if err != nil {
		return nil, err
	}
	var imdb string
	var id int
	if _, err := pgx.ForEachRow(rows, []any{&imdb, &id}, func() error {
		for _, candidate := range candidates {
			// The candidate is another TMDB title, or ref's title is
			// another IMDb one.
			if candidate == imdb && id != ref.TMDB || candidate != imdb && id == ref.TMDB {
				contradicted[candidate] = true
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if !s.tmdb.Enabled() {
		return contradicted, nil
	}
	for _, imdb := range candidates {
		if contradicted[imdb] {
			continue
		}
		found, err := s.tmdb.FindByIMDb(ctx, tmdbKind, imdb)
		switch {
		case errors.Is(err, tmdb.ErrNotFound):
		case err != nil:
			return nil, err
		case found != own:
			contradicted[imdb] = true
		}
	}
	return contradicted, nil
}
