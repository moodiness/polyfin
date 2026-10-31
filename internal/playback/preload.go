package playback

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
)

// SourcePolicy is how much the source cache keeps, and for how long, as
// the server's settings say: CacheSizeGB, SourceCacheDays and
// SourceCacheCreditDays.
func SourcePolicy(s accounts.Settings) source.Policy {
	const day = 24 * time.Hour
	return source.Policy{Limit: int64(s.CacheSizeGB) * 1e9, Keep: time.Duration(s.SourceCacheDays) * day,
		Credit: time.Duration(s.SourceCacheCreditDays) * day}
}

// What a preload reads of a file but for a whole one: its opening, the
// first minutes of a 1080p episode, and its end, where many files keep
// their index, which a play's analysis reads.
const (
	preloadOpening = 128 << 20
	preloadEnd     = 4 << 20
)

// Preload reads a version's file into the source cache ahead of a play
// that may come, within p (see source.Source.Preload): its opening and
// its end, or the whole file with whole. It reads in the background,
// giving way to the playbacks reading the same host. A torrent's file is
// never read: only playbacks join its swarm, and its pieces are not kept.
// It returns the bytes fetched.
func (s *Service) Preload(ctx context.Context, version library.Version, whole bool, p source.Preloading) (int64, error) {
	if version.Torrent != nil || s.Failed(version.ID) {
		return 0, nil
	}
	src := s.open(version)
	defer src.Release()
	return src.Preload(Background(ctx), p, func(size int64) []source.Span {
		if whole {
			return []source.Span{{Off: 0, End: size}}
		}
		return []source.Span{{Off: 0, End: min(size, preloadOpening)}, {Off: max(size-preloadEnd, preloadOpening), End: size}}
	})
}
