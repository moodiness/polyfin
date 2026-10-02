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
	ttl      time.Duration
	capacity int
	now      func() time.Time

	mu      sync.Mutex
	order   *list.List
	entries map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key     K
	value   V
	expires time.Time
}

// New returns a cache of at most capacity entries, each kept for ttl.
func New[K comparable, V any](capacity int, ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{ttl: ttl, capacity: capacity, now: time.Now, order: list.New(), entries: map[K]*list.Element{}}
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
	if c.now().After(e.expires) {
		c.order.Remove(element)
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	c.order.MoveToFront(element)
	return e.value, true
}

// Put keeps value for key, for the cache's time to live from now.
func (c *Cache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		e := element.Value.(*entry[K, V])
		e.value, e.expires = value, c.now().Add(c.ttl)
		c.order.MoveToFront(element)
		return
	}
	c.entries[key] = c.order.PushFront(&entry[K, V]{key: key, value: value, expires: c.now().Add(c.ttl)})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*entry[K, V]).key)
	}
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
