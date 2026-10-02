package library

import (
	"container/list"
	"sync"
	"time"
)

// cache keeps recent values for a while, evicting the least recently used
// entries beyond its capacity.
type cache[K comparable, V any] struct {
	ttl      time.Duration
	capacity int
	now      func() time.Time

	mu      sync.Mutex
	order   *list.List
	entries map[K]*list.Element
}

type cacheEntry[K comparable, V any] struct {
	key     K
	value   V
	expires time.Time
}

func newCache[K comparable, V any](capacity int, ttl time.Duration) *cache[K, V] {
	return &cache[K, V]{ttl: ttl, capacity: capacity, now: time.Now, order: list.New(), entries: map[K]*list.Element{}}
}

func (c *cache[K, V]) get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	entry := element.Value.(*cacheEntry[K, V])
	if c.now().After(entry.expires) {
		c.order.Remove(element)
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	c.order.MoveToFront(element)
	return entry.value, true
}

func (c *cache[K, V]) put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*cacheEntry[K, V])
		entry.value, entry.expires = value, c.now().Add(c.ttl)
		c.order.MoveToFront(element)
		return
	}
	c.entries[key] = c.order.PushFront(&cacheEntry[K, V]{key: key, value: value, expires: c.now().Add(c.ttl)})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry[K, V]).key)
	}
}
