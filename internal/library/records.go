package library

import (
	"context"
	"encoding/json"
	"errors"
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
	SeriesID string         `json:"seriesId,omitempty"`
	Season   int            `json:"season,omitempty"`
	Video    *stremio.Video `json:"video,omitempty"`
	// Channel is the Stremio ID of a programme's channel; Number, a
	// channel's number when it was last listed.
	Channel string `json:"channel,omitempty"`
	Number  int    `json:"number,omitempty"`
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
}

func (s *Service) save(ctx context.Context, records []record) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]accounts.ID, 0, len(records))
	keys := make([]string, 0, len(records))
	kinds := make([]string, 0, len(records))
	data := make([]string, 0, len(records))
	seen := map[accounts.ID]bool{}
	for _, r := range records {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.Meta != nil {
			preview := *r.Meta
			preview.Videos = nil
			r.Meta = &preview
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			return err
		}
		ids, keys, kinds, data = append(ids, r.ID), append(keys, r.Key), append(kinds, string(r.Kind)), append(data, string(encoded))
	}
	// A search result has no folder: it keeps the one it was last listed in.
	// A listing carries no rating or genres: the title keeps those learned
	// before.
	_, err := s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[])
		ON CONFLICT (id) DO UPDATE SET data = `+keptData+`, updated_at = now()
		WHERE items.data IS DISTINCT FROM `+keptData, ids, keys, kinds, data)
	return err
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
