package watchlist

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/userdata"
)

// library tells what each title makes in a watchlist, as the library does:
// a movie itself, a series the episodes the test airs, nothing known while
// it airs none. It counts how often each title is asked about.
type library struct {
	mu     sync.Mutex
	series map[accounts.ID][]accounts.ID
	asked  map[accounts.ID]int
}

func (l *library) entries(_ context.Context, _, title accounts.ID) (Made, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked[title]++
	episodes, isSeries := l.series[title]
	switch {
	case !isSeries:
		return Made{Items: []accounts.ID{title}, Known: true}, nil
	case len(episodes) == 0:
		return Made{}, nil
	}
	return Made{Items: slices.Clone(episodes), Series: title, Known: true}, nil
}

// air makes series a series whose aired episodes of regular seasons are
// episodes, in order: none for one not known or not aired yet.
func (l *library) air(series accounts.ID, episodes ...accounts.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.series[series] = episodes
}

func (l *library) times(title accounts.ID) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.asked[title]
}

type harness struct {
	*Store
	pool  *pgxpool.Pool
	data  *userdata.Store
	users *accounts.Store
	lib   *library
	user  accounts.ID
	clock *atomic.Pointer[time.Time]
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
	lib := &library{series: map[accounts.ID][]accounts.ID{}, asked: map[accounts.ID]int{}}
	clock := &atomic.Pointer[time.Time]{}
	clock.Store(new(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)))
	store := New(pool).WithEntries(lib.entries).WithClock(func() time.Time { return *clock.Load() })
	return harness{Store: store, pool: pool, data: userdata.New(pool), users: users, lib: lib, user: user.ID, clock: clock}
}

// pass moves the store's clock d ahead.
func (h harness) pass(d time.Duration) {
	h.clock.Store(new(h.clock.Load().Add(d)))
}

// item returns a distinct title identifier.
func item(n byte) accounts.ID { return accounts.ID{15: n} }

func items(numbers ...byte) []accounts.ID {
	ids := make([]accounts.ID, len(numbers))
	for i, n := range numbers {
		ids[i] = item(n)
	}
	return ids
}

// episodes returns the identifiers of episodes numbers of series item(series).
func episodes(series byte, numbers ...byte) []accounts.ID {
	ids := make([]accounts.ID, len(numbers))
	for i, n := range numbers {
		ids[i] = accounts.ID{14: series, 15: n}
	}
	return ids
}

func day(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }

func dated(n byte, d int) Title {
	return Title{ID: item(n), AddedAt: new(day(d))}
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

// want fails unless the watchlist's entries are ids, in order.
func (h harness) want(t *testing.T, step string, ids ...accounts.ID) {
	t.Helper()
	if got := h.titles(t); !slices.Equal(got, ids) {
		t.Fatalf("%s: entries %v, want %v", step, got, ids)
	}
}

// pending lists the titles that wait for their entries, the latest first.
func (h harness) pending(t *testing.T) []accounts.ID {
	t.Helper()
	held, err := h.Pending(t.Context(), h.user)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]accounts.ID, len(held))
	for i, p := range held {
		ids[i] = p.ID
	}
	return ids
}

// lists lists the lists of the ledger that hold title, by name.
func (h harness) lists(t *testing.T, title accounts.ID) []string {
	t.Helper()
	rows, _ := h.pool.Query(t.Context(), "SELECT list FROM watchlist WHERE user_id = $1 AND item_id = $2 ORDER BY list", h.user, title)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return names
}

// anHourPasses moves the store's clock an hour and a minute ahead.
func (h harness) anHourPasses(t *testing.T) {
	t.Helper()
	h.pass(61 * time.Minute)
}

func (h harness) expandAll(t *testing.T) {
	t.Helper()
	if err := h.ExpandAll(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (h harness) playlist(t *testing.T) (accounts.ID, bool) {
	t.Helper()
	id, exists, err := h.Playlist(t.Context(), h.user)
	if err != nil {
		t.Fatal(err)
	}
	return id, exists
}

func (h harness) change(t *testing.T, change func(*userdata.Data), items ...userdata.Item) {
	t.Helper()
	if _, err := h.data.Change(t.Context(), h.user, items, change); err != nil {
		t.Fatal(err)
	}
}

// entry returns the entry of the watchlist that holds item.
func (h harness) entry(t *testing.T, item accounts.ID) accounts.ID {
	t.Helper()
	var id accounts.ID
	if err := h.pool.QueryRow(t.Context(), `SELECT e.id FROM playlist_entries e JOIN playlists p ON p.id = e.playlist_id
		WHERE p.owner_id = $1 AND p.watchlist AND e.item_id = $2`, h.user, item).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// The ledger keeps which lists hold each title, with the latest date they
// give, and the entries follow it: a title leaves once no list holds it,
// a list read in part takes nothing off, and a list no longer read takes
// off what it alone held.
func TestTheEntriesFollowTheLedger(t *testing.T) {
	t.Parallel()
	h := setup(t)
	trakt, simkl := Tracking("trakt"), Tracking("simkl")
	if added := h.save(t, trakt, true, dated(1, 1), dated(2, 3)); added != 2 {
		t.Errorf("first import added %d", added)
	}
	h.want(t, "the latest first", items(2, 1)...)
	// Simkl holds title 2 too: only title 3 is new to the playlist.
	if added := h.save(t, simkl, true, dated(2, 5), dated(3, 4)); added != 1 {
		t.Errorf("second list added %d", added)
	}
	h.want(t, "after the second list", items(3, 2, 1)...)
	if got := h.lists(t, item(2)); !slices.Equal(got, []string{simkl, trakt}) {
		t.Errorf("lists holding title 2: %v", got)
	}
	held, err := h.Held(t.Context(), h.user)
	must(t, err)
	if len(held) != 3 || held[1].ID != item(2) || !held[1].AddedAt.Equal(day(5)) || held[1].Series != (accounts.ID{}) {
		t.Errorf("held: %+v", held)
	}

	// Title 1 leaves with Trakt; title 2 stays, as Simkl holds it.
	h.save(t, trakt, true, dated(4, 2))
	h.want(t, "after Trakt's next import", items(4, 3, 2)...)
	if got := h.lists(t, item(1)); len(got) != 0 {
		t.Errorf("a title no list holds stays in the ledger: %v", got)
	}
	h.save(t, simkl, true)
	h.want(t, "after Simkl's list emptied", item(4))
	// A list read in part takes nothing off; a title listed twice keeps
	// its latest date.
	h.save(t, trakt, false, dated(5, 6), dated(5, 9))
	h.want(t, "after a partial import", items(5, 4)...)
	held, err = h.Held(t.Context(), h.user)
	must(t, err)
	if !held[0].AddedAt.Equal(day(9)) {
		t.Errorf("a title listed twice: %+v", held[0])
	}
	must(t, h.Drop(t.Context(), h.user, trakt))
	h.want(t, "after Trakt was dropped")
	if got := h.lists(t, item(5)); len(got) != 0 {
		t.Errorf("a dropped list's title stays in the ledger: %v", got)
	}
}

// A one-off import only adds; a title it holds outlasts a connection's
// list that held it too.
func TestOneOffImportsOnlyAdd(t *testing.T) {
	t.Parallel()
	h := setup(t)
	h.save(t, Import, false, Title{ID: item(1)}, Title{ID: item(2)})
	if added := h.save(t, Import, false, Title{ID: item(3)}, Title{ID: item(1)}); added != 1 {
		t.Errorf("second import added %d", added)
	}
	// Undated titles come in order, a later import's above.
	h.want(t, "titles", items(3, 1, 2)...)
	if added := h.save(t, Connection("stremio"), true, Title{ID: item(2)}); added != 0 {
		t.Errorf("a title the playlist shows counted as added: %d", added)
	}
	h.save(t, Connection("stremio"), true)
	h.want(t, "after the connection's list emptied", items(3, 1, 2)...)
}

// What a list newly holds waits, then goes at the top: the latest title
// first, a series' episodes in order, those the playlist holds keeping
// their place. An import never moves an entry.
func TestImportsWaitThenGoAtTheTop(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.lib.air(item(101), episodes(101, 1, 2, 3)...)
	h.lib.air(item(102), episodes(102, 1, 2)...)
	h.save(t, Import, false, dated(1, 1))

	// A store that cannot tell entries yet leaves them waiting.
	added, err := New(h.pool).Save(ctx, h.user, trakt, []Title{dated(2, 2), dated(101, 4), dated(3, 3), dated(102, 5)}, true)
	if err != nil || added != 4 {
		t.Fatalf("saved %d, %v", added, err)
	}
	h.want(t, "before the entries are made", item(1))
	if got := h.pending(t); !slices.Equal(got, items(102, 101, 3, 2)) {
		t.Fatalf("waiting: %v", got)
	}
	if made, err := h.Expand(ctx, h.user, false); err != nil || made != 4 {
		t.Fatalf("expanded %d, %v", made, err)
	}
	h.want(t, "expanded", slices.Concat(episodes(102, 1, 2), episodes(101, 1, 2, 3), items(3, 2, 1))...)
	if got := h.pending(t); len(got) != 0 {
		t.Errorf("still waiting: %v", got)
	}
	held, err := h.Held(ctx, h.user)
	must(t, err)
	if held[0].Series != item(102) || !held[0].AddedAt.Equal(day(5)) || held[5].Series != (accounts.ID{}) {
		t.Errorf("held: %+v", held[:6])
	}

	// The member moves a movie and adds an episode by hand; a list then
	// brings that episode's series: its other episodes go at the top, the
	// one held keeps its place, and nothing else moves.
	must(t, h.Move(ctx, h.user, h.entry(t, item(1)), h.entry(t, episodes(102, 1)[0])))
	h.lib.air(item(103), episodes(103, 1, 2, 3)...)
	must(t, h.Add(ctx, h.user, []Group{{Title: episodes(103, 2)[0], Series: item(103), Items: episodes(103, 2)}}, 2))
	before := slices.Concat(items(1), episodes(102, 1), episodes(103, 2), episodes(102, 2), episodes(101, 1, 2, 3), items(3, 2))
	h.want(t, "moved and added", before...)
	if added := h.save(t, Tracking("simkl"), true, dated(103, 6), dated(2, 7), dated(1, 8)); added != 1 {
		t.Errorf("the second list added %d", added)
	}
	h.want(t, "after the second list", slices.Concat(episodes(103, 1, 3), before)...)
}

// A series not known yet, or none of whose episodes aired, waits; it is
// expanded once its episodes are, by the next save's Expand or by
// ExpandAll, which tries each title at most once an hour. Episodes that
// air later are not added.
func TestASeriesNotKnownYetIsExpandedOnce(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.lib.air(item(101))
	h.save(t, trakt, true, dated(101, 2), dated(1, 1))
	h.want(t, "the movie alone", item(1))
	if got := h.pending(t); !slices.Equal(got, items(101)) {
		t.Fatalf("waiting: %v", got)
	}
	h.expandAll(t)
	if asked := h.lib.times(item(101)); asked != 1 {
		t.Errorf("asked %d times within the hour", asked)
	}
	h.lib.air(item(101), episodes(101, 1, 2)...)
	h.expandAll(t)
	h.want(t, "within the hour", item(1))
	h.anHourPasses(t)
	h.expandAll(t)
	h.want(t, "once known", slices.Concat(episodes(101, 1, 2), items(1))...)
	if got := h.pending(t); len(got) != 0 {
		t.Errorf("still waiting: %v", got)
	}

	// A later episode is never added.
	h.lib.air(item(101), episodes(101, 1, 2, 3)...)
	h.anHourPasses(t)
	h.expandAll(t)
	if _, err := h.Expand(ctx, h.user, false); err != nil {
		t.Fatal(err)
	}
	h.save(t, trakt, true, dated(101, 2), dated(1, 1))
	h.want(t, "after the next episode aired", slices.Concat(episodes(101, 1, 2), items(1))...)
	if asked := h.lib.times(item(101)); asked != 2 {
		t.Errorf("asked %d times, want 2", asked)
	}

	// Another series waits, then the next save's Expand makes it.
	h.lib.air(item(102))
	h.save(t, Tracking("simkl"), true, dated(102, 3))
	h.lib.air(item(102), episodes(102, 1)...)
	h.save(t, Import, false, dated(2, 4))
	h.want(t, "made by the next save", slices.Concat(items(2), episodes(102, 1), episodes(101, 1, 2), items(1))...)
}

// An episode the member takes off stays off while a list holds its
// series, and so does a movie while a list holds it; they come back once
// a list newly holds the title.
func TestRemovalsHoldWhileAListHoldsTheTitle(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.lib.air(item(101), episodes(101, 1, 2, 3)...)
	again := func() {
		t.Helper()
		if added := h.save(t, trakt, true, dated(101, 1), dated(1, 2)); added != 0 {
			t.Errorf("the same list again added %d", added)
		}
	}
	h.save(t, trakt, true, dated(101, 1), dated(1, 2))
	h.want(t, "imported", slices.Concat(items(1), episodes(101, 1, 2, 3))...)

	must(t, h.Remove(ctx, h.user, episodes(101, 2), nil))
	again()
	h.want(t, "an episode taken off", slices.Concat(items(1), episodes(101, 1, 3))...)
	must(t, h.RemoveEntries(ctx, h.user, []accounts.ID{h.entry(t, episodes(101, 3)[0])}))
	again()
	h.want(t, "an entry taken off", slices.Concat(items(1), episodes(101, 1))...)
	must(t, h.Remove(ctx, h.user, items(1), []accounts.ID{item(101)}))
	again()
	h.want(t, "the series and the movie taken off")
	if got := h.pending(t); len(got) != 0 {
		t.Errorf("waiting: %v", got)
	}

	// Another list newly holding the series brings all its episodes back;
	// the list holding the movie again after dropping it brings it back.
	h.save(t, Tracking("simkl"), true, dated(101, 5))
	h.save(t, trakt, true, dated(101, 1))
	h.save(t, trakt, true, dated(101, 1), dated(1, 6))
	h.want(t, "listed anew", slices.Concat(items(1), episodes(101, 1, 2, 3))...)

	// A list newly holding a series some of whose episodes the member took
	// off brings those back at the top, once each, the others keeping
	// their place; so does a list holding it again after dropping it.
	// The list that holds it already brings back nothing.
	must(t, h.Remove(ctx, h.user, episodes(101, 2, 3), nil))
	h.save(t, trakt, true, dated(101, 1), dated(1, 6))
	h.want(t, "two episodes taken off", slices.Concat(items(1), episodes(101, 1))...)
	stremio := Connection("stremio")
	if added := h.save(t, stremio, true, dated(101, 7)); added != 0 {
		t.Errorf("a series the playlist shows counted as added: %d", added)
	}
	h.want(t, "another list newly holding the series", slices.Concat(episodes(101, 2, 3), items(1), episodes(101, 1))...)
	must(t, h.Remove(ctx, h.user, episodes(101, 2), nil))
	h.save(t, stremio, true, dated(101, 7))
	h.want(t, "the same list again", slices.Concat(episodes(101, 3), items(1), episodes(101, 1))...)
	h.save(t, trakt, true, dated(1, 6))
	h.save(t, trakt, true, dated(101, 1), dated(1, 6))
	h.want(t, "held again after dropping it", slices.Concat(episodes(101, 2, 3), items(1), episodes(101, 1))...)
}

// A title that waits for its entries is tried again hourly during its
// first day, then daily; a list newly holding it again starts that day
// over.
func TestATitleThatWaitsIsTriedLessOften(t *testing.T) {
	t.Parallel()
	h := setup(t)
	h.lib.air(item(101))
	h.save(t, Tracking("trakt"), true, dated(101, 1))
	tries := func(step string, want int) {
		t.Helper()
		if got := h.lib.times(item(101)); got != want {
			t.Fatalf("%s: tried %d times, want %d", step, got, want)
		}
	}
	tries("saved", 1)
	for range 23 {
		h.anHourPasses(t)
		h.expandAll(t)
	}
	tries("through the first day", 24)
	// The first day is over: the next try comes a day after the last.
	h.anHourPasses(t)
	h.expandAll(t)
	h.pass(22 * time.Hour)
	h.expandAll(t)
	tries("within a day of the last", 24)
	h.pass(time.Hour)
	h.expandAll(t)
	tries("a day after the last", 25)
	h.pass(23 * time.Hour)
	h.expandAll(t)
	tries("within the next day", 25)

	// Simkl newly holding it tries it again at once, then hourly.
	h.save(t, Tracking("simkl"), true, dated(101, 2))
	tries("newly held", 26)
	h.anHourPasses(t)
	h.expandAll(t)
	tries("an hour later", 27)
	h.lib.air(item(101), episodes(101, 1)...)
	h.anHourPasses(t)
	h.expandAll(t)
	h.want(t, "once aired", episodes(101, 1)...)
}

// A list no longer holding a title takes its entries off, but those the
// member started or watched, and those they made their own, which stay
// as titles of their own.
func TestAListDroppingATitleKeepsStartedAndOwnEntries(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.lib.air(item(101), episodes(101, 1, 2, 3, 4)...)
	h.save(t, trakt, true, dated(101, 1), dated(1, 2), dated(2, 3), dated(3, 4))
	h.want(t, "imported", slices.Concat(items(3, 2, 1), episodes(101, 1, 2, 3, 4))...)
	e := episodes(101, 1, 2, 3, 4)
	h.change(t, func(d *userdata.Data) { d.Played = true }, userdata.Item{ID: e[0], Series: item(101)})
	h.change(t, func(d *userdata.Data) { d.Position = time.Minute }, userdata.Item{ID: e[1], Series: item(101)}, userdata.Item{ID: item(1)})
	must(t, h.Add(ctx, h.user, []Group{{Title: item(2), Items: items(2)}, {Title: e[2], Series: item(101), Items: e[2:3]}}, 0))
	h.want(t, "titles held keep their place", slices.Concat(items(3, 2, 1), e)...)

	h.save(t, trakt, true)
	h.want(t, "after Trakt emptied", slices.Concat(items(2, 1), e[:3])...)
	held, err := h.Held(ctx, h.user)
	must(t, err)
	if !held[2].AddedAt.Equal(day(1)) || held[2].Series != item(101) {
		t.Errorf("a watched episode kept: %+v", held[2])
	}
	// They are the member's now, whatever Trakt does; a list newly holding
	// the series brings back the episode that left.
	h.save(t, trakt, true, dated(101, 5))
	h.want(t, "the series listed anew", slices.Concat(e[3:], items(2, 1), e[:3])...)
	h.save(t, trakt, true)
	h.want(t, "after Trakt emptied again", slices.Concat(items(2, 1), e[:3])...)
}

// Moving an entry places it, and makes it the member's own: it outlasts
// the lists.
func TestAMoveMakesTheEntryTheMembersOwn(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.save(t, trakt, true, dated(1, 1), dated(2, 2), dated(3, 3))
	must(t, h.Move(ctx, h.user, h.entry(t, item(1)), h.entry(t, item(3))))
	h.want(t, "moved before", items(1, 3, 2)...)
	must(t, h.Move(ctx, h.user, h.entry(t, item(3)), accounts.ID{}))
	h.want(t, "moved last", items(1, 2, 3)...)
	if got := h.lists(t, item(3)); !slices.Equal(got, []string{Own, trakt}) {
		t.Errorf("lists of a moved title: %v", got)
	}
	must(t, h.Move(ctx, h.user, accounts.ID{1}, h.entry(t, item(1))))
	h.want(t, "an unknown entry moved", items(1, 2, 3)...)
	h.save(t, trakt, true)
	h.want(t, "after Trakt emptied", items(1, 3)...)
}

// Deleting the playlist forgets the member's own titles and what waits;
// the other lists' titles stay off. The playlist comes back, as a new
// one, only once an entry is added.
func TestADeletedWatchlistComesBackWithItsNextEntry(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	before, exists := h.playlist(t)
	if !exists {
		t.Fatal("a new user has no watchlist")
	}
	h.lib.air(item(101))
	h.save(t, trakt, true, dated(1, 1), dated(101, 2))
	must(t, h.Add(ctx, h.user, []Group{{Title: item(2), Items: items(2)}}, 0))
	must(t, h.Delete(ctx, h.user))
	if _, exists := h.playlist(t); exists {
		t.Fatal("still there once deleted")
	}
	if got := h.pending(t); len(got) != 0 {
		t.Errorf("waiting after the deletion: %v", got)
	}
	if got, trakts := h.lists(t, item(2)), h.lists(t, item(1)); len(got) != 0 || !slices.Equal(trakts, []string{trakt}) {
		t.Errorf("ledger after the deletion: %v, %v", got, trakts)
	}

	// Nothing brings it back but an entry.
	h.save(t, trakt, true, dated(1, 1), dated(101, 2))
	h.lib.air(item(101), episodes(101, 1)...)
	h.anHourPasses(t)
	h.expandAll(t)
	h.lib.air(item(102))
	h.save(t, Tracking("simkl"), true, dated(102, 3))
	h.lib.air(item(103))
	must(t, h.Add(ctx, h.user, []Group{{Title: item(103)}}, 0))
	must(t, h.Remove(ctx, h.user, items(4), nil))
	must(t, h.Move(ctx, h.user, accounts.ID{1}, accounts.ID{}))
	if _, exists := h.playlist(t); exists {
		t.Fatal("back without an entry")
	}
	h.save(t, trakt, true, dated(1, 1), dated(101, 2), dated(3, 4))
	after, exists := h.playlist(t)
	if !exists || after == before {
		t.Fatalf("back as %v, %v; before %v", after, exists, before)
	}
	h.want(t, "back", item(3))
}

// A shared editor's changes are the owner's: they go on the owner's own
// list and playlist. An entry of someone else's watchlist is out of
// reach of the member's own.
func TestSharedEditorsActForTheOwner(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	bob, err := h.users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	must(t, err)
	playlist, _ := h.playlist(t)
	if _, err := h.pool.Exec(ctx, "INSERT INTO playlist_shares (playlist_id, user_id, can_edit) VALUES ($1, $2, true)", playlist, bob.ID); err != nil {
		t.Fatal(err)
	}
	trakt := Tracking("trakt")
	h.save(t, trakt, true, dated(1, 1))
	// Bob, sharing Alice's watchlist, adds and moves there: as Alice.
	h.lib.air(item(101), episodes(101, 1, 2)...)
	must(t, h.Add(ctx, h.user, []Group{{Title: item(101), Series: item(101), Items: episodes(101, 1, 2)}}, 1))
	must(t, h.Move(ctx, h.user, h.entry(t, item(1)), accounts.ID{}))
	h.want(t, "the owner's watchlist", slices.Concat(episodes(101, 1, 2), items(1))...)
	if got := h.lists(t, item(101)); !slices.Equal(got, []string{Own}) {
		t.Errorf("lists of the title an editor added: %v", got)
	}
	if got, err := h.Titles(ctx, bob.ID); err != nil || len(got) != 0 {
		t.Errorf("the editor's own watchlist: %v %v", got, err)
	}
	h.save(t, trakt, true)
	h.want(t, "the owner's list emptied", slices.Concat(episodes(101, 1, 2), items(1))...)

	// Bob's own changes do not reach Alice's entries.
	must(t, h.RemoveEntries(ctx, bob.ID, []accounts.ID{h.entry(t, item(1))}))
	must(t, h.Move(ctx, bob.ID, h.entry(t, item(1)), h.entry(t, episodes(101, 1)[0])))
	must(t, h.Remove(ctx, bob.ID, items(1), []accounts.ID{item(101)}))
	h.want(t, "after the editor's own changes", slices.Concat(episodes(101, 1, 2), items(1))...)
}

// The checks and the rows follow the ledger's titles the playlist shows,
// a series as one title, an episode added by hand as itself, a title that
// waits too, never one the member started or took off. Due takes those
// never checked first, then the longest unchecked, in the playlist's
// order; Available the latest found first; Waiting the playlist's order,
// those that wait last.
func TestTheRowsFollowTheTitlesShown(t *testing.T) {
	t.Parallel()
	h := setup(t)
	ctx := t.Context()
	trakt := Tracking("trakt")
	h.lib.air(item(101))
	h.lib.air(item(102), episodes(102, 1, 2)...)
	h.save(t, trakt, true, dated(1, 1), dated(101, 2), dated(102, 3), dated(3, 4), dated(4, 5))
	must(t, h.Remove(ctx, h.user, items(3), nil))
	h.change(t, func(d *userdata.Data) { d.Played = true }, userdata.Item{ID: item(4)})
	episode := episodes(103, 1)
	must(t, h.Add(ctx, h.user, []Group{{Title: episode[0], Series: item(103), Items: episode}}, 2))
	h.want(t, "the playlist", slices.Concat(items(4), episodes(102, 1), episode, episodes(102, 2), items(1))...)

	far := day(30)
	due := func(before time.Time, limit int) []accounts.ID {
		t.Helper()
		ids, err := h.Due(ctx, h.user, before, limit)
		must(t, err)
		return ids
	}
	if got, want := due(far, 10), []accounts.ID{item(102), episode[0], item(1), item(101)}; !slices.Equal(got, want) {
		t.Fatalf("due: %v, want %v", got, want)
	}
	if got := due(far, 2); !slices.Equal(got, []accounts.ID{item(102), episode[0]}) {
		t.Errorf("due, two: %v", got)
	}
	record := func(title accounts.ID, available bool, at int) {
		t.Helper()
		if _, err := h.Record(ctx, h.user, title, available, day(at)); err != nil {
			t.Fatal(err)
		}
	}
	record(item(1), true, 10)
	record(item(102), true, 11)
	must(t, h.Checked(ctx, h.user, episode[0], day(12)))
	record(item(101), false, 13)
	// Neither the title taken off nor the one started shows, whatever
	// their checks found.
	record(item(3), true, 14)
	record(item(4), true, 14)
	h.save(t, Import, false, dated(5, 6))

	if got := due(day(12), 10); !slices.Equal(got, items(5, 1, 102)) {
		t.Errorf("due after checks: %v", got)
	}
	for name, c := range map[string]struct {
		list func(context.Context, accounts.ID) ([]accounts.ID, error)
		want []accounts.ID
	}{
		"available": {h.Available, items(102, 1)},
		"waiting":   {h.Waiting, []accounts.ID{episode[0], item(101)}},
	} {
		got, err := c.list(ctx, h.user)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: %v %v, want %v", name, got, err, c.want)
		}
	}
	// A title waiting for its entries follows the playlist's order once
	// made.
	h.lib.air(item(101), episodes(101, 1)...)
	h.anHourPasses(t)
	h.expandAll(t)
	must(t, h.Move(ctx, h.user, h.entry(t, episodes(101, 1)[0]), accounts.ID{}))
	if got, err := h.Waiting(ctx, h.user); err != nil || !slices.Equal(got, []accounts.ID{episode[0], item(101)}) {
		t.Errorf("waiting once made: %v %v", got, err)
	}
	must(t, h.Move(ctx, h.user, h.entry(t, episodes(101, 1)[0]), h.entry(t, episode[0])))
	if got, err := h.Waiting(ctx, h.user); err != nil || !slices.Equal(got, []accounts.ID{item(101), episode[0]}) {
		t.Errorf("waiting once moved: %v %v", got, err)
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
	h.want(t, "saved while not held")
	if added, err := h.SaveWhile(t.Context(), h.user, list, []Title{dated(1, 1)}, true, held(true, nil)); err != nil || added != 1 {
		t.Errorf("held: %d %v", added, err)
	}
	h.want(t, "held", item(1))
}
