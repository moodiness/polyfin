package library

import (
	"errors"
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

// Requests recording the same new titles and people at once, as apps
// opening a person's page do, all succeed: none fails on the key of an
// item another one inserts, and every credit is kept.
func TestRecordingTheSameNewItemsAtOnceSucceeds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	const writers = 4
	for round := range 20 {
		titles := make([]record, 20)
		for i := range titles {
			key := fmt.Sprintf("movie|tt9%03d%03d", round, i)
			titles[i] = record{ID: itemID(key), Key: key, Kind: KindMovie}
		}
		person := fmt.Sprintf("Ida Round%d", round)
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2*writers)
		for writer := range writers {
			wg.Go(func() {
				<-start
				errs <- e.service.save(t.Context(), titles)
				errs <- e.service.saveCredits(t.Context(), titles[writer].ID, []Person{{ID: PersonID(person), Name: person, Type: "Actor"}}, false)
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		r, err := e.service.load(t.Context(), PersonID(person))
		if err != nil || len(r.Credits) != writers {
			t.Fatalf("round %d, the person's credits: %v %v", round, r.Credits, err)
		}
	}
}

// A person's record takes its identifier from the name it is recorded
// under: a credit whose identifier is another name's adds to the record of
// its name, and a record whose key is another item's is left out without
// failing the others.
func TestRecordsKeepTheirKeysIdentifier(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	first, second := itemID(titleKey(KindMovie, "tt1")), itemID(titleKey(KindMovie, "tt2"))
	if err := e.service.saveCredits(t.Context(), first, []Person{{ID: PersonID("Ann Lee"), Name: "Ann Lee", Type: "Actor"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.service.saveCredits(t.Context(), second, []Person{{ID: PersonID("Other Name"), Name: "ann lee", Type: "Director"}}, false); err != nil {
		t.Fatal(err)
	}
	ann, err := e.service.load(t.Context(), PersonID("Ann Lee"))
	if err != nil || !slices.Equal(ann.Credits[first.String()], []string{"Actor"}) || !slices.Equal(ann.Credits[second.String()], []string{"Director"}) {
		t.Errorf("Ann Lee's credits: %v %v", ann.Credits, err)
	}
	if _, err := e.service.load(t.Context(), PersonID("Other Name")); !errors.Is(err, ErrNotFound) {
		t.Errorf("the other name: %v", err)
	}

	// A record stored under another identifier than its key's, which no
	// version of Polyfin wrote.
	stray := accounts.ID{9}
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'movie', '{"kind": "movie"}')`,
		stray, titleKey(KindMovie, "tt3")); err != nil {
		t.Fatal(err)
	}
	records := []record{{ID: itemID(titleKey(KindMovie, "tt3")), Key: titleKey(KindMovie, "tt3"), Kind: KindMovie},
		{ID: itemID(titleKey(KindMovie, "tt4")), Key: titleKey(KindMovie, "tt4"), Kind: KindMovie}}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	if err := e.service.saveNew(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.load(t.Context(), records[1].ID); err != nil {
		t.Errorf("the other record: %v", err)
	}
	if _, err := e.service.load(t.Context(), stray); err != nil {
		t.Errorf("the stored record: %v", err)
	}
}
