// Package boxset records the TMDB collection each movie belongs to, which
// Silo's apps show as box sets (see the silo package's boxsets.go).
//
// Addons do not tell which collection a movie is in. While the settings
// hold the server's TMDB key, Polyfin looks movies up on TMDB in the
// background: by their TMDB identifier, else their IMDb one, through
// TMDB's details of the movie, which name its collection, with the
// collection's poster and backdrop. The answers are kept in the database
// for every user, and asked again after fresh, or missing for a movie TMDB
// did not know.
//
// Two kinds of lookups wait their turn. A pass (see passes) learns the
// first PassMost movies of each of the server's libraries of movies ahead
// of any request, once a key is saved, then each passEvery, which finds
// the answers grown old. A movie a request asks for, a movie's page or a
// search, goes first, ahead of the pass.
//
// The lookups respect TMDB's rate: one at a time, at most one every
// interval, each one or two requests to TMDB, which the TMDB client bounds
// with the server's other requests to it (see tmdb.Client.Hold). A lookup
// TMDB refuses for its rate pauses them all as long as its Retry-After
// asks, else rateWait, and is tried again. At most maxPending movies wait
// their turn; others are queued again when next asked for. Without a key,
// nothing is looked up, and nothing is answered.
package boxset

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/tmdb"
)

const (
	// fresh is how long a movie's collection, or its being in none, is
	// kept before TMDB is asked again; missing, how long a movie TMDB did
	// not know is believed unknown.
	fresh   = 90 * 24 * time.Hour
	missing = 30 * 24 * time.Hour
	// interval is the least time between the starts of two lookups: at
	// most four lookups a second, well below TMDB's rate, leaving room for
	// the server's other requests to it.
	interval = 250 * time.Millisecond
	// rateWait is how long the lookups pause after TMDB refused one for
	// its rate without saying how long; rateWaitMost bounds every pause.
	rateWait     = 10 * time.Second
	rateWaitMost = 5 * time.Minute
	// maxPending bounds the movies waiting for their lookup.
	maxPending = 20000
	// lookupTime bounds one lookup.
	lookupTime = time.Minute
	// passEvery is how often a pass runs while a key is saved, and
	// passCheck how often the passes look whether one is due.
	passEvery = 24 * time.Hour
	passCheck = time.Minute
	// passTime bounds the listing of a pass.
	passTime = 30 * time.Minute
)

// PassMost is how many of each library's first titles a pass learns, as
// many as a box set is made of (see the silo package's boxsets.go).
const PassMost = 2000

// ErrNotFound reports a collection, or an image of it, not known.
var ErrNotFound = errors.New("collection not known")

// Key identifies a movie as TMDB is asked about its collection, and as
// the answer is kept: by the identifier it is asked by, "tmdb" or "imdb".
type Key struct {
	Provider string
	ID       string
}

// Keys returns the keys of a movie by its identifiers, under Jellyfin's
// names: its TMDB identifier, then its IMDb one. It is asked about by the
// first; what is kept under any of them tells of it.
func Keys(ids map[string]string) []Key {
	var keys []Key
	if id := ids["Tmdb"]; id != "" {
		keys = append(keys, Key{"tmdb", id})
	}
	if id := ids["Imdb"]; id != "" {
		keys = append(keys, Key{"imdb", id})
	}
	return keys
}

// Collection is a TMDB collection: its identifier, its name, and the
// paths of its poster and backdrop on TMDB's image server, empty when
// TMDB has none.
type Collection struct {
	ID           int64
	Name         string
	PosterPath   string
	BackdropPath string
}

// Catalog lists the movies a pass learns: the identifiers, under
// Jellyfin's names, of the first most movies of each of the server's
// libraries of movies.
type Catalog interface {
	MovieIDs(ctx context.Context, most int) ([]map[string]string, error)
}

// Options configure a Store; their zero values are the defaults.
type Options struct {
	// Catalog lists the movies passes learn; nil runs no pass.
	Catalog Catalog
	// Interval is the least time between the starts of two lookups, and
	// PassCheck how often the passes look whether one is due.
	Interval  time.Duration
	PassCheck time.Duration
	// Now is the clock the answers are kept by.
	Now    func() time.Time
	Logger *slog.Logger
}

// Store keeps the collections of movies, and looks up those it does not
// know.
type Store struct {
	db        *pgxpool.Pool
	tmdb      *tmdb.Client
	catalog   Catalog
	logger    *slog.Logger
	now       func() time.Time
	interval  time.Duration
	passCheck time.Duration

	mu sync.Mutex
	// asked are the lookups requests wait for, before those of passes,
	// ahead; queued holds both, and the one under way.
	asked, ahead []Key
	queued       map[Key]bool
	busy         bool
	// changed is closed, and replaced, whenever the queue changes.
	changed chan struct{}
	wake    chan struct{}
	stop    context.CancelFunc
	done    sync.WaitGroup
}

// New returns a store of db, which asks client, and starts its lookups
// and passes; Close stops them.
func New(db *pgxpool.Pool, client *tmdb.Client, options Options) *Store {
	if options.Interval <= 0 {
		options.Interval = interval
	}
	if options.PassCheck <= 0 {
		options.PassCheck = passCheck
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	s := &Store{db: db, tmdb: client, catalog: options.Catalog, logger: options.Logger, now: options.Now, interval: options.Interval,
		passCheck: options.PassCheck, queued: map[Key]bool{}, changed: make(chan struct{}), wake: make(chan struct{}, 1), stop: stop}
	s.done.Add(1)
	go s.run(ctx)
	if s.catalog != nil {
		s.done.Add(1)
		go s.passes(ctx)
	}
	return s
}

// Close stops the lookups and passes, and waits for those under way.
func (s *Store) Close() {
	if s == nil {
		return
	}
	s.stop()
	s.done.Wait()
}

// Enabled reports whether box sets are known: while the server's TMDB key
// is saved.
func (s *Store) Enabled() bool { return s != nil && s.tmdb.Enabled() }

// kept is what is kept of a movie's collection.
type kept struct {
	found      bool
	collection *Collection
	asked      time.Time
}

// due reports whether TMDB is asked again about the movie.
func (k kept) due(now time.Time) bool {
	if !k.found {
		return now.Sub(k.asked) >= missing
	}
	return now.Sub(k.asked) >= fresh
}

// Of returns the collection each movie belongs to, by its keys (see Keys),
// nil for one in none or not known yet. The movies not known, or known
// for too long, are queued for their lookup, ahead of passes.
func (s *Store) Of(ctx context.Context, movies [][]Key) ([]*Collection, error) {
	return s.of(ctx, movies, true)
}

// of is Of, queueing the movies due for a request when asked, else for a
// pass.
func (s *Store) of(ctx context.Context, movies [][]Key, asked bool) ([]*Collection, error) {
	collections := make([]*Collection, len(movies))
	if !s.Enabled() {
		return collections, nil
	}
	known, err := s.kept(ctx, movies)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var due []Key
	for n, keys := range movies {
		if len(keys) == 0 {
			continue
		}
		var k kept
		ok := false
		for _, key := range keys {
			if k, ok = known[key]; ok {
				break
			}
		}
		if k.due(now) {
			due = append(due, keys[0])
		}
		if ok {
			collections[n] = k.collection
		}
	}
	s.queue(due, asked)
	return collections, nil
}

// kept reads what is kept of movies, by key.
func (s *Store) kept(ctx context.Context, movies [][]Key) (map[Key]kept, error) {
	var providers, ids []string
	for _, keys := range movies {
		for _, key := range keys {
			providers, ids = append(providers, key.Provider), append(ids, key.ID)
		}
	}
	known := map[Key]kept{}
	if len(ids) == 0 {
		return known, nil
	}
	rows, err := s.db.Query(ctx, `SELECT m.provider, m.id, m.found, m.asked_at, c.id, c.name, c.poster_path, c.backdrop_path
		FROM movie_collections m JOIN unnest($1::text[], $2::text[]) AS asked (provider, id) USING (provider, id)
		LEFT JOIN tmdb_collections c ON c.id = m.collection_id`, providers, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key Key
		var k kept
		var id *int64
		var name, poster, backdrop *string
		if err := rows.Scan(&key.Provider, &key.ID, &k.found, &k.asked, &id, &name, &poster, &backdrop); err != nil {
			return nil, err
		}
		if id != nil {
			k.collection = &Collection{ID: *id, Name: *name, PosterPath: *poster, BackdropPath: *backdrop}
		}
		known[key] = k
	}
	return known, rows.Err()
}

// Wait waits, at most for d, until the lookups of movies, those queued,
// are made.
func (s *Store) Wait(ctx context.Context, movies [][]Key, d time.Duration) {
	if !s.Enabled() {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		s.mu.Lock()
		waiting := false
		for _, keys := range movies {
			if len(keys) > 0 && s.queued[keys[0]] {
				waiting = true
			}
		}
		changed := s.changed
		s.mu.Unlock()
		if !waiting {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// queue adds keys to the lookups to make, each once, up to maxPending: a
// request's ahead of those of passes, a key a pass queued moving ahead
// when a request asks for it.
func (s *Store) queue(keys []Key, asked bool) {
	if len(keys) == 0 {
		return
	}
	s.mu.Lock()
	added := false
	for _, key := range keys {
		if s.queued[key] {
			if asked {
				if i := index(s.ahead, key); i >= 0 {
					s.ahead = append(s.ahead[:i], s.ahead[i+1:]...)
					s.asked = append(s.asked, key)
					added = true
				}
			}
			continue
		}
		if len(s.asked)+len(s.ahead) >= maxPending {
			continue
		}
		s.queued[key] = true
		if asked {
			s.asked = append(s.asked, key)
		} else {
			s.ahead = append(s.ahead, key)
		}
		added = true
	}
	if added {
		s.changedLocked()
	}
	s.mu.Unlock()
	if added {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

func index(keys []Key, key Key) int {
	for i, k := range keys {
		if k == key {
			return i
		}
	}
	return -1
}

// changedLocked tells those waiting for the queue that it changed.
func (s *Store) changedLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// Settled waits until no lookup is waiting or under way.
func (s *Store) Settled(ctx context.Context) error {
	for {
		s.mu.Lock()
		idle, changed := len(s.asked)+len(s.ahead) == 0 && !s.busy, s.changed
		s.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// next takes the first lookup waiting, a request's first, and marks the
// store busy.
func (s *Store) next() (Key, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var key Key
	switch {
	case len(s.asked) > 0:
		key, s.asked = s.asked[0], s.asked[1:]
	case len(s.ahead) > 0:
		key, s.ahead = s.ahead[0], s.ahead[1:]
	default:
		return Key{}, false
	}
	s.busy = true
	s.changedLocked()
	return key, true
}

// finished ends a lookup of key: again puts it back first in line, among
// a request's.
func (s *Store) finished(key Key, again bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if again {
		s.asked = append([]Key{key}, s.asked...)
	} else {
		delete(s.queued, key)
	}
	s.busy = false
	s.changedLocked()
}

// drop forgets every lookup waiting: without a key, none is made.
func (s *Store) drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked, s.ahead, s.queued = nil, nil, map[Key]bool{}
	s.changedLocked()
}

// run makes the lookups waiting, one at a time, at most one every
// interval, until ctx ends.
func (s *Store) run(ctx context.Context) {
	defer s.done.Done()
	var last time.Time
	for {
		key, ok := s.next()
		if !ok {
			select {
			case <-s.wake:
				continue
			case <-ctx.Done():
				return
			}
		}
		if !s.Enabled() {
			s.finished(key, false)
			s.drop()
			continue
		}
		if wait := s.interval - time.Since(last); wait > 0 {
			if !sleep(ctx, wait) {
				s.finished(key, false)
				return
			}
		}
		last = time.Now()
		lookup, cancel := context.WithTimeout(ctx, lookupTime)
		err := s.lookUp(lookup, key)
		cancel()
		var limited *tmdb.RateLimited
		switch {
		case errors.As(err, &limited):
			pause := min(max(limited.RetryAfter, 0), rateWaitMost)
			if pause == 0 {
				pause = rateWait
			}
			s.logger.Info("TMDB asked to slow down: looking up the collections of movies again later", "wait", pause)
			s.finished(key, true)
			if !sleep(ctx, pause) {
				return
			}
			continue
		case err != nil && ctx.Err() == nil:
			s.logger.Debug("A movie's collection could not be looked up", "provider", key.Provider, "error", err)
		}
		s.finished(key, false)
	}
}

// passes runs a pass when one is due: while a key is saved, the first
// time, and passEvery after the last, each run's lookups being queued
// after a request's.
func (s *Store) passes(ctx context.Context) {
	defer s.done.Done()
	var last time.Time
	for {
		// The first check waits passCheck too: the server is starting.
		if !sleep(ctx, s.passCheck) {
			return
		}
		if s.Enabled() && (last.IsZero() || time.Since(last) >= passEvery) {
			if err := s.pass(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Warn("The movies' collections could not be listed for box sets", "error", err)
				// Tried again in an hour.
				last = time.Now().Add(time.Hour - passEvery)
			} else {
				last = time.Now()
			}
		}
		if !s.Enabled() {
			last = time.Time{}
		}
	}
}

// pass queues the movies of the server's libraries of movies whose
// collection is not known, or known for too long.
func (s *Store) pass(ctx context.Context) error {
	listing, cancel := context.WithTimeout(ctx, passTime)
	defer cancel()
	ids, err := s.catalog.MovieIDs(listing, PassMost)
	if err != nil {
		return err
	}
	movies := make([][]Key, 0, len(ids))
	for _, movie := range ids {
		if keys := Keys(movie); len(keys) > 0 {
			movies = append(movies, keys)
		}
	}
	// Kept answers are read a thousand movies at a time.
	for start := 0; start < len(movies); start += 1000 {
		if _, err := s.of(ctx, movies[start:min(start+1000, len(movies))], false); err != nil {
			return err
		}
	}
	return nil
}

// sleep waits for d, and reports false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// lookUp asks TMDB for a movie's collection and keeps its answer, with
// the collection's name and artwork. A movie TMDB does not know is kept as
// such; a lookup that fails keeps nothing.
func (s *Store) lookUp(ctx context.Context, key Key) error {
	id := key.ID
	var err error
	if key.Provider == "imdb" {
		id, err = s.tmdb.FindByIMDb(ctx, tmdb.Movie, key.ID)
	}
	var ref *tmdb.CollectionRef
	if err == nil {
		ref, err = s.tmdb.MovieCollection(ctx, id)
	}
	found := err == nil
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return err
	}
	now := s.now()
	var collection *int64
	if ref != nil {
		if _, err := s.db.Exec(ctx, `INSERT INTO tmdb_collections (id, name, poster_path, backdrop_path, updated_at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, poster_path = excluded.poster_path,
				backdrop_path = excluded.backdrop_path, updated_at = excluded.updated_at`,
			ref.ID, ref.Name, ref.PosterPath, ref.BackdropPath, now); err != nil {
			return err
		}
		collection = &ref.ID
	}
	_, err = s.db.Exec(ctx, `INSERT INTO movie_collections (provider, id, found, collection_id, asked_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, id) DO UPDATE SET found = excluded.found, collection_id = excluded.collection_id,
			asked_at = excluded.asked_at`,
		key.Provider, key.ID, found, collection, now)
	return err
}

// Image returns the address on TMDB's image server of a collection's
// poster ("Primary") or backdrop ("Backdrop"), in its original size, for
// Polyfin to download and relay as it does titles' artwork; ErrNotFound
// for a collection or an image not known, or without a key.
func (s *Store) Image(ctx context.Context, id int64, kind string) (string, error) {
	if !s.Enabled() {
		return "", ErrNotFound
	}
	column := map[string]string{"Primary": "poster_path", "Backdrop": "backdrop_path"}[kind]
	if column == "" {
		return "", ErrNotFound
	}
	var path string
	err := s.db.QueryRow(ctx, `SELECT `+column+` FROM tmdb_collections WHERE id = $1`, id).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && path == "" {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return s.tmdb.ImageURL(path, "original"), nil
}
