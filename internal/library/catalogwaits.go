package library

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Addons that ask to wait, and the catalog pages asked of them at once.
//
// An addon that answers a catalog page with 429, or 503 with Retry-After,
// asks Polyfin to slow down. Its host is then asked for no catalog page
// until the wait ends: as long as Retry-After says, else catalogWaitFirst,
// doubled at each refusal that follows a wait, catalogWaitMost at most.
// Meanwhile the pages kept are served and none is refreshed, and a read of
// a page not kept fails at once with stremio.ErrAskedToWait. Searches are
// still asked, at once, as their callers bound and pace them, and a
// refused one starts the wait. The other pages asked of a host at once are
// bounded too (catalogHostFetches), so that a listing and the refreshes of
// its stale pages do not reach it in one burst. Both are the host's: the
// addons a server serves, and users' own copies of them, share its limits.

const (
	// catalogHostFetches bounds the catalog pages asked of one host at
	// once, as addonFetches bounds music pages.
	catalogHostFetches = addonFetches
	// catalogWaitFirst is the wait after a refusal that says none, doubled
	// at each refusal that follows a wait, and catalogWaitMost bounds every
	// wait, Retry-After's included.
	catalogWaitFirst = 5 * time.Second
	catalogWaitMost  = 15 * time.Minute
	// failedRefreshWait is how long a page whose refresh failed is not
	// refreshed again: the page kept is served meanwhile.
	failedRefreshWait = time.Minute
)

// catalogWaits are the waits hosts asked for, by host.
type catalogWaits struct {
	mu    sync.Mutex
	hosts map[string]catalogWait
}

// catalogWait is the wait a host asked for: until when, and how long the
// last one was, which the next refusal that says no wait doubles; 0 once
// the host answered.
type catalogWait struct {
	until time.Time
	last  time.Duration
}

// catalogWaitLeft returns what is left of the wait host asked for, false
// when none runs.
func (s *Service) catalogWaitLeft(host string) (time.Duration, bool) {
	s.catalogWaits.mu.Lock()
	defer s.catalogWaits.mu.Unlock()
	left := s.catalogWaits.hosts[host].until.Sub(s.now())
	return left, left > 0
}

// catalogRefused notes that the addon manifestURL installs answered a
// catalog page with err. When err asks to wait, its host's wait starts,
// or a running one lasts as long as a longer Retry-After asks; started
// reports a wait that started, with how long it lasts. The refusals of
// the requests already under way when the wait started extend it only so.
func (s *Service) catalogRefused(manifestURL string, err error) (wait time.Duration, started bool) {
	asked, said := stremio.RetryAfter(err)
	if errors.Is(err, stremio.ErrAskedToWait) || !said && stremio.StatusOf(err) != http.StatusTooManyRequests {
		return 0, false
	}
	host := stremio.HostOf(manifestURL)
	now := s.now()
	s.catalogWaits.mu.Lock()
	if s.catalogWaits.hosts == nil {
		s.catalogWaits.hosts = map[string]catalogWait{}
	}
	w := s.catalogWaits.hosts[host]
	running := now.Before(w.until)
	switch {
	case said:
		wait = asked
	case running:
		wait = 0
	case w.last > 0:
		wait = 2 * w.last
	default:
		wait = catalogWaitFirst
	}
	wait = min(wait, catalogWaitMost)
	if until := now.Add(wait); until.After(w.until) {
		w.until = until
	}
	if !running {
		w.last = wait
	}
	s.catalogWaits.hosts[host] = w
	left := w.until.Sub(now)
	s.catalogWaits.mu.Unlock()
	if left <= 0 {
		return 0, false
	}
	if s.client != nil {
		s.client.CatalogsWait(manifestURL, time.Now().Add(left))
	}
	return wait, !running
}

// catalogAnswered notes that host answered a catalog page: its next
// refusal that says no wait waits catalogWaitFirst again.
func (s *Service) catalogAnswered(host string) {
	s.catalogWaits.mu.Lock()
	defer s.catalogWaits.mu.Unlock()
	if w, ok := s.catalogWaits.hosts[host]; ok && !s.now().Before(w.until) {
		delete(s.catalogWaits.hosts, host)
	}
}
