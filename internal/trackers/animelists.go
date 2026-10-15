package trackers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
)

// AniList and MyAnimeList keep, for each user, a list of anime entries,
// split as AniDB splits series into seasons and parts: each with its
// status (Watching, Completed, On hold, Dropped, Plan to Watch) and how
// many of its episodes were watched. They follow no playback: a title
// counts once Polyfin marks it played, as for PublicMetaDB, and raises its
// entry's progress to its episode, never lowering it (see nextProgress).
// Unplayed marks send nothing. Titles are found as entries through the
// anime mapping (see entryOf), and the imports read the lists back (see
// readAnimeHistory).
//
// Users connect AniList by allowing Polyfin on its site and pasting the
// code it then shows, and MyAnimeList by allowing Polyfin there with PKCE
// (its plain method), its site sending their browser back to the admin
// app with a code, or the user pasting the address it sent them to (see
// Exchange). AniList's tokens last a year and cannot be refreshed: an
// expired one stops the connection; MyAnimeList's are refreshed.

// anilistPIN is the redirect URL of AniList apps whose users copy the code
// AniList shows them rather than being sent back.
const anilistPIN = "https://anilist.co/api/v2/oauth/pin"

// maxPasted bounds a pasted code or address, in bytes.
const maxPasted = 4096

// maxEntryEpisodes bounds the episodes an import marks played of one
// entry.
const maxEntryEpisodes = 5000

// ErrInvalidCode reports a pasted code or address the service refused, or
// one for no connection waiting.
var ErrInvalidCode = errors.New("invalid authorization code")

// randomToken is n random bytes, base64url without padding: characters
// PKCE verifiers and OAuth states take.
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// startPaste opens, for key, the page of a paste service where its user
// allows Polyfin, in place of the one waiting: it waits for them to paste
// back its code until it expires.
func (s *Service) startPaste(ctx context.Context, key laneKey, settings accounts.Settings) (Status, error) {
	p := &pending{user: key.user, service: key.service, expiresAt: s.now().UTC().Add(s.timing.signInTime).Truncate(time.Second),
		done: make(chan struct{})}
	query := url.Values{"response_type": {"code"}}
	if key.service == AniList {
		query.Set("client_id", settings.AniListClientID)
		query.Set("redirect_uri", anilistPIN)
	} else {
		// MyAnimeList takes PKCE's plain method only: the challenge is the
		// verifier. No redirect URI is named, so its site sends the
		// browser to the one the app registered, and the token request
		// names none either.
		p.deviceCode, p.state = randomToken(48), randomToken(16)
		query.Set("client_id", settings.MyAnimeListClientID)
		query.Set("code_challenge", p.deviceCode)
		query.Set("code_challenge_method", "plain")
		query.Set("state", p.state)
	}
	p.verificationURL = s.authURLs[key.service] + "/authorize?" + query.Encode()
	s.mu.Lock()
	if old := s.pending[key]; old != nil {
		old.cancel()
	}
	s.pending[key] = p
	s.mu.Unlock()
	return s.status(ctx, key.user, key.service)
}

// pastedCode reads what a user pasted: the address a service sent their
// browser to, with its code and state, or the code alone.
func pastedCode(pasted string) (code, state string) {
	pasted = strings.TrimSpace(pasted)
	if i := strings.Index(pasted, "code="); i >= 0 {
		raw := pasted
		if j := strings.LastIndexAny(pasted[:i], "?#"); j >= 0 {
			raw = pasted[j+1:]
		}
		raw, _, _ = strings.Cut(raw, "#")
		if values, err := url.ParseQuery(raw); err == nil && values.Get("code") != "" {
			return values.Get("code"), values.Get("state")
		}
	}
	for i := range len(pasted) {
		if pasted[i] <= ' ' || pasted[i] > '~' {
			return "", ""
		}
	}
	return pasted, ""
}

// Exchange connects user to a paste service with what they pasted: the
// code AniList showed them, or the address MyAnimeList sent their browser
// to (or its code), once the service gave a token for it, in place of the
// connection there was.
func (s *Service) Exchange(ctx context.Context, user accounts.ID, service, pasted string) (Status, error) {
	if !ByPaste(service) {
		return Status{}, ErrUnknownService
	}
	settings := s.settings()
	if !Available(service, settings) {
		return Status{}, ErrNotAvailable
	}
	if len(pasted) > maxPasted {
		return Status{}, ErrInvalidCode
	}
	code, state := pastedCode(pasted)
	if code == "" || len(code) > maxPasted {
		return Status{}, ErrInvalidCode
	}
	key := laneKey{user, service}
	s.mu.Lock()
	p := s.pending[key]
	if p != nil && !s.now().Before(p.expiresAt) {
		delete(s.pending, key)
		p = nil
	}
	s.mu.Unlock()
	var (
		r   reply
		err error
	)
	if service == AniList {
		header := http.Header{}
		header.Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "client_id": settings.AniListClientID,
			"client_secret": settings.AniListClientSecret, "redirect_uri": anilistPIN, "code": code})
		r, err = s.call(ctx, http.MethodPost, s.authURLs[AniList]+"/token", header, body)
	} else {
		if p == nil || state != "" && state != p.state {
			return Status{}, ErrInvalidCode
		}
		r, err = s.oauthForm(ctx, MyAnimeList, url.Values{"grant_type": {"authorization_code"}, "client_id": {settings.MyAnimeListClientID},
			"client_secret": {settings.MyAnimeListClientSecret}, "code": {code}, "code_verifier": {p.deviceCode}})
	}
	switch {
	case err != nil || r.status >= 500 || r.status == http.StatusTooManyRequests:
		return Status{}, ErrUnreachable
	case r.status == http.StatusUnauthorized || errorCode(r.body) == "invalid_client":
		s.mu.Lock()
		s.refusedApps[key] = appCredentials(service, settings)
		s.mu.Unlock()
		s.logger.Info("A tracking service refused this server's app: its credentials need checking under Settings › Tracking",
			"user_id", user.String(), "service", service)
		return Status{}, ErrAppRefused
	case r.status != http.StatusOK:
		return Status{}, ErrInvalidCode
	}
	var t tokens
	if json.Unmarshal(r.body, &t) != nil || t.AccessToken == "" {
		return Status{}, ErrUnreachable
	}
	c := connection{token: t.AccessToken, expires: s.expiry(t), connectedAt: s.now().UTC().Truncate(time.Second)}
	if service == MyAnimeList {
		// AniList's refresh tokens do not work: none is kept.
		c.refresh = t.RefreshToken
	}
	if account := s.listAccount(ctx, service, t.AccessToken); account != "" {
		c.account = &account
	}
	s.mu.Lock()
	delete(s.refusedApps, key)
	if p != nil && s.pending[key] == p {
		p.cancel()
		delete(s.pending, key)
	}
	s.mu.Unlock()
	if err := s.connect(ctx, user, service, c); err != nil {
		return Status{}, err
	}
	return s.status(ctx, user, service)
}

// oauthForm sends a form to the OAuth endpoint of service that gives
// tokens.
func (s *Service) oauthForm(ctx context.Context, service string, values url.Values) (reply, error) {
	header := http.Header{}
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.call(ctx, http.MethodPost, s.authURLs[service]+"/token", header, []byte(values.Encode()))
}

// listAccount asks AniList or MyAnimeList the name of the account token
// is for, empty when it does not answer.
func (s *Service) listAccount(ctx context.Context, service, token string) string {
	if service == AniList {
		var answer struct {
			Data struct {
				Viewer struct {
					Name string `json:"name"`
				} `json:"Viewer"`
			} `json:"data"`
		}
		if r, err := s.graphQL(ctx, token, `query { Viewer { id name } }`, nil); err == nil && r.status == http.StatusOK {
			_ = json.Unmarshal(r.body, &answer)
		}
		return answer.Data.Viewer.Name
	}
	var answer struct {
		Name string `json:"name"`
	}
	if r, err := s.api(ctx, MyAnimeList, token, s.settings(), http.MethodGet, "/users/@me", nil, nil); err == nil && r.status == http.StatusOK {
		_ = json.Unmarshal(r.body, &answer)
	}
	return answer.Name
}

// graphQL sends a query of AniList's GraphQL API.
func (s *Service) graphQL(ctx context.Context, token, query string, variables map[string]any) (reply, error) {
	payload := map[string]any{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	return s.api(ctx, AniList, token, s.settings(), http.MethodPost, "", nil, payload)
}

// invalidToken reports whether a GraphQL error body of AniList refuses the
// token, which it answers 400 or 401.
func invalidToken(body []byte) bool {
	var answer struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &answer)
	for _, e := range answer.Errors {
		if strings.Contains(strings.ToLower(e.Message), "invalid token") {
			return true
		}
	}
	return false
}

// classifyList reads the status of a reply of AniList or MyAnimeList: an
// entry it does not know is refused.
func classifyList(service string, r reply) result {
	res := classify(service, r)
	if service == AniList && r.status == http.StatusBadRequest && invalidToken(r.body) {
		res.outcome = reconnect
	}
	return res
}

// listEntry is an entry of AniList or MyAnimeList, by the service's
// identifier, and the episode of it a title is, 0 for a movie or a
// series.
type listEntry struct {
	ID      int  `json:"id"`
	Episode int  `json:"episode,omitempty"`
	Movie   bool `json:"movie,omitempty"`
}

// entryFind is how a title was looked for among the entries.
type entryFind int

const (
	entryNone entryFind = iota
	entryFound
	// entrySpread: a series spread over several entries.
	entrySpread
)

// listID is the identifier service gives the entry ids.
func listID(service string, ids anime.IDs) int {
	if service == AniList {
		return ids.AniList
	}
	return ids.MyAnimeList
}

// distinct leaves one of the entries found per AniDB entry.
func distinct(found []anime.IDs) []anime.IDs {
	var result []anime.IDs
	seen := map[int]bool{}
	for _, ids := range found {
		if ids.AniDB > 0 && seen[ids.AniDB] {
			continue
		}
		seen[ids.AniDB] = true
		result = append(result, ids)
	}
	return result
}

// entryOf finds the entry of service that t is: the entry an anime addon
// names, with the addon's episode number; else for an episode, the AniDB
// entry whose mapping covers its TVDB season and number (see
// anime.Service.Entry), with its own number there; for a movie, the entry
// of its IMDb or TMDB identifier; for a series, its only entry, spread
// when it has several.
func (s *Service) entryOf(service string, t Title) (listEntry, entryFind) {
	if s.anime == nil {
		return listEntry{}, entryNone
	}
	movie := !t.Episode && !t.Series
	one := func(found []anime.IDs, number int) (listEntry, entryFind) {
		found = distinct(found)
		switch {
		case len(found) == 0:
			return listEntry{}, entryNone
		case len(found) > 1 && t.Series:
			return listEntry{}, entrySpread
		case len(found) > 1:
			return listEntry{}, entryNone
		}
		id := listID(service, found[0])
		if id <= 0 {
			return listEntry{}, entryNone
		}
		return listEntry{ID: id, Episode: number, Movie: movie}, entryFound
	}
	if t.Anime != nil {
		if t.Episode && t.Anime.Episode <= 0 {
			return listEntry{}, entryNone
		}
		if service == MyAnimeList && t.Anime.Source == anime.MyAnimeList {
			return listEntry{ID: t.Anime.ID, Episode: t.Anime.Episode, Movie: movie}, entryFound
		}
		return one(s.anime.IDs(t.Anime.Source, strconv.Itoa(t.Anime.ID)), t.Anime.Episode)
	}
	movies := func(found []anime.IDs) []anime.IDs {
		var result []anime.IDs
		for _, ids := range found {
			if ids.Movie || len(ids.TMDBMovies) > 0 {
				result = append(result, ids)
			}
		}
		return result
	}
	series := func() []anime.IDs {
		if t.TVDB > 0 {
			return s.anime.IDs(anime.TVDB, strconv.Itoa(t.TVDB))
		}
		var result []anime.IDs
		if t.IMDb != "" {
			for _, ids := range s.anime.IDs(anime.IMDb, t.IMDb) {
				if !ids.Movie && len(ids.TMDBMovies) == 0 {
					result = append(result, ids)
				}
			}
		}
		return result
	}
	switch {
	case movie:
		found := movies(s.anime.IDs(anime.IMDb, t.IMDb))
		if len(found) == 0 && t.TMDB > 0 {
			found = movies(s.anime.IDs(anime.TMDBMovie, strconv.Itoa(t.TMDB)))
		}
		return one(found, 0)
	case t.Series:
		return one(series(), 0)
	}
	tvdb := t.TVDB
	if tvdb == 0 {
		// A series known by IMDb only: by the TVDB series its entries are
		// part of, when they agree.
		for _, ids := range series() {
			if tvdb != 0 && ids.TVDB != tvdb {
				return listEntry{}, entryNone
			}
			tvdb = ids.TVDB
		}
	}
	anidb, episode, ok := s.anime.Entry(tvdb, t.Season, t.Number, t.Absolute)
	if !ok {
		return listEntry{}, entryNone
	}
	return one(s.anime.IDs(anime.AniDB, strconv.Itoa(anidb)), episode)
}

// queueProgress queues, for service, the progress of each title played to
// its entry; a title the mapping does not place sends nothing.
func (s *Service) queueProgress(ctx context.Context, user accounts.ID, service string, titles []Title) {
	for _, t := range titles {
		entry, find := s.entryOf(service, t)
		if find != entryFound || t.Series {
			continue
		}
		s.queue(ctx, user, service, change{kind: kindProgress, durable: true, event: event{Titles: []Title{t}, Entry: &entry}})
	}
}

// The kinds of change sent to AniList and MyAnimeList: the progress of an
// entry raised to the episode played, and its score.
const (
	kindProgress = "progress"
	kindScore    = "score"
)

// The statuses of the entries, as MyAnimeList names them; AniList's are
// read and written as these.
const (
	statusWatching  = "watching"
	statusCompleted = "completed"
	statusOnHold    = "on_hold"
	statusDropped   = "dropped"
	statusPlanned   = "plan_to_watch"
	// statusRepeating is AniList's Rewatching, MyAnimeList's rewatched
	// entries.
	statusRepeating = "repeating"
)

// anilistStatuses map AniList's statuses to MyAnimeList's names.
var anilistStatuses = map[string]string{
	"CURRENT": statusWatching, "COMPLETED": statusCompleted, "PAUSED": statusOnHold, "DROPPED": statusDropped,
	"PLANNING": statusPlanned, "REPEATING": statusRepeating,
}

// anilistStatus is AniList's name of status.
func anilistStatus(status string) string {
	for name, s := range anilistStatuses {
		if s == status {
			return name
		}
	}
	return ""
}

// nextProgress is what playing episode of an entry of total episodes (0
// when unknown) makes of it, listed with status (empty when it is not)
// and progress: the progress raised to the episode, never lowered;
// Watching for an entry not listed or planned; Completed at its last
// episode, or for a movie, unless it is being watched again. changed
// tells whether anything changed.
func nextProgress(status string, progress, total, episode int, movie bool) (next int, nextStatus string, changed bool) {
	target := episode
	if movie {
		target = max(total, 1)
	}
	next = max(progress, target)
	if total > 0 {
		// The mapping may count past the entry's end.
		next = max(progress, min(next, total))
	}
	nextStatus = status
	if status == "" || status == statusPlanned {
		nextStatus = statusWatching
	}
	if (movie || total > 0 && next >= total) && status != statusRepeating {
		nextStatus = statusCompleted
	}
	return next, nextStatus, next != progress || nextStatus != status
}

// listScore is the score of an entry for rating, from 0 to 10: AniList's
// raw score from 0 to 100, MyAnimeList's from 1 to 10, rounded; 0 clears
// it.
func listScore(service string, rating *float64) int {
	if rating == nil {
		return 0
	}
	if service == AniList {
		return int(math.Round(min(max(*rating, 0), 10) * 10))
	}
	return int(math.Round(min(max(*rating, 1), 10)))
}

// sendAniList raises the progress of an entry of AniList, or sets its
// score, from the entry as AniList lists it.
func (s *Service) sendAniList(ctx context.Context, token, kind string, ev event) result {
	e := ev.Entry
	if e == nil || kind != kindProgress && kind != kindScore {
		return result{outcome: refused}
	}
	r, err := s.graphQL(ctx, token, `query ($id: Int) { Media(id: $id, type: ANIME) { id episodes format mediaListEntry { status progress } } }`,
		map[string]any{"id": e.ID})
	if err != nil {
		return result{outcome: retry}
	}
	if res := classifyList(AniList, r); res.outcome != delivered {
		return res
	}
	var answer struct {
		Data struct {
			Media *struct {
				Episodes int    `json:"episodes"`
				Format   string `json:"format"`
				Entry    *struct {
					Status   string `json:"status"`
					Progress int    `json:"progress"`
				} `json:"mediaListEntry"`
			} `json:"Media"`
		} `json:"data"`
	}
	if json.Unmarshal(r.body, &answer) != nil || answer.Data.Media == nil {
		return result{outcome: refused, status: r.status}
	}
	media := answer.Data.Media
	status, progress := "", 0
	if media.Entry != nil {
		status, progress = anilistStatuses[media.Entry.Status], media.Entry.Progress
	}
	var variables map[string]any
	var mutation string
	switch kind {
	case kindProgress:
		next, nextStatus, changed := nextProgress(status, progress, media.Episodes, e.Episode, e.Movie || media.Format == "MOVIE")
		if !changed {
			return result{outcome: delivered, status: r.status}
		}
		mutation = `mutation ($id: Int, $progress: Int, $status: MediaListStatus) { SaveMediaListEntry(mediaId: $id, progress: $progress, status: $status) { id } }`
		variables = map[string]any{"id": e.ID, "progress": next, "status": anilistStatus(nextStatus)}
	default:
		if media.Entry == nil {
			// Only an entry of the list gets a score.
			return result{outcome: delivered, status: r.status}
		}
		mutation = `mutation ($id: Int, $score: Int) { SaveMediaListEntry(mediaId: $id, scoreRaw: $score) { id } }`
		variables = map[string]any{"id": e.ID, "score": listScore(AniList, ev.Score)}
	}
	r, err = s.graphQL(ctx, token, mutation, variables)
	if err != nil {
		return result{outcome: retry}
	}
	return classifyList(AniList, r)
}

// sendMyAnimeList raises the progress of an entry of MyAnimeList, or sets
// its score, from the entry as MyAnimeList lists it.
func (s *Service) sendMyAnimeList(ctx context.Context, token, kind string, ev event) result {
	e := ev.Entry
	if e == nil || kind != kindProgress && kind != kindScore {
		return result{outcome: refused}
	}
	path := "/anime/" + strconv.Itoa(e.ID)
	r, err := s.api(ctx, MyAnimeList, token, s.settings(), http.MethodGet, path, url.Values{"fields": {"num_episodes,media_type,my_list_status"}}, nil)
	if err != nil {
		return result{outcome: retry}
	}
	if res := classifyList(MyAnimeList, r); res.outcome != delivered {
		return res
	}
	var answer struct {
		Episodes  int    `json:"num_episodes"`
		MediaType string `json:"media_type"`
		Listed    *struct {
			Status     string `json:"status"`
			Watched    int    `json:"num_episodes_watched"`
			Rewatching bool   `json:"is_rewatching"`
		} `json:"my_list_status"`
	}
	if json.Unmarshal(r.body, &answer) != nil {
		return result{outcome: refused, status: r.status}
	}
	status, progress := "", 0
	if answer.Listed != nil {
		status, progress = answer.Listed.Status, answer.Listed.Watched
		if answer.Listed.Rewatching {
			status = statusRepeating
		}
	}
	form := url.Values{}
	switch kind {
	case kindProgress:
		next, nextStatus, changed := nextProgress(status, progress, answer.Episodes, e.Episode, e.Movie || answer.MediaType == "movie")
		if !changed {
			return result{outcome: delivered, status: r.status}
		}
		form.Set("num_watched_episodes", strconv.Itoa(next))
		if nextStatus != statusRepeating {
			form.Set("status", nextStatus)
		}
	default:
		if answer.Listed == nil {
			// Only an entry of the list gets a score.
			return result{outcome: delivered, status: r.status}
		}
		form.Set("score", strconv.Itoa(listScore(MyAnimeList, ev.Score)))
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	r, err = s.call(ctx, http.MethodPatch, s.urls[MyAnimeList]+path+"/my_list_status", header, []byte(form.Encode()))
	if err != nil {
		return result{outcome: retry}
	}
	return classifyList(MyAnimeList, r)
}

// Rating is a rating a user gave a movie or a series, or took off it.
type Rating struct {
	// Score is the rating, from 0 to 10, nil once taken off.
	Score *float64
	// Title identifies the movie or series (see Movie, Series and
	// AnimeSeries). It runs in the background.
	Title func(context.Context) (Title, bool)
}

// Rate sends a rating of user to the services they sync their ratings
// with, in the background.
func (s *Service) Rate(user accounts.ID, rating Rating) {
	if s == nil {
		return
	}
	s.later(user, func(ctx context.Context) { s.rate(ctx, user, rating) })
}

// rate sends a rating as the score of its title's entry, to each service
// user syncs ratings with: a movie, or a series of one entry; a series
// spread over several entries counts as skipped.
func (s *Service) rate(ctx context.Context, user accounts.ID, rating Rating) {
	rows, err := s.db.Query(ctx, `SELECT c.service FROM tracking_connections c
		JOIN tracking_imports i ON i.user_id = c.user_id AND i.service = c.service
		WHERE c.user_id = $1 AND i.ratings AND c.problem IS DISTINCT FROM 'reconnect' ORDER BY c.service`, user)
	if err != nil {
		s.logger.Warn("A rating could not be sent to tracking services", "error", err)
		return
	}
	services, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(services) == 0 {
		return
	}
	title, ok := rating.Title(ctx)
	if !ok || title.Episode {
		return
	}
	settings := s.settings()
	for _, service := range services {
		if !animeLists(service) || !Available(service, settings) {
			continue
		}
		entry, find := s.entryOf(service, title)
		switch find {
		case entrySpread:
			if _, err := s.db.Exec(ctx, "UPDATE tracking_imports SET ratings_skipped = ratings_skipped + 1 WHERE user_id = $1 AND service = $2",
				user, service); err != nil {
				s.logger.Debug("A skipped rating could not be counted", "error", err)
			}
		case entryFound:
			s.queue(ctx, user, service, change{kind: kindScore, durable: true, event: event{Titles: []Title{title}, Entry: &entry, Score: rating.Score}})
		}
	}
}
