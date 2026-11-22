package collections

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// title returns a distinct title identifier.
func title(n byte) accounts.ID { return accounts.ID{15: n} }

func get(t *testing.T, store *Store, id accounts.ID) Collection {
	t.Helper()
	c, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func create(t *testing.T, store *Store, name string, items ...accounts.ID) accounts.ID {
	t.Helper()
	id, err := store.Create(t.Context(), name, false, items)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCollectionsHoldCollectionsInTheOrderAdded(t *testing.T) {
	t.Parallel()
	store := New(migrated.New(t))
	ctx := t.Context()
	avengers := create(t, store, "Avengers", title(1), title(2))
	ironMan := create(t, store, "Iron Man", title(3))
	marvel := create(t, store, "Marvel", title(4), ironMan)
	if err := store.Add(ctx, marvel, []accounts.ID{avengers, title(5), ironMan}); err != nil {
		t.Fatal(err)
	}
	c := get(t, store, marvel)
	if !slices.Equal(c.Entries, []accounts.ID{title(4), ironMan, avengers, title(5)}) ||
		!slices.Equal(c.Items, []accounts.ID{title(4), title(5)}) || !slices.Equal(c.Collections, []accounts.ID{ironMan, avengers}) {
		t.Errorf("entries %v, titles %v, collections %v", c.Entries, c.Items, c.Collections)
	}
	some, err := store.Some(ctx, []accounts.ID{marvel, title(1), avengers})
	if err != nil || len(some) != 2 || some[0].ID != avengers || some[1].ID != marvel {
		t.Errorf("some: %v %v", some, err)
	}

	if err := store.Remove(ctx, marvel, []accounts.ID{ironMan}); err != nil {
		t.Fatal(err)
	}
	if c := get(t, store, marvel); !slices.Equal(c.Collections, []accounts.ID{avengers}) || len(c.Items) != 2 {
		t.Errorf("after removing one: %v %v", c.Collections, c.Items)
	}
	// Deleting a collection takes it out of those that held it.
	if err := store.Delete(ctx, avengers); err != nil {
		t.Fatal(err)
	}
	if c := get(t, store, marvel); len(c.Collections) != 0 || !slices.Equal(c.Entries, []accounts.ID{title(4), title(5)}) {
		t.Errorf("after deleting the one held: %v", c.Entries)
	}
	if err := store.Delete(ctx, avengers); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting it again: %v", err)
	}
}

func TestCollectionsRefuseCyclesAndDepth(t *testing.T) {
	t.Parallel()
	store := New(migrated.New(t))
	ctx := t.Context()
	levels := make([]accounts.ID, MaxDepth)
	for i := range levels {
		levels[i] = create(t, store, "Level", title(byte(10+i)))
	}
	for i := range MaxDepth - 1 {
		if err := store.Add(ctx, levels[i], []accounts.ID{levels[i+1]}); err != nil {
			t.Fatalf("nesting level %d: %v", i+2, err)
		}
	}
	top, bottom := levels[0], levels[MaxDepth-1]
	for _, tc := range []struct {
		name       string
		into, item accounts.ID
	}{
		{"itself", top, top},
		{"one that holds it", bottom, top},
		{"one that holds it directly", levels[1], top},
	} {
		if err := store.Add(ctx, tc.into, []accounts.ID{title(99), tc.item}); !errors.Is(err, ErrCycle) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	more := create(t, store, "More")
	if err := store.Add(ctx, bottom, []accounts.ID{more}); !errors.Is(err, ErrTooDeep) {
		t.Errorf("one level too deep: %v", err)
	}
	if _, err := store.Create(ctx, "Above", false, []accounts.ID{top}); !errors.Is(err, ErrTooDeep) {
		t.Errorf("a collection above the deepest: %v", err)
	}
	// A refused change adds nothing, the titles named with it included.
	for _, id := range append(levels, more) {
		if c := get(t, store, id); slices.Contains(c.Items, title(99)) || slices.Contains(c.Entries, more) {
			t.Errorf("a refused change added to %v: %v", id, c.Entries)
		}
	}
	all, err := store.All(ctx)
	if err != nil || len(all) != MaxDepth+1 {
		t.Errorf("a refused creation created: %d %v", len(all), err)
	}
	// Beside the deepest branch, a shallower one may take more.
	if err := store.Add(ctx, levels[MaxDepth-2], []accounts.ID{more}); err != nil {
		t.Errorf("a level that fits: %v", err)
	}
}

// Two changes that would make a cycle together, each alone allowed, never
// both succeed.
func TestCollectionsRefuseCyclesMadeAtOnce(t *testing.T) {
	t.Parallel()
	store := New(migrated.New(t))
	ctx := t.Context()
	for range 10 {
		a := create(t, store, "A")
		b := create(t, store, "B")
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Go(func() { errs[0] = store.Add(ctx, a, []accounts.ID{b}) })
		wg.Go(func() { errs[1] = store.Add(ctx, b, []accounts.ID{a}) })
		wg.Wait()
		if errs[0] == nil && errs[1] == nil {
			t.Fatal("both nested in each other")
		}
		for _, err := range errs {
			if err != nil && !errors.Is(err, ErrCycle) {
				t.Fatalf("refused with %v", err)
			}
		}
	}
}
