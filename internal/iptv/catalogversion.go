package iptv

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// CatalogVersion returns the version of what a source's movie and series
// catalogs list (see CatalogWindow): it changes whenever they may. They
// read its stored titles and the options of its row in iptv_sources
// (shownTitle); every statement that writes its titles (storeTitles, from
// storeDownload and reconcile) runs in a transaction that also updates
// that row, and so does every change of its options. The version is the
// row's: the transaction that wrote it last (xmin), with when its lists
// were last fetched. stremio.ErrNotFound reports a source of no row.
func (s *Service) CatalogVersion(ctx context.Context, source accounts.ID) (string, error) {
	var version string
	err := s.db.QueryRow(ctx, `SELECT xmin::text || ' ' || coalesce(fetched_at::text, '') FROM iptv_sources WHERE addon_id = $1`,
		source).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", stremio.ErrNotFound
	}
	return version, err
}
