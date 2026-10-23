package library

import (
	"sync"
	"time"
)

// clock is a time tests move by hand. The service reads it from its
// background work too, such as a stale page it asks for again after a read
// returned: a test moves it under a lock, never through a variable such a
// read would race with.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
