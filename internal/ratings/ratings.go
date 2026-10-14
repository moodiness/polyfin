// Package ratings looks movies and series up on MDBList, with the server's
// own API key (Settings.MDBListRatingsKey), for three ratings: their IMDb
// rating, their critics' score and their audience score.
//
// Nothing waits for MDBList. Kept answers at once with what the database
// keeps, and has the titles it does not keep, or keeps for too long, asked
// about in the background, in batches of up to batchSize titles a request:
// the next listing shows them. A title is asked about by its IMDb
// identifier, else its TMDB one, and its answer is kept by that identifier
// for every user: asked again after freshFor, or oldFreshFor for a title
// released over a year before, or missingFor when MDBList did not know it.
//
// MDBList counts a daily allowance of requests, which it tells in the
// headers of its answers, and a limit over 5 minutes. Past either, it
// answers 429 with a Retry-After: Polyfin asks nothing more until then,
// and after "Daily API limit exceeded!" until the daily reset it told. A
// request that fails otherwise leaves its titles waiting, and MDBList
// unasked for a while.
package ratings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// MDBList is the address of MDBList's API.
const MDBList = "https://api.mdblist.com"

const (
	// batchSize is the most titles one request asks about: MDBList's
	// batch lookup takes up to 200.
	batchSize = 100
	// freshFor is how long a title's ratings are kept before they are
	// asked for again, oldFreshFor for a title released over a year
	// before, whose ratings change little, and missingFor how long a title
	// MDBList did not know is believed unknown.
	freshFor    = 7 * 24 * time.Hour
	oldFreshFor = 30 * 24 * time.Hour
	missingFor  = 30 * 24 * time.Hour
	oldAfter    = 365 * 24 * time.Hour
	// requestGap is the least time between two requests, well within
	// MDBList's 1,000 requests over 5 minutes.
	requestGap = time.Second
	// failedFor leaves MDBList unasked for a while after a request failed
	// or answered 429 without a Retry-After, and refusedFor after it
	// refused the key or answered what Polyfin does not read.
	failedFor  = time.Minute
	refusedFor = time.Hour
	// maxWaiting bounds the titles waiting to be asked about: those listed
	// past it are asked about when they are listed again.
	maxWaiting = 20000
	// askTimeout bounds a request to MDBList, saveTimeout the write of its
	// answers, and maxAnswer the bytes read of one.
	askTimeout  = 30 * time.Second
	saveTimeout = 10 * time.Second
	maxAnswer   = 16 << 20
)

// dailyLimit is the error MDBList answers once the daily allowance is
// spent, as opposed to the limit over 5 minutes.
const dailyLimit = "Daily API limit exceeded!"

// Title is a movie or a series, by the identifiers MDBList is asked with:
// its IMDb identifier, else its TMDB one; Released, when known, tells its
// age when MDBList does not.
type Title struct {
	Series   bool
	IMDb     string
	TMDB     string
	Released *time.Time
}

// Scores are a title's ratings, each nil when MDBList has none: the IMDb
// rating, from 0 to 10, and the critics' and audience scores, from 0 to
// 100.
type Scores struct {
	IMDb     *float64
	Critics  *float64
	Audience *float64
}

// key identifies a title as MDBList is asked about it, and as its answer
// is kept: its kind, "movie" or "show", and its identifier.
type key struct {
	kind     string
	provider string
	id       string
}

// keyOf returns the key a title is asked about by; ok is false for a title
// without an IMDb or a TMDB identifier.
func keyOf(t Title) (key, bool) {
	kind := "movie"
	if t.Series {
		kind = "show"
	}
	if imdb := strings.ToLower(strings.TrimSpace(t.IMDb)); len(imdb) > 2 && strings.HasPrefix(imdb, "tt") && digits(imdb[2:]) {
		return key{kind, "imdb", imdb}, true
	}
	if tmdb := strings.TrimSpace(t.TMDB); digits(tmdb) {
		return key{kind, "tmdb", tmdb}, true
	}
	return key{}, false
}

func digits(text string) bool {
	if text == "" || len(text) > 12 {
		return false
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// answer is what MDBList said of a title: its scores, whether it knew it,
// its release when known, and when it was asked.
type answer struct {
	scores   Scores
	found    bool
	released *time.Time
	asked    time.Time
}

// stale reports whether a title answered about is asked about again.
func (a answer) stale(now time.Time) bool {
	age := now.Sub(a.asked)
	switch {
	case !a.found:
		return age >= missingFor
	case a.released != nil && a.asked.Sub(*a.released) > oldAfter:
		return age >= oldFreshFor
	}
	return age >= freshFor
}

// Status is how the lookups stand, for System › Health.
type Status struct {
	// Enabled tells that a key is saved.
	Enabled bool
	// Known tells that MDBList told its daily allowance: Limit requests,
	// Remaining of them left until ResetsAt.
	Known     bool
	Limit     int
	Remaining int
	ResetsAt  time.Time
	// HeldUntil is when MDBList is asked again after it refused requests
	// or failed, zero while it is not held; Held tells why: "limit" for a
	// 429, "refused" for a key refused, "failed" otherwise.
	HeldUntil time.Time
	Held      string
	// Waiting counts the titles waiting to be asked about, and Kept those
	// whose answer is kept.
	Waiting int
	Kept    int
}

// Service looks titles' ratings up on MDBList while the settings hold a
// key. A nil Service looks nothing up.
type Service struct {
	db *pgxpool.Pool
	// base is MDBList's address, without a trailing slash.
	base      string
	settings  func() accounts.Settings
	client    *http.Client
	userAgent string
	logger    *slog.Logger
	now       func() time.Time
	// gap is the least time between two requests: requestGap, shorter in
	// tests.
	gap time.Duration

	// loaded tells that kept holds the database's answers; loadMu orders
	// their loading.
	loaded atomic.Bool
	loadMu sync.Mutex

	mu sync.Mutex
	// kept are the answers, by key.
	kept map[key]answer
	// waiting are the titles to ask about, in the order they were listed,
	// with their release; asking those in the request under way.
	waiting map[key]*time.Time
	order   []key
	asking  map[key]bool
	// forKey is the key the limits below are for: another one starts
	// them anew.
	forKey string
	limit  limits
	// wake wakes the worker; stop and done end it.
	wake    chan struct{}
	started bool
	stop    context.CancelFunc
	done    chan struct{}
}

// limits are what MDBList told of its allowance, and how long it is left
// alone.
type limits struct {
	known     bool
	limit     int
	remaining int
	resetsAt  time.Time
	heldUntil time.Time
	held      string
}

// New returns a service asking MDBList at base, and keeping its answers in
// db, while settings hold a key. version is Polyfin's, which the
// User-Agent names.
func New(db *pgxpool.Pool, base, version string, logger *slog.Logger, settings func() accounts.Settings) *Service {
	return &Service{
		db:        db,
		base:      strings.TrimSuffix(base, "/"),
		settings:  settings,
		client:    &http.Client{},
		userAgent: "Polyfin/" + version,
		logger:    logger,
		now:       time.Now,
		gap:       requestGap,
		kept:      map[key]answer{},
		waiting:   map[key]*time.Time{},
		asking:    map[key]bool{},
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

// SetClock replaces the clock answers are dated and aged on, before the
// service is used: tests move it to age them.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Close stops the lookups under way and waits for them to end.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	started, stop := s.started, s.stop
	s.mu.Unlock()
	if started {
		stop()
		<-s.done
	}
}

// key is the server's key, empty for none.
func (s *Service) key() string {
	if s == nil {
		return ""
	}
	return s.settings().MDBListRatingsKey
}

// Enabled reports whether titles' ratings are looked up: the settings hold
// a key.
func (s *Service) Enabled() bool { return s.key() != "" }

// Kept returns the scores kept of titles, by their place in titles, nil
// when none are kept or no key is saved. Titles without an IMDb or TMDB
// identifier have none. Those not kept, or kept for too long, are asked
// about in the background: Kept never waits for MDBList.
func (s *Service) Kept(ctx context.Context, titles []Title) []Scores {
	if len(titles) == 0 || !s.Enabled() || !s.load(ctx) {
		return nil
	}
	now := s.now()
	var result []Scores
	asked := false
	s.mu.Lock()
	for i, t := range titles {
		k, ok := keyOf(t)
		if !ok {
			continue
		}
		a, kept := s.kept[k]
		if kept && a.found {
			if result == nil {
				result = make([]Scores, len(titles))
			}
			result[i] = a.scores
		}
		if (!kept || a.stale(now)) && s.want(k, t.Released) {
			asked = true
		}
	}
	if asked {
		s.startLocked()
	}
	s.mu.Unlock()
	if asked {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return result
}

// Pending reports whether a title is waiting to be asked about, or being
// asked about.
func (s *Service) Pending(t Title) bool {
	k, ok := keyOf(t)
	if !ok || !s.Enabled() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, waiting := s.waiting[k]
	return waiting || s.asking[k]
}

// want adds a title to those waiting, unless it is already waiting or
// asked about, or too many wait. It reports whether it is added; mu is
// held.
func (s *Service) want(k key, released *time.Time) bool {
	if _, waiting := s.waiting[k]; waiting || s.asking[k] || len(s.waiting) >= maxWaiting {
		return false
	}
	s.waiting[k] = released
	s.order = append(s.order, k)
	return true
}

// startLocked starts the worker once; mu is held.
func (s *Service) startLocked() {
	if s.started {
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	s.started, s.stop = true, stop
	go s.run(ctx)
}

// load reads the answers kept in the database, once. It reports whether
// they are read; they are read again at the next call when they could
// not be.
func (s *Service) load(ctx context.Context) bool {
	if s.loaded.Load() {
		return true
	}
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	if s.loaded.Load() {
		return true
	}
	rows, err := s.db.Query(ctx, "SELECT kind, provider, id, found, imdb, critics, audience, released, asked_at FROM title_ratings")
	if err != nil {
		s.loadFailed(ctx, err)
		return false
	}
	kept := map[key]answer{}
	var k key
	var a answer
	var imdb, critics, audience *float32
	_, err = pgx.ForEachRow(rows, []any{&k.kind, &k.provider, &k.id, &a.found, &imdb, &critics, &audience, &a.released, &a.asked}, func() error {
		a.scores = Scores{IMDb: widen(imdb), Critics: widen(critics), Audience: widen(audience)}
		kept[k] = a
		// The next row's are its own.
		imdb, critics, audience, a.released = nil, nil, nil, nil
		return nil
	})
	if err != nil {
		s.loadFailed(ctx, err)
		return false
	}
	s.mu.Lock()
	// Answers saved meanwhile are newer.
	for k, a := range s.kept {
		kept[k] = a
	}
	s.kept = kept
	s.mu.Unlock()
	s.loaded.Store(true)
	return true
}

func (s *Service) loadFailed(ctx context.Context, err error) {
	if ctx.Err() == nil {
		s.logger.Warn("Reading the ratings kept failed", "error", err)
	}
}

// widen returns a score read as a real, rounded to the tenth it was given
// with.
func widen(value *float32) *float64 {
	if value == nil {
		return nil
	}
	wide, _ := strconv.ParseFloat(strconv.FormatFloat(float64(*value), 'f', -1, 32), 64)
	return &wide
}

// Forget drops what is kept of titles, which are asked about again when
// next listed.
func (s *Service) Forget(ctx context.Context, titles ...Title) error {
	if s == nil {
		return nil
	}
	var kinds, providers, ids []string
	s.mu.Lock()
	for _, t := range titles {
		if k, ok := keyOf(t); ok {
			delete(s.kept, k)
			kinds, providers, ids = append(kinds, k.kind), append(providers, k.provider), append(ids, k.id)
		}
	}
	s.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM title_ratings r USING unnest($1::text[], $2::text[], $3::text[]) AS f(kind, provider, id)
		WHERE r.kind = f.kind AND r.provider = f.provider AND r.id = f.id`, kinds, providers, ids)
	return err
}

// Status tells how the lookups stand.
func (s *Service) Status() Status {
	key := s.key()
	if key == "" {
		return Status{}
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Enabled: true, Waiting: len(s.waiting) + len(s.asking), Kept: len(s.kept)}
	if s.forKey != key {
		return status
	}
	l := s.limit
	if l.known {
		status.Known, status.Limit, status.Remaining, status.ResetsAt = true, l.limit, l.remaining, l.resetsAt
		// The allowance is whole again once the reset it told is past.
		if !l.resetsAt.IsZero() && !now.Before(l.resetsAt) {
			status.Remaining, status.ResetsAt = l.limit, time.Time{}
		}
	}
	if now.Before(l.heldUntil) {
		status.HeldUntil, status.Held = l.heldUntil, l.held
	}
	return status
}

// ErrKeyRefused reports a key MDBList refuses.
var ErrKeyRefused = errors.New("MDBList refused the key")

// CheckKey asks MDBList whether it accepts key, and keeps the allowance it
// tells: ErrKeyRefused when it refuses it, another error when it could not
// be asked.
func (s *Service) CheckKey(ctx context.Context, key string) error {
	if s == nil {
		return errors.New("no MDBList service")
	}
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/user?"+url.Values{"apikey": {key}}.Encode(), nil)
	if err != nil {
		return errors.New("malformed request")
	}
	response, body, err := s.do(request)
	if err != nil {
		return err
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized, response.StatusCode == http.StatusForbidden:
		return ErrKeyRefused
	case response.StatusCode == http.StatusTooManyRequests:
		// A key past its allowance is a key MDBList knows.
		s.told(key, response.Header, s.now())
		return nil
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	s.told(key, response.Header, s.now())
	var user struct {
		Limit     *int   `json:"rate_limit"`
		Remaining *int   `json:"rate_limit_remaining"`
		Reset     *int64 `json:"rate_limit_reset"`
	}
	if json.Unmarshal(body, &user) == nil && user.Limit != nil && user.Remaining != nil {
		s.mu.Lock()
		s.adopt(key)
		s.limit.known, s.limit.limit, s.limit.remaining = true, *user.Limit, *user.Remaining
		if user.Reset != nil {
			s.limit.resetsAt = time.Unix(*user.Reset, 0)
		}
		s.mu.Unlock()
	}
	return nil
}

// do sends a request and reads the answer. The error never holds the URL,
// which carries the key.
func (s *Service) do(request *http.Request) (*http.Response, []byte, error) {
	request.Header.Set("User-Agent", s.userAgent)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, nil, withoutURL(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer))
	if err != nil {
		return nil, nil, withoutURL(err)
	}
	return response, body, nil
}

func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// adopt starts the limits anew when they were for another key; mu is held.
func (s *Service) adopt(key string) {
	if s.forKey != key {
		s.forKey, s.limit = key, limits{}
	}
}

// told keeps the allowance MDBList's headers tell, for key.
func (s *Service) told(key string, header http.Header, now time.Time) {
	limit, errLimit := strconv.Atoi(header.Get("X-RateLimit-Limit"))
	remaining, errRemaining := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	if errLimit != nil || errRemaining != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adopt(key)
	s.limit.known, s.limit.limit, s.limit.remaining = true, limit, remaining
	if reset, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		s.limit.resetsAt = time.Unix(reset, 0)
	} else if !s.limit.resetsAt.After(now) {
		s.limit.resetsAt = time.Time{}
	}
}

// hold leaves MDBList unasked until until, for why; mu is held.
func (s *Service) hold(until time.Time, why string) {
	if until.After(s.limit.heldUntil) {
		s.limit.heldUntil, s.limit.held = until, why
	}
}

// batch is a request to make: titles of one kind, by one provider.
type batch struct {
	kind, provider string
	keys           []key
	released       map[key]*time.Time
}

// run asks about the titles waiting, a batch at a time, gap apart, until
// ctx ends. Titles listed while MDBList is held wake it, so that a new key
// is used at once.
func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		b, wait := s.next()
		if b != nil {
			s.ask(ctx, *b)
			timer.Reset(s.gap)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		if wait > 0 {
			timer.Reset(wait)
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// next takes the next batch to ask about, nil when there is none now, with
// how long until MDBList may be asked again, 0 to wait for titles. Without
// a key, the titles waiting are dropped.
func (s *Service) next() (*batch, time.Duration) {
	apiKey := s.key()
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if apiKey == "" {
		clear(s.waiting)
		s.order = nil
		return nil, 0
	}
	s.adopt(apiKey)
	if len(s.order) == 0 {
		return nil, 0
	}
	if wait := s.limit.heldUntil.Sub(now); wait > 0 {
		return nil, wait
	}
	first := s.order[0]
	b := &batch{kind: first.kind, provider: first.provider, released: map[key]*time.Time{}}
	rest := s.order[:0]
	for _, k := range s.order {
		if len(b.keys) < batchSize && k.kind == first.kind && k.provider == first.provider {
			b.keys = append(b.keys, k)
			b.released[k] = s.waiting[k]
			delete(s.waiting, k)
			s.asking[k] = true
			continue
		}
		rest = append(rest, k)
	}
	clear(s.order[len(rest):])
	s.order = rest
	return b, 0
}

// ask asks MDBList about a batch and keeps its answers. A batch MDBList did
// not answer waits again, first in line.
func (s *Service) ask(ctx context.Context, b batch) {
	answers, err := s.fetch(ctx, s.key(), b)
	now := s.now()
	if err != nil {
		s.mu.Lock()
		var again []key
		for _, k := range b.keys {
			delete(s.asking, k)
			if _, waiting := s.waiting[k]; !waiting {
				s.waiting[k] = b.released[k]
				again = append(again, k)
			}
		}
		s.order = append(again, s.order...)
		var limited *heldError
		if errors.As(err, &limited) {
			s.hold(limited.until, limited.why)
		} else {
			s.hold(now.Add(failedFor), "failed")
		}
		s.mu.Unlock()
		if ctx.Err() == nil {
			s.logger.Info("MDBList did not answer about titles' ratings", "titles", len(b.keys), "error", err)
		}
		return
	}
	saved := make(map[key]answer, len(b.keys))
	for _, k := range b.keys {
		a := answers[k]
		a.asked = now
		if a.released == nil {
			a.released = b.released[k]
		}
		saved[k] = a
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	if err := s.save(saveCtx, saved); err != nil {
		s.logger.Warn("Saving titles' ratings failed", "error", err)
	}
	cancel()
	s.mu.Lock()
	for k, a := range saved {
		delete(s.asking, k)
		s.kept[k] = a
	}
	s.mu.Unlock()
	s.logger.Debug("MDBList answered about titles' ratings", "titles", len(b.keys), "found", len(answers))
}

// save keeps answers in the database.
func (s *Service) save(ctx context.Context, answers map[key]answer) error {
	n := len(answers)
	kinds, providers, ids := make([]string, 0, n), make([]string, 0, n), make([]string, 0, n)
	found := make([]bool, 0, n)
	imdb, critics, audience := make([]*float64, 0, n), make([]*float64, 0, n), make([]*float64, 0, n)
	released, asked := make([]*time.Time, 0, n), make([]time.Time, 0, n)
	for k, a := range answers {
		kinds, providers, ids = append(kinds, k.kind), append(providers, k.provider), append(ids, k.id)
		found = append(found, a.found)
		imdb, critics, audience = append(imdb, a.scores.IMDb), append(critics, a.scores.Critics), append(audience, a.scores.Audience)
		released, asked = append(released, a.released), append(asked, a.asked)
	}
	_, err := s.db.Exec(ctx, `INSERT INTO title_ratings (kind, provider, id, found, imdb, critics, audience, released, asked_at)
		SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::boolean[], $5::real[], $6::real[], $7::real[], $8::date[], $9::timestamptz[])
		ON CONFLICT (kind, provider, id) DO UPDATE SET found = excluded.found, imdb = excluded.imdb, critics = excluded.critics,
			audience = excluded.audience, released = excluded.released, asked_at = excluded.asked_at`,
		kinds, providers, ids, found, imdb, critics, audience, released, asked)
	return err
}

// heldError reports an answer that leaves MDBList unasked until until.
type heldError struct {
	status int
	until  time.Time
	why    string
}

func (e *heldError) Error() string {
	return fmt.Sprintf("HTTP %d, asked again at %s", e.status, e.until.Format(time.RFC3339))
}

// fetch asks MDBList about a batch, and returns its answers by key: titles
// it does not know have none.
func (s *Service) fetch(ctx context.Context, apiKey string, b batch) (map[key]answer, error) {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	// TMDB identifiers are numbers, IMDb ones text.
	ids := make([]any, len(b.keys))
	for i, k := range b.keys {
		if b.provider == "tmdb" {
			ids[i] = json.Number(k.id)
		} else {
			ids[i] = k.id
		}
	}
	body, _ := json.Marshal(map[string]any{"ids": ids})
	target := s.base + "/" + b.provider + "/" + b.kind + "?" + url.Values{"apikey": {apiKey}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("malformed request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, data, err := s.do(request)
	if err != nil {
		return nil, err
	}
	now := s.now()
	s.told(apiKey, response.Header, now)
	switch status := response.StatusCode; {
	case status == http.StatusTooManyRequests:
		return nil, s.limited(response, data, now)
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		s.logger.Warn("MDBList refused the key for ratings: check it under Settings › Content")
		return nil, &heldError{status: status, until: now.Add(refusedFor), why: "refused"}
	case status >= 500:
		return nil, fmt.Errorf("HTTP %d", status)
	case status != http.StatusOK:
		return nil, &heldError{status: status, until: now.Add(refusedFor), why: "failed"}
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, &heldError{status: response.StatusCode, until: now.Add(refusedFor), why: "failed"}
	}
	answers := map[key]answer{}
	for _, raw := range items {
		var info mediaInfo
		if json.Unmarshal(raw, &info) != nil {
			continue
		}
		k := key{kind: b.kind, provider: b.provider}
		switch b.provider {
		case "imdb":
			k.id = strings.ToLower(info.IDs.IMDb)
		case "tmdb":
			if info.IDs.TMDB == nil {
				continue
			}
			k.id = strconv.FormatInt(*info.IDs.TMDB, 10)
		}
		if _, asked := b.released[k]; asked {
			answers[k] = info.answer()
		}
	}
	return answers, nil
}

// limited reads a 429: held for its Retry-After, and after the daily
// allowance is spent, until the reset MDBList told.
func (s *Service) limited(response *http.Response, body []byte, now time.Time) error {
	until := now.Add(failedFor)
	if seconds, err := strconv.Atoi(strings.TrimSpace(response.Header.Get("Retry-After"))); err == nil && seconds >= 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	}
	var refusal struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &refusal)
	if strings.EqualFold(strings.TrimSpace(refusal.Error), dailyLimit) {
		if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && time.Unix(reset, 0).After(until) {
			until = time.Unix(reset, 0)
		}
		s.mu.Lock()
		s.limit.known, s.limit.remaining = true, 0
		s.mu.Unlock()
	}
	return &heldError{status: response.StatusCode, until: until, why: "limit"}
}

// mediaInfo is what MDBList's batch lookup answers of a title, as Polyfin
// reads it.
type mediaInfo struct {
	IDs struct {
		IMDb string `json:"imdb"`
		TMDB *int64 `json:"tmdb"`
	} `json:"ids"`
	Released string `json:"released"`
	Year     *int   `json:"year"`
	Ratings  []struct {
		Source string   `json:"source"`
		Value  *float64 `json:"value"`
	} `json:"ratings"`
}

// The sources of MDBList's ratings Polyfin reads, by MDBList's names of
// them: the IMDb rating, the critics' score, and the audience score, which
// MDBList has named two ways.
var sources = map[string]string{
	"imdb":             "imdb",
	"tomatoes":         "critics",
	"popcorn":          "audience",
	"tomatoesaudience": "audience",
}

// answer returns what is kept of a title MDBList knows: scores out of
// range, or zero, are none.
func (info mediaInfo) answer() answer {
	a := answer{found: true}
	for _, rating := range info.Ratings {
		value := rating.Value
		if value == nil || *value <= 0 {
			continue
		}
		switch sources[strings.ToLower(rating.Source)] {
		case "imdb":
			if *value <= 10 {
				a.scores.IMDb = new(*value)
			}
		case "critics":
			if *value <= 100 {
				a.scores.Critics = new(*value)
			}
		case "audience":
			if *value <= 100 && a.scores.Audience == nil {
				a.scores.Audience = new(*value)
			}
		}
	}
	if released, err := time.Parse(time.DateOnly, info.Released); err == nil {
		a.released = &released
	} else if info.Year != nil && *info.Year > 1800 {
		released := time.Date(*info.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
		a.released = &released
	}
	return a
}
