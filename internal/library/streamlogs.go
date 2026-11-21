package library

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// noticeLogEvery is how often at most the line of an addon's answer of
	// notices alone is logged at its level for the same addon, title and
	// text (see noticeLine): 4 times a day at most, the repeats in between
	// logged at Debug, and counted on the next line. System › Health
	// counts every answer that carried a notice.
	noticeLogEvery = 6 * time.Hour
	// noticeLogLife is how long a line is remembered after the latest
	// answer that repeated it, past background work's longest wait (see
	// nothingAnswer): a title asked once a day stays at Debug. A line not
	// seen again within it is new again.
	noticeLogLife = nothingLife
)

// streamLogs keeps what the log lines of the requests for addons' streams
// need: the requests a user waits for, and the notices logged.
type streamLogs struct {
	mu sync.Mutex
	// waited are the requests under way that a user waits for, by flight
	// key (see streamsFlight).
	waited map[string]bool
	// notices are the answers of notices alone logged, kept noticeLogLife.
	notices *cache.Cache[noticeLine, noticeLogged]
}

// noticeLine is what an answer of notices alone is logged once for: the
// addon, the title, and the text of the notices with each run of digits
// one '#', as Health tells notices apart (see stremio.NoticeForm).
type noticeLine struct {
	key  streamKey
	text string
}

// noticeLogged is when a line was last logged, and how many answers
// repeated it since.
type noticeLogged struct {
	at      time.Time
	repeats int
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

// due tells, for an answer of notices alone at now, whether its line is
// logged, at most once within noticeLogEvery, whether it was before
// (again), and how many answers repeated it since; it counts the answer.
func (l *streamLogs) due(line noticeLine, now time.Time) (due, again bool, repeats int) {
	l.notices.Update(line, func(kept noticeLogged, ok bool) (noticeLogged, bool) {
		if ok && now.Sub(kept.at) < noticeLogEvery {
			kept.repeats++
			return kept, true
		}
		due, again, repeats = true, ok, kept.repeats
		return noticeLogged{at: now}, true
	})
	return due, again, repeats
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
	s.logger.Log(ctx, level, "An addon could not list streams", "addon", entry.addon.LogName(), "type", key.contentType,
		"id", key.id, "error", err)
}

// noticesLogged logs an addon's answer of notices alone with message,
// once within noticeLogEvery for the same addon, title and text (see
// noticeLine), and at Debug in between: a warning when they refuse the
// request for its rate (limited), else information. A line logged again
// tells the answers that repeated it since (repeats), at Debug unless a
// user waited for the answer (userWaited): System › Health counts
// background work's notices. The line tells the notices' text (see
// noticeText) and counts their shapes (see noticeShapes).
func (s *Service) noticesLogged(ctx context.Context, entry installed, key streamKey, message string, userWaited, limited bool,
	notices []stremio.Stream) {
	text := noticeText(notices)
	due, again, repeats := s.streamLogs.due(noticeLine{key, stremio.NoticeForm(text)}, s.now())
	level := slog.LevelInfo
	switch {
	case !due, again && !userWaited:
		level = slog.LevelDebug
	case limited:
		level = slog.LevelWarn
	}
	if !s.logger.Enabled(ctx, level) {
		return
	}
	attrs := []any{"addon", entry.addon.LogName(), "type", key.contentType, "id", key.id, "notice", text}
	if again {
		attrs = append(attrs, "repeats", repeats)
	}
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
