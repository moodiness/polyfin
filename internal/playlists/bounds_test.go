package playlists

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
)

// repeated returns count titles, the titles of pattern over and over.
func repeated(count int, pattern ...accounts.ID) []accounts.ID {
	result := make([]accounts.ID, count)
	for i := range result {
		result[i] = pattern[i%len(pattern)]
	}
	return result
}

// fill gives the playlist id count entries, of the titles of pattern over
// and over, as a playlist made before the bounds may hold.
func fill(t *testing.T, store *Store, id accounts.ID, count int, pattern ...accounts.ID) {
	t.Helper()
	if _, err := store.db.Exec(t.Context(), `DELETE FROM playlist_entries WHERE playlist_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(t.Context(), `INSERT INTO playlist_entries (playlist_id, item_id, position)
		SELECT $1, item_id, n - 1 FROM unnest($2::uuid[]) WITH ORDINALITY AS i(item_id, n)`, id, repeated(count, pattern...)); err != nil {
		t.Fatal(err)
	}
}

// A playlist's name stops at MaxNameLength characters and its entries at
// MaxEntries, at creation and at each change: what would pass a bound is
// refused whole, and the playlist stays as it was.
func TestAPlaylistKeepsToItsBoundsOfNameAndEntries(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	owner := createUser(t, users, "owner")
	ctx := t.Context()
	// The bound counts characters, not bytes.
	whole := strings.Repeat("é", MaxNameLength)
	if _, err := store.Create(ctx, Draft{Owner: owner.ID, Name: whole + "é"}); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("a name over the bound: %v", err)
	}
	if _, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Long", Items: repeated(MaxEntries+1, item(1))}); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("entries over the bound: %v", err)
	}
	if lists, err := store.Visible(ctx, owner.ID); err != nil || len(lists) != 1 || !lists[0].Watchlist {
		t.Fatalf("the refused playlists left the owner %d playlists, want the watchlist alone (%v)", len(lists), err)
	}
	id, err := store.Create(ctx, Draft{Owner: owner.ID, Name: whole, Items: repeated(MaxEntries-1, item(1))})
	if err != nil {
		t.Fatalf("a playlist within the bounds: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Name: new(whole + "é"), OpenAccess: new(true)}); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("renamed over the bound: %v", err)
	}
	// Two more would pass the bound: neither is added.
	if err := store.Add(ctx, id, []accounts.ID{item(2), item(3)}, 0); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("two entries where one fits: %v", err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(2)}, 0); err != nil {
		t.Errorf("the entry that fits: %v", err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(3)}, 0); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("the entry past the bound: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Name: new("Other"), Items: new(repeated(MaxEntries+1, item(4)))}); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("replaced by entries over the bound: %v", err)
	}
	p, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != whole || p.OpenAccess || len(p.Entries) != MaxEntries || p.Entries[0].Item != item(2) || p.Entries[1].Item != item(1) {
		t.Errorf("after the refusals: a name of %d characters, open %v, %d entries, the first of %v",
			utf8.RuneCountInString(p.Name), p.OpenAccess, len(p.Entries), p.Entries[0].Item)
	}
	if err := store.Update(ctx, id, Changes{Items: new(repeated(MaxEntries, item(5)))}); err != nil {
		t.Errorf("replaced by as many entries as the bound: %v", err)
	}
}

// A user owns MaxPerUser playlists at most, their watchlist aside: one
// more is refused until one is deleted, and other users keep their room.
func TestAUserOwnsABoundedNumberOfPlaylists(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	owner, other := createUser(t, users, "owner"), createUser(t, users, "other")
	ctx := t.Context()
	if _, err := store.db.Exec(ctx, "INSERT INTO playlists (owner_id, name) SELECT $1, 'List ' || n FROM generate_series(1, $2) n",
		owner.ID, MaxPerUser-1); err != nil {
		t.Fatal(err)
	}
	last, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Last"})
	if err != nil {
		t.Fatalf("the last playlist within the bound: %v", err)
	}
	if _, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "One more", Items: []accounts.ID{item(1)}}); !errors.Is(err, ErrTooMany) {
		t.Errorf("a playlist past the bound: %v", err)
	}
	if lists, err := store.Visible(ctx, owner.ID); err != nil || len(lists) != MaxPerUser+1 {
		t.Errorf("the owner has %d playlists, want %d and the watchlist (%v)", len(lists), MaxPerUser, err)
	}
	if _, err := store.Create(ctx, Draft{Owner: other.ID, Name: "Mine"}); err != nil {
		t.Errorf("another user's playlist: %v", err)
	}
	if err := store.Delete(ctx, last); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "One more"}); err != nil {
		t.Errorf("a playlist once another is deleted: %v", err)
	}
}

// A playlist that held more entries before the bound is read whole, in
// order, and shortened: entries are removed, and replaced by no more than
// it holds. It takes none besides until it is under the bound.
func TestAPlaylistOverTheBoundIsShortenedNotGrown(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	owner := createUser(t, users, "owner")
	ctx := t.Context()
	id, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	const held = MaxEntries + 4
	fill(t, store, id, held, item(1), item(2), item(3))
	p, err := store.Get(ctx, id)
	if err != nil || len(p.Entries) != held {
		t.Fatalf("read %d entries, want %d (%v)", len(p.Entries), held, err)
	}
	if got := items(t, store, id); !slices.Equal(got, repeated(held, item(1), item(2), item(3))) {
		t.Errorf("the entries are not read in order: the first are %v", got[:4])
	}
	if err := store.Add(ctx, id, []accounts.ID{item(4)}, 0); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("an entry added over the bound: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Items: new(repeated(held+1, item(4)))}); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("replaced by more entries than it held: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Name: new("Older"), Items: new(repeated(held, item(3), item(1)))}); err != nil {
		t.Errorf("replaced by as many entries, in another order: %v", err)
	}
	if p, err = store.Get(ctx, id); err != nil || p.Name != "Older" || len(p.Entries) != held {
		t.Fatalf("after the replacement: %q with %d entries (%v)", p.Name, len(p.Entries), err)
	}
	if err := store.Remove(ctx, id, []accounts.ID{p.Entries[0].ID}); err != nil {
		t.Errorf("an entry removed: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Items: new(repeated(held, item(5)))}); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("replaced by the entries it held before one was removed: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Items: new(repeated(MaxEntries+2, item(5)))}); err != nil {
		t.Errorf("replaced by fewer entries, still over the bound: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Items: new(repeated(MaxEntries+3, item(5)))}); !errors.Is(err, ErrTooManyEntries) {
		t.Errorf("grown again over the bound: %v", err)
	}
	if err := store.Update(ctx, id, Changes{Items: new([]accounts.ID{item(6)})}); err != nil {
		t.Fatalf("shortened under the bound: %v", err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(7)}, 1); err != nil {
		t.Errorf("an entry added under the bound: %v", err)
	}
	if got := items(t, store, id); !slices.Equal(got, []accounts.ID{item(6), item(7)}) {
		t.Errorf("the shortened playlist: %v", got)
	}
}

// A listing reads a row for each playlist the user may see: no entry,
// whether there are any, and the user's own share, which tells what they
// may do. The entries are read for the playlists asked, in order, and the
// titles of playlists each once.
func TestAListingReadsPlaylistsWithoutTheirEntries(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	owner, editor, viewer, stranger := createUser(t, users, "owner"), createUser(t, users, "editor"), createUser(t, users, "viewer"),
		createUser(t, users, "stranger")
	ctx := t.Context()
	mix, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Mix", Items: []accounts.ID{item(2), item(1), item(2)},
		Shares: []Share{{User: viewer.ID}, {User: editor.ID, CanEdit: true}}})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Empty", OpenAccess: true})
	if err != nil {
		t.Fatal(err)
	}
	// listed returns the playlists user is listed, their watchlist aside.
	listed := func(user accounts.User) []Playlist {
		t.Helper()
		lists, err := store.Visible(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(lists, func(p Playlist) bool { return p.Watchlist })
	}
	for name, tc := range map[string]struct {
		user     accounts.User
		shares   []Share
		editable bool
	}{
		"owner":  {owner, nil, true},
		"editor": {editor, []Share{{User: editor.ID, CanEdit: true}}, true},
		"viewer": {viewer, []Share{{User: viewer.ID}}, false},
	} {
		lists := listed(tc.user)
		if len(lists) != 2 || lists[0].ID != empty || lists[1].ID != mix {
			t.Fatalf("%s is listed %+v, want Empty then Mix", name, lists)
		}
		if p := lists[1]; p.Entries != nil || !p.HasEntries || !slices.Equal(p.Shares, tc.shares) || !p.Visible(tc.user.ID) ||
			p.Editable(tc.user.ID) != tc.editable || p.Name != "Mix" || p.Owner != owner.ID || p.LastAdded == nil {
			t.Errorf("%s is listed Mix as %+v", name, p)
		}
		if p := lists[0]; p.Entries != nil || p.HasEntries || !p.OpenAccess || p.LastAdded != nil {
			t.Errorf("%s is listed Empty as %+v", name, p)
		}
	}
	lists := listed(stranger)
	if len(lists) != 1 || lists[0].ID != empty || len(lists[0].Shares) != 0 || lists[0].Editable(stranger.ID) {
		t.Fatalf("a stranger is listed %+v, want Empty alone", lists)
	}

	lists = listed(viewer)
	if err := store.LoadEntries(ctx, lists[1:]); err != nil {
		t.Fatal(err)
	}
	read := make([]accounts.ID, len(lists[1].Entries))
	for i, entry := range lists[1].Entries {
		read[i] = entry.Item
	}
	if !slices.Equal(read, []accounts.ID{item(2), item(1), item(2)}) || !lists[1].HasEntries || lists[0].Entries != nil {
		t.Errorf("the entries read for Mix: %v, for Empty: %v", read, lists[0].Entries)
	}
	if err := store.LoadEntries(ctx, lists); err != nil || len(lists[1].Entries) != 3 || lists[0].HasEntries {
		t.Errorf("read again with Empty: %d entries for Mix, Empty has some: %v (%v)", len(lists[1].Entries), lists[0].HasEntries, err)
	}
	titles, err := store.Titles(ctx, []accounts.ID{mix, empty})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(titles[mix], func(a, b accounts.ID) int { return slices.Compare(a[:], b[:]) })
	if len(titles) != 1 || !slices.Equal(titles[mix], []accounts.ID{item(1), item(2)}) {
		t.Errorf("the titles of Mix and Empty: %v", titles)
	}
}

// An import keeps to the bounds as well: a longer name is cut, the
// entries stop at the bound, at creation and when a later import adds to
// the playlist, and a user who has every playlist they may own gets none
// created, which the import reports as it reports one without titles.
func TestAnImportKeepsToTheBounds(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	owner := createUser(t, users, "owner")
	ctx := t.Context()
	many := make([]accounts.ID, MaxEntries+5)
	for i := range many {
		many[i] = accounts.ID{0: 1, 14: byte(i >> 8), 15: byte(i)}
	}
	id, created, _, err := store.Import(ctx, "jellyfin:abc", "long", owner.ID, strings.Repeat("é", MaxNameLength+40), many[:MaxEntries-2])
	if err != nil || !created {
		t.Fatalf("the import: created %v, %v", created, err)
	}
	p, err := store.Get(ctx, id)
	if err != nil || p.Name != strings.Repeat("é", MaxNameLength) || len(p.Entries) != MaxEntries-2 {
		t.Fatalf("imported under a name of %d characters with %d entries (%v)", utf8.RuneCountInString(p.Name), len(p.Entries), err)
	}
	// Seven titles are new, two fit.
	if _, created, added, err := store.Import(ctx, "jellyfin:abc", "long", owner.ID, "Long", many); err != nil || created || added != 2 {
		t.Errorf("imported again: created %v, %d added, want 2 (%v)", created, added, err)
	}
	if got := items(t, store, id); !slices.Equal(got, many[:MaxEntries]) {
		t.Errorf("the playlist holds %d entries after the second import, want the first %d", len(got), MaxEntries)
	}
	id, created, _, err = store.Import(ctx, "jellyfin:abc", "longer", owner.ID, "Longer", many)
	if err != nil || !created || len(items(t, store, id)) != MaxEntries {
		t.Errorf("a longer playlist imported: created %v, %d entries (%v)", created, len(items(t, store, id)), err)
	}

	if _, err := store.db.Exec(ctx, "INSERT INTO playlists (owner_id, name) SELECT $1, 'List ' || n FROM generate_series(1, $2) n",
		owner.ID, MaxPerUser-2); err != nil {
		t.Fatal(err)
	}
	id, created, added, err := store.Import(ctx, "jellyfin:abc", "one-more", owner.ID, "One more", many[:3])
	if err != nil || created || added != 0 || id != (accounts.ID{}) {
		t.Errorf("imported past the user's bound: %v, created %v, %d added (%v)", id, created, added, err)
	}
	if lists, err := store.Visible(ctx, owner.ID); err != nil || len(lists) != MaxPerUser+1 {
		t.Errorf("the owner has %d playlists, want %d and the watchlist (%v)", len(lists), MaxPerUser, err)
	}
}
