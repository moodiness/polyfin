// Package throttle slows down password guessing.
package throttle

import (
	"container/list"
	"net/netip"
	"slices"
	"sync"
	"time"
)

const (
	// keysKept is how many keys count failures at once, at most: what
	// bounds the memory of failures from many addresses.
	keysKept = 100_000
	// expiredAtOnce is how many keys whose failures have all expired an
	// attempt forgets at most, in each order: no attempt walks every key.
	expiredAtOnce = 8
)

// Failures counts failed attempts per key over a sliding window and refuses
// further attempts once the limit is reached, until the oldest failure in
// the window expires. An attempt counts from the moment it is reserved,
// before it is checked, so that attempts made at once cannot all pass while
// none of them has failed yet.
//
// A key that is an IPv6 address counts with the other addresses of its
// /64, which one client holds; an IPv4 address, an IPv4-mapped one and any
// other key count as they are.
type Failures struct {
	limit   int
	window  time.Duration
	now     func() time.Time
	maxKeys int

	mu       sync.Mutex
	failures map[string]*list.Element
	// The keys kept, each a *kept, the one whose last attempt is the
	// oldest first: addresses in one order, every other key in the other.
	addresses, names list.List
	// visited counts the keys looked at beyond those of the attempts, for
	// the tests of the bound.
	visited int
}

// kept is the attempts counted for one key, the oldest first, never none.
type kept struct {
	key     string
	address bool
	at      []time.Time
}

// New allows limit failures per key within window.
func New(limit int, window time.Duration) *Failures {
	return &Failures{limit: limit, window: window, now: time.Now, maxKeys: keysKept, failures: map[string]*list.Element{}}
}

// Attempt is an attempt reserved by Reserve. It counts as a failure of its
// keys until it is settled: Fail keeps it counted, and Succeed and Cancel
// take it back, Cancel for an attempt that was neither right nor wrong.
// Only the first of them applies, so that a deferred Cancel takes back only
// an attempt left unsettled.
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
	now := f.now()
	counted := make([]string, len(keys))
	address := make([]bool, len(keys))
	refused := false
	var wait time.Duration
	for i, key := range keys {
		counted[i], address[i] = countedUnder(key)
		if recent := f.recent(counted[i], now); len(recent) >= f.limit {
			refused = true
			wait = max(wait, recent[0].Add(f.window).Sub(now))
		}
	}
	if refused {
		return nil, wait
	}
	for i, key := range counted {
		f.count(key, address[i], now)
	}
	f.forgetExpired(now)
	return &Attempt{f: f, keys: counted, at: now}, 0
}

// countedUnder returns the key the attempts of key count under, and
// whether it is an address.
func countedUnder(key string) (string, bool) {
	address, err := netip.ParseAddr(key)
	if err != nil {
		return key, false
	}
	if address.Is4() || address.Is4In6() {
		return key, true
	}
	// 64 bits fit any IPv6 address: Prefix cannot fail.
	prefix, _ := address.Prefix(64)
	return prefix.String(), true
}

// Fail keeps the attempt counted as a failure.
func (a *Attempt) Fail() {
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	a.settled = true
}

// Succeed takes the attempt back, as Cancel does. The other failures of
// its keys stay counted until they expire: a client that signs in to one
// account has still failed at the others.
func (a *Attempt) Succeed() {
	a.Cancel()
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
		element := a.f.failures[key]
		// Expired or forgotten meanwhile, the attempt is no longer there.
		if element == nil {
			continue
		}
		entry := element.Value.(*kept)
		if i := slices.IndexFunc(entry.at, a.at.Equal); i >= 0 {
			entry.at = slices.Delete(entry.at, i, i+1)
		}
		if len(entry.at) == 0 {
			a.f.forget(element)
		}
	}
}

// recent returns the attempts of key inside the window, and forgets the
// others.
func (f *Failures) recent(key string, now time.Time) []time.Time {
	element := f.failures[key]
	if element == nil {
		return nil
	}
	entry := element.Value.(*kept)
	cutoff := now.Add(-f.window)
	for len(entry.at) > 0 && !entry.at[0].After(cutoff) {
		entry.at = entry.at[1:]
	}
	if len(entry.at) == 0 {
		f.forget(element)
		return nil
	}
	return entry.at
}

// count adds an attempt of key at now, and makes key the last of its
// order. A new key takes the place of another when as many as allowed are
// kept already.
func (f *Failures) count(key string, address bool, now time.Time) {
	order := &f.names
	if address {
		order = &f.addresses
	}
	element := f.failures[key]
	if element == nil {
		if len(f.failures) >= f.maxKeys {
			f.giveWay()
		}
		element = order.PushBack(&kept{key: key, address: address})
		f.failures[key] = element
	} else {
		order.MoveToBack(element)
	}
	entry := element.Value.(*kept)
	entry.at = append(entry.at, now)
}

// giveWay forgets the key whose last attempt is the oldest, the next whose
// failures would all have expired, so that a new key is counted and nobody
// is refused because many others failed. An address gives way before any
// other key: whoever has many addresses has as many counts already, where
// the count of an account or of the whole server is the only one there is.
func (f *Failures) giveWay() {
	order := &f.addresses
	if order.Len() == 0 {
		order = &f.names
	}
	if oldest := order.Front(); oldest != nil {
		f.visited++
		f.forget(oldest)
	}
}

// forgetExpired forgets keys whose failures have all expired, a few at
// each attempt, from the oldest of each order until one that still counts.
func (f *Failures) forgetExpired(now time.Time) {
	cutoff := now.Add(-f.window)
	for _, order := range [...]*list.List{&f.addresses, &f.names} {
		for range expiredAtOnce {
			oldest := order.Front()
			if oldest == nil {
				break
			}
			f.visited++
			if at := oldest.Value.(*kept).at; at[len(at)-1].After(cutoff) {
				break
			}
			f.forget(oldest)
		}
	}
}

func (f *Failures) forget(element *list.Element) {
	entry := element.Value.(*kept)
	delete(f.failures, entry.key)
	if entry.address {
		f.addresses.Remove(element)
	} else {
		f.names.Remove(element)
	}
}
