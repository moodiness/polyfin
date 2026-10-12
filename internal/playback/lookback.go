package playback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// How far before a segment's start FFmpeg must start reading a searched
// version, so as to land on a keyframe before the segment (see
// hls.Remux.Lookback), depends on how far apart its file's keyframes are.
// An encoding learns it at its first seek that lands past the segment;
// it is kept in the database by what Polyfin knows the file by, so that
// the next playback of the file, by any user, after a restart too, starts
// with it.

// seekFile is what a lookback is kept by: kind and name, as the
// seek_lookbacks table has them, and state, that of a local file, which
// changes with what it holds.
type seekFile struct {
	kind, name, state string
}

// seekFileOf tells what the lookback of version is kept by: a local
// folder's file by its folder and path, with its size and modification
// time (see Inputs.Identity); a catch-up programme by its channel, whose
// programmes usually come from the same encoder; any other version, an
// addon's file, by its identifier, which follows the file from one answer
// of its addon to the next (see library.Version.ID).
func (s *Service) seekFileOf(ctx context.Context, version library.Version) (seekFile, error) {
	key, local := strings.CutPrefix(version.URL, s.localBase())
	if !local {
		key, local = strings.CutPrefix(version.URL, s.inputBase())
	}
	if local && key != "" {
		s.loopback.mu.Lock()
		inputs := s.loopback.inputs
		s.loopback.mu.Unlock()
		if inputs == nil {
			return seekFile{}, errors.New("no local folders")
		}
		path, state, err := inputs.Identity(ctx, key)
		if err != nil {
			return seekFile{}, err
		}
		return seekFile{kind: "local", name: path, state: state}, nil
	}
	if version.HoldsConnection {
		if channel, ok := iptv.ReplayChannel(version.Origin.ID); ok {
			return seekFile{kind: "channel", name: channel}, nil
		}
	}
	return seekFile{kind: "version", name: version.ID.String()}, nil
}

// useLookback has an encoding of a searched version start with the
// lookback learned for its file, and keep those it learns.
func (s *Service) useLookback(ctx context.Context, r *hls.Remux, version library.Version) {
	f, err := s.seekFileOf(ctx, version)
	if err != nil {
		s.logger.Debug("A version's file is not known: seeks in it start without a lookback", "addon", version.Addon, "error", err)
		return
	}
	r.Lookback = s.savedLookback(ctx, f)
	r.Learned = func(lookback time.Duration) { s.saveLookback(context.Background(), f, lookback) }
}

// savedLookback returns the lookback learned for f, zero for none or for
// a local file that changed since.
func (s *Service) savedLookback(ctx context.Context, f seekFile) time.Duration {
	var milliseconds int
	err := s.db.QueryRow(ctx, "SELECT lookback_ms FROM seek_lookbacks WHERE kind = $1 AND name = $2 AND file = $3", f.kind, f.name, f.state).
		Scan(&milliseconds)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("Reading how far back seeks start failed", "error", err)
		}
		return 0
	}
	return time.Duration(milliseconds) * time.Millisecond
}

// saveLookback keeps the lookback a playback of f learned, rounded up to
// the millisecond: the larger of it and the one kept, which only a local
// file that changed since drops.
func (s *Service) saveLookback(ctx context.Context, f seekFile, lookback time.Duration) {
	milliseconds := (lookback + time.Millisecond - 1) / time.Millisecond
	if _, err := s.db.Exec(ctx, `INSERT INTO seek_lookbacks (kind, name, file, lookback_ms) VALUES ($1, $2, $3, $4)
		ON CONFLICT (kind, name) DO UPDATE SET file = excluded.file, learned_at = now(),
			lookback_ms = CASE WHEN seek_lookbacks.file = excluded.file THEN GREATEST(seek_lookbacks.lookback_ms, excluded.lookback_ms)
				ELSE excluded.lookback_ms END`, f.kind, f.name, f.state, int64(milliseconds)); err != nil {
		s.logger.Warn("Keeping how far back seeks start failed", "error", err)
	}
}
