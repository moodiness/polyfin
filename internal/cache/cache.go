// Package cache keeps recent values in memory.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// Cache keeps recent values for a while, evicting the least recently used
// entries beyond its capacity. It is safe for concurrent use.
type Cache[K comparable, V any] struct {
	lifetime func() time.Duration
	capacity int
	now      func() time.Time

	mu      sync.Mutex
	order   *list.List
	entries map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key    K
	value  V
	stored time.Time
}

// New returns a cache of at most capacity entries, each kept for ttl.
func New[K comparable, V any](capacity int, ttl time.Duration) *Cache[K, V] {
	return NewLasting[K, V](capacity, func() time.Duration { return ttl }, time.Now)
}

// NewLasting returns a cache of at most capacity entries, each kept while
// its age on the clock now is at most what lifetime returns. lifetime is
// called at every read, so a change applies at once to the entries already
// kept.
func NewLasting[K comparable, V any](capacity int, lifetime func() time.Duration, now func() time.Time) *Cache[K, V] {
	return &Cache[K, V]{lifetime: lifetime, capacity: capacity, now: now, order: list.New(), entries: map[K]*list.Element{}}
}

// Get returns the value of key, if it is still kept.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	e := element.Value.(*entry[K, V])
	if c.now().Sub(e.stored) > c.lifetime() {
		c.order.Remove(element)
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	c.order.MoveToFront(element)
	return e.value, true
}

// Put keeps value for key, from now on.
func (c *Cache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.put(key, value)
}

// put is Put, with c.mu held.
func (c *Cache[K, V]) put(key K, value V) {
	if element, ok := c.entries[key]; ok {
		e := element.Value.(*entry[K, V])
		e.value, e.stored = value, c.now()
		c.order.MoveToFront(element)
		return
	}
	c.entries[key] = c.order.PushFront(&entry[K, V]{key: key, value: value, stored: c.now()})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*entry[K, V]).key)
	}
}

// Update keeps for key, from now on, what change makes of the value kept,
// if any (ok is false when none is, the key being deleted or expired), and
// tells whether it did: change reports whether to keep the value it
// returns. Unlike a Get followed by a Put, nothing changes the key in
// between, so change can refuse to bring back a key deleted or expired
// meanwhile. change must not use the cache.
func (c *Cache[K, V]) Update(key K, change func(kept V, ok bool) (V, bool)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	var kept V
	element, ok := c.entries[key]
	if ok {
		if e := element.Value.(*entry[K, V]); c.now().Sub(e.stored) > c.lifetime() {
			c.order.Remove(element)
			delete(c.entries, key)
			ok = false
		} else {
			kept = e.value
		}
	}
	value, keep := change(kept, ok)
	if keep {
		c.put(key, value)
	}
	return keep
}

// Delete forgets key.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.order.Remove(element)
		delete(c.entries, key)
	}
}
