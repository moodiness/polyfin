package trackers

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// A user who turns it on has their watchlist on Trakt, MDBList or
// PublicMetaDB, or their Plan to Watch list on Simkl, read whole with each
// import of the service, and put on their Polyfin watchlist (see package
// watchlist): its movies and series, a season or an episode listed
// counting for its series. MDBList's is read only once it changed since
// the last import read it. Titles are found as the history's are; those
// not found count among the titles not found. An import that reads the
// list whole takes off the watchlist what the list no longer holds, unless
// another list holds it; one that could not read it whole, or did not read
// it, only adds what it read.

// listedEntry is a movie or series a watchlist holds, and when it was
// added there, nil when the service does not tell.
type listedEntry struct {
	ref library.TitleRef
	at  *time.Time
}

// watchlistRead is what an import read of a watchlist: whole once it read
// it all, and how many titles it names that it could not name by any
// identifier Polyfin finds titles by. cursor tells when the list last
// changed, for the services that tell it: the next import reads it only
// once that changed.
type watchlistRead struct {
	entries []listedEntry
	whole   bool
	unknown int
	cursor  string
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

// readWatchlist reads the watchlist of the connection whole, or, when
// cursor tells it did not change since, not at all.
func (r *reader) readWatchlist(ctx context.Context, cursor string) (watchlistRead, error) {
	switch r.key.service {
	case Trakt:
		return r.readTraktWatchlist(ctx)
	case AniList, MyAnimeList:
		return r.readAnimeWatchlist(ctx)
	case PublicMetaDB:
		return r.readPublicMetaDBWatchlist(ctx)
	case MDBList:
		return r.readMDBListWatchlist(ctx, cursor)
	}
	return r.readSimklWatchlist(ctx)
}

// asSeries makes the ref of an anime series' first episode name the
// series.
func asSeries(ref library.TitleRef) library.TitleRef {
	ref.Episode, ref.Series, ref.Season, ref.Number, ref.Absolute, ref.Anime.Episode = false, true, 0, 0, false, 0
	return ref
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

// readPublicMetaDBWatchlist reads PublicMetaDB's watchlist, its movies and
// series, a page at a time. It is read whole only once found among the
// user's lists and read to its last page; the list is never created here.
func (r *reader) readPublicMetaDBWatchlist(ctx context.Context) (watchlistRead, error) {
	var list watchlistRead
	var id string
	for page := 1; id == ""; page++ {
		answer, err := r.get(ctx, "/api/external/lists", url.Values{"page": {strconv.Itoa(page)}, "perPage": {"500"}})
		if err != nil {
			return list, err
		}
		var lists struct {
			Items []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"items"`
			TotalPages int `json:"totalPages"`
		}
		if json.Unmarshal(answer.body, &lists) != nil {
			return list, ErrUnreachable
		}
		for _, l := range lists.Items {
			if l.Type == "watchlist" && l.ID != "" {
				id = l.ID
				break
			}
		}
		if len(lists.Items) == 0 || page >= lists.TotalPages {
			break
		}
	}
	if id == "" {
		// No watchlist yet: nothing is taken off.
		return list, nil
	}
	for page := 1; ; page++ {
		answer, err := r.get(ctx, "/api/external/lists/"+url.PathEscape(id)+"/items",
			url.Values{"page": {strconv.Itoa(page)}, "perPage": {"500"}})
		if err != nil {
			return list, err
		}
		var items struct {
			Items []struct {
				publicMetaDBListItem
				CreatedAt string `json:"created_at"`
			} `json:"items"`
			TotalPages int `json:"totalPages"`
		}
		if json.Unmarshal(answer.body, &items) != nil {
			return list, ErrUnreachable
		}
		for _, item := range items.Items {
			ref := library.TitleRef{TMDB: int(item.TMDB)}
			switch item.MediaType {
			case "movie":
			case "tv":
				ref.Series = true
			default:
				continue
			}
			list.add(ref, date(item.CreatedAt))
		}
		if len(items.Items) == 0 || page >= items.TotalPages {
			break
		}
	}
	list.whole = true
	return list, nil
}

// mdblistListed is a movie or show of MDBList's watchlist: its identifiers
// under ids, or beside it, id being TMDB's.
type mdblistListed struct {
	titleJSON
	ID     flexInt `json:"id"`
	IMDbID string  `json:"imdb_id"`
	TVDBID flexInt `json:"tvdb_id"`
}

func (m mdblistListed) ref() library.TitleRef {
	ref := m.titleJSON.ref(false, 0, 0)
	ref.IMDb = cmpOr(ref.IMDb, m.IMDbID)
	ref.TMDB = cmpOr(ref.TMDB, int(m.ID))
	ref.TVDB = cmpOr(ref.TVDB, int(m.TVDBID))
	return ref
}

// readMDBListWatchlist reads MDBList's watchlist, its movies and shows,
// the latest added first, a page at a time, unless its last activities
// tell it did not change since cursor. MDBList gives no date for them.
func (r *reader) readMDBListWatchlist(ctx context.Context, cursor string) (watchlistRead, error) {
	list := watchlistRead{cursor: cursor}
	answer, err := r.get(ctx, "/sync/last_activities", nil)
	if err != nil {
		return list, err
	}
	var activities struct {
		WatchlistedAt string `json:"watchlisted_at"`
	}
	if json.Unmarshal(answer.body, &activities) != nil {
		return list, ErrUnreachable
	}
	if cursor != "" && activities.WatchlistedAt == cursor {
		// Unchanged: nothing is read, nor taken off.
		return list, nil
	}
	query := url.Values{"limit": {"1000"}, "sort": {"added"}, "order": {"desc"}}
	for {
		answer, err := r.get(ctx, "/watchlist/items", query)
		if err != nil {
			return list, err
		}
		var page struct {
			Movies     []mdblistListed `json:"movies"`
			Shows      []mdblistListed `json:"shows"`
			NextCursor string          `json:"next_cursor"`
			Pagination struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		if json.Unmarshal(answer.body, &page) != nil {
			return list, ErrUnreachable
		}
		for _, movie := range page.Movies {
			list.add(movie.ref(), nil)
		}
		for _, show := range page.Shows {
			ref := show.ref()
			ref.Series = true
			list.add(ref, nil)
		}
		next := cmpOr(page.Pagination.NextCursor, page.NextCursor)
		if next == "" || next == query.Get("cursor") || len(page.Movies)+len(page.Shows) == 0 {
			break
		}
		query.Set("cursor", next)
	}
	list.whole, list.cursor = true, activities.WatchlistedAt
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
				ref = asSeries(ref)
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
// only when nothing else was found, as in the history. The titles linked
// by hand are named by their links. It returns what the import keeps of
// the titles it did not find; those no mapping knows are only counted.
func (s *Service) mergeWatchlist(ctx context.Context, r *reader, list watchlistRead, linked importlinks.Linked,
	result *ImportResult) ([]importlinks.Entry, error) {
	listName := watchlist.Tracking(r.key.service)
	refs := make([]library.TitleRef, len(list.entries))
	entries := make([]importlinks.Entry, len(list.entries))
	for i, e := range list.entries {
		refs[i], entries[i] = keptTitle(linked, e.ref, nil)
		entries[i].Watchlist, entries[i].AddedAt = listName, e.at
	}
	targets, err := s.resolve(ctx, r, refs)
	if err != nil {
		return nil, err
	}
	var titles []watchlist.Title
	var kept []importlinks.Entry
	unmapped := map[library.TitleRef]bool{}
	for i, e := range list.entries {
		if len(targets[i]) == 0 {
			unmapped[refs[i]] = true
			kept = append(kept, entries[i])
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
	result.Watchlist, err = s.watchlist.Save(ctx, r.key.user, listName, titles, list.whole)
	return kept, err
}
