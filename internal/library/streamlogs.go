package library

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// noticeLogLife is how long the line of an addon's answer of notices
// alone is logged once at its level for the same addon, title and text:
// the repeats within it are logged at Debug.
const noticeLogLife = 24 * time.Hour

// streamLogs keeps what the log lines of the requests for addons' streams
// need: the requests a user waits for, and the notices logged.
type streamLogs struct {
	mu sync.Mutex
	// waited are the requests under way that a user waits for, by flight
	// key (see streamsFlight).
	waited map[string]bool
	// notices are the answers of notices alone logged within
	// noticeLogLife.
	notices *cache.Cache[noticeLine, struct{}]
}

// noticeLine is what an answer of notices alone is logged once for.
type noticeLine struct {
	key  streamKey
	text string
}

// waits marks the request flight as one a user waits for, unless ctx is
// background work's (see Background): its failure is a warning then,
// whoever else shares it.
func (l *streamLogs) waits(ctx context.Context, flight string) {
	if markOf(ctx).background {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.waited == nil {
		l.waited = map[string]bool{}
	}
	l.waited[flight] = true
}

// ended forgets the request flight, once it is answered or failed, and
// reports whether a user waited for it.
func (l *streamLogs) ended(flight string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	waited := l.waited[flight]
	delete(l.waited, flight)
	return waited
}

// first reports whether line is logged for the first time within
// noticeLogLife, and remembers it.
func (l *streamLogs) first(line noticeLine) bool {
	return l.notices.Update(line, func(_ struct{}, ok bool) (struct{}, bool) { return struct{}{}, !ok })
}

// streamsFailed logs a request for an addon's streams for a title that
// failed, once whoever shares it: a warning when a user waited for it,
// as when a title is opened or played, else information, as for the
// watchlists, home rows listed ahead and preparing. The follow-ups ask
// in their own requests (see followUpList).
func (s *Service) streamsFailed(ctx context.Context, entry installed, key streamKey, userWaited bool, err error) {
	level := slog.LevelInfo
	if userWaited {
		level = slog.LevelWarn
	}
	s.logger.Log(ctx, level, "An addon could not list streams", "addon", entry.addon.Manifest.Name, "type", key.contentType,
		"id", key.id, "error", err)
}

// noticesLogged logs an addon's answer of notices alone with message: a
// warning when they refuse the request for its rate, else information,
// and Debug for the same addon, title and text again within
// noticeLogLife. The line tells the notices' text (see noticeText) and
// counts their shapes (see noticeShapes).
func (s *Service) noticesLogged(ctx context.Context, entry installed, key streamKey, message string, notices []stremio.Stream) {
	text := noticeText(notices)
	level := slog.LevelInfo
	switch {
	case !s.streamLogs.first(noticeLine{key, text}):
		level = slog.LevelDebug
	case refusedNotices(notices):
		level = slog.LevelWarn
	}
	if !s.logger.Enabled(ctx, level) {
		return
	}
	attrs := []any{"addon", entry.addon.Manifest.Name, "type", key.contentType, "id", key.id, "notice", text}
	s.logger.Log(ctx, level, message, append(attrs, noticeShapes(notices)...)...)
}

// noticeShapes counts, as log attributes, the entries of an answer of
// notices alone, and those of each shape Polyfin cannot play: with no
// link (a notice, a web page or a YouTube video), a Usenet download, an
// archive's file, a link of another scheme than http or https, and a
// torrent whose info hash is not 40 hexadecimal digits or whose file
// index is negative.
func noticeShapes(notices []stremio.Stream) []any {
	var noLink, usenet, archives, otherScheme, badHash int
	for _, notice := range notices {
		switch {
		case notice.Usenet != "":
			usenet++
		case notice.Archive():
			archives++
		case notice.URL != "":
			otherScheme++
		case notice.InfoHash != "":
			badHash++
		default:
			noLink++
		}
	}
	return []any{"entries", len(notices), "no_link", noLink, "usenet", usenet, "archives", archives,
		"other_schemes", otherScheme, "invalid_info_hash", badHash}
}
