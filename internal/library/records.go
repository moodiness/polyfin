package library

import (
	"context"
	"encoding/json"
	"errors"

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
	// Person is the name and photo of a person.
	Person *Person `json:"person,omitempty"`
	// Confined is true when the addon may only reach public addresses, which
	// also applies to its artwork.
	Confined bool `json:"confined,omitempty"`
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
	_, err := s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[])
		ON CONFLICT (id) DO UPDATE SET data = `+keptParent+`, updated_at = now()
		WHERE items.data IS DISTINCT FROM `+keptParent, ids, keys, kinds, data)
	return err
}

// keptParent is the record an upsert stores: the new one, with the stored
// folder when the new one has none.
const keptParent = `CASE WHEN excluded.data ? 'parent' OR NOT items.data ? 'parent' THEN excluded.data
	ELSE excluded.data || jsonb_build_object('parent', items.data->'parent') END`

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
