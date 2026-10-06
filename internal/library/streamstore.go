package library

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Each addon's last stream list for a title is saved in the database, so
// that a restart does not forget the versions known before: a list read
// back is stale (see list), listed at once while the addon is asked again,
// as a list that expired is, and kept as long (see staleLists). A refresh
// of the title deletes it (see forgetLists).

const (
	// savedTimeout bounds the reading and saving of a list.
	savedTimeout = 5 * time.Second
	// sweepEvery is how often saved lists past keeping are deleted.
	sweepEvery = time.Hour
)

// savedList is how a stream list is saved.
type savedList struct {
	Streams []stremio.Stream `json:"streams"`
}

// savedStreams reads back the stream list saved for key, still kept. A key
// found missing is not looked for again: every list saved since went
// through saveStreams, which forgets that.
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
	var saved savedList
	if err := json.Unmarshal(data, &saved); err != nil {
		return list[stremio.Stream]{}, false
	}
	l := list[stremio.Stream]{items: saved.Streams, answer: min(max(answer, 0), len(saved.Streams)), at: at, restored: true}
	if _, kept := l.state(s.now(), s.listLife()); !kept {
		return list[stremio.Stream]{}, false
	}
	return l, true
}

// saveStreams saves the stream list kept in memory for key, and now and
// then deletes the lists saved past keeping.
func (s *Service) saveStreams(key streamKey) {
	if s.db == nil {
		return
	}
	l, ok := s.streamLists.Get(key)
	if !ok || l.restored {
		return
	}
	data, err := json.Marshal(savedList{Streams: l.items})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), savedTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, `INSERT INTO stream_lists (addon_id, content_type, stremio_id, streams, answer, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (addon_id, content_type, stremio_id) DO UPDATE
		SET streams = excluded.streams, answer = excluded.answer, fetched_at = excluded.fetched_at`,
		key.addon, key.contentType, key.id, data, l.answer, l.at); err != nil {
		s.logger.Warn("Saving a stream list failed", "error", err)
		return
	}
	tracked := s.tracked()
	tracked.unsaved.Delete(key)
	now := s.now()
	if last := tracked.swept.Load(); now.UnixNano()-last >= int64(sweepEvery) && tracked.swept.CompareAndSwap(last, now.UnixNano()) {
		if _, err := s.db.Exec(ctx, "DELETE FROM stream_lists WHERE fetched_at < $1", now.Add(-s.keptListLife())); err != nil {
			s.logger.Warn("Deleting old stream lists failed", "error", err)
		}
	}
}

// deleteSavedStreams deletes the stream list saved for key.
func (s *Service) deleteSavedStreams(ctx context.Context, key streamKey) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), savedTimeout)
	defer cancel()
	if _, err := s.db.Exec(ctx, "DELETE FROM stream_lists WHERE addon_id = $1 AND content_type = $2 AND stremio_id = $3",
		key.addon, key.contentType, key.id); err != nil {
		s.logger.Warn("Deleting a saved stream list failed", "error", err)
	}
	s.tracked().unsaved.Put(key, true)
}
