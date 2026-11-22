// Package cache keeps recent values in memory.
package cache

import (
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
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
	// sliding renews an entry's age at each read (see Sliding).
	sliding bool

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
	// byAge is the order of storage, or of the last read of a sliding
	// cache. Entries all age alike, whatever the lifetime, so the expired
	// ones are its oldest.
	byAge
)

type entry[K comparable, V any] struct {
	key   K
	value V
	// stored is when the value was stored, or last read in a sliding
	// cache: its age counts from then.
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
// maxBytes, the least recently used entries are evicted. maxBytes is the
// budget of a server with memory to spare: Budget scales it to the share
// SetBudgetShare set. A value larger than that on its own is not kept.
// size is called once for each value stored, which keeps its size;
// JSONSize suits values decoded from JSON that Polyfin or a service it
// trusts wrote, MemorySize those decoded from what any other host sends.
// Sized is called on a new cache, before it is used.
func (c *Cache[K, V]) Sized(maxBytes int, size func(V) int) *Cache[K, V] {
	c.size, c.maxBytes = size, Budget(maxBytes)
	return c
}

// Sliding has each read of an entry keep it as long again as when it was
// stored, and returns c: an entry expires once unread for its lifetime.
// Sliding is called on a new cache, before it is used.
func (c *Cache[K, V]) Sliding() *Cache[K, V] {
	c.sliding = true
	return c
}

// budgetShare holds, as math.Float64bits, the share of their budgets the
// memory caches take; 0 stands for 1, all of it.
var budgetShare atomic.Uint64

// SetBudgetShare has the memory caches made afterwards take share of
// their budgets, above 0 and at most 1 (see Budget). Polyfin sets it once
// at start, from its memory limit, before it makes its caches.
func SetBudgetShare(share float64) {
	budgetShare.Store(math.Float64bits(min(max(share, 0), 1)))
}

// Budget is the bytes a memory cache whose budget is maxBytes keeps, at
// the share SetBudgetShare set (see scaled).
func Budget(maxBytes int) int {
	share := 1.0
	if bits := budgetShare.Load(); bits != 0 {
		share = math.Float64frombits(bits)
	}
	return scaled(maxBytes, share)
}

// scaled is share of maxBytes, a byte at least for a budget of any.
func scaled(maxBytes int, share float64) int {
	if maxBytes <= 0 || share >= 1 {
		return maxBytes
	}
	return max(int(float64(maxBytes)*share), 1)
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

// MemorySize estimates the memory value holds, a size for Sized that
// counts a value by what it takes rather than by what it is written in:
// its own size, the bytes of its strings, the elements of its slices,
// used or spare, the keys and values of its maps, and what its pointers
// lead to, each time one is met. An element that holds nothing counts
// for its size, where its JSON is a few bytes: this suits values decoded
// from what another host sends. A value is followed maxDepth deep.
func MemorySize[V any](value V) int {
	v := reflect.ValueOf(&value).Elem()
	return int(v.Type().Size()) + beyond(v, 0)
}

// maxDepth is how deep MemorySize follows a value: past any value the
// caches keep, and what ends the reading of one that leads to itself.
const maxDepth = 64

// beyond is the memory v leads to beyond its own size, depth deep in the
// value measured.
func beyond(v reflect.Value, depth int) int {
	if depth > maxDepth {
		return 0
	}
	n := 0
	switch v.Kind() {
	case reflect.String:
		return v.Len()
	case reflect.Slice:
		element := v.Type().Elem()
		n = v.Cap() * int(element.Size())
		if element.Kind() <= reflect.Complex128 {
			// Numbers lead nowhere.
			return n
		}
		for i := range v.Len() {
			n += beyond(v.Index(i), depth+1)
		}
	case reflect.Array:
		for i := range v.Len() {
			n += beyond(v.Index(i), depth+1)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			n += beyond(v.Field(i), depth+1)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			target := v.Elem()
			n = int(target.Type().Size()) + beyond(target, depth+1)
		}
	case reflect.Map:
		for entry := v.MapRange(); entry.Next(); {
			key, value := entry.Key(), entry.Value()
			n += int(key.Type().Size()+value.Type().Size()) + beyond(key, depth+1) + beyond(value, depth+1)
		}
	}
	return n
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

// Get returns the value of key, if it is still kept, and renews its age
// in a sliding cache (see Sliding).
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	now := c.now()
	if now.Sub(e.stored) > c.lifetime() {
		c.remove(e)
		var zero V
		return zero, false
	}
	e.unlink(byUse)
	c.link(e, byUse)
	if c.sliding {
		e.stored = now
		e.unlink(byAge)
		c.link(e, byAge)
	}
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

// DropExpired drops the entries expired by now, which otherwise go only as
// others are stored or as they are read: a cache that holds much and may
// stay unused calls it from time to time.
func (c *Cache[K, V]) DropExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpired(c.now())
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

// DeleteFunc forgets the entries drop reports, given their key and value.
// drop must not use the cache.
func (c *Cache[K, V]) DeleteFunc(drop func(key K, value V) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, e := range c.entries {
		if drop(key, e.value) {
			c.remove(e)
		}
	}
}

// ChangeFunc replaces in place the values change reports, given their key
// and value, with what it returns for them. The entries keep when they
// were stored, so they expire as they would have, and their place among
// the recently used. A changed value is not measured again: it counts for
// the size of the one it replaces, so change is for changes that keep the
// size, as a value's date. change must not use the cache.
func (c *Cache[K, V]) ChangeFunc(change func(key K, value V) (V, bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, e := range c.entries {
		if value, ok := change(key, e.value); ok {
			e.value = value
		}
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
