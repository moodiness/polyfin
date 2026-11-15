package library

import (
	"context"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// hintSpacing is the time between two reads of ReadConfigureHints: one
// addon at a time, spread out, as no user waits for them.
const hintSpacing = 2 * time.Second

// ReadConfigureHints reads, once at start, the configure hint of the
// Stremio and Eclipse addons whose stored manifest predates it (see
// addons.Store.HintsUnread), the server's and users' own, so that
// Configure shows on their page without a Refresh. It reads one addon at
// a time, hintSpacing apart, each request background work's that waits
// for its turn by its server (see pacing). An addon whose server asked to
// wait, for its catalog pages or its background requests, is skipped,
// and so is one whose read fails, a refusal for its rate pausing its
// server as any other: each stays unread, for the next start. Only the
// hint is stored (see addons.Store.SetHint). One line at Info tells how
// the pass went; the addons' addresses, which hold their settings and
// keys, are never logged. It returns when the pass ends or ctx ends.
func (s *Service) ReadConfigureHints(ctx context.Context) {
	unread, err := s.addons.HintsUnread(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The addons whose configure hint is to be read could not be listed", "error", err)
		}
		return
	}
	if len(unread) == 0 {
		return
	}
	var read, configurable, skipped, failed int
	for _, u := range unread {
		timer := time.NewTimer(s.hintSpacing)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		host := stremio.HostOf(u.Addon.ManifestURL)
		if _, waits := s.catalogWaitLeft(host); waits || s.pace.paused(host) {
			skipped++
			continue
		}
		hint, err := request(Background(ctx), s, u.Addon, paceMetas, func(ctx context.Context) (bool, error) {
			return s.addons.ReadHint(ctx, u.Addon, u.Confined)
		})
		if err == nil {
			_, err = s.addons.SetHint(ctx, u.Addon.ID, hint)
		}
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			failed++
		default:
			read++
			if hint {
				configurable++
			}
		}
	}
	s.logger.Info("Read the configure hint of the addons stored without it", "read", read, "configurable", configurable,
		"skipped", skipped, "failed", failed)
}
