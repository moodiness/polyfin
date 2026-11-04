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
// The lookups wait their turn in the queue of TMDB's background lookups
// (see tmdb.Queue), which respects TMDB's rate: a pass learns the first
// PassMost movies of each of the server's libraries of movies ahead of any
// request, and a movie a request asks for, a movie's page or a search,
// goes first. Without a key, nothing is looked up, and nothing is
// answered.
package boxset

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
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
// libraries of movies, with how many libraries of movies the server has
// and how many of them were read.
type Catalog interface {
	MovieIDs(ctx context.Context, most int) (ids []map[string]string, libraries, read int, err error)
}

// Options configure a Store; their zero values are the defaults.
type Options struct {
	// Catalog lists the movies passes learn; nil runs no pass.
	Catalog Catalog
	// Queue makes the lookups, with the server's other background lookups
	// on TMDB; nil makes them in a queue of the store's own, configured by
	// Interval and PassCheck (see tmdb.QueueOptions).
	Queue     *tmdb.Queue
	Interval  time.Duration
	PassCheck time.Duration
	// Now is the clock the answers are kept by.
	Now    func() time.Time
	Logger *slog.Logger
}

// Store keeps the collections of movies, and looks up those it does not
// know.
type Store struct {
	db      *pgxpool.Pool
	tmdb    *tmdb.Client
	catalog Catalog
	logger  *slog.Logger
	now     func() time.Time
	queue   *tmdb.Queue
	// own is the queue the store made, nil when it was given one: Close
	// closes it.
	own *tmdb.Queue
}

// New returns a store of db, which asks client, and starts its passes;
// Close stops them when the store made its queue.
func New(db *pgxpool.Pool, client *tmdb.Client, options Options) *Store {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	s := &Store{db: db, tmdb: client, catalog: options.Catalog, logger: options.Logger, now: options.Now, queue: options.Queue}
	if s.queue == nil {
		s.own = tmdb.NewQueue(client, tmdb.QueueOptions{Interval: options.Interval, PassCheck: options.PassCheck, Logger: options.Logger})
		s.queue = s.own
	}
	if s.catalog != nil {
		s.queue.Pass("box sets", s.pass)
	}
	return s
}

// Close stops the lookups and passes of a queue of the store's own, and
// waits for those under way.
func (s *Store) Close() {
	if s == nil {
		return
	}
	s.own.Close()
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
	collections, _, err := s.of(ctx, movies, true)
	return collections, err
}

// of is Of, queueing the movies due for a request when asked, else for a
// pass. It also tells how many movies were due.
func (s *Store) of(ctx context.Context, movies [][]Key, asked bool) ([]*Collection, int, error) {
	collections := make([]*Collection, len(movies))
	if !s.Enabled() {
		return collections, 0, nil
	}
	known, err := s.kept(ctx, movies)
	if err != nil {
		return nil, 0, err
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
	s.enqueue(due, asked)
	return collections, len(due), nil
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
	var keys []string
	for _, movie := range movies {
		if len(movie) > 0 {
			keys = append(keys, movie[0].lookup())
		}
	}
	s.queue.Wait(ctx, keys, d)
}

// lookup is the key of a movie's lookup in the queue.
func (k Key) lookup() string { return "collection|" + k.Provider + "|" + k.ID }

// enqueue queues the lookups of keys, a request's (asked) ahead of passes'.
func (s *Store) enqueue(keys []Key, asked bool) {
	lookups := make([]tmdb.Lookup, 0, len(keys))
	for _, key := range keys {
		lookups = append(lookups, tmdb.Lookup{Key: key.lookup(), Run: func(ctx context.Context) error { return s.lookUp(ctx, key) }})
	}
	s.queue.Add(lookups, asked)
}

// Settled waits until no lookup is waiting or under way.
func (s *Store) Settled(ctx context.Context) error { return s.queue.Settled(ctx) }

// pass queues the movies of the server's libraries of movies whose
// collection is not known, or known for too long, and logs what it did:
// a server whose libraries are all of collections or series has no box
// sets, and its passes learn nothing.
func (s *Store) pass(ctx context.Context) error {
	listing, cancel := context.WithTimeout(ctx, passTime)
	defer cancel()
	ids, libraries, read, err := s.catalog.MovieIDs(listing, PassMost)
	if err != nil {
		return err
	}
	if libraries == 0 {
		s.logger.Info("No library of movies: box sets have none until one is added")
		return nil
	}
	// A movie listed by several libraries is counted, and asked for, once.
	seen := make(map[Key]bool, len(ids))
	movies := make([][]Key, 0, len(ids))
	for _, movie := range ids {
		if keys := Keys(movie); len(keys) > 0 && !seen[keys[0]] {
			seen[keys[0]] = true
			movies = append(movies, keys)
		}
	}
	queued := 0
	// Kept answers are read a thousand movies at a time.
	for start := 0; start < len(movies); start += 1000 {
		_, due, err := s.of(ctx, movies[start:min(start+1000, len(movies))], false)
		if err != nil {
			return err
		}
		queued += due
	}
	s.logger.Info("Box sets: the movies of the libraries of movies were listed", "libraries", libraries, "read", read,
		"movies", len(movies), "queued", queued, "known", len(movies)-queued)
	return nil
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
	var tmdbID *int
	if err == nil {
		if n, convErr := strconv.Atoi(id); convErr == nil && n > 0 {
			tmdbID = &n
		}
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
	if !found {
		tmdbID = nil
	}
	_, err = s.db.Exec(ctx, `INSERT INTO movie_collections (provider, id, found, collection_id, asked_at, tmdb_id) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (provider, id) DO UPDATE SET found = excluded.found, collection_id = excluded.collection_id,
			asked_at = excluded.asked_at, tmdb_id = excluded.tmdb_id`,
		key.Provider, key.ID, found, collection, now, tmdbID)
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
