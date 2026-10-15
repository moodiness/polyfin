// Package commercials finds the commercial breaks of finished Live TV
// recordings, which apps offer to skip as Commercial segments.
//
// Once a recording is finished, partial ones included, it is read in the
// background, one recording at a time, while the settings turn the search
// on (Settings.RecordingCommercials). FFmpeg reads it at the lowest
// priority, its video scaled down as soon as it is decoded, and marks the
// moments that are black (blackdetect) and silent (silencedetect) at once:
// channels part their programmes from their commercials, and the
// commercials from one another, with such moments. A break is a run of
// pieces between such moments each lasting shortestPiece to longestPiece,
// and shortestBreak to longestBreak in all (see breaks). The recording's
// start counts as such a moment, as a recording that starts early opens on
// commercials.
//
// A recording longer than longestRead is not read, and a reading that runs
// past the recording's own length stops: such recordings get none. What
// was found is kept by the recording's identifier, never its file, so that
// it stays when the file is replaced, and is deleted with the recording.
// The recordings not read yet, such as those that ended while the server
// stopped or the search was off, are read by Run, a task of the server.
package commercials

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/recordings"
)

const (
	// longestRead bounds the recordings read: a longer one gets none.
	longestRead = 4 * time.Hour
	// queueLength bounds the recordings waiting to be read; one left out
	// is read by the next Run.
	queueLength = 64
	// RunInterval is how often the recordings not read yet should be
	// read (see Run).
	RunInterval = 24 * time.Hour
)

// Recordings finds the recordings and their files; recordings.Service is
// one.
type Recordings interface {
	Recording(ctx context.Context, id accounts.ID) (recordings.Recording, error)
	Path(r recordings.Recording) string
}

// Options are what a Service needs.
type Options struct {
	DB *pgxpool.Pool
	// FFmpeg reads the recordings; FFprobe tells how long they last.
	FFmpeg, FFprobe string
	Recordings      Recordings
	// Settings turn the search on, RecordingCommercials, and the segments
	// served, SkipButtons.
	Settings func() accounts.Settings
	Logger   *slog.Logger
}

// Service reads finished recordings, one at a time, and gives the
// commercial breaks found.
type Service struct {
	Options
	queue chan accounts.ID
	// reading lets one recording be read at a time, by the queue or Run.
	reading sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	// longest bounds the recordings read, longestRead but in tests, and
	// budget how long reading one lasting length may take.
	longest time.Duration
	budget  func(length time.Duration) time.Duration
}

// New starts a Service; Close stops it.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{Options: options, queue: make(chan accounts.ID, queueLength), cancel: cancel, done: make(chan struct{}),
		longest: longestRead, budget: func(length time.Duration) time.Duration { return length }}
	go s.run(ctx)
	return s
}

// Close stops the reading under way, and waits for it.
func (s *Service) Close() {
	s.cancel()
	<-s.done
}

// on tells whether the settings turn the search on.
func (s *Service) on() bool {
	return s.Settings().RecordingCommercials
}

// Ended has a recording that ended read in the background, once those
// queued before it are, if it was finished and the settings turn the
// search on. It never blocks: a recording the queue has no room for is
// left to Run.
func (s *Service) Ended(_ context.Context, ended recordings.Ended) {
	if s == nil || ended.Failed || !s.on() {
		return
	}
	select {
	case s.queue <- ended.Recording:
	default:
	}
}

// run reads the recordings queued, one at a time, until ctx ends.
func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.queue:
			if !s.on() {
				continue
			}
			if err := s.read(ctx, id); err != nil && ctx.Err() == nil {
				s.Logger.Warn("A recording could not be read for its commercials", "recording", id.String(), "error", err)
			}
		}
	}
}

// Run reads the finished recordings not read yet, the oldest first, one
// at a time, while the settings turn the search on. It is the server's
// task.
func (s *Service) Run(ctx context.Context) error {
	if !s.on() {
		return nil
	}
	rows, err := s.DB.Query(ctx, `SELECT r.id FROM live_recordings r WHERE r.status = 'Completed'
		AND NOT EXISTS (SELECT 1 FROM recording_commercial_reads c WHERE c.recording_id = r.id) ORDER BY r.ended_at, r.id`)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !s.on() {
			return nil
		}
		if err := s.read(ctx, id); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.Logger.Warn("A recording could not be read for its commercials", "recording", id.String(), "error", err)
		}
	}
	return nil
}

// read reads a finished recording for its commercial breaks, unless it was
// read already, and keeps those found, none included. A recording deleted,
// still written, or whose file is missing, as when recording is off, is
// left as it is; one that cannot be read, or not within its bounds, gets
// none.
func (s *Service) read(ctx context.Context, id accounts.ID) error {
	s.reading.Lock()
	defer s.reading.Unlock()
	var read bool
	if err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM recording_commercial_reads WHERE recording_id = $1)", id).Scan(&read); err != nil {
		return err
	}
	if read {
		return nil
	}
	recording, err := s.Recordings.Recording(ctx, id)
	if errors.Is(err, recordings.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	path := s.Recordings.Path(recording)
	if recording.InProgress() || path == "" || !exists(path) {
		return nil
	}
	length, err := duration(ctx, s.FFprobe, path)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.Logger.Warn("A recording's length could not be read: it gets no commercials", "recording", id.String(), "error", err)
		return s.save(ctx, id, nil)
	}
	if length > s.longest {
		s.Logger.Info("A recording is too long to be read for its commercials", "recording", id.String(), "length", length.String())
		return s.save(ctx, id, nil)
	}
	reading, cancel := context.WithTimeout(ctx, s.budget(length))
	defer cancel()
	started := time.Now()
	moments, err := detect(reading, s.FFmpeg, path, length)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case reading.Err() != nil:
		s.Logger.Info("Reading a recording for its commercials took longer than the recording lasts: it gets none", "recording", id.String(),
			"length", length.String())
		return s.save(ctx, id, nil)
	case err != nil:
		s.Logger.Warn("A recording could not be read for its commercials: it gets none", "recording", id.String(), "error", err)
		return s.save(ctx, id, nil)
	}
	found := breaks(moments)
	s.Logger.Info("A recording was read for its commercials", "recording", id.String(), "breaks", len(found),
		"took", time.Since(started).Round(time.Second).String())
	return s.save(ctx, id, found)
}

// save keeps that a recording was read, and the breaks found in it, by
// start. A recording deleted meanwhile keeps nothing.
func (s *Service) save(ctx context.Context, id accounts.ID, found []span) error {
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "INSERT INTO recording_commercial_reads (recording_id) VALUES ($1) ON CONFLICT DO NOTHING", id)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		for _, b := range found {
			if _, err := tx.Exec(ctx, "INSERT INTO recording_commercials (recording_id, start_ms, end_ms) VALUES ($1, $2, $3)",
				id, b.start.Milliseconds(), b.end.Milliseconds()); err != nil {
				return err
			}
		}
		return nil
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23503" {
		return nil
	}
	return err
}

// Segments returns the commercial breaks found in a recording, by start,
// as Commercial segments, while the settings turn the skip buttons on.
func (s *Service) Segments(ctx context.Context, recording accounts.ID) []mediasegments.Segment {
	if s == nil || !s.Settings().SkipButtons {
		return nil
	}
	rows, err := s.DB.Query(ctx, "SELECT start_ms, end_ms FROM recording_commercials WHERE recording_id = $1 ORDER BY start_ms", recording)
	if err != nil {
		s.Logger.Warn("The commercials of a recording could not be read", "error", err)
		return nil
	}
	segments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (mediasegments.Segment, error) {
		var start, end int64
		err := row.Scan(&start, &end)
		return mediasegments.Segment{Type: mediasegments.Commercial, Start: time.Duration(start) * time.Millisecond,
			End: time.Duration(end) * time.Millisecond}, err
	})
	if err != nil {
		s.Logger.Warn("The commercials of a recording could not be read", "error", err)
		return nil
	}
	return segments
}

// Has reports whether commercial breaks were found in a recording, while
// the settings turn the skip buttons on: whether it has segments.
func (s *Service) Has(ctx context.Context, recording accounts.ID) bool {
	if s == nil || !s.Settings().SkipButtons {
		return false
	}
	var has bool
	if err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM recording_commercials WHERE recording_id = $1)", recording).Scan(&has); err != nil {
		s.Logger.Warn("The commercials of a recording could not be read", "error", err)
		return false
	}
	return has
}
