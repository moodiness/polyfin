// Package throttle slows down password guessing.
package throttle

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Failures counts failed attempts per key over a sliding window and refuses
// further attempts once the limit is reached, until the oldest failure in
// the window expires.
type Failures struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time
}

// New allows limit failures per key within window.
func New(limit int, window time.Duration) *Failures {
	return &Failures{limit: limit, window: window, now: time.Now, failures: map[string][]time.Time{}}
}

// Allowed reports whether key may try again, and if not, how long it must
// wait.
func (f *Failures) Allowed(key string) (bool, time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	recent := f.prune(key)
	if len(recent) < f.limit {
		return true, 0
	}
	return false, recent[0].Add(f.window).Sub(f.now())
}

// Fail records a failed attempt.
func (f *Failures) Fail(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[key] = append(f.prune(key), f.now())
	// Bound memory against many distinct keys: forget keys whose failures
	// have all expired.
	if len(f.failures) > 10_000 {
		for other := range f.failures {
			f.prune(other)
		}
	}
}

// Succeed forgets the failures of key.
func (f *Failures) Succeed(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failures, key)
}

func (f *Failures) prune(key string) []time.Time {
	cutoff := f.now().Add(-f.window)
	recent := f.failures[key]
	for len(recent) > 0 && !recent[0].After(cutoff) {
		recent = recent[1:]
	}
	if len(recent) == 0 {
		delete(f.failures, key)
		return nil
	}
	f.failures[key] = recent
	return recent
}

// ClientKey identifies the client of a request by the address of the
// connection. Forwarded headers are ignored: they are set by the client
// unless a trusted proxy rewrites them, so behind a reverse proxy every
// client shares the proxy's budget.
func ClientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
