// Package throttle slows down password guessing.
package throttle

import (
	"slices"
	"sync"
	"time"
)

// Failures counts failed attempts per key over a sliding window and refuses
// further attempts once the limit is reached, until the oldest failure in
// the window expires. An attempt counts from the moment it is reserved,
// before it is checked, so that attempts made at once cannot all pass while
// none of them has failed yet.
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

// Attempt is an attempt reserved by Reserve. It counts as a failure of its
// keys until it is settled: Fail keeps it counted, Succeed forgets the
// failures of its keys, and Cancel takes it back, for an attempt that was
// neither right nor wrong. Only the first of them applies, so that a
// deferred Cancel takes back only an attempt left unsettled.
type Attempt struct {
	f       *Failures
	keys    []string
	at      time.Time
	settled bool
}

// Reserve counts an attempt of every key before it is checked. When a key
// has reached its limit, it counts nothing and answers nil, with how long
// the client must wait.
func (f *Failures) Reserve(keys ...string) (*Attempt, time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	refused := false
	var wait time.Duration
	for _, key := range keys {
		if recent := f.prune(key); len(recent) >= f.limit {
			refused = true
			wait = max(wait, recent[0].Add(f.window).Sub(f.now()))
		}
	}
	if refused {
		return nil, wait
	}
	at := f.now()
	for _, key := range keys {
		f.failures[key] = append(f.failures[key], at)
	}
	// Bound memory against many distinct keys: forget keys whose failures
	// have all expired.
	if len(f.failures) > 10_000 {
		for other := range f.failures {
			f.prune(other)
		}
	}
	return &Attempt{f: f, keys: keys, at: at}, 0
}

// Fail keeps the attempt counted as a failure.
func (a *Attempt) Fail() {
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	a.settled = true
}

// Succeed forgets the failures of the attempt's keys, the attempt included.
func (a *Attempt) Succeed() {
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	if a.settled {
		return
	}
	a.settled = true
	for _, key := range a.keys {
		delete(a.f.failures, key)
	}
}

// Cancel takes the attempt back, unless it was settled already: it no
// longer counts, and the other failures of its keys still do.
func (a *Attempt) Cancel() {
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	if a.settled {
		return
	}
	a.settled = true
	for _, key := range a.keys {
		recent := a.f.failures[key]
		// Expired or forgotten meanwhile, the attempt is no longer there.
		if i := slices.IndexFunc(recent, a.at.Equal); i >= 0 {
			recent = slices.Delete(recent, i, i+1)
		}
		if len(recent) == 0 {
			delete(a.f.failures, key)
		} else {
			a.f.failures[key] = recent
		}
	}
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
