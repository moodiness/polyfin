package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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

// A save merges the items it finds stored with what it records, whatever
// another writer does to them meanwhile: it never inserts one, which the
// key of another item would fail. Here the merge waits for an item another
// writer holds, which deletes the next one and gives its key to a record of
// another identifier: the save succeeds, merges the first, and leaves the
// second out, telling it.
func TestMergingItemsChangedMeanwhileSucceeds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, nil))
	records := []record{{Key: titleKey(KindMovie, "tt1"), Kind: KindMovie}, {Key: titleKey(KindMovie, "tt2"), Kind: KindMovie}}
	for i := range records {
		records[i].ID = itemID(records[i].Key)
	}
	slices.SortFunc(records, func(a, b record) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	first, second := records[0], records[1]
	if err := e.service.save(ctx, records); err != nil {
		t.Fatal(err)
	}

	holder, err := e.service.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(context.WithoutCancel(ctx)) }()
	var holderPID int
	if err := holder.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, "SELECT 1 FROM items WHERE id = $1 FOR UPDATE", first.ID); err != nil {
		t.Fatal(err)
	}
	folder := accounts.ID{7}
	listed := slices.Clone(records)
	for i := range listed {
		listed[i].Parent = &folder
	}
	saved := make(chan error, 1)
	go func() { saved <- e.service.save(ctx, listed) }()
	eventually(t, "the save to wait for the first item", func() bool {
		var waiting int
		_ = e.service.db.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))", holderPID).Scan(&waiting)
		return waiting > 0
	})
	stray := accounts.ID{9}
	if _, err := holder.Exec(ctx, "DELETE FROM items WHERE id = $1", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'movie', '{"kind": "movie"}')`, stray, second.Key); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("the save: %v", err)
	}
	if r, err := e.service.load(ctx, first.ID); err != nil || r.Parent == nil || *r.Parent != folder {
		t.Errorf("the first item: %+v %v", r, err)
	}
	if _, err := e.service.load(ctx, second.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the second item: %v", err)
	}
	if r, err := e.service.load(ctx, stray); err != nil || r.Key != second.Key {
		t.Errorf("the record holding its key: %+v %v", r, err)
	}
	if !strings.Contains(log.String(), "Items were not recorded") || !strings.Contains(log.String(), second.Key) {
		t.Errorf("log:\n%s", log)
	}
}
