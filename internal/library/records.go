package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/maphash"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// record is what Polyfin remembers about an identifier: enough to find the
// item again without the listing it came from.
type record struct {
	ID   accounts.ID `json:"-"`
	Key  string      `json:"-"`
	Kind Kind        `json:"kind"`
	// Addon provided the item: the library's or collection's addon, or the
	// catalog a title was listed in first (see savedData).
	Addon       *accounts.ID `json:"addon,omitempty"`
	CatalogType string       `json:"catalogType,omitempty"`
	CatalogID   string       `json:"catalogId,omitempty"`
	// Parent is the folder the item was listed in: a movie's or series'
	// is that of its first catalog (see savedData).
	Parent *accounts.ID `json:"parent,omitempty"`
	// Meta is the preview of a title or collection.
	Meta *stremio.Meta `json:"meta,omitempty"`
	// SeriesID is the Stremio ID of the series of a season or episode.
	// Poster is a season's own artwork when it was last listed (see
	// seasonPosters).
	SeriesID string         `json:"seriesId,omitempty"`
	Season   int            `json:"season,omitempty"`
	Poster   string         `json:"poster,omitempty"`
	Video    *stremio.Video `json:"video,omitempty"`
	// Group is the TMDB episode group a season of an episode order is a
	// season of, whose number is Season (see episodeorder.go).
	Group string `json:"group,omitempty"`
	// Channel is the Stremio ID of a programme's channel; Number, a
	// channel's number when it was last listed, and GuideNumber a tuner's
	// number of it ("5.1"); ChannelGroup, the key of its channel group
	// then (see channelGroup).
	Channel      string `json:"channel,omitempty"`
	Number       int    `json:"number,omitempty"`
	GuideNumber  string `json:"guideNumber,omitempty"`
	ChannelGroup string `json:"channelGroup,omitempty"`
	// Person is the name and photo of a person.
	Person *Person `json:"person,omitempty"`
	// TMDBID is a person's TMDB identifier as an addon gave it, and TMDB
	// what TMDB told of them (see persondetails.go). DeadPhotos are the
	// photos of theirs found dead, by address, each with when (see
	// PhotoFailed).
	TMDBID     string               `json:"tmdbId,omitempty"`
	TMDB       *personDetails       `json:"tmdb,omitempty"`
	DeadPhotos map[string]time.Time `json:"deadPhotos,omitempty"`
	// Credits maps the titles a person is credited in, by item identifier,
	// to their credits there ("Actor", "Director", "Writer", "Producer").
	Credits map[string][]string `json:"credits,omitempty"`
	// SameAs is the identifier of the record a person kept by name
	// became, once they were known by their TMDB identifier (see
	// people.go and sameAs): the name's record then only points to it.
	SameAs string `json:"sameAs,omitempty"`
	// Confined is true when the addon may only reach public addresses, which
	// also applies to its artwork.
	Confined bool `json:"confined,omitempty"`
	// Rating is the rating a description by one of the server's addons
	// gave, empty when it gave none, at RatedAt; nil until one was read
	// (see learnTraits). Genres are the genres the same description gave;
	// nil until one was read since Polyfin keeps them.
	Rating  *string    `json:"rating,omitempty"`
	RatedAt *time.Time `json:"ratedAt,omitempty"`
	Genres  *[]string  `json:"genres,omitempty"`
	// EpisodeTitle is the episode title of a programme from an XMLTV guide.
	EpisodeTitle string `json:"episodeTitle,omitempty"`
	// Music describes an item of an Eclipse addon (see music.go).
	Music *musicEntry `json:"music,omitempty"`
}

// save stores records, merging each into the stored record of its item
// with savedData, and leaves out those it saved before that left their
// item as it is stored (see savedRecords).
func (s *Service) save(ctx context.Context, records []record) error {
	if len(records) == 0 {
		return nil
	}
	unique := inLockOrder(records)
	for i, r := range unique {
		if r.Meta != nil || r.Video != nil && r.Video.Streams != nil {
			trimmed := *r
			if r.Meta != nil {
				preview := *r.Meta
				preview.Videos = nil
				trimmed.Meta = &preview
			}
			// A video's own streams are read from its title's description
			// (see embed), whose links change.
			if r.Video != nil {
				video := *r.Video
				video.Streams = nil
				trimmed.Video = &video
			}
			unique[i] = &trimmed
		}
	}
	ids, keys, kinds, data, err := columns(unique)
	if err != nil {
		return err
	}
	hashes := make([]uint64, len(data))
	for i := range data {
		hashes[i] = savedHash(data[i])
	}
	generation, versions := s.saved.lookUp(s.addons.Changes(), ids, hashes)
	kept, err := s.stillStored(ctx, ids, versions)
	if err != nil {
		return err
	}
	n := 0
	for i := range ids {
		if !kept[i] {
			ids[n], keys[n], kinds[n], data[n], hashes[n] = ids[i], keys[i], kinds[i], data[i], hashes[i]
			n++
		}
	}
	if n == 0 {
		return nil
	}
	ids, keys, kinds, data, hashes = ids[:n], keys[:n], kinds[:n], data[:n], hashes[:n]
	stored, taken, err := s.upsertColumns(ctx, ids, keys, kinds, data, savedData)
	if err != nil {
		return err
	}
	// A record whose key is another item's was not stored as it is.
	s.saved.learn(generation, s.addons.Changes(), ids, hashes, stored, func(i int) bool { return slices.Contains(taken, keys[i]) })
	return nil
}

// stillStored tells which of the items ids are stored at the version
// versions gives each, -1 for none: those left out of a save, as what it
// saves left them as they are (see savedRecords). One statement reads
// them all; none is read when no version is given.
func (s *Service) stillStored(ctx context.Context, ids []accounts.ID, versions []int64) ([]bool, error) {
	known := make([]accounts.ID, 0, len(ids))
	for i, id := range ids {
		if versions[i] >= 0 {
			known = append(known, id)
		}
	}
	kept := make([]bool, len(ids))
	if len(known) == 0 {
		return kept, nil
	}
	rows, err := s.db.Query(ctx, "SELECT id, xmin FROM items WHERE id = ANY($1::uuid[]) ORDER BY id", known)
	if err != nil {
		return nil, err
	}
	// Both are in identifier order (see inLockOrder), the database's.
	i := 0
	var id accounts.ID
	var version uint32
	_, err = pgx.ForEachRow(rows, []any{&id, &version}, func() error {
		for i < len(ids) && bytes.Compare(ids[i][:], id[:]) < 0 {
			i++
		}
		if i < len(ids) && ids[i] == id && versions[i] == int64(version) {
			kept[i] = true
		}
		return nil
	})
	return kept, err
}

// savedItems bounds the items savedRecords remembers, and savedPerItem
// the records it remembers of each.
const (
	savedItems   = 1 << 16
	savedPerItem = 4
)

// savedRecords remembers, of the items saves stored lately, the records
// that leave them as they are stored. Listings save every title, season
// and episode they list, and Next Up and the notifications every episode
// of whole series, while what they save seldom changes: a save leaves
// those records out, sparing the database an upsert's look-ups and row
// locks. A title several catalogs list is known by the record of each,
// as it keeps the first one's (see savedData).
//
// What it remembers holds for one version of an item, its xmin, which
// every write of the item changes, and which its deletion takes away: a
// save reads the versions of the items it would leave out, in one
// statement, and saves again those written or deleted since, whoever did.
// A save's merge also reads the catalog arrivals (see savedData): their
// deletions forget everything (see forgetAllSaved), and so does a change
// of the addons or libraries, as removing an addon deletes its arrivals.
type savedRecords struct {
	mu sync.Mutex
	// known holds the items' versions and records, made on first use.
	known *cache.Cache[accounts.ID, savedItem]
	// generation counts the times everything was forgotten, and addons
	// is the count of changes of the addons known holds for (see
	// addons.Store.Changes): a save that began before either moved
	// learns nothing.
	generation uint64
	addons     uint64
}

// savedItem is what savedRecords knows of an item: its version, and the
// hashes of the records that leave it as it is at that version, the
// newest first, 0 for none.
type savedItem struct {
	version uint32
	records [savedPerItem]uint64
}

// savedSeed seeds the hashes of the records savedRecords knows.
var savedSeed = maphash.MakeSeed()

// savedHash is the hash of the encoded record data, never 0.
func savedHash(data string) uint64 { return max(maphash.String(savedSeed, data), 1) }

// lookUp returns, for the records of ids of the given hashes, the
// version of each item that the record leaves as it is, -1 when none is
// known, with the generation of what it knows. changes is the addons'
// count of changes now.
func (m *savedRecords) lookUp(changes uint64, ids []accounts.ID, hashes []uint64) (uint64, []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.follow(changes)
	versions := make([]int64, len(ids))
	for i, id := range ids {
		versions[i] = -1
		if item, ok := m.known.Get(id); ok && slices.Contains(item.records[:], hashes[i]) {
			versions[i] = int64(item.version)
		}
	}
	return m.generation, versions
}

// learn remembers that the records of ids, of the given hashes, leave
// their items as they are at the versions stored gives, unless
// everything was forgotten since generation, or the addons changed since
// (changes being their count of changes now). It leaves out the items
// stored does not give, and the records skip reports.
func (m *savedRecords) learn(generation, changes uint64, ids []accounts.ID, hashes []uint64, stored map[accounts.ID]uint32, skip func(i int) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.follow(changes)
	if generation != m.generation {
		return
	}
	for i, id := range ids {
		version, ok := stored[id]
		if !ok || skip(i) {
			continue
		}
		item, kept := m.known.Get(id)
		if !kept || item.version != version {
			item = savedItem{version: version}
		}
		if !slices.Contains(item.records[:], hashes[i]) {
			copy(item.records[1:], item.records[:])
			item.records[0] = hashes[i]
		}
		m.known.Put(id, item)
	}
}

// follow forgets everything when the addons changed, changes being
// their count of changes now.
func (m *savedRecords) follow(changes uint64) {
	if m.known == nil || changes != m.addons {
		m.forgetAll()
		m.addons = changes
	}
}

// forgetAll forgets everything, from now on.
func (m *savedRecords) forgetAll() {
	m.known = cache.New[accounts.ID, savedItem](savedItems, math.MaxInt64)
	m.generation++
}

// forgetAllSaved forgets what saves learned of every item: catalog
// arrivals were deleted, which a save's merge reads (see savedData).
func (s *Service) forgetAllSaved() {
	s.saved.mu.Lock()
	defer s.saved.mu.Unlock()
	s.saved.forgetAll()
}

// saveNew stores the records of identifiers Polyfin does not know yet,
// leaving those it knows as they are: the albums and artists a track names
// stand in for them only until Polyfin learns more of them. A record is
// known by its identifier or by its key (see upsert).
func (s *Service) saveNew(ctx context.Context, records []record) error {
	if len(records) == 0 {
		return nil
	}
	ids, keys, kinds, data, err := columns(inLockOrder(records))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[]) ON CONFLICT DO NOTHING`, ids, keys, kinds, data)
	return err
}

// upsert stores records, in lock order (see inLockOrder), merging each
// into the stored record of its identifier with merged: an expression of
// the stored data (items.data) and the new one (excluded.data).
//
// An item has two unique keys, its identifier and the key the identifier
// derives from, and an INSERT ... ON CONFLICT (id) only looks the first up
// before inserting: requests recording the same new items at once, as
// apps opening a person's page do, failed on the second (a duplicate
// items_key_key) or deadlocked. So the new items are inserted first,
// skipping those either key finds, which waits for the requests inserting
// them; then the others are merged by an UPDATE, which never inserts. An
// INSERT ... ON CONFLICT (id) DO UPDATE inserted an item its look-up of
// the identifier missed, and failed on the key: the look-up misses an item
// another request updates as it looks (PostgreSQL reads the entries of an
// index page before their rows), or one deleted since the statement began.
func (s *Service) upsert(ctx context.Context, records []*record, merged string) error {
	ids, keys, kinds, data, err := columns(records)
	if err != nil {
		return err
	}
	_, _, err = s.upsertColumns(ctx, ids, keys, kinds, data, merged)
	return err
}

// upsertColumns is upsert of records encoded as the columns of the items
// table (see columns), which it leaves as they are. It returns the
// version (xmin) of each item it inserted or merged, as it left it, and
// the keys of the records it left out.
func (s *Service) upsertColumns(ctx context.Context, ids []accounts.ID, keys, kinds, data []string, merged string) (map[accounts.ID]uint32, []string, error) {
	rows, err := s.db.Query(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[]) ON CONFLICT DO NOTHING RETURNING id, xmin`, ids, keys, kinds, data)
	if err != nil {
		return nil, nil, err
	}
	stored := make(map[accounts.ID]uint32, len(ids))
	var id accounts.ID
	var version uint32
	if _, err := pgx.ForEachRow(rows, []any{&id, &version}, func() error {
		stored[id] = version
		return nil
	}); err != nil || len(stored) == len(ids) {
		return stored, nil, err
	}
	if len(stored) > 0 {
		merging := len(ids) - len(stored)
		storedIDs, storedKeys, storedKinds, storedData := make([]accounts.ID, 0, merging), make([]string, 0, merging), make([]string, 0, merging),
			make([]string, 0, merging)
		for i, id := range ids {
			if _, inserted := stored[id]; !inserted {
				storedIDs, storedKeys, storedKinds, storedData = append(storedIDs, id), append(storedKeys, keys[i]), append(storedKinds, kinds[i]),
					append(storedData, data[i])
			}
		}
		ids, keys, kinds, data = storedIDs, storedKeys, storedKinds, storedData
	}
	// Only the items stored under their identifier are merged, locked first
	// in order so that requests merging the same items never deadlock; a
	// batch is one transaction, which holds the locks until the merge, and
	// gives its xmin to the items it changes. A record whose key is another
	// identifier's is left out, and told: its identifier was not derived
	// from its key.
	var changed []accounts.ID
	var changedVersion *uint32
	var taken []string
	batch := &pgx.Batch{}
	batch.Queue(`SELECT id, xmin FROM items WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids).Query(func(rows pgx.Rows) error {
		_, err := pgx.ForEachRow(rows, []any{&id, &version}, func() error {
			stored[id] = version
			return nil
		})
		return err
	})
	batch.Queue(`WITH saved AS (UPDATE items SET data = `+merged+`, updated_at = now()
			FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[]) AS excluded(id, key, kind, data)
			WHERE items.id = excluded.id AND items.data IS DISTINCT FROM `+merged+` RETURNING items.id, items.xmin)
		SELECT ARRAY(SELECT id FROM saved), (SELECT xmin FROM saved LIMIT 1), ARRAY(SELECT incoming.key FROM unnest($1::uuid[], $2::text[]) AS incoming(id, key)
			WHERE EXISTS (SELECT 1 FROM items WHERE items.key = incoming.key AND items.id <> incoming.id))`, ids, keys, kinds, data).
		QueryRow(func(row pgx.Row) error { return row.Scan(&changed, &changedVersion, &taken) })
	if err := s.db.SendBatch(ctx, batch).Close(); err != nil {
		return nil, nil, err
	}
	if changedVersion != nil {
		for _, id := range changed {
			stored[id] = *changedVersion
		}
	}
	if len(taken) > 0 {
		s.logger.Warn("Items were not recorded: their keys are other items'", "keys", taken)
	}
	return stored, taken, nil
}

// savedData is the record a save stores: keptData, except for a movie or a
// series that another catalog lists than the one that listed it first,
// while both list it (see listedElsewhere). The title then keeps the
// first catalog, its kind, its folder and the preview that catalog gave,
// which go together: the catalog's addon describes it first (see
// describers), and its artwork is reached as that addon may (Confined).
// The new record only brings what describing the title learned (see
// learnTraits), and its folder when the title had none. A catalog that
// stops listing a title, Polyfin no longer seeing it there (see
// forgetArrivals), lets the next one take it. Titles of other videos
// follow the catalog that listed them last, which shapes them (see
// misplaced).
const savedData = `CASE WHEN ` + listedElsewhere + `
	THEN items.data || (excluded.data - '{kind,addon,catalogType,catalogId,meta,confined}'::text[]
		- CASE WHEN items.data ? 'parent' THEN 'parent' ELSE '' END)
	ELSE ` + keptData + ` END`

// listedElsewhere tells whether the stored record (items.data) of a movie
// or a series and the new one (excluded.data) name different catalogs
// that both list the item, as their arrivals tell (see recordArrivals).
// Only the catalogs Polyfin follows have arrivals: a local folder, an IPTV
// source, a search, a live or a collection catalog takes the title as
// before. The types are those titleKind makes movies and series of; an
// addon's identifier is stored as the array of its bytes.
const listedElsewhere = `items.data->'meta'->>'type' IN ('movie', 'anime.movie', 'series', 'anime.series', 'anime')
	AND excluded.data->'meta'->>'type' IN ('movie', 'anime.movie', 'series', 'anime.series', 'anime')
	AND items.data ? 'catalogId' AND excluded.data ? 'catalogId'
	AND (items.data->'addon', items.data->'catalogType', items.data->'catalogId')
		IS DISTINCT FROM (excluded.data->'addon', excluded.data->'catalogType', excluded.data->'catalogId')
	AND EXISTS (SELECT FROM catalog_arrivals WHERE addon_id = (SELECT string_agg(lpad(to_hex(b::int), 2, '0'), '' ORDER BY n)
			FROM jsonb_array_elements_text(items.data->'addon') WITH ORDINALITY AS addon(b, n))::uuid
		AND catalog_type = items.data->>'catalogType' AND catalog_id = items.data->>'catalogId' AND item_id = items.id)
	AND EXISTS (SELECT FROM catalog_arrivals WHERE addon_id = (SELECT string_agg(lpad(to_hex(b::int), 2, '0'), '' ORDER BY n)
			FROM jsonb_array_elements_text(excluded.data->'addon') WITH ORDINALITY AS addon(b, n))::uuid
		AND catalog_type = excluded.data->>'catalogType' AND catalog_id = excluded.data->>'catalogId' AND item_id = items.id)`

// columns encodes records as the columns of the items table.
func columns(records []*record) (ids []accounts.ID, keys, kinds, data []string, err error) {
	ids = make([]accounts.ID, 0, len(records))
	keys = make([]string, 0, len(records))
	kinds = make([]string, 0, len(records))
	data = make([]string, 0, len(records))
	for _, r := range records {
		encoded, err := json.Marshal(r)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		ids, keys, kinds, data = append(ids, r.ID), append(keys, r.Key), append(kinds, string(r.Kind)), append(data, string(encoded))
	}
	return ids, keys, kinds, data, nil
}

// inLockOrder returns the first record of each identifier, ordered by
// identifier. Upserts lock the rows they reach in that order, so listings
// saving the same items at once in other orders, as smart lists sorted
// differently do when an app opens its home, take them in the same order
// and never deadlock (see saveCredits).
func inLockOrder(records []record) []*record {
	unique := make([]*record, 0, len(records))
	seen := make(map[accounts.ID]bool, len(records))
	for i := range records {
		if !seen[records[i].ID] {
			seen[records[i].ID] = true
			unique = append(unique, &records[i])
		}
	}
	slices.SortFunc(unique, func(a, b *record) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	return unique
}

// keptData is the record an upsert stores: the new one, with the stored
// folder, rating and genres when the new one has none.
const keptData = `coalesce((SELECT jsonb_object_agg(key, value) FROM jsonb_each(items.data)
	WHERE key IN ('parent', 'rating', 'ratedAt', 'genres')), '{}'::jsonb) || excluded.data`

func (s *Service) load(ctx context.Context, id accounts.ID) (record, error) {
	if r, ok, err := preloaded(ctx, id); ok {
		return r, err
	}
	var r record
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT key, data FROM items WHERE id = $1", id).Scan(&r.Key, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return record{}, ErrNotFound
	}
	if err != nil {
		return record{}, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return record{}, err
	}
	r.ID = id
	return r, nil
}

// loadAll returns the records of the identifiers Polyfin knows, in no
// particular order.
func (s *Service) loadAll(ctx context.Context, ids []accounts.ID) ([]record, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.loadWhere(ctx, "id = ANY($1::uuid[])", ids)
}

// loadWhere returns the records of the items condition, on the items
// table, selects with args, in no particular order.
func (s *Service) loadWhere(ctx context.Context, condition string, args ...any) ([]record, error) {
	rows, err := s.db.Query(ctx, "SELECT id, key, data FROM items WHERE "+condition, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []record
	for rows.Next() {
		var r record
		var id accounts.ID
		var data []byte
		if err := rows.Scan(&id, &r.Key, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, err
		}
		r.ID = id
		records = append(records, r)
	}
	return records, rows.Err()
}
