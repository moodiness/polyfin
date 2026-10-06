// Package cache keeps recent values in memory.
package cache

import (
	"encoding/json"
	"sync"
	"time"
)

// Cache keeps recent values for a while, evicting the least recently used
// entries beyond its capacity, and beyond its size in bytes when it has one
// (see Sized). Expired entries are dropped as others are stored, whether
// they are read again or not. It is safe for concurrent use.
type Cache[K comparable, V any] struct {
	lifetime func() time.Duration
	capacity int
	now      func() time.Time
	// size tells the size of each value stored, and maxBytes bounds their
	// sum; without size, every value counts for nothing (see Sized).
	size     func(V) int
	maxBytes int

	mu      sync.Mutex
	entries map[K]*entry[K, V]
	// root links the entries in each order (see byUse): its next entry is
	// the newest, its previous one the oldest.
	root entry[K, V]
	// bytes is the sum of the sizes of the entries.
	bytes int
}

// The orders entries are linked in, from the newest to the oldest.
const (
	// byUse is the order of use, whose oldest entry is the first evicted.
	byUse = iota
	// byAge is the order of storage. Entries all age alike, whatever the
	// lifetime, so the expired ones are its oldest.
	byAge
)

type entry[K comparable, V any] struct {
	key    K
	value  V
	stored time.Time
	// size is the value's, as the cache's size function told it when the
	// value was stored.
	size int
	// links are the entry's neighbours in each order: next is older, prev
	// newer.
	links [2]struct{ prev, next *entry[K, V] }
}

// New returns a cache of at most capacity entries, each kept for ttl.
func New[K comparable, V any](capacity int, ttl time.Duration) *Cache[K, V] {
	return NewLasting[K, V](capacity, func() time.Duration { return ttl }, time.Now)
}

// NewLasting returns a cache of at most capacity entries, each kept while
// its age on the clock now is at most what lifetime returns. lifetime is
// called at every read and every write, so a change applies at once to the
// entries already kept.
func NewLasting[K comparable, V any](capacity int, lifetime func() time.Duration, now func() time.Time) *Cache[K, V] {
	c := &Cache[K, V]{lifetime: lifetime, capacity: capacity, now: now, entries: map[K]*entry[K, V]{}}
	for order := range c.root.links {
		c.root.links[order].prev, c.root.links[order].next = &c.root, &c.root
	}
	return c
}

// Sized bounds c in bytes as well as in entries, and returns it: once the
// sizes of the values kept, as size tells them, add up to more than
// maxBytes, the least recently used entries are evicted. A value larger
// than maxBytes on its own is not kept. size is called once for each value
// stored, which keeps its size; JSONSize suits values decoded from JSON.
// Sized is called on a new cache, before it is used.
func (c *Cache[K, V]) Sized(maxBytes int, size func(V) int) *Cache[K, V] {
	c.size, c.maxBytes = size, maxBytes
	return c
}

// JSONSize is the length of the JSON encoding of value, a size for Sized
// that follows what values decoded from JSON take in memory: catalog metas
// take about 1.4 times their JSON length. Only what JSON encodes counts,
// exported fields; a value JSON cannot encode counts for nothing, leaving
// it to the cache's capacity.
func JSONSize[V any](value V) int {
	var n counter
	if json.NewEncoder(&n).Encode(value) != nil {
		return 0
	}
	// Encode ends the value with a newline.
	return int(n) - 1
}

// counter counts the bytes written to it.
type counter int

// Write counts p.
func (n *counter) Write(p []byte) (int, error) {
	*n += counter(len(p))
	return len(p), nil
}

// Len is the number of entries kept, expired ones not dropped yet included.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Bytes is the sum of the sizes of the values kept (see Sized), expired
// ones not dropped yet included; 0 for a cache without a size.
func (c *Cache[K, V]) Bytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// Get returns the value of key, if it is still kept.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	if c.now().Sub(e.stored) > c.lifetime() {
		c.remove(e)
		var zero V
		return zero, false
	}
	e.unlink(byUse)
	c.link(e, byUse)
	return e.value, true
}

// Put keeps value for key, from now on. A value larger than the cache's
// size in bytes is not kept, and the key's previous value is forgotten.
func (c *Cache[K, V]) Put(key K, value V) {
	// Sizing a large value takes a while: other requests are not kept
	// waiting meanwhile.
	size := c.sizeOf(value)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.put(key, value, size)
}

// sizeOf is the size of value, 0 for a cache without a size.
func (c *Cache[K, V]) sizeOf(value V) int {
	if c.size == nil {
		return 0
	}
	return c.size(value)
}

// put is Put, with c.mu held, value being of size; it tells whether value
// is kept. A cache without a size has maxBytes 0, as all its values have
// size 0, so the bound in bytes never applies.
func (c *Cache[K, V]) put(key K, value V, size int) bool {
	now := c.now()
	c.dropExpired(now)
	e, ok := c.entries[key]
	if size > c.maxBytes {
		if ok {
			c.remove(e)
		}
		return false
	}
	if ok {
		e.unlink(byUse)
		e.unlink(byAge)
		c.bytes -= e.size
		e.value, e.stored, e.size = value, now, size
	} else {
		e = &entry[K, V]{key: key, value: value, stored: now, size: size}
		c.entries[key] = e
	}
	c.bytes += size
	c.link(e, byUse)
	c.link(e, byAge)
	for len(c.entries) > 0 && (len(c.entries) > c.capacity || c.bytes > c.maxBytes) {
		c.remove(c.root.links[byUse].prev)
	}
	return true
}

// dropExpired drops the entries older at now than the lifetime, from the
// oldest stored to the first one still kept.
func (c *Cache[K, V]) dropExpired(now time.Time) {
	life := c.lifetime()
	for e := c.root.links[byAge].prev; e != &c.root && now.Sub(e.stored) > life; e = c.root.links[byAge].prev {
		c.remove(e)
	}
}

// Update keeps for key, from now on, what change makes of the value kept,
// if any (ok is false when none is, the key being deleted or expired), and
// tells whether it did: change reports whether to keep the value it
// returns, and a value larger than the cache's size in bytes is not kept,
// as with Put. Unlike a Get followed by a Put, nothing changes the key in
// between, so change can refuse to bring back a key deleted or expired
// meanwhile. change must not use the cache.
func (c *Cache[K, V]) Update(key K, change func(kept V, ok bool) (V, bool)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	var kept V
	e, ok := c.entries[key]
	if ok {
		if c.now().Sub(e.stored) > c.lifetime() {
			c.remove(e)
			ok = false
		} else {
			kept = e.value
		}
	}
	value, keep := change(kept, ok)
	return keep && c.put(key, value, c.sizeOf(value))
}

// Delete forgets key.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.remove(e)
	}
}

// remove forgets e, with c.mu held.
func (c *Cache[K, V]) remove(e *entry[K, V]) {
	e.unlink(byUse)
	e.unlink(byAge)
	delete(c.entries, e.key)
	c.bytes -= e.size
}

// link makes e the newest entry in order.
func (c *Cache[K, V]) link(e *entry[K, V], order int) {
	newest := c.root.links[order].next
	e.links[order].prev, e.links[order].next = &c.root, newest
	newest.links[order].prev = e
	c.root.links[order].next = e
}

// unlink takes e out of order.
func (e *entry[K, V]) unlink(order int) {
	l := &e.links[order]
	l.prev.links[order].next = l.next
	l.next.links[order].prev = l.prev
	l.prev, l.next = nil, nil
}
