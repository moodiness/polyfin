package library

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// StaleAddon tells what MarkAddonStale made old or forgot of an addon:
// its catalog pages and descriptions kept in the database, and its
// version lists.
type StaleAddon struct {
	Pages        int64
	Descriptions int64
	VersionLists int
}

// MarkAddonStale has what Polyfin keeps of an addon's answers asked for
// again as it is next needed, as the source page's Refresh everything
// asks, while asking the addon nothing now. Its catalog pages, in memory
// and in the database, are dated past the catalog refresh age: a read
// shows the page kept and asks for it again in the background, and the
// hourly read of the libraries' first pages asks again for those (see
// Warm). Its descriptions are dated past metaTTL: a title shown asks for
// its description again the same way. Pages and descriptions already
// that old keep their date. Its page sizes are forgotten, and learnt
// again from the pages it gives next (see learnPageSize), and so are its
// version lists (see ForgetAddonVersionLists).
func (s *Service) MarkAddonStale(ctx context.Context, addon accounts.ID) (StaleAddon, error) {
	var stale StaleAddon
	now := s.now()
	pagesAt := now.Add(-s.catalogLife() - time.Second)
	metasAt := now.Add(-metaTTL - time.Second)
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE catalog_pages SET fetched_at = $2 WHERE addon_id = $1 AND fetched_at > $2", addon, pagesAt)
		if err != nil {
			return err
		}
		stale.Pages = tag.RowsAffected()
		if tag, err = tx.Exec(ctx, "UPDATE metas SET fetched_at = $2 WHERE addon_id = $1 AND fetched_at > $2", addon, metasAt); err != nil {
			return err
		}
		stale.Descriptions = tag.RowsAffected()
		_, err = tx.Exec(ctx, "DELETE FROM catalog_sizes WHERE addon_id = $1", addon)
		return err
	})
	if err != nil {
		return stale, err
	}
	s.pages.ChangeFunc(func(key pageKey, kept fetched[[]stremio.Meta]) (fetched[[]stremio.Meta], bool) {
		if key.addon != addon || !kept.at.After(pagesAt) {
			return kept, false
		}
		kept.at = pagesAt
		return kept, true
	})
	s.metas.ChangeFunc(func(key metaKey, kept fetched[stremio.Meta]) (fetched[stremio.Meta], bool) {
		if key.addon != addon || !kept.at.After(metasAt) {
			return kept, false
		}
		kept.at = metasAt
		return kept, true
	})
	s.sizes.mu.Lock()
	for key := range s.sizes.byCatalog {
		if key.addon == addon {
			delete(s.sizes.byCatalog, key)
		}
	}
	s.sizes.mu.Unlock()
	stale.VersionLists, err = s.ForgetAddonVersionLists(ctx, addon)
	return stale, err
}
