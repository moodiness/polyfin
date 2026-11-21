package library

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/moodiness/polyfin/internal/accounts"
)

// choicesReader is a service whose version choices are read by read,
// counting the reads.
func choicesReader(read func() (choiceSet, error)) (*Service, *atomic.Int32) {
	s := &Service{logger: slog.New(slog.DiscardHandler), lifetime: context.Background()}
	reads := &atomic.Int32{}
	s.choices.read = func(context.Context) (choiceSet, error) {
		reads.Add(1)
		return read()
	}
	return s, reads
}

// After a restart the rows of a home screen describe their items at once,
// each asking for the pinned and blocked versions: they wait for one read
// rather than each reading them.
func TestListingsAskingTogetherReadTheVersionChoicesOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		pinned := accounts.ID{1}
		release := make(chan struct{})
		s, reads := choicesReader(func() (choiceSet, error) {
			<-release
			return choiceSet{pins: map[accounts.ID]VersionPin{pinned: {Item: pinned, Name: "Pinned"}}}, nil
		})
		const listings = 12
		got := make([]choiceSet, listings)
		var wg sync.WaitGroup
		for i := range listings {
			wg.Go(func() { got[i] = s.versionChoices(t.Context()) })
		}
		// Every listing waits now: one on the read, the others on it.
		synctest.Wait()
		if n := reads.Load(); n != 1 {
			t.Errorf("%d listings asking at once read the choices %d times, want once", listings, n)
		}
		close(release)
		wg.Wait()
		for i, set := range got {
			if set.pins[pinned].Name != "Pinned" {
				t.Errorf("listing %d got pins %v, want the one read", i, set.pins)
			}
		}
		s.versionChoices(t.Context())
		if n := reads.Load(); n != 1 {
			t.Errorf("the choices were read %d times once kept, want once", n)
		}
	})
}

// A read that fails leaves nothing pinned for the listings that waited for
// it, and the next listing reads again.
func TestAFailedVersionChoicesReadIsTriedAgain(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		pinned := accounts.ID{1}
		failing := true
		s, reads := choicesReader(func() (choiceSet, error) {
			if failing {
				return choiceSet{}, errors.New("database down")
			}
			return choiceSet{pins: map[accounts.ID]VersionPin{pinned: {Item: pinned}}}, nil
		})
		if set := s.versionChoices(t.Context()); len(set.pins) != 0 {
			t.Fatalf("a failed read gave pins %v", set.pins)
		}
		failing = false
		if _, ok := s.versionChoices(t.Context()).pins[pinned]; !ok {
			t.Error("the next listing did not read the choices again")
		}
		if n := reads.Load(); n != 2 {
			t.Errorf("read %d times, want 2", n)
		}
	})
}
