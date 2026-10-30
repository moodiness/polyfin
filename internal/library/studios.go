package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/moodiness/polyfin/internal/tmdb"
)

// Studios and networks from TMDB, for smart lists' studio rules.
//
// Addons do not tell who made a title. While the settings hold the
// server's TMDB key, a smart list whose rules look at studios has TMDB
// asked, in the background, for the studios of the titles it judges and
// the networks of its series, a few at a time, by their TMDB identifier,
// else their IMDb one. The answers are kept in the database for every
// user and asked again after studiosFresh, or studiosMissing for a title
// TMDB did not know; until a title's are known, the rules that look at
// them leave it out, so that the list grows as they come, as it does with
// descriptions. An administrator's edit of a title's studios wins.

const (
	// studiosFresh is how long a title's studios are kept before TMDB is
	// asked again, and studiosMissing how long a title TMDB did not know is
	// believed unknown.
	studiosFresh   = 30 * 24 * time.Hour
	studiosMissing = 30 * 24 * time.Hour
	// studiosLookupTime bounds a lookup of a title's studios.
	studiosLookupTime = time.Minute
)

// studiosKey identifies a title as TMDB is asked about its studios, and as
// they are kept: its kind, and the identifier it is asked by.
type studiosKey struct {
	kind     tmdb.Kind
	provider string
	id       string
}

// studiosKeyOf returns the key of a movie or series, by its TMDB
// identifier, else its IMDb one; ok is false for other items and titles
// without either.
func studiosKeyOf(item Item) (studiosKey, bool) {
	kind := tmdb.Movie
	switch item.Kind {
	case KindMovie:
	case KindSeries:
		kind = tmdb.TV
	default:
		return studiosKey{}, false
	}
	if id := item.ProviderIDs["Tmdb"]; id != "" {
		return studiosKey{kind, "tmdb", id}, true
	}
	if id := item.ProviderIDs["Imdb"]; id != "" {
		return studiosKey{kind, "imdb", id}, true
	}
	return studiosKey{}, false
}

// keptStudios returns the studios kept of items, by their place in items:
// known is set for those whose studios are known, an administrator's edit
// of them or TMDB's answer, and for those that cannot be known, without
// TMDB key or identifier, which have none. Those not kept, or kept for too
// long, are asked about in the background, at most smartLookups of them.
func (s *Service) keptStudios(ctx context.Context, items []Item) (studios [][]string, known []bool, err error) {
	studios, known = make([][]string, len(items)), make([]bool, len(items))
	enabled := s.tmdb.Enabled()
	keys := make([]studiosKey, len(items))
	var kinds, providers, ids []string
	for i, item := range items {
		if len(item.Studios) > 0 {
			studios[i], known[i] = item.Studios, true
			continue
		}
		key, ok := studiosKeyOf(item)
		if !ok || !enabled {
			known[i] = true
			continue
		}
		keys[i] = key
		kinds, providers, ids = append(kinds, string(key.kind)), append(providers, key.provider), append(ids, key.id)
	}
	if len(ids) == 0 {
		return studios, known, nil
	}
	rows, err := s.db.Query(ctx, `SELECT t.kind, t.provider, t.id, t.found, t.studios, t.asked_at
		FROM title_studios t JOIN unnest($1::text[], $2::text[], $3::text[]) AS asked (kind, provider, id)
		USING (kind, provider, id)`, kinds, providers, ids)
	if err != nil {
		return nil, nil, err
	}
	type answer struct {
		found   bool
		studios []string
		asked   time.Time
	}
	kept := map[studiosKey]answer{}
	for rows.Next() {
		var key studiosKey
		var kind string
		var a answer
		if err := rows.Scan(&kind, &key.provider, &key.id, &a.found, &a.studios, &a.asked); err != nil {
			rows.Close()
			return nil, nil, err
		}
		key.kind = tmdb.Kind(kind)
		kept[key] = a
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	now := s.now()
	var due []studiosKey
	for i, key := range keys {
		if key.id == "" {
			continue
		}
		a, ok := kept[key]
		if ok {
			studios[i], known[i] = a.studios, true
		}
		if !ok || a.found && now.Sub(a.asked) >= studiosFresh || !a.found && now.Sub(a.asked) >= studiosMissing {
			due = append(due, key)
		}
	}
	s.studiosLater(ctx, due)
	return studios, known, nil
}

// studiosLater asks TMDB in the background for the studios of titles, at
// most smartLookups of them, each once at a time; the requests to TMDB are
// bounded across the server (see tmdb.Client.Hold).
func (s *Service) studiosLater(ctx context.Context, keys []studiosKey) {
	if keptOnly(ctx) {
		return
	}
	for _, key := range keys[:min(len(keys), smartLookups)] {
		s.background(ctx, fmt.Sprintf("studios %s %s %s", key.kind, key.provider, key.id), func() bool { return true }, func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, studiosLookupTime)
			defer cancel()
			return s.lookUpStudios(ctx, key)
		})
	}
}

// lookUpStudios asks TMDB for a title's studios and keeps its answer. A
// title TMDB does not know is kept as such; a lookup that fails keeps
// nothing, and the title is asked about again when next judged.
func (s *Service) lookUpStudios(ctx context.Context, key studiosKey) error {
	id := key.id
	var err error
	if key.provider == "imdb" {
		id, err = s.tmdb.FindByIMDb(ctx, key.kind, key.id)
	}
	var studios []string
	if err == nil {
		studios, err = s.tmdb.Studios(ctx, key.kind, id)
	}
	found := err == nil
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return err
	}
	if studios == nil {
		studios = []string{}
	}
	_, err = s.db.Exec(ctx, `INSERT INTO title_studios (kind, provider, id, found, studios, asked_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, provider, id) DO UPDATE SET found = excluded.found, studios = excluded.studios, asked_at = excluded.asked_at`,
		string(key.kind), key.provider, key.id, found, studios, s.now())
	return err
}
