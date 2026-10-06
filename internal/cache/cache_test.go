package cache

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestALifetimeChangeAppliesToKeptEntries(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	lifetime := 10 * time.Minute
	c := NewLasting[string, int](10, func() time.Duration { return lifetime }, func() time.Time { return now })
	c.Put("kept", 1)

	now = now.Add(10 * time.Minute)
	if v, ok := c.Get("kept"); !ok || v != 1 {
		t.Fatalf("at its lifetime: %d %v", v, ok)
	}
	// A longer lifetime keeps the entry longer.
	lifetime = time.Hour
	now = now.Add(30 * time.Minute)
	if _, ok := c.Get("kept"); !ok {
		t.Fatal("lengthened lifetime: dropped")
	}
	// A shorter one drops it at once.
	lifetime = 5 * time.Minute
	if _, ok := c.Get("kept"); ok {
		t.Fatal("shortened lifetime: still kept")
	}
	// A value put again is kept from then on.
	c.Put("kept", 2)
	now = now.Add(5 * time.Minute)
	if v, ok := c.Get("kept"); !ok || v != 2 {
		t.Fatalf("put again: %d %v", v, ok)
	}
	now = now.Add(time.Second)
	if _, ok := c.Get("kept"); ok {
		t.Fatal("past its lifetime: still kept")
	}
}

func TestNewKeepsEntriesForItsTTL(t *testing.T) {
	c := New[string, int](1, time.Hour)
	c.Put("first", 1)
	c.Put("second", 2)
	if _, ok := c.Get("first"); ok {
		t.Error("beyond the capacity, the oldest entry was kept")
	}
	if v, ok := c.Get("second"); !ok || v != 2 {
		t.Errorf("second: %d %v", v, ok)
	}
	c.now = func() time.Time { return time.Now().Add(time.Hour + time.Second) }
	if _, ok := c.Get("second"); ok {
		t.Error("kept past its time to live")
	}
}

func TestUpdateReplacesWhatIsKeptAsChangeDecides(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	c := NewLasting[string, int](10, func() time.Duration { return 10 * time.Minute }, func() time.Time { return now })
	// larger keeps value over a smaller kept one, and only over a kept one.
	larger := func(value int) func(int, bool) (int, bool) {
		return func(kept int, ok bool) (int, bool) { return value, ok && value > kept }
	}
	c.Put("kept", 2)
	if c.Update("kept", larger(1)) {
		t.Error("a refused value replaced the one kept")
	}
	now = now.Add(9 * time.Minute)
	if !c.Update("kept", larger(3)) {
		t.Error("an accepted value was refused")
	}
	// Replaced, the value is kept from then on.
	now = now.Add(9 * time.Minute)
	if v, ok := c.Get("kept"); !ok || v != 3 {
		t.Fatalf("updated: %d %v", v, ok)
	}
	// Deleted or expired, a key is not kept: change is told, and need not
	// bring it back.
	c.Delete("kept")
	if c.Update("kept", larger(4)) {
		t.Error("a deleted key was brought back")
	}
	c.Put("expiring", 1)
	now = now.Add(11 * time.Minute)
	if c.Update("expiring", larger(5)) {
		t.Error("an expired key was brought back")
	}
	if _, ok := c.Get("kept"); ok {
		t.Error("deleted, still kept")
	}
	if _, ok := c.Get("expiring"); ok {
		t.Error("expired, still kept")
	}
	// A change may store a value where none is kept.
	if !c.Update("new", func(kept int, ok bool) (int, bool) { return kept + 6, !ok }) {
		t.Error("a value for a key not kept was refused")
	}
	if v, ok := c.Get("new"); !ok || v != 6 {
		t.Errorf("stored where none was kept: %d %v", v, ok)
	}
}

// clock is a test's time, and the lifetime of its cache's entries.
type clock struct {
	now      time.Time
	lifetime time.Duration
}

// sized returns a cache of at most 100 strings, of maxBytes bytes, each
// string's size being its length, its entries lasting k's lifetime on k's
// time.
func (k *clock) sized(maxBytes int) *Cache[string, string] {
	return NewLasting[string, string](100, func() time.Duration { return k.lifetime }, func() time.Time { return k.now }).
		Sized(maxBytes, func(value string) int { return len(value) })
}

func newClock(lifetime time.Duration) *clock {
	return &clock{now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), lifetime: lifetime}
}

// keeps lists those of keys c keeps, reading them in turn.
func keeps(c *Cache[string, string], keys ...string) []string {
	var found []string
	for _, key := range keys {
		if _, ok := c.Get(key); ok {
			found = append(found, key)
		}
	}
	return found
}

func TestBeyondItsBytesACacheEvictsTheLeastRecentlyUsed(t *testing.T) {
	c := newClock(time.Hour).sized(10)
	c.Put("a", "aaaa")
	c.Put("b", "bbbb")
	c.Get("a")
	// 12 bytes: b, the least recently used, goes.
	c.Put("c", "cccc")
	if got := keeps(c, "a", "b", "c"); !slices.Equal(got, []string{"a", "c"}) || c.Bytes() != 8 {
		t.Errorf("past the bound: %v kept, %d bytes", got, c.Bytes())
	}
	// As many go as the value needs room for.
	c.Put("d", "dddddddddd")
	if got := keeps(c, "a", "c", "d"); !slices.Equal(got, []string{"d"}) || c.Bytes() != 10 {
		t.Errorf("a value of the whole bound: %v kept, %d bytes", got, c.Bytes())
	}
}

func TestASizedCacheKeepsItsCapacity(t *testing.T) {
	c := NewLasting[string, string](2, func() time.Duration { return time.Hour }, time.Now).Sized(100, func(value string) int { return len(value) })
	c.Put("a", "a")
	c.Put("b", "b")
	c.Put("c", "c")
	if got := keeps(c, "a", "b", "c"); !slices.Equal(got, []string{"b", "c"}) || c.Bytes() != 2 {
		t.Errorf("past the capacity: %v kept, %d bytes", got, c.Bytes())
	}
}

func TestAValueLargerThanTheBytesIsNotKept(t *testing.T) {
	c := newClock(time.Hour).sized(10)
	c.Put("a", "aaaa")
	c.Put("b", "bbbb")
	c.Put("large", strings.Repeat("x", 11))
	if got := keeps(c, "a", "b", "large"); !slices.Equal(got, []string{"a", "b"}) || c.Bytes() != 8 {
		t.Errorf("a value too large: %v kept, %d bytes", got, c.Bytes())
	}
	// Put for a key, a value too large forgets the value it replaces.
	c.Put("a", strings.Repeat("x", 11))
	if got := keeps(c, "a", "b"); !slices.Equal(got, []string{"b"}) || c.Bytes() != 4 {
		t.Errorf("a key given a value too large: %v kept, %d bytes", got, c.Bytes())
	}
	// So does Update, which tells it did not keep it.
	if c.Update("b", func(string, bool) (string, bool) { return strings.Repeat("x", 11), true }) {
		t.Error("Update kept a value too large")
	}
	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("updated with a value too large: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	c.Put("full", strings.Repeat("x", 10))
	if got := keeps(c, "full"); len(got) != 1 {
		t.Error("a value of the whole bound was not kept")
	}
}

func TestAReplacedValueCountsForItsNewSize(t *testing.T) {
	c := newClock(time.Hour).sized(10)
	c.Put("a", "aa")
	c.Put("b", "bbbb")
	c.Put("a", "aaaaaa")
	if c.Len() != 2 || c.Bytes() != 10 {
		t.Errorf("a value replaced by a larger one: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	// 11 bytes: b, now the least recently used, goes.
	c.Put("c", "c")
	if got := keeps(c, "a", "b", "c"); !slices.Equal(got, []string{"a", "c"}) || c.Bytes() != 7 {
		t.Errorf("past the bound: %v kept, %d bytes", got, c.Bytes())
	}
	// A smaller value frees what the larger one took.
	c.Put("a", "a")
	if c.Bytes() != 2 {
		t.Errorf("a value replaced by a smaller one: %d bytes", c.Bytes())
	}
}

func TestExpiredEntriesAreDroppedAsOthersAreStored(t *testing.T) {
	k := newClock(10 * time.Minute)
	c := k.sized(1000)
	c.Put("a", "aaaa")
	k.now = k.now.Add(time.Minute)
	c.Put("b", "bbbb")
	// Stored again, a ages from then on.
	k.now = k.now.Add(7 * time.Minute)
	c.Put("a", "aaaa")
	k.now = k.now.Add(150 * time.Second)
	c.Put("c", "cccc")
	if c.Len() != 3 || c.Bytes() != 12 {
		t.Errorf("none expired: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	// b, never read again, is past its lifetime; a, stored after it, is not.
	k.now = k.now.Add(time.Minute)
	c.Put("d", "dddd")
	if c.Len() != 3 || c.Bytes() != 12 {
		t.Errorf("one expired: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	if got := keeps(c, "a", "c", "d"); len(got) != 3 {
		t.Errorf("entries in their lifetime dropped: %v kept", got)
	}
	// Every expired entry goes at the next one stored.
	k.now = k.now.Add(time.Hour)
	c.Put("e", "e")
	if c.Len() != 1 || c.Bytes() != 1 {
		t.Errorf("all expired: %d entries, %d bytes", c.Len(), c.Bytes())
	}
}

func TestAShorterLifetimeDropsOlderEntriesAtTheNextPut(t *testing.T) {
	k := newClock(time.Hour)
	c := k.sized(1000)
	for _, key := range []string{"a", "b", "c"} {
		c.Put(key, key+key)
		k.now = k.now.Add(20 * time.Minute)
	}
	// a is 60 minutes old, b 40 and c 20.
	k.lifetime = 30 * time.Minute
	c.Put("d", "dd")
	if c.Len() != 2 || c.Bytes() != 4 {
		t.Errorf("past the shorter lifetime: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	if got := keeps(c, "c", "d"); len(got) != 2 {
		t.Errorf("entries in the shorter lifetime dropped: %v kept", got)
	}
}

func TestUpdateKeepsTheBytesCounted(t *testing.T) {
	k := newClock(10 * time.Minute)
	c := k.sized(10)
	add := func(more string) func(string, bool) (string, bool) {
		return func(value string, _ bool) (string, bool) { return value + more, true }
	}
	c.Update("a", add("aaaa"))
	c.Update("a", add("aaaa"))
	c.Update("a", func(string, bool) (string, bool) { return "", false })
	if c.Len() != 1 || c.Bytes() != 8 {
		t.Errorf("updated twice, then refused: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	// 11 bytes: a, the least recently used, goes.
	c.Put("b", "bb")
	if !c.Update("b", add("b")) {
		t.Error("b grown was refused")
	}
	if got := keeps(c, "a", "b"); !slices.Equal(got, []string{"b"}) || c.Bytes() != 3 {
		t.Errorf("updated past the bound: %v kept, %d bytes", got, c.Bytes())
	}
	// Expired, what was kept counts no more.
	k.now = k.now.Add(11 * time.Minute)
	if !c.Update("b", func(_ string, ok bool) (string, bool) { return "c", !ok }) {
		t.Error("an expired key's new value was refused")
	}
	if c.Len() != 1 || c.Bytes() != 1 {
		t.Errorf("updated once expired: %d entries, %d bytes", c.Len(), c.Bytes())
	}
	c.Delete("b")
	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("deleted: %d entries, %d bytes", c.Len(), c.Bytes())
	}
}

func TestJSONSizeIsTheLengthOfTheEncoding(t *testing.T) {
	value := struct {
		Name   string
		Tags   []string
		hidden string
	}{"<b>Name</b>", []string{"one", "two"}, "not encoded"}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if got := JSONSize(value); got != len(encoded) {
		t.Errorf("JSONSize %d, encoded in %d bytes", got, len(encoded))
	}
	if got := JSONSize(make(chan int)); got != 0 {
		t.Errorf("a value JSON cannot encode: %d", got)
	}
}
