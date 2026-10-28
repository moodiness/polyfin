package localfiles

import (
	"context"
	"errors"
	"strconv"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Matching through TMDB when the metadata addons cannot search.
//
// A file's title is searched with the metadata addons (see unitMatch).
// When every addon's search failed, rather than answered that it knows no
// such title, and the server has a TMDB API key, TMDB is searched for it,
// as Jellyfin's TMDB providers search a title by its name and year
// (MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs, lines
// 198-214, and Movies/TmdbMovieProvider.cs, lines 160-176, at v12.4). The
// title is kept by the exact rule a scan keeps an addon's title by first
// (see chooseItem): the one title of its name, kind and year, when the
// name gives one. The file is then matched to that title's IMDb
// identifier, from TMDB's external identifiers as Jellyfin takes it
// (TmdbSeriesProvider.cs, lines 306-310), as a link would match it:
// named and pictured by the addons once they answer again, and by TMDB's
// name, year and poster meanwhile. A title TMDB does not find, finds
// several of, or knows no IMDb identifier of, leaves the file unmatched
// as before, its search failed, so that the next scan searches it again.
// Only a search every addon failed is asked of TMDB: one an addon
// answered, finding nothing, is not.

// byTMDB is the matched_by of a title TMDB's search found.
const byTMDB = "tmdb"

// UseTMDB sets what searches TMDB for the titles the metadata addons
// cannot search (see searchFallback); it is called before the service is
// used. Without it, or without a key, such titles stay unmatched.
func (s *Service) UseTMDB(c *tmdb.Client) { s.tmdb = c }

// searchFallback is the match of a unit whose search of the metadata
// addons failed with err: the title TMDB finds, when every addon's search
// failed and the server has a TMDB key, else unmatched, the search failed
// (see searches.fail).
func (s *Service) searchFallback(ctx context.Context, kind string, n name, cache *searches, err error) (match, error) {
	failed, ok := errors.AsType[*library.SearchError](err)
	if !ok || !failed.AllFailed() || !s.tmdb.Enabled() {
		return cache.fail(err), nil
	}
	m, found, tmdbErr := s.tmdbSearch(ctx, kind, n)
	if ctx.Err() != nil {
		return match{}, ctx.Err()
	}
	if tmdbErr != nil && !errors.Is(tmdbErr, tmdb.ErrNotFound) {
		s.logger.Warn("TMDB could not be searched for the title of a local folder's files, whose metadata addons failed",
			"title", n.title, "error", tmdbErr)
	}
	if !found {
		return cache.fail(err), nil
	}
	return m, nil
}

// tmdbSearch finds on TMDB the one title of n's name, of the kind of a
// folder, and of its year when it gives one, and its IMDb identifier;
// found is false when there is no such title, several, or one without an
// IMDb identifier.
func (s *Service) tmdbSearch(ctx context.Context, kind string, n name) (m match, found bool, err error) {
	tmdbKind := tmdb.Movie
	if kind == KindShows {
		tmdbKind = tmdb.TV
	}
	titles, err := s.tmdb.SearchTitles(ctx, tmdbKind, n.title, n.year)
	if err != nil {
		return match{}, false, err
	}
	byID := make(map[string]tmdb.FoundTitle, len(titles))
	results := make([]library.RemoteResult, 0, 2*len(titles))
	for _, t := range titles {
		id := strconv.Itoa(t.ID)
		byID[id] = t
		// A title is known by its name and its original name alike.
		for _, named := range []string{t.Name, t.OriginalName} {
			if named != "" {
				results = append(results, library.RemoteResult{Provider: "TMDB", Item: library.Item{Kind: titleKind(kind), Name: named,
					ProductionYear: t.Year, StremioID: "tmdb:" + id, ProviderIDs: map[string]string{"Tmdb": id}}})
			}
		}
	}
	item, _, reason := chooseItem(titleKind(kind), n.title, n.year, results, true)
	if reason != "" {
		return match{}, false, nil
	}
	chosen := byID[item.ProviderIDs["Tmdb"]]
	imdb, err := s.tmdb.IMDbID(ctx, tmdbKind, chosen.ID)
	if err != nil || !imdbPattern.MatchString(imdb) {
		return match{}, false, err
	}
	known := n
	known.title, known.year = chosen.Name, chosen.Year
	m = s.described(ctx, kind, imdb, known, byTMDB)
	m.tmdb = strconv.Itoa(chosen.ID)
	if m.poster == "" && chosen.PosterPath != "" {
		m.poster = s.tmdb.ImageURL(chosen.PosterPath, "original")
	}
	return m, true, nil
}
