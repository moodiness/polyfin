package cache

import (
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

func TestImproveReplacesOnlyAKeptValueWithABetterOne(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	c := NewLasting[string, int](10, func() time.Duration { return 10 * time.Minute }, func() time.Time { return now })
	larger := func(value int) func(int) bool { return func(kept int) bool { return value > kept } }
	c.Put("kept", 2)
	if c.Improve("kept", 1, larger(1)) {
		t.Error("a worse value replaced the one kept")
	}
	now = now.Add(9 * time.Minute)
	if !c.Improve("kept", 3, larger(3)) {
		t.Error("a better value was refused")
	}
	// Replaced, the value is kept from then on.
	now = now.Add(9 * time.Minute)
	if v, ok := c.Get("kept"); !ok || v != 3 {
		t.Fatalf("improved: %d %v", v, ok)
	}
	// Deleted or expired, a key is not brought back.
	c.Delete("kept")
	if c.Improve("kept", 4, larger(4)) {
		t.Error("a deleted key was brought back")
	}
	c.Put("expiring", 1)
	now = now.Add(11 * time.Minute)
	if c.Improve("expiring", 5, larger(5)) {
		t.Error("an expired key was brought back")
	}
	if _, ok := c.Get("kept"); ok {
		t.Error("deleted, still kept")
	}
	if _, ok := c.Get("expiring"); ok {
		t.Error("expired, still kept")
	}
}
