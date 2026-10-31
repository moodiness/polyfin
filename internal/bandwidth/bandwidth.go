// Package bandwidth measures the bytes Polyfin moves for each playback:
// those it sends to apps, and those it receives from sources. A Meter
// counts with atomic additions, with no lock and no allocation per write;
// Meters samples every meter it holds once a second, and tells a meter's
// rate as the moving average of its last Window.
package bandwidth

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// Window is how long a rate is averaged over.
	Window = 10 * time.Second
	// Every is how often Run samples the meters.
	Every = time.Second
	// idleFor is how long a meter nothing holds is kept without a byte:
	// its total stays known for a playback paused that long.
	idleFor = time.Hour
	// kept is how many samples a meter keeps: those of the window, and the
	// one before, which starts it.
	kept = int(Window/Every) + 1
)

// Meter counts the bytes of one flow: a playback's, or a source's.
type Meter struct {
	total atomic.Int64
	// sum is the total of the meters it belongs to, nil for that total.
	sum *Meter
	// redirected tells that the last answer to an app was a redirection
	// to the source, which the app then reads itself: none of its bytes
	// pass through Polyfin.
	redirected atomic.Bool

	// The rest is guarded by the lock of the Meters it belongs to. refs
	// counts its holders; samples are its totals, the oldest at next once
	// count reaches kept; changed is when its total last changed.
	refs    int
	samples [kept]sample
	next    int
	count   int
	changed time.Time
}

type sample struct {
	at    time.Time
	total int64
}

// Add counts n bytes.
func (m *Meter) Add(n int) {
	m.total.Add(int64(n))
	if m.sum != nil {
		m.sum.total.Add(int64(n))
	}
}

// Total is the bytes counted since the meter was made.
func (m *Meter) Total() int64 {
	return m.total.Load()
}

// Redirect notes how the last answer to an app went: redirected to the
// source, or with the bytes themselves.
func (m *Meter) Redirect(redirected bool) {
	m.redirected.Store(redirected)
}

// take records the meter's total at now.
func (m *Meter) take(now time.Time) {
	total := m.total.Load()
	if m.count == 0 || m.samples[(m.next+kept-1)%kept].total != total {
		m.changed = now
	}
	m.samples[m.next] = sample{at: now, total: total}
	m.next = (m.next + 1) % kept
	m.count = min(m.count+1, kept)
}

// rate is the bytes per second between the meter's oldest sample within
// Window of its newest, half a tick of jitter allowed, and its newest: 0
// before two samples.
func (m *Meter) rate() float64 {
	newest := m.samples[(m.next+kept-1)%kept]
	for i := m.count; i >= 2; i-- {
		oldest := m.samples[(m.next+kept-i)%kept]
		elapsed := newest.at.Sub(oldest.at)
		switch {
		case elapsed > Window+Every/2:
			continue
		case elapsed <= 0:
			return 0
		}
		return float64(newest.total-oldest.total) / elapsed.Seconds()
	}
	return 0
}

// Meters are meters by key, and their total: each byte a meter counts,
// the total counts too.
type Meters struct {
	now func() time.Time

	mu     sync.Mutex
	meters map[string]*Meter
	total  Meter
}

// New returns meters, sampled by Run.
func New() *Meters {
	return &Meters{now: time.Now, meters: map[string]*Meter{}}
}

// Hold returns the meter of key, made if there is none, kept until the
// caller calls Release: what the caller counts goes to the meter whose
// rate and total key tells.
func (ms *Meters) Hold(key string) *Meter {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.meters[key]
	if m == nil {
		m = &Meter{sum: &ms.total, changed: ms.now()}
		ms.meters[key] = m
	}
	m.refs++
	return m
}

// Release ends a hold of m (see Hold).
func (ms *Meters) Release(m *Meter) {
	ms.mu.Lock()
	m.refs--
	ms.mu.Unlock()
}

// Rate is the bytes per second the meters of keys count together, averaged
// over Window; 0 for a key no meter has.
func (ms *Meters) Rate(keys ...string) float64 {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	rate := 0.0
	for i, key := range keys {
		if m := ms.meters[key]; m != nil && !repeated(keys[:i], key) {
			rate += m.rate()
		}
	}
	return rate
}

// Total is the bytes the meters of keys counted together.
func (ms *Meters) Total(keys ...string) int64 {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var total int64
	for i, key := range keys {
		if m := ms.meters[key]; m != nil && !repeated(keys[:i], key) {
			total += m.Total()
		}
	}
	return total
}

// Redirected reports whether the last answer of the meter of key was a
// redirection to the source (see Meter.Redirect).
func (ms *Meters) Redirected(key string) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m := ms.meters[key]
	return m != nil && m.redirected.Load()
}

// repeated reports whether key is among keys, or empty: a key counts once.
func repeated(keys []string, key string) bool {
	if key == "" {
		return true
	}
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// TotalRate is the bytes per second every meter counts together, averaged
// over Window.
func (ms *Meters) TotalRate() float64 {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.total.rate()
}

// Sample records each meter's total now, and forgets the meters nothing
// holds that counted nothing for idleFor.
func (ms *Meters) Sample() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	now := ms.now()
	ms.total.take(now)
	for key, m := range ms.meters {
		m.take(now)
		if m.refs == 0 && now.Sub(m.changed) >= idleFor {
			delete(ms.meters, key)
		}
	}
}

// Run samples the meters every Every until ctx ends.
func (ms *Meters) Run(ctx context.Context) {
	ticker := time.NewTicker(Every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ms.Sample()
		}
	}
}
