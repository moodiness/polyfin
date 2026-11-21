package library

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Versions are remembered in memory (see VersionOwner), and the item each
// belongs to is saved too, so that an app opening a version as an item by
// its identifier, as Jellyfin apps do, finds it after a restart, before
// anything lists the item again (see SavedVersionOwner). A listing notes
// the owners of the versions it remembers first; the owners noted are
// saved together, ownersSaveDelay after the first of them, in one
// statement: a page of episodes, each listing its versions, saves them
// once, and a listing of versions already remembered notes nothing.
// Owners not saved again for versionOwnersLife are deleted with the
// stream lists saved past keeping (see saveStreamsAfter).

const (
	// versionOwnersLife is how long a version's owner stays saved.
	versionOwnersLife = 30 * 24 * time.Hour
	// ownersSaveDelay is how long the owners noted wait to be saved
	// together.
	ownersSaveDelay = 2 * time.Second
)

// ownerSaves holds the owners of versions noted and not saved yet, by
// version, and whether their save is due.
type ownerSaves struct {
	mu      sync.Mutex
	pending map[accounts.ID]accounts.ID
	due     bool
}

// keepOwners notes the owners of the versions of kept that before does
// not hold, those remembered until now, and has them saved.
func (s *Service) keepOwners(kept, before map[accounts.ID]Version) {
	if s.db == nil || len(kept) == len(before) {
		return
	}
	saves := &s.tracked().owners
	saves.mu.Lock()
	defer saves.mu.Unlock()
	if saves.pending == nil {
		saves.pending = make(map[accounts.ID]accounts.ID, len(kept)-len(before))
	}
	for id, version := range kept {
		if _, known := before[id]; !known {
			saves.pending[id] = version.Item
		}
	}
	if !saves.due {
		saves.due = true
		s.saveLater(ownersSaveDelay, s.saveOwners)
	}
}

// saveOwners saves the owners noted, in one statement.
func (s *Service) saveOwners() {
	saves := &s.tracked().owners
	saves.mu.Lock()
	pending := saves.pending
	saves.pending, saves.due = nil, false
	saves.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	versions := make([]accounts.ID, 0, len(pending))
	items := make([]accounts.ID, 0, len(pending))
	for id, item := range pending {
		versions, items = append(versions, id), append(items, item)
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
// as VersionOwner does, else as noted or saved: after a restart, a version
// not listed since is still opened as an item.
func (s *Service) SavedVersionOwner(ctx context.Context, id accounts.ID) (accounts.ID, bool) {
	if owner, ok := s.VersionOwner(id); ok {
		return owner, true
	}
	if s.db == nil {
		return accounts.ID{}, false
	}
	saves := &s.tracked().owners
	saves.mu.Lock()
	owner, noted := saves.pending[id]
	saves.mu.Unlock()
	if noted {
		return owner, true
	}
	err := s.db.QueryRow(ctx, "SELECT item_id FROM version_owners WHERE version_id = $1", id).Scan(&owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Warn("Reading the owner of a version failed", "error", err)
	}
	return owner, err == nil
}
