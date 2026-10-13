package watchlist

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
)

type harness struct {
	*Store
	data *userdata.Store
	user accounts.ID
}

func setup(t *testing.T) harness {
	t.Helper()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return harness{Store: New(pool), data: userdata.New(pool), user: user.ID}
}

// item returns a distinct title identifier.
func item(n byte) accounts.ID { return accounts.ID{15: n} }

func dated(n byte, day int) Title {
	return Title{ID: item(n), AddedAt: new(time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC))}
}

func (h harness) save(t *testing.T, list string, whole bool, titles ...Title) int {
	t.Helper()
	added, err := h.Save(t.Context(), h.user, list, titles, whole)
	if err != nil {
		t.Fatal(err)
	}
	return added
}

func (h harness) titles(t *testing.T) []accounts.ID {
	t.Helper()
	ids, err := h.Titles(t.Context(), h.user)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestAListsNextImportTakesOffWhatItAloneHeld(t *testing.T) {
	t.Parallel()
	h := setup(t)
	trakt, simkl := Tracking("trakt"), Tracking("simkl")
	if added := h.save(t, trakt, true, dated(1, 1), dated(2, 3)); added != 2 {
		t.Errorf("first import added %d", added)
	}
	// Simkl holds title 2 too, added later there: the latest date counts.
	if added := h.save(t, simkl, true, dated(2, 5), dated(3, 4)); added != 1 {
		t.Errorf("second list added %d", added)
	}
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(2), item(3), item(1)}) {
		t.Fatalf("latest first: %v", got)
	}
	// Taken off Trakt: title 2 stays, as Simkl holds it; title 1 leaves.
	h.save(t, trakt, true, dated(4, 2))
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(2), item(3), item(4)}) {
		t.Fatalf("after Trakt's next import: %v", got)
	}
	h.save(t, simkl, true)
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(4)}) {
		t.Fatalf("after Simkl's list emptied: %v", got)
	}
	// A list read in part takes nothing off.
	h.save(t, trakt, false, dated(5, 6))
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(5), item(4)}) {
		t.Fatalf("after a partial import: %v", got)
	}
}

func TestOneOffImportsOnlyAdd(t *testing.T) {
	t.Parallel()
	h := setup(t)
	h.save(t, Import, false, Title{ID: item(1)}, Title{ID: item(2)})
	if added := h.save(t, Import, false, Title{ID: item(3)}, Title{ID: item(1)}); added != 1 {
		t.Errorf("second import added %d", added)
	}
	// Undated titles keep their place: the first of an import the latest,
	// a later import's above.
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(3), item(1), item(2)}) {
		t.Errorf("titles: %v", got)
	}
	// The same title on a connection's list counts once, and stays when
	// the connection no longer holds it.
	if added := h.save(t, Connection("stremio"), true, Title{ID: item(2)}); added != 0 {
		t.Errorf("a title already held counted as added: %d", added)
	}
	h.save(t, Connection("stremio"), true)
	if got := h.titles(t); len(got) != 3 {
		t.Errorf("after the connection's list emptied: %v", got)
	}
}

func TestStartedTitlesAreLeftOut(t *testing.T) {
	t.Parallel()
	h := setup(t)
	movie, played, resumed, series, watching, other := item(1), item(2), item(3), item(4), item(5), item(6)
	change := func(items []userdata.Item, change func(*userdata.Data)) {
		t.Helper()
		if _, err := h.data.Change(t.Context(), h.user, items, change); err != nil {
			t.Fatal(err)
		}
	}
	// A favorite is not started; a played movie and one with a resume
	// point are; so is a series with an episode played, or started.
	change([]userdata.Item{{ID: movie}}, func(d *userdata.Data) { d.Favorite = true })
	change([]userdata.Item{{ID: played}}, func(d *userdata.Data) { d.Played = true })
	change([]userdata.Item{{ID: resumed}}, func(d *userdata.Data) { d.Position = time.Minute })
	change([]userdata.Item{{ID: item(10), Series: series, Season: item(11)}}, func(d *userdata.Data) { d.Played = true })
	change([]userdata.Item{{ID: item(12), Series: watching, Season: item(13)}}, func(d *userdata.Data) { d.Position = time.Minute })
	added := h.save(t, Tracking("trakt"), true, dated(1, 1), dated(2, 2), dated(3, 3), dated(4, 4), dated(5, 5), dated(6, 6))
	if added != 2 {
		t.Errorf("added %d, the started ones counted", added)
	}
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{other, movie}) {
		t.Fatalf("titles: %v", got)
	}
	// Starting a title takes it out at once, whatever list holds it.
	change([]userdata.Item{{ID: other}}, func(d *userdata.Data) { d.Position = time.Minute })
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{movie}) {
		t.Errorf("after starting one: %v", got)
	}
}

// SaveWhile saves only while held reports true, and fails with it.
func TestSaveWhileSavesOnlyWhileHeld(t *testing.T) {
	t.Parallel()
	h := setup(t)
	list := Connection("stremio")
	held := func(answer bool, err error) func(context.Context, pgx.Tx) (bool, error) {
		return func(context.Context, pgx.Tx) (bool, error) { return answer, err }
	}
	if added, err := h.SaveWhile(t.Context(), h.user, list, []Title{dated(1, 1)}, true, held(false, nil)); err != nil || added != 0 {
		t.Errorf("not held: %d %v", added, err)
	}
	failure := errors.New("the check failed")
	if _, err := h.SaveWhile(t.Context(), h.user, list, []Title{dated(1, 1)}, true, held(true, failure)); !errors.Is(err, failure) {
		t.Errorf("failed check: %v", err)
	}
	if got := h.titles(t); len(got) != 0 {
		t.Fatalf("saved while not held: %v", got)
	}
	if added, err := h.SaveWhile(t.Context(), h.user, list, []Title{dated(1, 1)}, true, held(true, nil)); err != nil || added != 1 {
		t.Errorf("held: %d %v", added, err)
	}
	if got := h.titles(t); !slices.Equal(got, []accounts.ID{item(1)}) {
		t.Errorf("held: %v", got)
	}
}
