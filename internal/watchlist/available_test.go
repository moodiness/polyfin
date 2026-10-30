package watchlist

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/userdata"
)

// fakeFinds answers each title's check with what the test sets, counting
// the checks.
type fakeFinds struct {
	mu      sync.Mutex
	found   map[accounts.ID]Found
	checked map[accounts.ID]int
	told    []accounts.ID
}

func (f *fakeFinds) find(_ context.Context, _ accounts.User, title accounts.ID) (Found, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checked[title]++
	return f.found[title], nil
}

func (f *fakeFinds) set(title accounts.ID, found Found) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.found[title] = found
}

func (f *fakeFinds) tell(_ context.Context, _ accounts.User, title accounts.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.told = append(f.told, title)
}

// A title is told of the first time versions are found for it after a
// check found none, never when it had some at its first check, and never
// again; a check that could not ask every addon changes nothing and is
// tried again at the next run; a title is checked again only after the
// hours set, never while they are 0, and at most PerPass titles of a user
// at a run, never one the user started.
func TestTheChecksTellOnceOfTheTitlesThatGainVersions(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	var titles []Title
	for n := range byte(PerPass + 3) {
		titles = append(titles, dated(n+1, 1))
	}
	h.save(t, Import, false, titles...)
	if _, err := h.data.Change(ctx, h.user, []userdata.Item{{ID: item(PerPass + 3)}}, func(d *userdata.Data) { d.Played = true }); err != nil {
		t.Fatal(err)
	}
	finds := &fakeFinds{found: map[accounts.ID]Found{}, checked: map[accounts.ID]int{}}
	for n := range byte(PerPass + 3) {
		finds.set(item(n+1), Missing)
	}
	finds.set(item(1), Available)
	finds.set(item(3), Unknown)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	hours := 6
	checker := NewChecker(CheckerOptions{Store: h.Store, Hours: func() int { return hours }, Find: finds.find, Tell: finds.tell,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now },
		Users: func(context.Context) ([]accounts.User, error) {
			return []accounts.User{{ID: h.user, Name: "alice"}}, nil
		}})
	run := func() {
		t.Helper()
		if err := checker.Check(ctx); err != nil {
			t.Fatal(err)
		}
	}
	list := func(of func(context.Context, accounts.ID) ([]accounts.ID, error)) []accounts.ID {
		t.Helper()
		ids, err := of(ctx, h.user)
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}

	run()
	if checkedTitles := len(finds.checked); checkedTitles != PerPass {
		t.Errorf("%d titles checked at the first run, want %d", checkedTitles, PerPass)
	}
	if !slices.Equal(list(h.Available), []accounts.ID{item(1)}) || len(finds.told) != 0 {
		t.Errorf("after the first run: available %v, told %v", list(h.Available), finds.told)
	}
	// The title the check could not settle is in neither row.
	if waiting := list(h.Waiting); slices.Contains(waiting, item(3)) || len(waiting) != PerPass-2 {
		t.Errorf("waiting after the first run: %v", waiting)
	}

	// Within the hours set, only the titles not checked yet and the one
	// unsettled are; the title started never is.
	finds.set(item(2), Available)
	now = now.Add(time.Hour)
	run()
	if finds.checked[item(3)] != 2 || finds.checked[item(2)] != 1 || finds.checked[item(PerPass+1)] != 1 || finds.checked[item(PerPass+3)] != 0 {
		t.Errorf("checks within the hours set: %v", finds.checked)
	}
	now = now.Add(6 * time.Hour)
	run()
	if !slices.Equal(finds.told, []accounts.ID{item(2)}) {
		t.Errorf("told %v, want the title that gained versions", finds.told)
	}

	// Lost, then found again: back to waiting, and not told again.
	finds.set(item(2), Missing)
	now = now.Add(7 * time.Hour)
	run()
	if slices.Contains(list(h.Available), item(2)) || !slices.Contains(list(h.Waiting), item(2)) {
		t.Errorf("a title that lost its versions: available %v", list(h.Available))
	}
	finds.set(item(2), Available)
	now = now.Add(7 * time.Hour)
	run()
	if !slices.Contains(list(h.Available), item(2)) || len(finds.told) != 1 {
		t.Errorf("a title that found versions again: available %v, told %v", list(h.Available), finds.told)
	}

	// 0 hours checks nothing.
	before := finds.checked[item(1)]
	hours = 0
	now = now.Add(24 * time.Hour)
	run()
	if finds.checked[item(1)] != before {
		t.Error("checked while the checks are off")
	}
}
