package trackers

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// A user who turns it on has their watchlist on Trakt, or their Plan to
// Watch list on Simkl, read whole with each import of the service, and
// put on their Polyfin watchlist (see package watchlist): its movies and
// series, a season or an episode listed counting for its series. Titles
// are found as the history's are; those not found count among the titles
// not found. An import that reads the list whole takes off the watchlist
// what the list no longer holds, unless another list holds it; one that
// could not read it whole only adds what it read.

// listedEntry is a movie or series a watchlist holds, and when it was
// added there, nil when the service does not tell.
type listedEntry struct {
	ref library.TitleRef
	at  *time.Time
}

// watchlistRead is what an import read of a watchlist: whole once it read
// it all, and how many titles it names that it could not name by any
// identifier Polyfin finds titles by.
type watchlistRead struct {
	entries []listedEntry
	whole   bool
	unknown int
}

// add adds the title ref names, added at at, unless it has no identifier
// Polyfin finds titles by: then it counts as unknown.
func (l *watchlistRead) add(ref library.TitleRef, at *time.Time) {
	if !usable(ref) && ref.Anime == (library.AnimeRef{}) {
		l.unknown++
		return
	}
	l.entries = append(l.entries, listedEntry{ref: ref, at: at})
}

// seriesRef names the series t is.
func (t titleJSON) seriesRef() library.TitleRef {
	ref := t.ref(false, 0, 0)
	ref.Series = true
	return ref
}

// readWatchlist reads the watchlist of the connection whole.
func (r *reader) readWatchlist(ctx context.Context) (watchlistRead, error) {
	if r.key.service == Trakt {
		return r.readTraktWatchlist(ctx)
	}
	return r.readSimklWatchlist(ctx)
}

// readTraktWatchlist reads Trakt's watchlist, its movies, shows, seasons
// and episodes, a page at a time.
func (r *reader) readTraktWatchlist(ctx context.Context) (watchlistRead, error) {
	var list watchlistRead
	for page := 1; ; page++ {
		answer, err := r.get(ctx, "/sync/watchlist", url.Values{"page": {strconv.Itoa(page)}, "limit": {"250"}})
		if err != nil {
			return list, err
		}
		var items []struct {
			ListedAt string     `json:"listed_at"`
			Type     string     `json:"type"`
			Movie    *titleJSON `json:"movie"`
			Show     *titleJSON `json:"show"`
		}
		if json.Unmarshal(answer.body, &items) != nil {
			return list, ErrUnreachable
		}
		for _, item := range items {
			switch {
			case item.Type == "movie" && item.Movie != nil:
				list.add(item.Movie.ref(false, 0, 0), date(item.ListedAt))
			case item.Show != nil:
				// A show, or one of its seasons or episodes.
				list.add(item.Show.seriesRef(), date(item.ListedAt))
			}
		}
		pages, _ := strconv.Atoi(answer.header.Get("X-Pagination-Page-Count"))
		if len(items) == 0 || page >= pages {
			break
		}
	}
	list.whole = true
	return list, nil
}

// simklPlanned are the movies, shows and anime of a Plan to Watch answer
// of Simkl.
type simklPlanned struct {
	Movies []struct {
		AddedAt string    `json:"added_to_watchlist_at"`
		Movie   titleJSON `json:"movie"`
	} `json:"movies"`
	Shows []struct {
		AddedAt string    `json:"added_to_watchlist_at"`
		Show    titleJSON `json:"show"`
	} `json:"shows"`
	Anime []struct {
		simklAnime
		AddedAt string `json:"added_to_watchlist_at"`
	} `json:"anime"`
}

// readSimklWatchlist reads Simkl's Plan to Watch list: its movies, shows
// and anime. Anime is found through the anime mapping: until the mapping is
// read, it is left for a later import, and the list is not read whole.
func (r *reader) readSimklWatchlist(ctx context.Context) (watchlistRead, error) {
	var list watchlistRead
	mapping := animeEntry{mapping: r.s.anime, unknown: map[string]bool{}}
	mapped := r.s.anime.Ready()
	for _, kind := range []string{"movies", "shows", "anime"} {
		if kind == "anime" && !mapped {
			continue
		}
		answer, err := r.get(ctx, "/sync/all-items/"+kind+"/plantowatch", url.Values{"extended": {"full"}})
		if err != nil {
			return list, err
		}
		var items simklPlanned
		// An empty list is {} or null.
		if json.Unmarshal(answer.body, &items) != nil && strings.TrimSpace(string(answer.body)) != "null" {
			return list, ErrUnreachable
		}
		for _, movie := range items.Movies {
			list.add(movie.Movie.ref(false, 0, 0), date(movie.AddedAt))
		}
		for _, show := range items.Shows {
			list.add(show.Show.seriesRef(), date(show.AddedAt))
		}
		for _, item := range items.Anime {
			a := item.Anime
			if a == nil {
				a = item.Show
			}
			if a == nil {
				continue
			}
			movie := strings.EqualFold(cmpOr(a.AnimeType, item.AnimeType), "movie")
			// A series is named by its first episode's mapping.
			ref, ok := mapping.ref(*a, movie, 1, 1)
			if !ok {
				continue
			}
			if !movie {
				ref.Episode, ref.Series, ref.Season, ref.Number, ref.Absolute, ref.Anime.Episode = false, true, 0, 0, false, 0
			}
			list.add(ref, date(item.AddedAt))
		}
	}
	list.unknown += len(mapping.unknown)
	list.whole = mapped
	return list, nil
}

// mergeWatchlist finds the titles of list and saves them as the
// connection's watchlist, counting what it adds and what it does not find.
// Of the titles found for one entry, one an anime catalog lists counts
// only when nothing else was found, as in the history.
func (s *Service) mergeWatchlist(ctx context.Context, r *reader, list watchlistRead, result *ImportResult) error {
	refs := make([]library.TitleRef, len(list.entries))
	for i, e := range list.entries {
		refs[i] = e.ref
	}
	targets, err := s.resolve(ctx, r, refs)
	if err != nil {
		return err
	}
	var titles []watchlist.Title
	unmapped := map[library.TitleRef]bool{}
	for i, e := range list.entries {
		if len(targets[i]) == 0 {
			unmapped[e.ref] = true
		}
		for _, t := range targets[i] {
			if !counts(targets[i], t) {
				continue
			}
			id := t.ID
			if t.Series != (accounts.ID{}) {
				id = t.Series
			}
			titles = append(titles, watchlist.Title{ID: id, AddedAt: e.at})
		}
	}
	result.Unmapped += len(unmapped) + list.unknown
	result.Watchlist, err = s.watchlist.Save(ctx, r.key.user, watchlist.Tracking(r.key.service), titles, list.whole)
	return err
}
