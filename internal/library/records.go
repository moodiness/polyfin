package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// record is what Polyfin remembers about an identifier: enough to find the
// item again without the listing it came from.
type record struct {
	ID   accounts.ID `json:"-"`
	Key  string      `json:"-"`
	Kind Kind        `json:"kind"`
	// Addon provided the item: the library's or collection's addon, or the
	// catalog a title was listed in.
	Addon       *accounts.ID `json:"addon,omitempty"`
	CatalogType string       `json:"catalogType,omitempty"`
	CatalogID   string       `json:"catalogId,omitempty"`
	// Parent is the folder the item was last listed in.
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
	// Channel is the Stremio ID of a programme's channel; Number, a
	// channel's number when it was last listed, and GuideNumber a tuner's
	// number of it ("5.1").
	Channel     string `json:"channel,omitempty"`
	Number      int    `json:"number,omitempty"`
	GuideNumber string `json:"guideNumber,omitempty"`
	// Person is the name and photo of a person.
	Person *Person `json:"person,omitempty"`
	// Credits maps the titles a person is credited in, by item identifier,
	// to their credits there ("Actor", "Director", "Writer").
	Credits map[string][]string `json:"credits,omitempty"`
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
	// A search result has no folder: it keeps the one it was last listed in.
	// A listing carries no rating or genres: the title keeps those learned
	// before.
	return s.upsert(ctx, unique, keptData)
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
// them; then the others are merged, by a statement that sees them.
func (s *Service) upsert(ctx context.Context, records []*record, merged string) error {
	ids, keys, kinds, data, err := columns(records)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[]) ON CONFLICT DO NOTHING RETURNING id`, ids, keys, kinds, data)
	if err != nil {
		return err
	}
	inserted, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil || len(inserted) == len(ids) {
		return err
	}
	fresh := make(map[accounts.ID]bool, len(inserted))
	for _, id := range inserted {
		fresh[id] = true
	}
	stored := 0
	for i, id := range ids {
		if !fresh[id] {
			ids[stored], keys[stored], kinds[stored], data[stored] = id, keys[i], kinds[i], data[i]
			stored++
		}
	}
	ids, keys, kinds, data = ids[:stored], keys[:stored], kinds[:stored], data[:stored]
	// Only the items stored under their identifier are merged, in order. A
	// record whose key is another identifier's is left out, and told: its
	// identifier was not derived from its key.
	rows, err = s.db.Query(ctx, `WITH saved AS (INSERT INTO items (id, key, kind, data)
			SELECT id, key, kind, data FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[]) WITH ORDINALITY AS incoming(id, key, kind, data, n)
			WHERE EXISTS (SELECT 1 FROM items WHERE items.id = incoming.id) ORDER BY n
			ON CONFLICT (id) DO UPDATE SET data = `+merged+`, updated_at = now()
			WHERE items.data IS DISTINCT FROM `+merged+`)
		SELECT incoming.key FROM unnest($1::uuid[], $2::text[]) AS incoming(id, key)
		WHERE EXISTS (SELECT 1 FROM items WHERE items.key = incoming.key AND items.id <> incoming.id)`, ids, keys, kinds, data)
	if err != nil {
		return err
	}
	taken, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(taken) > 0 {
		s.logger.Warn("Items were not recorded: their keys are other items'", "keys", taken)
	}
	return nil
}

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
	rows, err := s.db.Query(ctx, "SELECT id, key, data FROM items WHERE id = ANY($1::uuid[])", ids)
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
