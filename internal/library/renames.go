package library

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/addons"
)

// CarryRenamed rewrites, in tx, the transaction storing addon's manifest,
// what names the catalogs the manifest renamed, for it to name their new
// identifiers: catalog home rows, smart lists, the Live TV channel limits
// of their channel groups, the titles they listed first, their library's
// item and their channels' groups, and the pages, reach and page sizes
// kept of them, arrivals following reach. Their guide health records go:
// the next check tells them again. Their libraries keep their items'
// identifiers (see addons.Addon.LibraryKeys), and with them what names
// those: hidden libraries, apps' views, images and edits. It is the addons
// store's CarryRenames (see addons.SetCarry).
func (s *Service) CarryRenamed(ctx context.Context, tx pgx.Tx, addon addons.Addon, renames []addons.Rename) (func(), error) {
	addonJSON, err := json.Marshal(addon.ID)
	if err != nil {
		return nil, err
	}
	hexID := addon.ID.String()
	types, froms, tos := make([]string, len(renames)), make([]string, len(renames)), make([]string, len(renames))
	for i, r := range renames {
		types[i], froms[i], tos[i] = r.CatalogType, r.From, r.To
	}
	for _, r := range renames {
		statements := []struct {
			query string
			args  []any
		}{
			{`UPDATE home_rows SET catalog_id = $4 WHERE id IN (SELECT id FROM home_rows
				WHERE kind = 'catalog' AND addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 ORDER BY id FOR UPDATE)`,
				[]any{addon.ID, r.CatalogType, r.From, r.To}},
			{`UPDATE smart_lists SET catalogs = (SELECT jsonb_agg(CASE WHEN c->>'addonId' = $1 AND c->>'type' = $2 AND c->>'id' = $3
					THEN jsonb_set(c, '{id}', to_jsonb($4::text)) ELSE c END ORDER BY n) FROM jsonb_array_elements(catalogs) WITH ORDINALITY e(c, n))
				WHERE id IN (SELECT s.id FROM smart_lists s WHERE EXISTS (SELECT FROM jsonb_array_elements(s.catalogs) c
					WHERE c->>'addonId' = $1 AND c->>'type' = $2 AND c->>'id' = $3) ORDER BY s.id FOR UPDATE)`,
				[]any{hexID, r.CatalogType, r.From, r.To}},
			// What is kept under the new identifier, from a catalog of
			// that identifier the addon had before, gives way.
			{`DELETE FROM catalog_pages WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`, []any{addon.ID, r.CatalogType, r.To}},
			{`UPDATE catalog_pages SET catalog_id = $4 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
				[]any{addon.ID, r.CatalogType, r.From, r.To}},
			{`DELETE FROM catalog_reach WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`, []any{addon.ID, r.CatalogType, r.To}},
			{`UPDATE catalog_reach SET catalog_id = $4 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
				[]any{addon.ID, r.CatalogType, r.From, r.To}},
			{`DELETE FROM catalog_sizes WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`, []any{addon.ID, r.CatalogType, r.To}},
			{`UPDATE catalog_sizes SET catalog_id = $4 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
				[]any{addon.ID, r.CatalogType, r.From, r.To}},
			{`UPDATE items SET data = jsonb_set(data, '{channelGroup}', to_jsonb($3 || substr(data->>'channelGroup', length($2) + 1)))
				WHERE id = ANY(ARRAY(SELECT id FROM items WHERE kind = $1 AND starts_with(data->>'channelGroup', $2) ORDER BY id FOR UPDATE))`,
				[]any{string(KindChannel), channelGroupPrefix(hexID, r.From), channelGroupPrefix(hexID, r.To)}},
		}
		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
				return nil, err
			}
		}
		for _, table := range []string{"notification_health", "notification_health_found"} {
			if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE key = $1", "guide:"+hexID+":"+r.CatalogType+":"+r.From); err != nil {
				return nil, err
			}
		}
	}
	// The titles they listed first, and their library's item, all items
	// scanned once.
	if _, err := tx.Exec(ctx, `UPDATE items SET data = jsonb_set(data, '{catalogId}', to_jsonb(r.t)), updated_at = now()
		FROM unnest($2::text[], $3::text[], $4::text[]) r(ty, f, t)
		WHERE items.id = ANY(ARRAY(SELECT id FROM items WHERE data->'addon' = $1::jsonb
			AND (data->>'catalogType', data->>'catalogId') IN (SELECT * FROM unnest($2::text[], $3::text[])) ORDER BY id FOR UPDATE))
			AND items.data->>'catalogType' = r.ty AND items.data->>'catalogId' = r.f`,
		string(addonJSON), types, froms, tos); err != nil {
		return nil, err
	}
	if _, err := editChannelRules(ctx, tx, func(entries []string) []string {
		var edited []string
		for i, entry := range entries {
			for _, r := range renames {
				if rest, ok := strings.CutPrefix(entry, channelGroupPrefix(hexID, r.From)); ok {
					if edited == nil {
						edited = append([]string{}, entries...)
					}
					edited[i] = channelGroupPrefix(hexID, r.To) + rest
					break
				}
			}
		}
		if edited == nil {
			return entries
		}
		return edited
	}); err != nil {
		return nil, err
	}
	return func() {
		// Saves read the titles' catalogs and arrivals (see savedData).
		s.forgetAllSaved()
		s.sizes.mu.Lock()
		for _, r := range renames {
			from := sizeKey{addon: addon.ID, catalogType: r.CatalogType, catalogID: r.From}
			to := sizeKey{addon: addon.ID, catalogType: r.CatalogType, catalogID: r.To}
			delete(s.sizes.byCatalog, to)
			if size, ok := s.sizes.byCatalog[from]; ok {
				s.sizes.byCatalog[to] = size
				delete(s.sizes.byCatalog, from)
			}
		}
		s.sizes.mu.Unlock()
	}, nil
}

// channelGroupPrefix is what the keys of the channel groups of an addon's
// catalog start with (see channelGroup).
func channelGroupPrefix(addonHex, catalogID string) string {
	return addonHex + "/" + catalogID + "/"
}
