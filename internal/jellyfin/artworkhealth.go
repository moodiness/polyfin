package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/netfail"
	"github.com/moodiness/polyfin/internal/stremio"
)

// What becomes of the artwork downloads that fail: Health counts them
// (see ArtworkStatus), and a summary line is logged every hour that has
// some, at INFO, as each one is logged at DEBUG only.

// The kinds of artwork download failures, as Health counts them.
const (
	ArtworkNotFound    = "not_found"
	ArtworkRateLimited = "rate_limited"
	ArtworkTimeout     = "timeout"
	ArtworkServerError = "server_error"
	ArtworkOther       = "other"
)

// ArtworkKinds are the kinds of failures, in the order they are told.
var ArtworkKinds = []string{ArtworkNotFound, ArtworkRateLimited, ArtworkTimeout, ArtworkServerError, ArtworkOther}

// artworkSummaryEvery is how often the summary line is logged.
const artworkSummaryEvery = time.Hour

// failureEvent is a download that failed: when, from which host, and its
// kind.
type failureEvent struct {
	at   time.Time
	host string
	kind string
}

// failureKind names the kind of a download's failure, which answers
// status.
func failureKind(status int, err error) string {
	if _, waits := stremio.RetryAfter(err); waits {
		return ArtworkRateLimited
	}
	switch code := stremio.StatusOf(err); {
	case status == http.StatusNotFound:
		return ArtworkNotFound
	case code == http.StatusTooManyRequests:
		return ArtworkRateLimited
	case code >= 500:
		return ArtworkServerError
	case errors.Is(err, context.DeadlineExceeded), netfail.TimedOut(err):
		return ArtworkTimeout
	}
	return ArtworkOther
}

// count counts a download of target that failed at now, keeping the
// failures of the last imageFailureWindow, maxImageFailureEvents at most.
func (c *imageCache) count(target string, status int, err error, now time.Time) {
	host := ""
	if parsed, parseErr := url.Parse(target); parseErr == nil {
		host = parsed.Host
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trimEvents(now)
	if len(c.events) >= maxImageFailureEvents {
		c.events = slices.Delete(c.events, 0, len(c.events)-maxImageFailureEvents+1)
	}
	c.events = append(c.events, failureEvent{at: now, host: host, kind: failureKind(status, err)})
}

// trimEvents drops the failures older than imageFailureWindow; c.mu is
// held.
func (c *imageCache) trimEvents(now time.Time) {
	old := 0
	for old < len(c.events) && now.Sub(c.events[old].at) > imageFailureWindow {
		old++
	}
	c.events = slices.Delete(c.events, 0, old)
}

// ArtworkStatus is how the artwork downloads fared in the last hour:
// Failed of them failed, counted by kind (see ArtworkKinds) and by host,
// most first; Waiting are the hosts that asked to wait, asked nothing
// until then, and Spared those that showed they were overloaded, asked
// less at once until then. No address is told, only host names.
type ArtworkStatus struct {
	Failed  int
	Kinds   map[string]int
	Hosts   []ArtworkHostFailures
	Waiting []ArtworkHostWait
	Spared  []ArtworkHostWait
}

// ArtworkHostFailures is the downloads that failed from a host.
type ArtworkHostFailures struct {
	Host   string
	Failed int
}

// ArtworkHostWait is a host waited for until a time.
type ArtworkHostWait struct {
	Host  string
	Until time.Time
}

// status tells how the downloads fared in the last imageFailureWindow.
func (c *imageCache) status() ArtworkStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.trimEvents(now)
	status := ArtworkStatus{Failed: len(c.events), Kinds: map[string]int{}, Hosts: []ArtworkHostFailures{},
		Waiting: []ArtworkHostWait{}, Spared: []ArtworkHostWait{}}
	for _, kind := range ArtworkKinds {
		status.Kinds[kind] = 0
	}
	byHost := map[string]int{}
	for _, event := range c.events {
		status.Kinds[event.kind]++
		byHost[event.host]++
	}
	for host, failed := range byHost {
		status.Hosts = append(status.Hosts, ArtworkHostFailures{Host: host, Failed: failed})
	}
	slices.SortFunc(status.Hosts, func(a, b ArtworkHostFailures) int {
		return cmp.Or(cmp.Compare(b.Failed, a.Failed), strings.Compare(a.Host, b.Host))
	})
	for host, fetches := range c.hosts {
		if now.Before(fetches.waitUntil) {
			status.Waiting = append(status.Waiting, ArtworkHostWait{Host: host, Until: fetches.waitUntil})
		}
		if now.Before(fetches.slowUntil) {
			status.Spared = append(status.Spared, ArtworkHostWait{Host: host, Until: fetches.slowUntil})
		}
	}
	for _, hosts := range [][]ArtworkHostWait{status.Waiting, status.Spared} {
		slices.SortFunc(hosts, func(a, b ArtworkHostWait) int { return strings.Compare(a.Host, b.Host) })
	}
	return status
}

// ArtworkStatus tells how the artwork downloads fared in the last hour.
func (h *Handler) ArtworkStatus() ArtworkStatus {
	h.images.open(h.imageDir(), h.Logger)
	return h.images.status()
}

// summarizeArtwork logs, every artworkSummaryEvery until ctx is done, how
// the artwork downloads fared in the last hour, when some failed.
func (h *Handler) summarizeArtwork(ctx context.Context) {
	ticker := time.NewTicker(artworkSummaryEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.logArtworkSummary()
		}
	}
}

// logArtworkSummary logs how the artwork downloads fared in the last
// hour, at INFO, when some failed: by kind, the five hosts that failed
// most, and the hosts waited for.
func (h *Handler) logArtworkSummary() {
	status := h.ArtworkStatus()
	if status.Failed == 0 {
		return
	}
	attrs := []any{"failed", status.Failed}
	for _, kind := range ArtworkKinds {
		attrs = append(attrs, kind, status.Kinds[kind])
	}
	hosts := make([]string, 0, 5)
	for _, host := range status.Hosts[:min(5, len(status.Hosts))] {
		hosts = append(hosts, fmt.Sprintf("%s (%d)", host.Host, host.Failed))
	}
	attrs = append(attrs, "hosts", strings.Join(hosts, ", "))
	if len(status.Waiting) > 0 {
		waiting := make([]string, 0, len(status.Waiting))
		for _, host := range status.Waiting {
			waiting = append(waiting, host.Host)
		}
		attrs = append(attrs, "waiting", strings.Join(waiting, ", "))
	}
	h.Logger.Info("Artwork downloads failed in the last hour", attrs...)
}

// withoutDead returns images without the artwork found dead, which apps
// are then not told of: they draw their placeholder rather than an empty
// card, as Jellyfin drops such an image from its item.
func (h *Handler) withoutDead(images library.Images) library.Images {
	h.images.open(h.imageDir(), h.Logger)
	for _, image := range []*string{&images.Primary, &images.Backdrop, &images.Logo, &images.Thumb, &images.Banner, &images.Art, &images.Disc} {
		if *image != "" && h.images.isDead(*image) {
			*image = ""
		}
	}
	return images
}

// artworkDead remembers the artwork at url of an item found dead: the item
// stops advertising it, and a person stops showing it as their photo (see
// library.Service.PhotoFailed), showing TMDB's, if any, in its place.
func (h *Handler) artworkDead(ctx context.Context, id accounts.ID, found library.Artwork, url string) {
	if !h.images.isDead(url) {
		h.images.markDead(url)
		h.Logger.Debug("Artwork is dead: its item stops showing it", "item", id.String())
	}
	if found.Photo {
		if err := h.Library.PhotoFailed(ctx, id, url); err != nil {
			h.Logger.Debug("A person's dead photo could not be kept", "person", id.String(), "error", err)
		}
	}
}

// ArtworkFailing reports whether the download of the artwork at url
// failed last, or found it dead (see library.ArtworkFailures).
func (h *Handler) ArtworkFailing(url string) bool {
	h.images.open(h.imageDir(), h.Logger)
	_, failed := h.images.failures.Get(url)
	return failed || h.images.isDead(url)
}

// ForgetArtwork forgets the failures of the artwork at urls, which are
// downloaded again at their next request, their hosts waited for no more
// (see library.ArtworkFailures).
func (h *Handler) ForgetArtwork(urls []string) {
	h.images.open(h.imageDir(), h.Logger)
	h.images.forget(urls)
}
