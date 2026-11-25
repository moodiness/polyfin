package library

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/database/unsynced"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Each addon's last stream list for a title is saved in the database, so
// that a restart does not forget the versions known before: a list read
// back is stale (see list), listed at once while the addon is asked again,
// as a list that expired is, and kept as long (see staleLists). A refresh
// of the title deletes it (see forgetLists), and so does forgetting the
// version lists of every addon, or of one (see forgetVersionLists). A
// list's saving, reading back and dropping take its lock (see listLocks),
// so that the copy saved ends as the list in memory: a list dropped while
// it was being saved is not saved back, nor read back from a copy read
// before it was dropped. A save that fails, the database being too busy
// to answer in time, is tried again later (see savedRetries), with the
// list as it stands then.

const (
	// savedTimeout bounds the reading and saving of a list.
	savedTimeout = 5 * time.Second
	// forgetTimeout bounds the deleting of many lists at once (see
	// deleteSavedLists).
	forgetTimeout = 30 * time.Second
	// sweepEvery is how often saved lists past keeping are deleted.
	sweepEvery = time.Hour
)

// savedRetries are the waits before a stream list whose save failed is
// saved again, one for each failure in a row; past the last, the list
// stays in memory only.
var savedRetries = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// savedList is how a stream list is saved. Identities are the identities
// the streams carry from earlier answers (see carryIdentities), in order,
// "" for a stream without; none when no stream carries one.
type savedList struct {
	Streams    []stremio.Stream `json:"streams"`
	Identities []string         `json:"identities,omitempty"`
}

// newSavedList is how streams are saved.
func newSavedList(streams []stremio.Stream) savedList {
	saved := savedList{Streams: streams}
	for i, stream := range streams {
		if stream.Identity == "" {
			continue
		}
		if saved.Identities == nil {
			saved.Identities = make([]string, len(streams))
		}
		saved.Identities[i] = stream.Identity
	}
	return saved
}

// streams returns the streams saved, with their identities.
func (saved savedList) streams() []stremio.Stream {
	if len(saved.Identities) == len(saved.Streams) {
		for i, identity := range saved.Identities {
			saved.Streams[i].Identity = identity
		}
	}
	return saved.Streams
}

// savedStreams reads back the stream list saved for key, still kept. A key
// found missing, or saved past keeping, is not looked for again: every
// list saved since went through saveStreams, which forgets that.
func (s *Service) savedStreams(ctx context.Context, key streamKey) (list[stremio.Stream], bool) {
	if s.db == nil {
		return list[stremio.Stream]{}, false
	}
	unsaved := s.tracked().unsaved
	if _, missing := unsaved.Get(key); missing {
		return list[stremio.Stream]{}, false
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), savedTimeout)
	defer cancel()
	var data []byte
	var answer int
	var at time.Time
	err := s.db.QueryRow(ctx, "SELECT streams, answer, fetched_at FROM stream_lists WHERE addon_id = $1 AND content_type = $2 AND stremio_id = $3",
		key.addon, key.contentType, key.id).Scan(&data, &answer, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		unsaved.Put(key, true)
		return list[stremio.Stream]{}, false
	}
	if err != nil {
		s.logger.Warn("Reading a saved stream list failed", "error", err)
		return list[stremio.Stream]{}, false
	}
	return s.savedList(key, data, answer, at)
}

// savedList is the stream list saved for key as data, answer and at
// tell, when it is still kept. One saved past keeping is not looked for
// again (see savedStreams).
func (s *Service) savedList(key streamKey, data []byte, answer int, at time.Time) (list[stremio.Stream], bool) {
	var saved savedList
	if err := json.Unmarshal(data, &saved); err != nil {
		return list[stremio.Stream]{}, false
	}
	streams := saved.streams()
	l := list[stremio.Stream]{items: streams, answer: min(max(answer, 0), len(streams)), at: at, restored: true}
	if _, kept := l.state(s.now(), s.listLife()); !kept {
		s.tracked().unsaved.Put(key, true)
		return list[stremio.Stream]{}, false
	}
	return l, true
}

// savedListsOf reads the stream lists saved for keys, by their place
// among them (see restoreStreams).
var savedListsOf = database.Listing(`SELECT k.n, l.streams, l.answer, l.fetched_at
	FROM unnest($1::uuid[], $2::text[], $3::text[]) WITH ORDINALITY AS k(addon_id, content_type, stremio_id, n)
	JOIN stream_lists l ON l.addon_id = k.addon_id AND l.content_type = k.content_type AND l.stremio_id = k.stremio_id`)

// restoreStreams reads back, in one statement, the stream lists saved for
// keys that are neither kept in memory nor known missing, and keeps them
// as streamListOf keeps the one it reads back, under their locks: a
// listing reads its titles' lists so, rather than one by one after a
// restart (see Service.ListingAhead).
func (s *Service) restoreStreams(ctx context.Context, keys []streamKey) {
	if s.db == nil {
		return
	}
	tracked := s.tracked()
	slices.SortFunc(keys, compareStreamKeys)
	keys = slices.Compact(keys)
	keys = slices.DeleteFunc(keys, func(key streamKey) bool {
		_, _, kept := listOf(s, s.streamLists, key)
		_, missing := tracked.unsaved.Get(key)
		return kept || missing
	})
	if len(keys) == 0 {
		return
	}
	defer tracked.saving.lockMany(keys)()
	addonIDs := make([]accounts.ID, len(keys))
	types := make([]string, len(keys))
	ids := make([]string, len(keys))
	for i, key := range keys {
		addonIDs[i], types[i], ids[i] = key.addon, key.contentType, key.id
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), savedTimeout)
	defer cancel()
	rows, err := s.db.Query(ctx, savedListsOf, addonIDs, types, ids)
	if err != nil {
		s.logger.Warn("Reading saved stream lists failed", "error", err)
		return
	}
	found := make([]bool, len(keys))
	var (
		n      int
		data   []byte
		answer int
		at     time.Time
	)
	if _, err := pgx.ForEachRow(rows, []any{&n, &data, &answer, &at}, func() error {
		key := keys[n-1]
		found[n-1] = true
		if saved, ok := s.savedList(key, data, answer, at); ok {
			// An answer stored meanwhile wins.
			s.streamLists.Update(key, func(kept list[stremio.Stream], had bool) (list[stremio.Stream], bool) {
				if had {
					return kept, false
				}
				return saved, true
			})
		}
		return nil
	}); err != nil {
		s.logger.Warn("Reading saved stream lists failed", "error", err)
		return
	}
	for i, key := range keys {
		if !found[i] {
			tracked.unsaved.Put(key, true)
		}
	}
}

// compareStreamKeys orders stream keys, as lockMany locks them.
func compareStreamKeys(a, b streamKey) int {
	return cmp.Or(bytes.Compare(a.addon[:], b.addon[:]), strings.Compare(a.contentType, b.contentType), strings.Compare(a.id, b.id))
}

// saveStreams saves the stream list kept in memory for key, and now and
// then deletes the lists saved past keeping.
func (s *Service) saveStreams(key streamKey) {
	s.saveStreamsAfter(key, 0)
}

// saveStreamsAfter saves the stream list kept for key after failures
// failed saves in a row, and has it saved again later when it fails.
func (s *Service) saveStreamsAfter(key streamKey, failures int) {
	if s.db == nil || s.lifetime.Err() != nil {
		return
	}
	saved, err := s.writeStreams(key)
	if err != nil {
		if failures == len(savedRetries) {
			s.logger.Warn("Saving a stream list failed", "error", err)
			return
		}
		wait := savedRetries[failures]
		s.logger.Warn("Saving a stream list failed: it is saved again later", "error", err, "wait", wait)
		s.saveLater(wait, func() { s.saveStreamsAfter(key, failures+1) })
		return
	}
	if !saved {
		return
	}
	tracked := s.tracked()
	now := s.now()
	if last := tracked.swept.Load(); now.UnixNano()-last >= int64(sweepEvery) && tracked.swept.CompareAndSwap(last, now.UnixNano()) {
		ctx, cancel := context.WithTimeout(context.Background(), savedTimeout)
		defer cancel()
		if _, err := s.db.Exec(ctx, "DELETE FROM stream_lists WHERE fetched_at < $1", now.Add(-s.keptListLife())); err != nil {
			s.logger.Warn("Deleting old stream lists failed", "error", err)
		}
		if _, err := s.db.Exec(ctx, "DELETE FROM version_owners WHERE seen_at < $1", now.Add(-versionOwnersLife)); err != nil {
			s.logger.Warn("Deleting the owners of old versions failed", "error", err)
		}
	}
}

// writeStreams writes the stream list kept in memory for key to the
// database, reading it under the key's lock, and reports whether it did,
// or the database's error. The list is a cache, whose write does not wait
// for the disk (see unsynced.Exec).
func (s *Service) writeStreams(key streamKey) (bool, error) {
	tracked := s.tracked()
	defer tracked.saving.lock(key)()
	l, ok := s.streamLists.Get(key)
	if !ok || l.restored {
		return false, nil
	}
	data, err := json.Marshal(newSavedList(l.items))
	if err != nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), savedTimeout)
	defer cancel()
	if _, err := unsynced.Exec(ctx, s.db, `INSERT INTO stream_lists (addon_id, content_type, stremio_id, streams, answer, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (addon_id, content_type, stremio_id) DO UPDATE
		SET streams = excluded.streams, answer = excluded.answer, fetched_at = excluded.fetched_at`,
		key.addon, key.contentType, key.id, data, l.answer, l.at); err != nil {
		return false, err
	}
	tracked.unsaved.Delete(key)
	return true, nil
}

// dropStreams drops the stream list kept for key, in memory and saved,
// under the key's lock: a save under way does not save it back (see
// writeStreams), nor does a read under way restore it (see streamListOf).
func (s *Service) dropStreams(ctx context.Context, key streamKey) {
	tracked := s.tracked()
	defer tracked.saving.lock(key)()
	s.streamLists.Delete(key)
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), savedTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, "DELETE FROM stream_lists WHERE addon_id = $1 AND content_type = $2 AND stremio_id = $3",
		key.addon, key.contentType, key.id); err != nil {
		s.logger.Warn("Deleting a saved stream list failed", "error", err)
	}
	tracked.unsaved.Put(key, true)
}

// deleteSavedLists deletes the stream lists saved, every addon's, or
// addon's alone unless it is nil, and passes the key of each one deleted
// to deleted. The deletion goes on when ctx ends, bounded by
// forgetTimeout: the lists dropped from memory are not left saved.
func (s *Service) deleteSavedLists(ctx context.Context, addon *accounts.ID, deleted func(streamKey)) error {
	if s.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forgetTimeout)
	defer cancel()
	query, args := "DELETE FROM stream_lists RETURNING addon_id, content_type, stremio_id", []any(nil)
	if addon != nil {
		query, args = "DELETE FROM stream_lists WHERE addon_id = $1 RETURNING addon_id, content_type, stremio_id", []any{*addon}
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	var key streamKey
	_, err = pgx.ForEachRow(rows, []any{&key.addon, &key.contentType, &key.id}, func() error {
		deleted(key)
		return nil
	})
	return err
}

// listLocks holds a lock for each stream list being saved, read back or
// dropped (see writeStreams, streamListOf, restoreStreams and
// dropStreams), and a lock of every list, which forgetting many lists at
// once holds (see lockAll). A list's lock is dropped once nothing holds or
// awaits it. Its zero value is ready.
type listLocks struct {
	// all is held shared with the lists' locks, once by whoever holds one
	// or more, and alone by lockAll.
	all   sync.RWMutex
	mu    sync.Mutex
	locks map[streamKey]*listLock
}

type listLock struct {
	sync.Mutex
	// users counts those holding or awaiting it.
	users int
}

// lock locks key's list, and returns what unlocks it.
func (l *listLocks) lock(key streamKey) func() {
	l.all.RLock()
	k := l.lockKey(key)
	return func() {
		l.unlockKey(key, k)
		l.all.RUnlock()
	}
}

// lockMany locks the lists of keys, in the order compareStreamKeys gives,
// which keys are in, each once, and returns what unlocks them. Locking
// them in one order, while every list's lock is held once, keeps two
// callers, or one and lockAll, from waiting on each other.
func (l *listLocks) lockMany(keys []streamKey) func() {
	l.all.RLock()
	locked := make([]*listLock, len(keys))
	for i, key := range keys {
		locked[i] = l.lockKey(key)
	}
	return func() {
		for i, key := range keys {
			l.unlockKey(key, locked[i])
		}
		l.all.RUnlock()
	}
}

// lockKey locks key's list alone, l.all being held, and returns its lock.
func (l *listLocks) lockKey(key streamKey) *listLock {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[streamKey]*listLock{}
	}
	k := l.locks[key]
	if k == nil {
		k = &listLock{}
		l.locks[key] = k
	}
	k.users++
	l.mu.Unlock()
	k.Lock()
	return k
}

// unlockKey unlocks k, the lock of key's list lockKey returned.
func (l *listLocks) unlockKey(key streamKey, k *listLock) {
	k.Unlock()
	l.mu.Lock()
	if k.users--; k.users == 0 {
		delete(l.locks, key)
	}
	l.mu.Unlock()
}

// lockAll locks every list, once the lists locked now are unlocked, and
// returns what unlocks them: a save or a read back under way ends first,
// and the next ones wait.
func (l *listLocks) lockAll() func() {
	l.all.Lock()
	return l.all.Unlock
}
