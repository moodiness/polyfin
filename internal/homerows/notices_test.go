package homerows

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// fakeNotifier records whom it told of what, and words each suggestion by
// its row's name and signer.
type fakeNotifier struct {
	mu   sync.Mutex
	told []string
}

func (n *fakeNotifier) SuggestionWords(s Suggestion) (string, string) {
	return s.From.Name + " suggests " + s.Row.Name, s.Row.Note
}

func (n *fakeNotifier) TitlesSuggested(user accounts.User, s Suggestion) (string, string) {
	title, message := n.SuggestionWords(s)
	n.mu.Lock()
	n.told = append(n.told, user.Name+": "+title)
	n.mu.Unlock()
	return title, message
}

func (n *fakeNotifier) take() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	told := n.told
	n.told = nil
	return told
}

// fakeApps shows messages on the apps of the users it has apps open for,
// and records them.
type fakeApps struct {
	mu    sync.Mutex
	open  map[accounts.ID]bool
	shown []string
}

func (a *fakeApps) ShowMessage(_ context.Context, user accounts.ID, header, text string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.open[user] {
		return 0
	}
	a.shown = append(a.shown, header+" | "+text)
	return 1
}

func (a *fakeApps) take() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	shown := a.shown
	a.shown = nil
	return shown
}

type tellFixture struct {
	store              *Store
	users              *accounts.Store
	notifier           *fakeNotifier
	apps               *fakeApps
	now                time.Time
	sees               map[accounts.ID]bool
	morgan, sam, robin accounts.User
}

func newTellFixture(t *testing.T) *tellFixture {
	t.Helper()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	f := &tellFixture{store: New(pool), users: users, notifier: &fakeNotifier{}, apps: &fakeApps{open: map[accounts.ID]bool{}},
		now: at(time.November, 6, 20, 0)}
	for _, u := range []struct {
		into  *accounts.User
		name  string
		admin bool
	}{{&f.morgan, "Morgan", true}, {&f.sam, "Sam", false}, {&f.robin, "Robin", false}} {
		if *u.into, err = users.CreateUser(t.Context(), accounts.NewUser{Name: u.name, Password: "correct horse", IsAdministrator: u.admin}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// teller is a Teller on the fixture's database, as after a restart when
// made again.
func (f *tellFixture) teller() *Teller {
	c := TellerConfig{Store: f.store, Users: f.users, Notifier: f.notifier, Apps: f.apps,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return f.now }}
	if f.sees != nil {
		c.Sees = func(_ context.Context, user accounts.User, _ Row) (bool, error) { return f.sees[user.ID], nil }
	}
	return NewTeller(c)
}

// suggestion adds a New row that tells its users, shown to users, its note
// signed by Morgan.
func (f *tellFixture) suggestion(t *testing.T, name string, schedule Schedule, users ...accounts.ID) Row {
	t.Helper()
	row, err := f.store.Create(t.Context(), Row{Name: name, Kind: KindNew, Place: PlaceTop, Count: DefaultCount, Users: users, Notify: true,
		Note: "Watch it this weekend.", NoteBy: f.morgan.ID, Schedule: schedule})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

// Each user of a row that tells them is told once, on their targets and on
// their apps open then: neither looking again nor a restart tells them
// again. A user added later is told then, alone; a row that does not tell
// its users, or that is shown to everyone, tells nobody.
func TestSuggestionsAreToldOnce(t *testing.T) {
	t.Parallel()
	f := newTellFixture(t)
	ctx := t.Context()
	f.apps.open[f.sam.ID] = true
	row := f.suggestion(t, "For you", Schedule{}, f.sam.ID)
	if _, err := f.store.Create(ctx, Row{Name: "Quiet", Kind: KindNew, Place: PlaceTop, Count: DefaultCount, Users: []accounts.ID{f.sam.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Create(ctx, Row{Name: "Everyone", Kind: KindNew, Place: PlaceTop, Count: DefaultCount, Everyone: true}); err != nil {
		t.Fatal(err)
	}
	teller := f.teller()
	teller.Check(ctx)
	if got := f.notifier.take(); !slices.Equal(got, []string{"Sam: Morgan suggests For you"}) {
		t.Errorf("told: %q", got)
	}
	if got := f.apps.take(); !slices.Equal(got, []string{"Morgan suggests For you | Watch it this weekend."}) {
		t.Errorf("shown on the apps: %q", got)
	}
	teller.Check(ctx)
	f.teller().Check(ctx)
	if got := f.notifier.take(); len(got) != 0 {
		t.Errorf("told again: %q", got)
	}
	row.Users = append(row.Users, f.robin.ID)
	if _, err := f.store.Update(ctx, row); err != nil {
		t.Fatal(err)
	}
	f.teller().Check(ctx)
	if got := f.notifier.take(); !slices.Equal(got, []string{"Robin: Morgan suggests For you"}) {
		t.Errorf("told once Robin was added: %q", got)
	}
	counts, err := f.store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := counts[row.ID]; got != (Counts{Users: 2, Told: 2}) {
		t.Errorf("counts: %+v", got)
	}
}

// A user is told as the row starts showing to them: not before its
// schedule's start, nor while they turned it off, nor while they may not
// see what it lists; and never once they saw it.
func TestSuggestionsWaitUntilTheyShow(t *testing.T) {
	t.Parallel()
	f := newTellFixture(t)
	ctx := t.Context()
	f.sees = map[accounts.ID]bool{f.sam.ID: true}
	later := f.suggestion(t, "Next week", Schedule{Start: day(time.November, 13)}, f.sam.ID, f.robin.ID)
	off := f.suggestion(t, "Turned off", Schedule{}, f.sam.ID)
	seen := f.suggestion(t, "Seen", Schedule{}, f.sam.ID)
	if err := f.store.SetOff(ctx, f.sam.ID, off.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetSeen(ctx, f.sam.ID, seen.ID, f.now); err != nil {
		t.Fatal(err)
	}
	f.teller().Check(ctx)
	if got := f.notifier.take(); len(got) != 0 {
		t.Errorf("told before: %q", got)
	}
	f.now = at(time.November, 13, 0, 0)
	if err := f.store.SetOff(ctx, f.sam.ID, off.ID, false); err != nil {
		t.Fatal(err)
	}
	f.teller().Check(ctx)
	got := f.notifier.take()
	slices.Sort(got)
	if !slices.Equal(got, []string{"Sam: Morgan suggests Next week", "Sam: Morgan suggests Turned off"}) {
		t.Errorf("told once shown: %q", got)
	}
	f.sees[f.robin.ID] = true
	f.teller().Check(ctx)
	if got := f.notifier.take(); !slices.Equal(got, []string{"Robin: Morgan suggests Next week"}) {
		t.Errorf("told once Robin sees it: %q", got)
	}
	_ = later
}

// A user none of whose apps was open when told sees the suggestion on the
// first app that connects while the row shows, once; not after they saw
// the row, nor once it no longer shows.
func TestSuggestionsShowOnTheNextAppToConnect(t *testing.T) {
	t.Parallel()
	f := newTellFixture(t)
	ctx := t.Context()
	shown := f.suggestion(t, "For you", Schedule{}, f.sam.ID, f.robin.ID)
	ending := f.suggestion(t, "Tonight", Schedule{End: day(time.November, 6)}, f.sam.ID)
	teller := f.teller()
	teller.Check(ctx)
	if got := len(f.notifier.take()); got != 3 {
		t.Fatalf("told %d", got)
	}
	if got := f.apps.take(); len(got) != 0 {
		t.Fatalf("shown with no app open: %q", got)
	}
	if err := f.store.SetSeen(ctx, f.robin.ID, shown.ID, f.now); err != nil {
		t.Fatal(err)
	}
	f.now = at(time.November, 7, 9, 0)
	f.apps.open[f.sam.ID], f.apps.open[f.robin.ID] = true, true
	teller.ShowPending(ctx, f.robin.ID)
	teller.ShowPending(ctx, f.sam.ID)
	if got := f.apps.take(); !slices.Equal(got, []string{"Morgan suggests For you | Watch it this weekend."}) {
		t.Errorf("shown at the next connection: %q", got)
	}
	f.teller().ShowPending(ctx, f.sam.ID)
	if got := f.apps.take(); len(got) != 0 {
		t.Errorf("shown again: %q", got)
	}
	_ = ending
}

// A user's seen state is kept for the rows shown to chosen users that are
// shown to them only, first time kept; the counts never say who.
func TestSeenRowsAndCounts(t *testing.T) {
	t.Parallel()
	f := newTellFixture(t)
	ctx := t.Context()
	row := f.suggestion(t, "For you", Schedule{}, f.sam.ID, f.robin.ID)
	everyone, err := f.store.Create(ctx, Row{Name: "Everyone", Kind: KindNew, Place: PlaceTop, Count: DefaultCount, Everyone: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetSeen(ctx, f.sam.ID, everyone.ID, f.now); !errors.Is(err, ErrNotFound) {
		t.Errorf("seeing a row shown to everyone: %v", err)
	}
	if err := f.store.SetSeen(ctx, f.morgan.ID, row.ID, f.now); !errors.Is(err, ErrNotFound) {
		t.Errorf("seeing a row not shown to them: %v", err)
	}
	for range 2 {
		if err := f.store.SetSeen(ctx, f.sam.ID, row.ID, f.now); err != nil {
			t.Fatal(err)
		}
	}
	seen, err := f.store.Seen(ctx, f.sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !seen[row.ID] || len(seen) != 1 {
		t.Errorf("seen by Sam: %v", seen)
	}
	if seen, _ := f.store.Seen(ctx, f.robin.ID); len(seen) != 0 {
		t.Errorf("seen by Robin: %v", seen)
	}
	counts, err := f.store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := counts[row.ID]; got != (Counts{Users: 2, Seen: 1}) {
		t.Errorf("counts: %+v", got)
	}
	if _, ok := counts[everyone.ID]; ok {
		t.Errorf("counts of a row shown to everyone: %+v", counts)
	}
}

// A row's note, its telling and the titles played it leaves out are
// kept. The note is signed by the administrator who last changed it, and
// a change of anything else keeps its signature. Telling is refused for a
// row shown to everyone, a note too long, and titles played left out by
// a kind that does not offer it.
func TestRowNotesAndSwitches(t *testing.T) {
	t.Parallel()
	f := newTellFixture(t)
	ctx := t.Context()
	row, err := f.store.Create(ctx, Row{Name: "For you", Kind: KindCollection, Collection: accounts.ID{0x01}, Place: PlaceTop,
		Count: DefaultCount, Users: []accounts.ID{f.sam.ID}, Notify: true, LeavePlayed: true, Note: "First note.", NoteBy: f.morgan.ID})
	if err != nil {
		t.Fatal(err)
	}
	if row.Note != "First note." || row.NoteBy != f.morgan.ID || !row.Notify || !row.LeavePlayed {
		t.Errorf("created: %+v", row)
	}
	row.Name, row.NoteBy = "Renamed", f.sam.ID
	if row, err = f.store.Update(ctx, row); err != nil || row.NoteBy != f.morgan.ID {
		t.Errorf("the note kept: %+v, %v", row, err)
	}
	row.Note, row.NoteBy = "Second note.", f.sam.ID
	if row, err = f.store.Update(ctx, row); err != nil || row.NoteBy != f.sam.ID {
		t.Errorf("the note changed: %+v, %v", row, err)
	}
	row.Note = ""
	if row, err = f.store.Update(ctx, row); err != nil || row.NoteBy != (accounts.ID{}) {
		t.Errorf("the note emptied: %+v, %v", row, err)
	}
	base := Row{Name: "Row", Kind: KindNew, Place: PlaceTop, Count: DefaultCount, Users: []accounts.ID{f.sam.ID}}
	for name, change := range map[string]func(*Row){
		"telling everyone":            func(r *Row) { r.Everyone, r.Users, r.Notify = true, nil, true },
		"a note too long":             func(r *Row) { r.Note = strings.Repeat("é", MaxNoteLength+1) },
		"Because you watched, played": func(r *Row) { r.Kind, r.LeavePlayed = KindBecause, true },
	} {
		r := base
		change(&r)
		if _, err := f.store.Create(ctx, r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	long := base
	long.Note = strings.Repeat("é", MaxNoteLength)
	if _, err := f.store.Create(ctx, long); err != nil {
		t.Errorf("a note of %d characters: %v", MaxNoteLength, err)
	}
}
