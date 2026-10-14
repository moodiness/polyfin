package library

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Listings that save the same titles at once in other orders, as smart
// lists sorted differently do when an app opens its home, all succeed:
// none is cut short by a deadlock in the database.
func TestListingsSavingTheSameTitlesAtOnceSucceed(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	titles := make([]record, 200)
	for i := range titles {
		key := fmt.Sprintf("movie|tt%07d", i)
		titles[i] = record{ID: itemID(key), Key: key, Kind: KindMovie}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for list := range 2 {
		folder := accounts.ID{byte(list + 1)}
		listed := slices.Clone(titles)
		if list == 1 {
			slices.Reverse(listed)
		}
		for i := range listed {
			listed[i].Parent = &folder
		}
		wg.Go(func() {
			for range 10 {
				if err := e.service.save(t.Context(), listed); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
