package library

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Versions are remembered in memory (see VersionOwner), and the item each
// belongs to is saved too, so that an app opening a version as an item by
// its identifier, as Jellyfin apps do, finds it after a restart, before
// anything lists the item again (see SavedVersionOwner). A listing saves
// the owners of the versions it remembers first, in one statement: one
// that lists versions already remembered saves nothing. Owners not saved
// again for versionOwnersLife are deleted with the stream lists saved
// past keeping (see saveStreamsAfter).

// versionOwnersLife is how long a version's owner stays saved.
const versionOwnersLife = 30 * 24 * time.Hour

// keepOwners saves the owners of the versions of kept that before does
// not hold, those remembered until now.
func (s *Service) keepOwners(kept, before map[accounts.ID]Version) {
	if s.db == nil || len(kept) == len(before) {
		return
	}
	versions := make([]accounts.ID, 0, len(kept)-len(before))
	items := make([]accounts.ID, 0, len(kept)-len(before))
	for id, version := range kept {
		if _, known := before[id]; !known {
			versions, items = append(versions, id), append(items, version.Item)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), savedTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, `INSERT INTO version_owners (version_id, item_id, seen_at)
		SELECT version_id, item_id, $3 FROM unnest($1::uuid[], $2::uuid[]) AS v (version_id, item_id)
		ON CONFLICT (version_id) DO UPDATE SET item_id = excluded.item_id, seen_at = excluded.seen_at`,
		versions, items, s.now()); err != nil {
		s.logger.Warn("Saving the owners of versions failed", "error", err)
	}
}

// SavedVersionOwner returns the item a version listed earlier belongs to,
// as VersionOwner does, else as saved: after a restart, a version not
// listed since is still opened as an item.
func (s *Service) SavedVersionOwner(ctx context.Context, id accounts.ID) (accounts.ID, bool) {
	if owner, ok := s.VersionOwner(id); ok {
		return owner, true
	}
	if s.db == nil {
		return accounts.ID{}, false
	}
	var owner accounts.ID
	err := s.db.QueryRow(ctx, "SELECT item_id FROM version_owners WHERE version_id = $1", id).Scan(&owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Warn("Reading the owner of a version failed", "error", err)
	}
	return owner, err == nil
}
