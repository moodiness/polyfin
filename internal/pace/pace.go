// Package pace spaces the requests the server sends to a service whose
// rate limit counts the server's address, however many parts of the
// server ask it.
package pace

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrLate reports a turn that would come after the deadline of the
// context waiting for it.
var ErrLate = errors.New("no turn before the deadline")

// Pace gives turns at least gap apart, and none while a service's request
// to wait lasts. The zero gap spaces nothing but those waits.
type Pace struct {
	gap time.Duration

	mu sync.Mutex
	// next is the earliest next turn, gap after the last one; notBefore
	// holds every turn back after the service asked to wait.
	next, notBefore time.Time
}

// New returns a pace giving turns gap apart.
func New(gap time.Duration) *Pace {
	return &Pace{gap: gap}
}

// Wait waits for a turn. It returns ctx's error if ctx ends first, and
// ErrLate at once when the turn would come after ctx's deadline.
func (p *Pace) Wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.mu.Lock()
		now := time.Now()
		turn := latest(now, latest(p.next, p.notBefore))
		if deadline, ok := ctx.Deadline(); ok && turn.After(deadline) {
			p.mu.Unlock()
			return ErrLate
		}
		if !turn.After(now) {
			p.next = now.Add(p.gap)
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		// Another waiter may take the turn meanwhile: it is asked for
		// again then.
		timer := time.NewTimer(turn.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Hold gives no turn for wait, unless an earlier request to wait lasts
// longer.
func (p *Pace) Hold(wait time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notBefore = latest(p.notBefore, time.Now().Add(wait))
}

// latest is the later of a and b.
func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
