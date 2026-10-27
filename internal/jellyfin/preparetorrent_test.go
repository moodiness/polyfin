package jellyfin

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// loggedErrors keeps the errors the handler logs, at every level.
type loggedErrors struct {
	mu   sync.Mutex
	errs []error
}

func (l *loggedErrors) Enabled(context.Context, slog.Level) bool { return true }

func (l *loggedErrors) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if err, ok := attr.Value.Any().(error); ok {
			l.mu.Lock()
			l.errs = append(l.errs, err)
			l.mu.Unlock()
		}
		return true
	})
	return nil
}

func (l *loggedErrors) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l *loggedErrors) WithGroup(string) slog.Handler { return l }

func (l *loggedErrors) logged(target error) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.ContainsFunc(l.errs, func(err error) bool { return errors.Is(err, target) })
}

// Opening a title's page, again and again, prepares none of its torrent
// versions while no playback runs their torrent: nothing reads them, and
// nothing is logged of them each time.
func TestOpeningATitlePreparesNoTorrentNoPlaybackRuns(t *testing.T) {
	t.Parallel()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(300 << 10)})
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1")
	logged := &loggedErrors{}
	p := newTorrentPlay(t, "ffprobe-not-installed", 0, []*torrenttest.Torrent{tor}, []string{tracker.URL()},
		func(o *Options) { o.Logger = slog.New(logged) })
	p.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	for range 3 {
		p.details(t)
		p.settle(t)
	}
	if logged.logged(playback.ErrTorrentIdle) {
		t.Error("a torrent version no playback runs was prepared")
	}
	if len(tracker.Remotes()) != 0 {
		t.Error("opening the title's page joined the torrent's swarm")
	}
}
