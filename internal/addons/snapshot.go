package addons

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// changesChannel is where the database notifies changes to the addons,
// libraries and guides, once their transaction commits, naming the schema
// of the table changed (see the triggers of the migrations).
const changesChannel = "polyfin_addons"

// The delays before following changes again once the connection
// listening to them failed.
const (
	firstListenRetry = time.Second
	maxListenRetry   = time.Minute
)

// snapshot is the addons, with their manifests decoded, and libraries of
// every scope, as read in one transaction. It is shared: nothing in it
// changes once it is made.
type snapshot struct {
	// changes is the count of changes the snapshot holds (see
	// Store.changes).
	changes uint64
	// scopes are the addons and libraries of each scope, by owner, the
	// zero ID for the server's.
	scopes map[accounts.ID]scoped
	// addons are the addons of every scope, by identifier.
	addons map[accounts.ID]Addon
}

// scoped is what Addons and Libraries list for a scope.
type scoped struct {
	addons    []Addon
	libraries []Library
}

// ownerOf is the key of scope in snapshot.scopes.
func ownerOf(scope Scope) accounts.ID {
	if scope.Owner == nil {
		return accounts.ID{}
	}
	return *scope.Owner
}

// changed counts a change to the addons, libraries or guides: the
// snapshot read before it is not served again. The store's writes call
// it once their change is stored, so that the next read sees it.
func (s *Store) changed() {
	s.changes.Add(1)
}

// current returns a snapshot holding every change made before the call.
// While Watch follows the changes, the last snapshot is served until a
// change comes, and one read of the database serves every caller waiting
// for it; otherwise every call reads the database.
func (s *Store) current(ctx context.Context) (*snapshot, error) {
	if !s.watching.Load() {
		return s.load(ctx)
	}
	changes := s.changes.Load()
	if snap := s.snapshot.Load(); snap != nil && snap.changes == changes {
		return snap, nil
	}
	s.loading.Lock()
	defer s.loading.Unlock()
	// The count is read before the database: a change stored meanwhile
	// counts after it, so the snapshot read now is not served for it.
	changes = s.changes.Load()
	if snap := s.snapshot.Load(); snap != nil && snap.changes == changes {
		return snap, nil
	}
	snap, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	snap.changes = changes
	s.snapshot.Store(snap)
	return snap, nil
}

// load reads the addons, libraries and guides of every scope in one
// transaction, so that they agree.
func (s *Store) load(ctx context.Context) (*snapshot, error) {
	type ownedAddon struct {
		owner *accounts.ID
		addon Addon
	}
	var installed []ownedAddon
	var rows []libraryRow
	var guides []guideRow
	err := pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		result, err := tx.Query(ctx, "SELECT "+addonColumns+", owner_id FROM addons ORDER BY position, created_at")
		if err != nil {
			return err
		}
		if installed, err = pgx.CollectRows(result, func(row pgx.CollectableRow) (ownedAddon, error) {
			var owned ownedAddon
			addon, err := scanAddon(row, &owned.owner)
			owned.addon = addon
			return owned, err
		}); err != nil {
			return err
		}
		if rows, err = libraryRows(ctx, tx, ""); err != nil {
			return err
		}
		guides, err = guideRows(ctx, tx, "")
		return err
	})
	if err != nil {
		return nil, err
	}
	owner := func(id *accounts.ID) accounts.ID { return ownerOf(Scope{Owner: id}) }
	byScope := map[accounts.ID][]Addon{}
	snap := &snapshot{scopes: map[accounts.ID]scoped{}, addons: make(map[accounts.ID]Addon, len(installed))}
	for _, owned := range installed {
		key := owner(owned.owner)
		byScope[key] = append(byScope[key], owned.addon)
		snap.addons[owned.addon.ID] = owned.addon
	}
	rowsOf := map[accounts.ID][]libraryRow{}
	for _, row := range rows {
		rowsOf[owner(row.owner)] = append(rowsOf[owner(row.owner)], row)
	}
	guidesOf := map[accounts.ID][]guideRow{}
	for _, guide := range guides {
		guidesOf[owner(guide.owner)] = append(guidesOf[owner(guide.owner)], guide)
	}
	for key, list := range byScope {
		snap.scopes[key] = scoped{addons: list, libraries: listLibraries(list, rowsOf[key], guidesOf[key])}
	}
	return snap, nil
}

// Watch follows the changes made to the addons, libraries and guides of
// every scope, by this server or anyone else, until ctx ends, so that
// Addons, Libraries and Find are answered from memory. Until it listens,
// or while the database cannot be listened to, they read the database
// every time. A connection of its own listens, outside the pool.
func (s *Store) Watch(ctx context.Context, logger *slog.Logger) {
	delay := firstListenRetry
	for {
		listened, err := s.listen(ctx)
		s.watching.Store(false)
		s.changed()
		if ctx.Err() != nil {
			return
		}
		if listened {
			delay = firstListenRetry
		}
		logger.Warn("Changes to addons cannot be followed; they are read from the database until they can", "error", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxListenRetry)
	}
}

// listen listens to the changes until the connection fails or ctx ends,
// and reports whether it got to listen.
func (s *Store) listen(ctx context.Context) (bool, error) {
	conn, err := pgx.ConnectConfig(ctx, s.db.Config().ConnConfig)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	// The channel is the database's: servers, or tests, sharing it keep
	// their tables in schemas of their own, and each follows only the
	// changes of the schema its queries find the addons in.
	var schema string
	if err := conn.QueryRow(ctx, `SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.oid = 'addons'::regclass`).Scan(&schema); err != nil {
		return false, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+changesChannel); err != nil {
		return false, err
	}
	// What changed before the server listened is read again.
	s.changed()
	s.watching.Store(true)
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, err
		}
		if notification.Payload == schema {
			s.changed()
		}
	}
}
