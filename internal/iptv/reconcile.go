package iptv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ChannelItemID is the Jellyfin item identifier of the channel with a
// Stremio identifier, as the library derives it.
func ChannelItemID(stremioID string) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:item:channel|" + stremioID))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

func randomKey() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// loadOptions reads a source's import options.
func loadOptions(ctx context.Context, db queryer, source accounts.ID) (Options, error) {
	var o Options
	err := db.QueryRow(ctx, "SELECT category_mode, channel_mode, excluded, new_channels, numbering FROM iptv_sources WHERE addon_id = $1", source).
		Scan(&o.Categories, &o.Channels, &o.Excluded, &o.NewChannels, &o.Numbering)
	return o, err
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// loadEntries reads a source's stored list in order.
func loadEntries(ctx context.Context, db queryer, source accounts.ID) ([]storedEntry, error) {
	rows, err := db.Query(ctx, `SELECT key, name, logo, group_title, guide_id, coalesce(number, 0) FROM iptv_entries
		WHERE addon_id = $1 ORDER BY position`, source)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedEntry, error) {
		var e storedEntry
		err := row.Scan(&e.Key, &e.Name, &e.Logo, &e.Group, &e.GuideID, &e.Number)
		return e, err
	})
}

type existingChannel struct {
	id, key string
	name    *string
}

// reconcile makes a source's line-up follow its stored list and options,
// in tx, keeping the administrator's edits (see the package's line-up
// rules): provider categories by key, channels by key or, failing that,
// by any of their former streams, streams by entry. Rows the list no
// longer derives are deleted, with the guide mappings of their channels.
func reconcile(ctx context.Context, tx pgx.Tx, source accounts.ID, at time.Time) error {
	var first bool
	var included []string
	if err := tx.QueryRow(ctx, "SELECT lineup_at IS NULL, included_groups FROM iptv_sources WHERE addon_id = $1 FOR UPDATE", source).
		Scan(&first, &included); err != nil {
		return err
	}
	options, err := loadOptions(ctx, tx, source)
	if err != nil {
		return err
	}
	entries, err := loadEntries(ctx, tx, source)
	if err != nil {
		return err
	}
	categories, channels := derive(entries, options)

	// Categories: new ones come last, enabled, but for the groups the
	// administrator did not show before line-ups existed.
	existing := map[string]bool{}
	rows, err := tx.Query(ctx, "SELECT key FROM iptv_categories WHERE addon_id = $1", source)
	if err != nil {
		return err
	}
	var key string
	if _, err := pgx.ForEachRow(rows, []any{&key}, func() error { existing[key] = true; return nil }); err != nil {
		return err
	}
	var last int
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(position), 0) FROM iptv_categories WHERE addon_id = $1", source).Scan(&last); err != nil {
		return err
	}
	shown := map[string]bool{}
	for _, group := range included {
		shown[groupKey(group)] = true
	}
	keys, names, positions, enabled := []string{}, []string{}, []int{}, []bool{}
	for _, c := range categories {
		keys, names = append(keys, c.key), append(names, c.name)
		// An existing category keeps its place: the one given is unused.
		position := 1
		if !existing[c.key] {
			last++
			position = last
		}
		positions = append(positions, position)
		enabled = append(enabled, !first || included == nil || shown[c.key])
	}
	if _, err := tx.Exec(ctx, `INSERT INTO iptv_categories (addon_id, key, provider_name, position, enabled)
		SELECT $1, k, n, p, e FROM unnest($2::text[], $3::text[], $4::int[], $5::bool[]) AS t(k, n, p, e)
		ON CONFLICT (addon_id, key) DO UPDATE SET provider_name = excluded.provider_name`, source, keys, names, positions, enabled); err != nil {
		return err
	}
	categoryIDs := map[string]accounts.ID{}
	rows, err = tx.Query(ctx, "SELECT key, id FROM iptv_categories WHERE addon_id = $1", source)
	if err != nil {
		return err
	}
	var id accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&key, &id}, func() error { categoryIDs[key] = id; return nil }); err != nil {
		return err
	}

	// Channels: by key, else adopted through a former stream, else new.
	byKey, byID := map[string]*existingChannel{}, map[string]*existingChannel{}
	rows, err = tx.Query(ctx, "SELECT id, key, name FROM iptv_lineup WHERE addon_id = $1", source)
	if err != nil {
		return err
	}
	var row existingChannel
	if _, err := pgx.ForEachRow(rows, []any{&row.id, &row.key, &row.name}, func() error {
		c := row
		byKey[c.key], byID[c.id] = &c, &c
		return nil
	}); err != nil {
		return err
	}
	streamChannel := map[string]string{}
	rows, err = tx.Query(ctx, "SELECT key, channel_id FROM iptv_streams WHERE addon_id = $1 AND custom_url IS NULL", source)
	if err != nil {
		return err
	}
	var channelID string
	if _, err := pgx.ForEachRow(rows, []any{&key, &channelID}, func() error { streamChannel[key] = channelID; return nil }); err != nil {
		return err
	}
	assigned := make([]*existingChannel, len(channels))
	used := map[string]bool{}
	for i, c := range channels {
		if e := byKey[c.key]; e != nil {
			assigned[i], used[e.id] = e, true
		}
	}
	for i, c := range channels {
		if assigned[i] != nil {
			continue
		}
		for _, stream := range c.streams {
			if e := byID[streamChannel[stream.entry]]; e != nil && !used[e.id] {
				assigned[i], used[e.id] = e, true
				break
			}
		}
	}
	taken := map[string]bool{}
	for id := range byID {
		taken[id] = true
	}
	lineup := make([][]any, 0, len(channels))
	streams := make([][]any, 0, len(entries))
	prefix := prefix(source)
	for i, c := range channels {
		var override *string
		isNew := assigned[i] == nil
		id := ""
		if isNew {
			id = c.id
			if taken[id] {
				id = randomKey()
			}
			taken[id] = true
		} else {
			id, override = assigned[i].id, assigned[i].name
		}
		shownName := c.name
		if override != nil {
			shownName = *override
		}
		var number *int
		if c.number > 0 {
			number = &c.number
		}
		lineup = append(lineup, []any{id, ChannelItemID(prefix + id), c.key, categoryIDs[c.category], c.name, c.logo, c.guideID, number,
			float64(c.position), isNew && !options.NewChannels, Fold(shownName + "\n" + c.name + "\n" + c.guideID)})
		for rank, stream := range c.streams {
			label := stream.label
			if len(label) > 32 {
				label = label[:32]
			}
			streams = append(streams, []any{stream.entry, id, label, rank})
		}
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE reconciled_lineup (id text, item_id uuid, key text, category_id uuid, provider_name text,
		provider_logo text, guide_id text, provider_number int, sort double precision, disabled bool, search text) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"reconciled_lineup"}, []string{"id", "item_id", "key", "category_id", "provider_name",
		"provider_logo", "guide_id", "provider_number", "sort", "disabled", "search"}, pgx.CopyFromRows(lineup)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO iptv_lineup (addon_id, id, item_id, key, category_id, provider_name, provider_logo, guide_id,
			provider_number, sort, enabled, search)
		SELECT $1, id, item_id, key, category_id, provider_name, provider_logo, guide_id, provider_number, sort, NOT disabled, search
		FROM reconciled_lineup
		ON CONFLICT (addon_id, id) DO UPDATE SET key = excluded.key, category_id = excluded.category_id,
			provider_name = excluded.provider_name, provider_logo = excluded.provider_logo, guide_id = excluded.guide_id,
			provider_number = excluded.provider_number, search = excluded.search,
			sort = CASE WHEN iptv_lineup.sort_set THEN iptv_lineup.sort ELSE excluded.sort END`, source); err != nil {
		return fmt.Errorf("line-up channels: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE reconciled_streams (key text, channel_id text, label text, rank int) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"reconciled_streams"}, []string{"key", "channel_id", "label", "rank"}, pgx.CopyFromRows(streams)); err != nil {
		return err
	}
	// A stream moved to another channel loses the order it had there.
	if _, err := tx.Exec(ctx, `INSERT INTO iptv_streams (addon_id, key, channel_id, label, rank)
		SELECT $1, key, channel_id, label, rank FROM reconciled_streams
		ON CONFLICT (addon_id, key) DO UPDATE SET label = excluded.label, rank = excluded.rank, channel_id = excluded.channel_id,
			sort = CASE WHEN iptv_streams.channel_id = excluded.channel_id THEN iptv_streams.sort END`, source); err != nil {
		return fmt.Errorf("line-up streams: %w", err)
	}
	for _, statement := range []string{
		`DELETE FROM iptv_lineup l WHERE addon_id = $1 AND NOT EXISTS (SELECT 1 FROM reconciled_lineup r WHERE r.id = l.id)`,
		`DELETE FROM iptv_streams s WHERE addon_id = $1 AND custom_url IS NULL AND NOT EXISTS (SELECT 1 FROM reconciled_streams r WHERE r.key = s.key)`,
		`DELETE FROM iptv_categories c WHERE addon_id = $1 AND NOT custom AND NOT (key = ANY($2))`,
		`UPDATE iptv_categories c SET position = o.n FROM (SELECT id, row_number() OVER (ORDER BY position, id) AS n FROM iptv_categories
			WHERE addon_id = $1) o WHERE c.id = o.id AND c.position <> o.n`,
		`DELETE FROM live_guide_maps m WHERE addon_id = $1 AND NOT EXISTS (SELECT 1 FROM iptv_lineup l WHERE l.addon_id = $1 AND l.item_id = m.channel_id)`,
		`DROP TABLE reconciled_lineup`,
		`DROP TABLE reconciled_streams`,
	} {
		args := []any{source}
		if strings.Contains(statement, "$2") {
			args = append(args, keys)
		}
		if strings.HasPrefix(statement, "DROP") {
			args = nil
		}
		if _, err := tx.Exec(ctx, statement, args...); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "UPDATE iptv_sources SET lineup_at = $2, included_groups = NULL WHERE addon_id = $1", source, at)
	return err
}
