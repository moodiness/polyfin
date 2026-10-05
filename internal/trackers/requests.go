package trackers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// maxReply bounds the bytes read from a service.
const maxReply = 1 << 20

// simklApp is the app name Simkl is told, with Polyfin's version.
const simklApp = "polyfin"

// traktRedirect is the redirect URI of apps without a web page of their
// own, which Trakt's token requests name.
const traktRedirect = "urn:ietf:wg:oauth:2.0:oob"

// reply is a service's answer.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// call sends a request and reads the reply. The error never holds the URL,
// which carries MDBList's key.
func (s *Service) call(ctx context.Context, method, target string, header http.Header, body []byte) (reply, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timing.request)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return reply{}, errors.New("malformed request")
	}
	request.Header = header
	request.Header.Set("User-Agent", "Polyfin/"+s.version)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return reply{}, withoutURL(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReply))
	if err != nil {
		return reply{}, withoutURL(err)
	}
	return reply{status: response.StatusCode, header: response.Header, body: data}, nil
}

func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// api sends a request to service on behalf of the connection holding
// token, with the credentials and parameters the service asks for, and
// payload, when not nil, as JSON.
func (s *Service) api(ctx context.Context, service, token string, settings accounts.Settings, method, path string, query url.Values, payload any) (reply, error) {
	header := http.Header{}
	if query == nil {
		query = url.Values{}
	}
	switch service {
	case Trakt:
		header.Set("trakt-api-key", settings.TraktClientID)
		header.Set("trakt-api-version", "2")
	case Simkl:
		query.Set("client_id", settings.SimklClientID)
		query.Set("app-name", simklApp)
		query.Set("app-version", s.version)
	case MDBList:
		// MDBList takes its keys in the query only.
		query.Set("apikey", token)
	}
	if token != "" && service != MDBList {
		header.Set("Authorization", "Bearer "+token)
	}
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
		header.Set("Content-Type", "application/json")
	}
	target := s.urls[service] + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return s.call(ctx, method, target, header, body)
}

// form sends a form to an OAuth endpoint of Simkl, which takes them so.
func (s *Service) form(ctx context.Context, path string, values url.Values) (reply, error) {
	header := http.Header{}
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	query := url.Values{"client_id": {values.Get("client_id")}, "app-name": {simklApp}, "app-version": {s.version}}
	return s.call(ctx, http.MethodPost, s.urls[Simkl]+path+"?"+query.Encode(), header, []byte(values.Encode()))
}

// outcome is what came of a send.
type outcome int

const (
	// delivered: the service accepted the change.
	delivered outcome = iota
	// retry: it may accept it later.
	retry
	// refused: it never will, as sent.
	refused
	// reconnect: it refused the token or key.
	reconnect
)

// result is what came of a send, how long to wait before sending again,
// and the titles the service now counts watched.
type result struct {
	outcome outcome
	after   time.Duration
	status  int
	watched []accounts.ID
}

// classify reads the status of a reply to a change.
func classify(service string, r reply) result {
	result := result{status: r.status}
	switch {
	case r.status >= 200 && r.status < 300, r.status == http.StatusConflict:
		// A conflict is a change the service already has, such as a stop
		// it already counted.
		result.outcome = delivered
	case r.status == http.StatusUnauthorized:
		result.outcome = reconnect
	case r.status == http.StatusTooManyRequests:
		result.outcome = retry
		result.after = retryAfter(r.header)
		// Simkl's per-second limit clears in a second; its Retry-After
		// is for its daily one.
		if service == Simkl && errorCode(r.body) == "rate_limit" {
			result.after = time.Second
		}
	case service == Simkl && r.status == http.StatusBadRequest && errorCode(r.body) == "RATE_LIMIT":
		// Simkl's lock on one write per user at a time: it clears as soon
		// as the write under way ends.
		result.outcome, result.after = retry, 2*time.Second
	case service == Simkl && r.status == http.StatusPreconditionFailed:
		// Simkl answers so while it throttles an app that wrote too fast.
		result.outcome = retry
	case r.status >= 500:
		result.outcome = retry
	default:
		result.outcome = refused
	}
	return result
}

// retryAfter reads a Retry-After header in seconds, zero when absent.
func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After")))
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// errorCode reads the error field of a JSON error body.
func errorCode(body []byte) string {
	var answer struct {
		Error any `json:"error"`
	}
	_ = json.Unmarshal(body, &answer)
	code, _ := answer.Error.(string)
	return code
}

// ids are a title's identifiers as the scrobbling services take them: an
// episode by its series'.
type ids struct {
	IMDb string `json:"imdb,omitempty"`
	TMDB int    `json:"tmdb,omitempty"`
	TVDB int    `json:"tvdb,omitempty"`
}

func (t Title) ids() ids {
	return ids{IMDb: t.IMDb, TMDB: t.TMDB, TVDB: t.TVDB}
}

type media struct {
	IDs ids `json:"ids"`
}

type episodeNumbers struct {
	Season int `json:"season"`
	Number int `json:"number"`
}

// scrobbleBody is a scrobble of Trakt and Simkl.
type scrobbleBody struct {
	Movie    *media          `json:"movie,omitempty"`
	Show     *media          `json:"show,omitempty"`
	Episode  *episodeNumbers `json:"episode,omitempty"`
	Progress float64         `json:"progress"`
}

// mdblistScrobble is a scrobble of MDBList, which nests an episode's
// numbers in its show.
type mdblistScrobble struct {
	Movie    *media       `json:"movie,omitempty"`
	Show     *mdblistShow `json:"show,omitempty"`
	Progress float64      `json:"progress"`
}

type mdblistShow struct {
	IDs    ids           `json:"ids"`
	Season mdblistSeason `json:"season"`
}

type mdblistSeason struct {
	Number  int `json:"number"`
	Episode struct {
		Number int `json:"number"`
	} `json:"episode"`
}

// historyBody adds titles to the history of Trakt, Simkl or MDBList, or
// removes them, episodes grouped by series and season.
type historyBody struct {
	Movies []historyMovie `json:"movies,omitempty"`
	Shows  []historyShow  `json:"shows,omitempty"`
}

type historyMovie struct {
	IDs       ids    `json:"ids"`
	WatchedAt string `json:"watched_at,omitempty"`
}

type historyShow struct {
	IDs     ids             `json:"ids"`
	Seasons []historySeason `json:"seasons"`
}

type historySeason struct {
	Number   int              `json:"number"`
	Episodes []historyEpisode `json:"episodes"`
}

type historyEpisode struct {
	Number    int    `json:"number"`
	WatchedAt string `json:"watched_at,omitempty"`
}

func history(titles []Title, watchedAt *time.Time) historyBody {
	var body historyBody
	at := ""
	if watchedAt != nil {
		at = watchedAt.UTC().Format(time.RFC3339)
	}
	for _, title := range titles {
		if !title.Episode {
			body.Movies = append(body.Movies, historyMovie{IDs: title.ids(), WatchedAt: at})
			continue
		}
		show := slices.IndexFunc(body.Shows, func(s historyShow) bool { return s.IDs == title.ids() })
		if show < 0 {
			body.Shows = append(body.Shows, historyShow{IDs: title.ids()})
			show = len(body.Shows) - 1
		}
		seasons := &body.Shows[show].Seasons
		season := slices.IndexFunc(*seasons, func(s historySeason) bool { return s.Number == title.Season })
		if season < 0 {
			*seasons = append(*seasons, historySeason{Number: title.Season})
			season = len(*seasons) - 1
		}
		(*seasons)[season].Episodes = append((*seasons)[season].Episodes, historyEpisode{Number: title.Number, WatchedAt: at})
	}
	return body
}

// round keeps two decimals of a percent, as Simkl takes them.
func round(percent float64) float64 {
	return math.Round(min(max(percent, 0), 100)*100) / 100
}

// sendTo sends a change of kind to service for the connection holding
// token.
func (s *Service) sendTo(ctx context.Context, service, token string, settings accounts.Settings, kind string, ev event) result {
	if service == PublicMetaDB {
		return s.sendPublicMetaDB(ctx, token, settings, kind, ev)
	}
	var (
		path    string
		payload any
	)
	title := ev.Titles[0]
	switch kind {
	case kindScrobble:
		path = "/scrobble/" + ev.Action
		if service == MDBList {
			body := mdblistScrobble{Progress: round(ev.Progress)}
			if title.Episode {
				body.Show = &mdblistShow{IDs: title.ids(), Season: mdblistSeason{Number: title.Season}}
				body.Show.Season.Episode.Number = title.Number
			} else {
				body.Movie = &media{IDs: title.ids()}
			}
			payload = body
		} else {
			body := scrobbleBody{Progress: round(ev.Progress)}
			if title.Episode {
				body.Show, body.Episode = &media{IDs: title.ids()}, &episodeNumbers{Season: title.Season, Number: title.Number}
			} else {
				body.Movie = &media{IDs: title.ids()}
			}
			payload = body
		}
	case kindHistoryAdd, kindHistoryRemove:
		path = "/sync/history"
		if service == MDBList {
			path = "/sync/watched"
		}
		at := ev.WatchedAt
		if kind == kindHistoryRemove {
			path, at = path+"/remove", nil
		}
		payload = history(ev.Titles, at)
	default:
		return result{outcome: refused}
	}
	r, err := s.api(ctx, service, token, settings, http.MethodPost, path, nil, payload)
	if err != nil {
		return result{outcome: retry}
	}
	result := classify(service, r)
	if result.outcome != delivered {
		return result
	}
	switch kind {
	case kindScrobble:
		// A stop the service counted watched, or one it already had.
		var answer struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(r.body, &answer)
		if ev.Action == "stop" && (answer.Action == "scrobble" || r.status == http.StatusConflict) {
			result.watched = []accounts.ID{title.Item}
		}
	case kindHistoryAdd:
		for _, title := range ev.Titles {
			result.watched = append(result.watched, title.Item)
		}
	}
	return result
}

// mediaType is how PublicMetaDB names the kind of a title.
func (t Title) mediaType() string {
	if t.Episode {
		return "tv"
	}
	return "movie"
}

// sendPublicMetaDB sends a resume point or a change to the watch history
// to PublicMetaDB.
func (s *Service) sendPublicMetaDB(ctx context.Context, key string, settings accounts.Settings, kind string, ev event) result {
	title := ev.Titles[0]
	switch kind {
	case kindResume:
		body := map[string]any{"media_type": title.mediaType(), "position_ms": ev.PositionMS, "runtime_ms": ev.RuntimeMS}
		// Resume points take any identifier PublicMetaDB maps.
		switch {
		case title.TMDB != 0:
			body["tmdb_id"] = title.TMDB
		case title.IMDb != "":
			body["id_type"], body["id_value"] = "imdb", title.IMDb
		default:
			body["id_type"], body["id_value"] = "tvdb", strconv.Itoa(title.TVDB)
		}
		if title.Episode {
			body["season"], body["episode"] = title.Season, title.Number
		}
		return s.publicMetaDB(ctx, key, settings, http.MethodPost, "/api/external/resume", nil, body)
	case kindWatched, kindUnwatched:
		tmdb, res := s.publicMetaDBTMDB(ctx, key, settings, title)
		if tmdb == 0 {
			return res
		}
		if kind == kindWatched {
			body := map[string]any{"tmdb_id": tmdb, "media_type": title.mediaType()}
			if title.Episode {
				body["season"], body["episode"] = title.Season, title.Number
			}
			if ev.WatchedAt != nil {
				body["watched_at"] = ev.WatchedAt.UTC().Format(time.RFC3339)
			}
			res := s.publicMetaDB(ctx, key, settings, http.MethodPost, "/api/external/watched", nil, body)
			if res.outcome == delivered {
				res.watched = []accounts.ID{title.Item}
			}
			return res
		}
		query := url.Values{"tmdb_id": {strconv.Itoa(tmdb)}, "media_type": {title.mediaType()}}
		if title.Episode && ev.Scope != ScopeSeries {
			query.Set("season", strconv.Itoa(title.Season))
			if ev.Scope == ScopeTitle {
				query.Set("episode", strconv.Itoa(title.Number))
			}
		}
		res = s.publicMetaDB(ctx, key, settings, http.MethodDelete, "/api/external/watched", query, nil)
		if res.status == http.StatusNotFound {
			// Nothing to remove.
			res.outcome = delivered
		}
		return res
	}
	return result{outcome: refused}
}

// publicMetaDB sends a request to PublicMetaDB.
func (s *Service) publicMetaDB(ctx context.Context, key string, settings accounts.Settings, method, path string, query url.Values, payload any) result {
	r, err := s.api(ctx, PublicMetaDB, key, settings, method, path, query, payload)
	if err != nil {
		return result{outcome: retry}
	}
	return classify(PublicMetaDB, r)
}

// publicMetaDBTMDB is the TMDB identifier of title, or of its series,
// which PublicMetaDB's history needs: looked up by the IMDb or TVDB one
// when the title lacks it. Zero comes with what came of the lookup.
func (s *Service) publicMetaDBTMDB(ctx context.Context, key string, settings accounts.Settings, title Title) (int, result) {
	if title.TMDB != 0 {
		return title.TMDB, result{}
	}
	query := url.Values{"media_type": {title.mediaType()}}
	if title.IMDb != "" {
		query.Set("id_type", "imdb")
		query.Set("id_value", title.IMDb)
	} else {
		query.Set("id_type", "tvdb")
		query.Set("id_value", strconv.Itoa(title.TVDB))
	}
	r, err := s.api(ctx, PublicMetaDB, key, settings, http.MethodGet, "/api/external/mappings/lookup", query, nil)
	if err != nil {
		return 0, result{outcome: retry}
	}
	if res := classify(PublicMetaDB, r); res.outcome != delivered {
		return 0, res
	}
	var answer struct {
		Results []struct {
			TMDB      int    `json:"tmdb_id"`
			MediaType string `json:"media_type"`
		} `json:"results"`
	}
	_ = json.Unmarshal(r.body, &answer)
	for _, found := range answer.Results {
		if found.TMDB > 0 && found.MediaType == title.mediaType() {
			return found.TMDB, result{}
		}
	}
	// PublicMetaDB does not know the title.
	return 0, result{outcome: refused, status: r.status}
}

// checkKey asks service whether key is valid, and returns the account name
// it gives, empty when it gives none.
func (s *Service) checkKey(ctx context.Context, service, key string) (string, error) {
	var (
		r   reply
		err error
	)
	settings := s.settings()
	switch service {
	case MDBList:
		r, err = s.api(ctx, service, key, settings, http.MethodGet, "/user", nil, nil)
	case PublicMetaDB:
		r, err = s.api(ctx, service, key, settings, http.MethodGet, "/api/external/lists", url.Values{"perPage": {"1"}}, nil)
	default:
		return "", ErrUnknownService
	}
	switch {
	case err != nil:
		return "", ErrUnreachable
	case r.status == http.StatusUnauthorized || r.status == http.StatusForbidden:
		return "", ErrInvalidKey
	case r.status != http.StatusOK:
		return "", ErrUnreachable
	}
	if service != MDBList {
		return "", nil
	}
	var user struct {
		Username string `json:"username"`
		Error    any    `json:"error"`
	}
	if json.Unmarshal(r.body, &user) != nil {
		return "", ErrUnreachable
	}
	if user.Error != nil {
		return "", ErrInvalidKey
	}
	return user.Username, nil
}
