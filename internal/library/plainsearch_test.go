package library

import (
	"fmt"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// A user's searches in plain words are kept for a day after they last
// made them, their last plainSearchesKept only; one made again keeps when
// it was first made unless its rules changed.
func TestPlainSearchesAreKeptForADayTheLastOnes(t *testing.T) {
	t.Parallel()
	var p plainSearches
	user, other := accounts.ID{1}, accounts.ID{2}
	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	search := func(name string, minYear float64, at time.Time) smartlists.List {
		return smartlists.List{ID: itemID("plainsearch|" + name), Name: name, UpdatedAt: at,
			Rules: smartlists.Rule{Field: smartlists.FieldGroup, Match: smartlists.MatchAll,
				Rules: []smartlists.Rule{{Field: smartlists.FieldYear, Min: new(minYear)}}}}
	}
	p.remember(other, search("other", 1980, start), start)
	for i := range plainSearchesKept + 1 {
		p.remember(user, search(fmt.Sprint(i), 1980, start), start)
	}
	lists := p.of(user, start)
	if len(lists) != plainSearchesKept || lists[0].Name != "1" || lists[len(lists)-1].Name != fmt.Sprint(plainSearchesKept) {
		t.Fatalf("kept: %d, from %q", len(lists), lists[0].Name)
	}
	if _, ok := p.find(user, SmartListItemID(search("0", 1980, start).ID), start); ok {
		t.Error("the oldest search is still kept")
	}

	later := start.Add(time.Hour)
	if kept := p.remember(user, search("1", 1980, later), later); !kept.UpdatedAt.Equal(start) {
		t.Errorf("made again: %v", kept.UpdatedAt)
	}
	if kept := p.remember(user, search("2", 1990, later), later); !kept.UpdatedAt.Equal(later) {
		t.Errorf("made again with other rules: %v", kept.UpdatedAt)
	}

	dayAfter := start.Add(plainSearchLife + time.Minute)
	if lists := p.of(user, dayAfter); len(lists) != 2 || lists[0].Name != "1" || lists[1].Name != "2" {
		t.Errorf("a day after: %v", lists)
	}
	if lists := p.of(other, dayAfter); len(lists) != 0 {
		t.Errorf("another user's search a day after: %v", lists)
	}
}
