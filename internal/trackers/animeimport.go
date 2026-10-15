package trackers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// An import of AniList or MyAnimeList reads the user's list whole, once,
// and makes of it the history, the watchlist and the ratings it imports,
// as turned on. Watching, On hold and Dropped entries count their episodes
// 1 to their progress played; Completed and rewatched ones all of them,
// Completed ones dated with their completion date when the list gives one.
// Plan to Watch goes to the watchlist. Episodes are numbered as the entry
// numbers them and placed through the anime mapping, as Simkl's anime are:
// until the mapping is read, nothing of the list is imported.

// listItem is an entry of a user's AniList or MyAnimeList list.
type listItem struct {
	// id is the service's identifier of the entry.
	id int
	// status is as MyAnimeList names it (see statusWatching).
	status string
	// progress is how many episodes were watched, of total, 0 when
	// unknown.
	progress, total int
	movie           bool
	// score is the user's score, from 0 to 10, 0 for none.
	score float64
	// completed is when the entry was completed, nil when unknown.
	completed *time.Time
}

// maxListPages bounds the pages an import reads of a list.
const maxListPages = 100

// animeList reads the list of the connection, once per import.
func (r *reader) animeList(ctx context.Context) ([]listItem, error) {
	if r.listRead {
		return r.listed, nil
	}
	var items []listItem
	var err error
	if r.key.service == AniList {
		items, err = r.readAniListList(ctx)
	} else {
		items, err = r.readMyAnimeListList(ctx)
	}
	if err != nil {
		return nil, err
	}
	r.listed, r.listRead = items, true
	return items, nil
}

// readAniListList reads the anime list of the AniList user the token is
// for, a chunk of 500 entries at a time.
func (r *reader) readAniListList(ctx context.Context) ([]listItem, error) {
	answer, err := r.fetch(ctx, http.MethodPost, "", nil, map[string]any{"query": `query { Viewer { id } }`})
	if err != nil {
		return nil, err
	}
	var viewer struct {
		Data struct {
			Viewer struct {
				ID int `json:"id"`
			} `json:"Viewer"`
		} `json:"data"`
	}
	if json.Unmarshal(answer.body, &viewer) != nil || viewer.Data.Viewer.ID <= 0 {
		return nil, ErrUnreachable
	}
	const query = `query ($user: Int, $chunk: Int) { MediaListCollection(userId: $user, type: ANIME, chunk: $chunk, perChunk: 500) {
		hasNextChunk lists { entries { status progress score(format: POINT_100) completedAt { year month day } media { id episodes format } } } } }`
	var items []listItem
	for chunk := 1; chunk <= maxListPages; chunk++ {
		answer, err := r.fetch(ctx, http.MethodPost, "", nil, map[string]any{"query": query,
			"variables": map[string]any{"user": viewer.Data.Viewer.ID, "chunk": chunk}})
		if err != nil {
			return nil, err
		}
		var page struct {
			Data struct {
				Collection *struct {
					HasNextChunk bool `json:"hasNextChunk"`
					Lists        []struct {
						Entries []struct {
							Status      string  `json:"status"`
							Progress    int     `json:"progress"`
							Score       float64 `json:"score"`
							CompletedAt struct {
								Year, Month, Day int
							} `json:"completedAt"`
							Media struct {
								ID       int    `json:"id"`
								Episodes int    `json:"episodes"`
								Format   string `json:"format"`
							} `json:"media"`
						} `json:"entries"`
					} `json:"lists"`
				} `json:"MediaListCollection"`
			} `json:"data"`
		}
		if json.Unmarshal(answer.body, &page) != nil || page.Data.Collection == nil {
			return nil, ErrUnreachable
		}
		for _, list := range page.Data.Collection.Lists {
			for _, e := range list.Entries {
				item := listItem{id: e.Media.ID, status: anilistStatuses[e.Status], progress: e.Progress, total: e.Media.Episodes,
					movie: e.Media.Format == "MOVIE", score: e.Score / 10}
				if d := e.CompletedAt; d.Year > 0 && d.Month > 0 && d.Day > 0 {
					item.completed = new(time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC))
				}
				if item.id > 0 && item.status != "" {
					items = append(items, item)
				}
			}
		}
		if !page.Data.Collection.HasNextChunk {
			break
		}
	}
	return items, nil
}

// readMyAnimeListList reads the anime list of the MyAnimeList user the
// token is for, a page of 1,000 entries at a time.
func (r *reader) readMyAnimeListList(ctx context.Context) ([]listItem, error) {
	var items []listItem
	const limit = 1000
	for page := range maxListPages {
		answer, err := r.get(ctx, "/users/@me/animelist", url.Values{"fields": {"list_status,num_episodes,media_type"},
			"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(page * limit)}, "nsfw": {"true"}})
		if err != nil {
			return nil, err
		}
		var list struct {
			Data []struct {
				Node struct {
					ID        int    `json:"id"`
					Episodes  int    `json:"num_episodes"`
					MediaType string `json:"media_type"`
				} `json:"node"`
				Status struct {
					Status     string  `json:"status"`
					Score      float64 `json:"score"`
					Watched    int     `json:"num_episodes_watched"`
					Rewatching bool    `json:"is_rewatching"`
					FinishDate string  `json:"finish_date"`
				} `json:"list_status"`
			} `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if json.Unmarshal(answer.body, &list) != nil {
			return nil, ErrUnreachable
		}
		for _, e := range list.Data {
			item := listItem{id: e.Node.ID, status: e.Status.Status, progress: e.Status.Watched, total: e.Node.Episodes,
				movie: e.Node.MediaType == "movie", score: e.Status.Score}
			if e.Status.Rewatching {
				item.status = statusRepeating
			}
			if d, err := time.Parse(time.DateOnly, e.Status.FinishDate); err == nil {
				item.completed = &d
			}
			if item.id > 0 && item.status != "" {
				items = append(items, item)
			}
		}
		if len(list.Data) == 0 || list.Paging.Next == "" {
			break
		}
	}
	return items, nil
}

// animeOf names the entry of item as Simkl's anime are named, for the
// anime mapping.
func (r *reader) animeOf(item listItem) animeJSON {
	a := animeJSON{}
	if r.key.service == AniList {
		a.IDs.AniList = flexInt(item.id)
	} else {
		a.IDs.MAL = flexInt(item.id)
	}
	return a
}

// readAnimeHistory reads the episodes and movies the list of the
// connection counts watched.
func (r *reader) readAnimeHistory(ctx context.Context) (watchHistory, string, error) {
	var h watchHistory
	if !r.s.anime.Ready() {
		return h, "", nil
	}
	items, err := r.animeList(ctx)
	if err != nil {
		return h, "", err
	}
	mapping := animeEntry{mapping: r.s.anime, unknown: map[string]bool{}}
	for _, item := range items {
		var last int
		var at *time.Time
		switch item.status {
		case statusWatching, statusOnHold, statusDropped:
			last = item.progress
		case statusCompleted, statusRepeating:
			last = max(item.total, item.progress)
			if item.status == statusCompleted {
				at = item.completed
			}
		default:
			continue
		}
		a := r.animeOf(item)
		if item.movie {
			if item.status == statusCompleted || item.status == statusRepeating {
				if ref, ok := mapping.ref(a, true, 1, 1); ok {
					h.watched = append(h.watched, watchedEntry{ref: ref, at: at})
				}
			}
			continue
		}
		for n := 1; n <= min(last, maxEntryEpisodes); n++ {
			if ref, ok := mapping.ref(a, false, 1, n); ok {
				h.watched = append(h.watched, watchedEntry{ref: ref, at: at})
			}
		}
	}
	h.unknown = mapping.unknown
	return h, "", nil
}

// readAnimeWatchlist reads the Plan to Watch entries of the list of the
// connection, AniList's Planning. Until the anime mapping is read, the
// list is not read whole.
func (r *reader) readAnimeWatchlist(ctx context.Context) (watchlistRead, error) {
	var list watchlistRead
	if !r.s.anime.Ready() {
		return list, nil
	}
	items, err := r.animeList(ctx)
	if err != nil {
		return list, err
	}
	mapping := animeEntry{mapping: r.s.anime, unknown: map[string]bool{}}
	for _, item := range items {
		if item.status != statusPlanned {
			continue
		}
		ref, ok := mapping.ref(r.animeOf(item), item.movie, 1, 1)
		if !ok {
			continue
		}
		if !item.movie {
			ref = asSeries(ref)
		}
		list.add(ref, nil)
	}
	list.unknown += len(mapping.unknown)
	list.whole = true
	return list, nil
}

// spread reports whether the TVDB series tvdb holds several anime entries.
func (s *Service) spread(tvdb int) bool {
	return tvdb > 0 && len(distinct(s.anime.IDs(anime.TVDB, strconv.Itoa(tvdb)))) > 1
}

// mergeRatings gives the titles of the scored entries of the list of the
// connection, those still unrated, the list's score: a movie's, or a
// series' when it is the only entry of its series. A series spread over
// several entries is skipped, but where an anime catalog lists the entry
// as a series of its own.
func (s *Service) mergeRatings(ctx context.Context, r *reader, result *ImportResult) error {
	if !s.anime.Ready() {
		return nil
	}
	items, err := r.animeList(ctx)
	if err != nil {
		// The import's problem tells it.
		return nil
	}
	mapping := animeEntry{mapping: s.anime, unknown: map[string]bool{}}
	var refs []library.TitleRef
	var scores []float64
	var spreads []bool
	for _, item := range items {
		if item.score <= 0 {
			continue
		}
		ref, ok := mapping.ref(r.animeOf(item), item.movie, 1, 1)
		if !ok {
			continue
		}
		spread := false
		if !item.movie {
			ref = asSeries(ref)
			if spread = s.spread(ref.TVDB); spread {
				if ref.Anime == (library.AnimeRef{}) {
					result.RatingsSkipped++
					continue
				}
				ref = library.TitleRef{Series: true, Anime: ref.Anime}
			}
		}
		refs, scores, spreads = append(refs, ref), append(scores, min(item.score, 10)), append(spreads, spread)
	}
	if len(refs) == 0 {
		return nil
	}
	targets, err := s.resolve(ctx, r, refs)
	if err != nil {
		return err
	}
	ratings := map[accounts.ID]float64{}
	var order []accounts.ID
	for i, found := range targets {
		if len(found) == 0 && spreads[i] {
			result.RatingsSkipped++
		}
		for _, t := range found {
			if !counts(found, t) {
				continue
			}
			id := t.ID
			if t.Series != (accounts.ID{}) {
				id = t.Series
			}
			if _, listed := ratings[id]; !listed {
				order = append(order, id)
			}
			ratings[id] = scores[i]
		}
	}
	for batch := range chunks(order) {
		items := make([]userdata.Item, 0, len(batch))
		for _, id := range batch {
			items = append(items, userdata.Item{ID: id})
		}
		if _, err := s.userData.ChangeEach(ctx, r.key.user, items, func(item userdata.Item, d *userdata.Data) {
			if d.Rating == nil {
				d.Rating = new(ratings[item.ID])
				result.Rated++
			}
		}); err != nil {
			return err
		}
	}
	return nil
}
