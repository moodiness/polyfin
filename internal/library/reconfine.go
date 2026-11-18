package library

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// Reconfine rewrites, in tx, the confinement kept with the items a source
// listed (see record.Confined) as the rule tells it now (see
// addons.Addon.Confined): that of source, or of every source when it is
// nil. A title, a channel or a library kept while its source could reach
// the local network no longer leads there once the source is told to be
// on the internet, without waiting to be listed again; one kept confined
// follows its source to the local network the same way. It is the addons
// store's Reconfine (see addons.Store.SetReconfine).
//
// Only the items that name an installed source are read, by that source's
// rule; an item that names none, as a title whose addon was removed or a
// person, is left as it is. A season's and an episode's own confinement
// may come from another addon than the one that listed their series: it
// is set when their source is confined, never cleared. Items are locked
// in order, as saves lock them.
//
// What is not kept with an item needs no rewrite: versions, subtitles,
// catalog pages and descriptions are read again by the view's rule.
func (s *Service) Reconfine(ctx context.Context, tx pgx.Tx, source *accounts.ID) error {
	// An addon's identifier is stored in an item as the array of its bytes.
	_, err := tx.Exec(ctx, `WITH rule AS (SELECT (SELECT jsonb_agg(get_byte(uuid_send(a.id), n) ORDER BY n) FROM generate_series(0, 15) AS n) AS addon,
				`+addons.ConfinedSQL("a")+` AS confined
			FROM addons a WHERE $1::uuid IS NULL OR a.id = $1),
		stale AS (SELECT i.id, rule.confined FROM items i JOIN rule ON i.data->'addon' = rule.addon
			WHERE coalesce((i.data->>'confined')::boolean, false) <> rule.confined
				AND (rule.confined OR i.kind NOT IN ($2, $3)) ORDER BY i.id FOR UPDATE OF i)
		UPDATE items SET data = CASE WHEN stale.confined THEN items.data || '{"confined": true}' ELSE items.data - 'confined' END
		FROM stale WHERE items.id = stale.id`, source, string(KindSeason), string(KindEpisode))
	return err
}
