package trackers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// errRateLimited reports a service that asked an import to wait longer
// than it waits.
var errRateLimited = errors.New("tracking service asked to wait too long")

// reader reads the watch history of one connection, a page at a time, at
// the pace its service allows.
type reader struct {
	s        *Service
	key      laneKey
	token    string
	settings accounts.Settings
	last     time.Time
	// refreshed is set once the token was refreshed after a refusal.
	refreshed bool
}

// sleep waits for d, and reports false if ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// get reads path from the service, waiting as long as it asks when it
// answers too many requests and trying again after failures. A refused
// token is refreshed once; refused again, the connection waits for the
// user to connect again.
func (r *reader) get(ctx context.Context, path string, query url.Values) (reply, error) {
	s := r.s
	failures, waits := 0, 0
	for {
		if !sleep(ctx, time.Until(r.last.Add(s.timing.importGaps[r.key.service]))) {
			return reply{}, ctx.Err()
		}
		if r.key.service == PublicMetaDB && !s.waitPace(ctx) {
			return reply{}, ctx.Err()
		}
		answer, err := s.api(ctx, r.key.service, r.token, r.settings, http.MethodGet, path, cloneQuery(query), nil)
		r.last = time.Now()
		if ctx.Err() != nil {
			return reply{}, ctx.Err()
		}
		var wait time.Duration
		switch {
		case err == nil && answer.status == http.StatusOK:
			return answer, nil
		case err == nil && answer.status == http.StatusUnauthorized:
			if err := r.refused(ctx); err != nil {
				return reply{}, err
			}
			continue
		case err == nil && answer.status == http.StatusTooManyRequests:
			wait = max(retryAfter(answer.header), time.Second)
			if r.key.service == Simkl && errorCode(answer.body) == "rate_limit" {
				// Simkl's limit per second; its Retry-After is its day's.
				wait = time.Second
			}
			if waits++; wait > s.timing.importMaxWait || waits > 5 {
				return reply{}, errRateLimited
			}
			if r.key.service == PublicMetaDB {
				s.holdPace(wait)
			}
		case err != nil || answer.status >= 500:
			if failures++; failures > s.timing.importRetries {
				return reply{}, ErrUnreachable
			}
			wait = s.timing.retryFirst << (failures - 1)
		default:
			s.logger.Debug("A tracking service refused to give a watch history", "service", r.key.service, "status", answer.status)
			return reply{}, ErrUnreachable
		}
		if !sleep(ctx, wait) {
			return reply{}, ctx.Err()
		}
	}
}

func cloneQuery(query url.Values) url.Values {
	clone := url.Values{}
	for key, values := range query {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

// refused handles a token or key the service refused: a code service's
// token is refreshed once, else the connection waits for the user to
// connect again.
func (r *reader) refused(ctx context.Context) error {
	if ByCode(r.key.service) && !r.refreshed {
		r.refreshed = true
		conn, err := r.s.fresh(ctx, r.key, r.token)
		if err == nil {
			r.token = conn.token
			return nil
		}
		if !errors.Is(err, errTokenRefused) {
			return ErrUnreachable
		}
	}
	r.s.refusedToken(ctx, r.key)
	return errTokenRefused
}

// waitPace waits for the turn of a request to PublicMetaDB, and reports
// false if ctx ended first.
func (s *Service) waitPace(ctx context.Context) bool {
	p := &s.pace
	p.mu.Lock()
	defer p.mu.Unlock()
	if !sleep(ctx, time.Until(latest(p.last.Add(s.timing.publicMetaDBGap), p.notBefore))) {
		return false
	}
	p.last = time.Now()
	return true
}

// holdPace holds every request to PublicMetaDB for wait.
func (s *Service) holdPace(wait time.Duration) {
	s.pace.mu.Lock()
	s.pace.notBefore = time.Now().Add(wait)
	s.pace.mu.Unlock()
}

// read reads the history of the connection: the titles watched and the
// resume points. cursor tells where the history stood at the last import,
// as the service gave it; next is where it stands now.
func (r *reader) read(ctx context.Context, cursor string) (h watchHistory, next string, err error) {
	// A code service's token is refreshed before it expires; a key stays.
	if ByCode(r.key.service) {
		conn, err := r.s.fresh(ctx, r.key, "")
		if errors.Is(err, errTokenRefused) {
			r.s.refusedToken(ctx, r.key)
			return h, cursor, errTokenRefused
		}
		if err != nil {
			return h, cursor, ErrUnreachable
		}
		r.token = conn.token
	}
	switch r.key.service {
	case Trakt:
		return r.readTrakt(ctx, cursor)
	case Simkl:
		return r.readSimkl(ctx, cursor)
	case MDBList:
		return r.readMDBList(ctx, cursor)
	}
	return r.readPublicMetaDB(ctx)
}

// flexInt decodes an identifier given as a number, a numeric string or
// null.
type flexInt int

func (n *flexInt) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(bytes.TrimSpace(data)), `"`)
	if text == "" || text == "null" {
		*n = 0
		return nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		*n = 0
		return nil
	}
	*n = flexInt(value)
	return nil
}

// flexFloat decodes a number given as a number, a numeric string, as
// MDBList gives a resume point's progress, or null.
type flexFloat float64

func (n *flexFloat) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(bytes.TrimSpace(data)), `"`)
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		value = 0
	}
	*n = flexFloat(value)
	return nil
}

// idsJSON are a title's identifiers as the services give them, under their
// usual names or spelled imdbid, tmdbid and tvdbid.
type idsJSON struct {
	IMDb   string  `json:"imdb"`
	TMDB   flexInt `json:"tmdb"`
	TVDB   flexInt `json:"tvdb"`
	IMDbID string  `json:"imdbid"`
	TMDBID flexInt `json:"tmdbid"`
	TVDBID flexInt `json:"tvdbid"`
}

// ref names a movie by i, or the episode numbered so of the series i
// identifies.
func (i idsJSON) ref(episode bool, season, number int) library.TitleRef {
	ref := library.TitleRef{Episode: episode, IMDb: cmpOr(i.IMDb, i.IMDbID), TMDB: int(cmpOr(i.TMDB, i.TMDBID)), TVDB: int(cmpOr(i.TVDB, i.TVDBID))}
	if episode {
		ref.Season, ref.Number = season, number
	}
	return ref
}

func cmpOr[T comparable](a, b T) T {
	var zero T
	if a != zero {
		return a
	}
	return b
}

// usable reports whether ref can name a title: an identifier, and an
// episode's number.
func usable(ref library.TitleRef) bool {
	return (ref.IMDb != "" || ref.TMDB > 0 || ref.TVDB > 0) && (!ref.Episode || ref.Season >= 0 && ref.Number > 0)
}

// date reads an RFC 3339 date, nil when there is none.
func date(text string) *time.Time {
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return &t
	}
	return nil
}

// playbackJSON is a resume point of Trakt, Simkl or MDBList. MDBList gives
// its progress as a string, and the runtime, in minutes, beside the title
// rather than in it.
type playbackJSON struct {
	Progress flexFloat `json:"progress"`
	PausedAt string    `json:"paused_at"`
	Type     string    `json:"type"`
	Runtime  flexInt   `json:"runtime"`
	Movie    *struct {
		Runtime flexInt `json:"runtime"`
		IDs     idsJSON `json:"ids"`
	} `json:"movie"`
	Episode *struct {
		Season  int     `json:"season"`
		Number  int     `json:"number"`
		Runtime flexInt `json:"runtime"`
	} `json:"episode"`
	Show *struct {
		IDs idsJSON `json:"ids"`
	} `json:"show"`
}

// addPlayback adds the resume points of a playback list to h.
func (h *watchHistory) addPlayback(body []byte) error {
	var items []playbackJSON
	if err := json.Unmarshal(body, &items); err != nil {
		return err
	}
	for _, item := range items {
		at := date(item.PausedAt)
		if at == nil {
			continue
		}
		entry := resumeEntry{percent: float64(item.Progress), at: *at}
		switch {
		case item.Type == "movie" && item.Movie != nil:
			entry.ref = item.Movie.IDs.ref(false, 0, 0)
			entry.runtime = time.Duration(cmpOr(item.Movie.Runtime, item.Runtime)) * time.Minute
		case item.Type == "episode" && item.Episode != nil && item.Show != nil:
			entry.ref = item.Show.IDs.ref(true, item.Episode.Season, item.Episode.Number)
			entry.runtime = time.Duration(cmpOr(item.Episode.Runtime, item.Runtime)) * time.Minute
		default:
			continue
		}
		if usable(entry.ref) {
			h.resumes = append(h.resumes, entry)
		}
	}
	return nil
}

// readTrakt reads Trakt's history a page at a time, unless its last
// activities tell nothing was watched since the last import, and its
// resume points.
func (r *reader) readTrakt(ctx context.Context, cursor string) (watchHistory, string, error) {
	var h watchHistory
	answer, err := r.get(ctx, "/sync/last_activities", nil)
	if err != nil {
		return h, cursor, err
	}
	var activities struct {
		Movies, Episodes struct {
			WatchedAt string `json:"watched_at"`
		}
	}
	if json.Unmarshal(answer.body, &activities) != nil {
		return h, cursor, ErrUnreachable
	}
	next := activities.Movies.WatchedAt + "|" + activities.Episodes.WatchedAt
	if next != cursor {
		for _, kind := range []string{"movies", "episodes"} {
			for page := 1; ; page++ {
				answer, err := r.get(ctx, "/sync/history/"+kind, url.Values{"page": {strconv.Itoa(page)}, "limit": {"250"}})
				if err != nil {
					return h, cursor, err
				}
				var plays []struct {
					WatchedAt string `json:"watched_at"`
					Type      string `json:"type"`
					Movie     *struct {
						IDs idsJSON `json:"ids"`
					} `json:"movie"`
					Episode *struct {
						Season int `json:"season"`
						Number int `json:"number"`
					} `json:"episode"`
					Show *struct {
						IDs idsJSON `json:"ids"`
					} `json:"show"`
				}
				if json.Unmarshal(answer.body, &plays) != nil {
					return h, cursor, ErrUnreachable
				}
				for _, play := range plays {
					var ref library.TitleRef
					switch {
					case play.Type == "movie" && play.Movie != nil:
						ref = play.Movie.IDs.ref(false, 0, 0)
					case play.Type == "episode" && play.Episode != nil && play.Show != nil:
						ref = play.Show.IDs.ref(true, play.Episode.Season, play.Episode.Number)
					default:
						continue
					}
					if usable(ref) {
						h.watched = append(h.watched, watchedEntry{ref: ref, at: date(play.WatchedAt)})
					}
				}
				pages, _ := strconv.Atoi(answer.header.Get("X-Pagination-Page-Count"))
				if len(plays) == 0 || page >= pages {
					break
				}
			}
		}
	}
	answer, err = r.get(ctx, "/sync/playback", url.Values{"extended": {"full"}})
	if err != nil {
		return h, cursor, err
	}
	if h.addPlayback(answer.body) != nil {
		return h, cursor, ErrUnreachable
	}
	return h, next, nil
}

// simklShows are the shows of an all-items answer of Simkl.
type simklItems struct {
	Movies []struct {
		LastWatchedAt string `json:"last_watched_at"`
		Movie         struct {
			IDs idsJSON `json:"ids"`
		} `json:"movie"`
	} `json:"movies"`
	Shows []struct {
		Show struct {
			IDs idsJSON `json:"ids"`
		} `json:"show"`
		Seasons []struct {
			Number   int `json:"number"`
			Episodes []struct {
				Number    int    `json:"number"`
				WatchedAt string `json:"watched_at"`
			} `json:"episodes"`
		} `json:"seasons"`
	} `json:"shows"`
}

// readSimkl reads Simkl's watched movies and the watched episodes of its
// shows, one list status at a time and only what changed since the last
// import unless its activities tell nothing did, and its resume points.
// Anime, numbered as AniDB numbers them, is left out.
func (r *reader) readSimkl(ctx context.Context, cursor string) (watchHistory, string, error) {
	var h watchHistory
	answer, err := r.get(ctx, "/sync/activities", nil)
	if err != nil {
		return h, cursor, err
	}
	var activities struct {
		All string `json:"all"`
	}
	if json.Unmarshal(answer.body, &activities) != nil {
		return h, cursor, ErrUnreachable
	}
	next := activities.All
	if next == "" || next != cursor {
		lists := []string{"movies/completed", "shows/watching", "shows/completed", "shows/hold", "shows/dropped"}
		for _, list := range lists {
			query := url.Values{"extended": {"full"}}
			if strings.HasPrefix(list, "shows/") {
				query.Set("episode_watched_at", "yes")
				query.Set("include_all_episodes", "original")
			}
			if cursor != "" {
				query.Set("date_from", cursor)
			}
			answer, err := r.get(ctx, "/sync/all-items/"+list, query)
			if err != nil {
				return h, cursor, err
			}
			var items simklItems
			// An empty list is {} or null.
			if json.Unmarshal(answer.body, &items) != nil && strings.TrimSpace(string(answer.body)) != "null" {
				return h, cursor, ErrUnreachable
			}
			for _, movie := range items.Movies {
				if ref := movie.Movie.IDs.ref(false, 0, 0); usable(ref) {
					h.watched = append(h.watched, watchedEntry{ref: ref, at: date(movie.LastWatchedAt)})
				}
			}
			for _, show := range items.Shows {
				for _, season := range show.Seasons {
					for _, episode := range season.Episodes {
						if ref := show.Show.IDs.ref(true, season.Number, episode.Number); usable(ref) {
							h.watched = append(h.watched, watchedEntry{ref: ref, at: date(episode.WatchedAt)})
						}
					}
				}
			}
		}
	}
	answer, err = r.get(ctx, "/sync/playback", nil)
	if err != nil {
		return h, cursor, err
	}
	if h.addPlayback(answer.body) != nil {
		return h, cursor, ErrUnreachable
	}
	return h, next, nil
}

// readMDBList reads MDBList's watched movies and episodes, a page at a
// time and only those updated since the last import, unless its last
// activities tell nothing was watched since, and its resume points.
func (r *reader) readMDBList(ctx context.Context, cursor string) (watchHistory, string, error) {
	var h watchHistory
	answer, err := r.get(ctx, "/sync/last_activities", nil)
	if err != nil {
		return h, cursor, err
	}
	var activities struct {
		WatchedAt        string `json:"watched_at"`
		EpisodeWatchedAt string `json:"episode_watched_at"`
	}
	if json.Unmarshal(answer.body, &activities) != nil {
		return h, cursor, ErrUnreachable
	}
	next := activities.WatchedAt + "|" + activities.EpisodeWatchedAt
	if next != cursor {
		// Items updated after the latest activity the last import saw.
		var since string
		if before, after, ok := strings.Cut(cursor, "|"); ok {
			since = max(before, after)
		}
		const limit = 1000
		for offset := 0; ; offset += limit {
			query := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)}}
			if since != "" {
				query.Set("since", since)
			}
			answer, err := r.get(ctx, "/sync/watched", query)
			if err != nil {
				return h, cursor, err
			}
			var page struct {
				Movies []struct {
					LastWatchedAt string `json:"last_watched_at"`
					Movie         struct {
						IDs idsJSON `json:"ids"`
					} `json:"movie"`
				} `json:"movies"`
				Episodes []struct {
					LastWatchedAt string `json:"last_watched_at"`
					Episode       struct {
						Season int `json:"season"`
						Number int `json:"number"`
						Show   struct {
							IDs idsJSON `json:"ids"`
						} `json:"show"`
					} `json:"episode"`
				} `json:"episodes"`
				Pagination struct {
					HasMore bool `json:"has_more"`
				} `json:"pagination"`
			}
			if json.Unmarshal(answer.body, &page) != nil {
				return h, cursor, ErrUnreachable
			}
			for _, movie := range page.Movies {
				if ref := movie.Movie.IDs.ref(false, 0, 0); usable(ref) {
					h.watched = append(h.watched, watchedEntry{ref: ref, at: date(movie.LastWatchedAt)})
				}
			}
			for _, episode := range page.Episodes {
				if ref := episode.Episode.Show.IDs.ref(true, episode.Episode.Season, episode.Episode.Number); usable(ref) {
					h.watched = append(h.watched, watchedEntry{ref: ref, at: date(episode.LastWatchedAt)})
				}
			}
			if !page.Pagination.HasMore {
				break
			}
		}
	}
	answer, err = r.get(ctx, "/sync/playback", nil)
	if err != nil {
		return h, cursor, err
	}
	if h.addPlayback(answer.body) != nil {
		return h, cursor, ErrUnreachable
	}
	return h, next, nil
}

// readPublicMetaDB reads PublicMetaDB's watch history and resume points, a
// page at a time. It tells no activity: the history is read whole.
func (r *reader) readPublicMetaDB(ctx context.Context) (watchHistory, string, error) {
	var h watchHistory
	type item struct {
		TMDB       flexInt `json:"tmdb_id"`
		MediaType  string  `json:"media_type"`
		Season     int     `json:"season"`
		Episode    int     `json:"episode"`
		WatchedAt  *string `json:"watched_at"`
		PositionMS int64   `json:"position_ms"`
		RuntimeMS  int64   `json:"runtime_ms"`
		Progress   float64 `json:"progress"`
		Updated    string  `json:"updated"`
	}
	ref := func(i item) library.TitleRef {
		return library.TitleRef{Episode: i.MediaType == "tv", TMDB: int(i.TMDB), Season: i.Season, Number: i.Episode}
	}
	for _, path := range []string{"/api/external/watched", "/api/external/resume"} {
		for page := 1; ; page++ {
			answer, err := r.get(ctx, path, url.Values{"page": {strconv.Itoa(page)}, "perPage": {"500"}})
			if err != nil {
				return h, "", err
			}
			var list struct {
				Items      []item `json:"items"`
				TotalPages int    `json:"totalPages"`
			}
			if json.Unmarshal(answer.body, &list) != nil {
				return h, "", ErrUnreachable
			}
			for _, i := range list.Items {
				title := ref(i)
				if !usable(title) || i.MediaType != "tv" && i.MediaType != "movie" {
					continue
				}
				if path == "/api/external/watched" {
					var at *time.Time
					if i.WatchedAt != nil {
						at = date(*i.WatchedAt)
					}
					h.watched = append(h.watched, watchedEntry{ref: title, at: at})
				} else if at := date(i.Updated); at != nil {
					h.resumes = append(h.resumes, resumeEntry{ref: title, percent: i.Progress,
						position: time.Duration(i.PositionMS) * time.Millisecond, runtime: time.Duration(i.RuntimeMS) * time.Millisecond, at: *at})
				}
			}
			if len(list.Items) == 0 || page >= list.TotalPages {
				break
			}
		}
	}
	return h, "", nil
}

// mapPublicMetaDB asks PublicMetaDB the IMDb identifier it maps the TMDB
// identifier of ref to, when it maps it to exactly one. Answers are kept
// for the life of the server.
func (s *Service) mapPublicMetaDB(ctx context.Context, r *reader, ref library.TitleRef) string {
	mediaType := "movie"
	if ref.Episode {
		mediaType = "tv"
	}
	cacheKey := mediaType + ":" + strconv.Itoa(ref.TMDB)
	s.mu.Lock()
	imdb, cached := s.publicMetaDBIMDb[cacheKey]
	s.mu.Unlock()
	if cached {
		return imdb
	}
	answer, err := r.get(ctx, "/api/external/mappings", url.Values{"tmdb_id": {strconv.Itoa(ref.TMDB)}, "media_type": {mediaType}, "id_type": {"imdb"}})
	if err != nil {
		return ""
	}
	var mappings struct {
		Mappings struct {
			IMDb []struct {
				Value string `json:"value"`
			} `json:"imdb"`
		} `json:"mappings"`
	}
	_ = json.Unmarshal(answer.body, &mappings)
	agreed := true
	for _, m := range mappings.Mappings.IMDb {
		switch {
		case !strings.HasPrefix(m.Value, "tt"):
		case imdb == "":
			imdb = m.Value
		case imdb != m.Value:
			// Contributors disagree: it is not reliable.
			agreed = false
		}
	}
	if !agreed {
		imdb = ""
	}
	s.mu.Lock()
	s.publicMetaDBIMDb[cacheKey] = imdb
	s.mu.Unlock()
	return imdb
}
