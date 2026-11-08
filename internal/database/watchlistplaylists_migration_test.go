package database

import (
	"context"
	"io/fs"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// watchlistPlaylistsState is what the migration to watchlist playlists
// leaves: each user's watchlist entries in order, what waits for its
// entries, the latest first, the ledger's rows, and the playlists.
type watchlistPlaylistsState struct {
	entries, pending map[string][]accounts.ID
	ledger           int
	watchlists       map[string]int
	playlists        int
}

func readWatchlistPlaylists(t *testing.T, pool *pgxpool.Pool) watchlistPlaylistsState {
	t.Helper()
	ctx := t.Context()
	s := watchlistPlaylistsState{entries: map[string][]accounts.ID{}, pending: map[string][]accounts.ID{}, watchlists: map[string]int{}}
	var name string
	var id accounts.ID
	rows, _ := pool.Query(ctx, `SELECT u.name, e.item_id FROM playlist_entries e JOIN playlists p ON p.id = e.playlist_id
		JOIN users u ON u.id = p.owner_id WHERE p.watchlist AND e.title_id = e.item_id ORDER BY u.name, e.position`)
	if _, err := pgx.ForEachRow(rows, []any{&name, &id}, func() error {
		s.entries[name] = append(s.entries[name], id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ = pool.Query(ctx, `SELECT u.name, q.item_id FROM watchlist_pending q JOIN users u ON u.id = q.user_id
		ORDER BY u.name, q.added_at DESC, q.item_id`)
	if _, err := pgx.ForEachRow(rows, []any{&name, &id}, func() error {
		s.pending[name] = append(s.pending[name], id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	rows, _ = pool.Query(ctx, "SELECT u.name, count(p.id) FROM users u LEFT JOIN playlists p ON p.owner_id = u.id AND p.watchlist AND p.name IS NULL GROUP BY u.name")
	if _, err := pgx.ForEachRow(rows, []any{&name, &count}, func() error {
		s.watchlists[name] = count
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM watchlist), (SELECT count(*) FROM playlists)").Scan(&s.ledger, &s.playlists); err != nil {
		t.Fatal(err)
	}
	return s
}

// The migration gives every user a watchlist playlist, and the trigger it
// adds gives each new user one. What the Watchlist view showed goes on:
// the movies not started become entries, the latest added first; the
// other titles wait for their entries; the ledger stays whole, and the
// view's images go. Running it again changes nothing. A member with
// hundreds of series whose episodes are not known yet gets them as they
// become known, through ExpandAll.
func TestWatchlistPlaylistsMigrationCarriesTheWatchlistsOver(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "watchlist_playlists" })
	if at < 0 {
		t.Fatal("no watchlist_playlists migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}

	users := map[string]accounts.ID{}
	for _, name := range []string{"alice", "bob", "carol"} {
		var id accounts.ID
		if err := pool.QueryRow(ctx, "INSERT INTO users (name, password_hash) VALUES ($1, 'x') RETURNING id", name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users[name] = id
	}
	title := func(n int) accounts.ID { return accounts.ID{0: 0xa0, 14: byte(n >> 8), 15: byte(n)} }
	item := func(n int, kind string) accounts.ID {
		t.Helper()
		id := title(n)
		if _, err := pool.Exec(ctx, "INSERT INTO items (id, key, kind, data) VALUES ($1, $2, $3, '{}')", id, id.String(), kind); err != nil {
			t.Fatal(err)
		}
		return id
	}
	day := func(d int) time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, d) }
	hold := func(user string, id accounts.ID, list string, d int) {
		t.Helper()
		if _, err := pool.Exec(ctx, "INSERT INTO watchlist (user_id, item_id, list, added_at) VALUES ($1, $2, $3, $4)",
			users[user], id, list, day(d)); err != nil {
			t.Fatal(err)
		}
	}
	// Alice holds two movies, the later on two lists; a movie she watched,
	// one she started, a series, an episode she added by hand and a title
	// the library does not know.
	old, later, watched, started := item(1, "movie"), item(2, "movie"), item(3, "movie"), item(4, "movie")
	series, episode, unknown := item(5, "series"), item(6, "episode"), title(7)
	hold("alice", old, "tracking:trakt", 1)
	hold("alice", later, "tracking:trakt", 2)
	hold("alice", later, "own", 6)
	hold("alice", watched, "tracking:trakt", 7)
	hold("alice", started, "own", 8)
	hold("alice", series, "tracking:trakt", 4)
	hold("alice", episode, "own", 3)
	hold("alice", unknown, "import", 5)
	if _, err := pool.Exec(ctx, `INSERT INTO user_data (user_id, item_id, played, position_ticks) VALUES ($1, $2, true, 0), ($1, $3, false, 600000000)`,
		users["alice"], watched, started); err != nil {
		t.Fatal(err)
	}
	// Bob holds hundreds of series none of whose episodes are known yet,
	// and a movie.
	const many = 400
	bobs := make([]accounts.ID, many)
	for i := range many {
		bobs[i] = item(100+i, "series")
		hold("bob", bobs[i], "app:stremio", i)
	}
	movie := item(99, "movie")
	hold("bob", movie, "app:stremio", many)
	// Alice's own playlist, and her image of the Watchlist view.
	var mix accounts.ID
	if err := pool.QueryRow(ctx, "INSERT INTO playlists (owner_id, name) VALUES ($1, 'Mix') RETURNING id", users["alice"]).Scan(&mix); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO playlist_entries (playlist_id, item_id, position) VALUES ($1, $2, 0), ($1, $3, 1)", mix, later, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO personal_images (user_id, view_id, name, titles, posters) VALUES ($1, gen_random_uuid(), 'Watchlist', '{}', '{}')`,
		users["alice"]); err != nil {
		t.Fatal(err)
	}

	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	got := readWatchlistPlaylists(t, pool)
	if want := map[string]int{"alice": 1, "bob": 1, "carol": 1}; !maps.Equal(got.watchlists, want) {
		t.Errorf("watchlists: %v, want %v", got.watchlists, want)
	}
	if want := []accounts.ID{later, old}; !slices.Equal(got.entries["alice"], want) || len(got.entries["carol"]) != 0 {
		t.Errorf("entries: %v, want Alice's %v", got.entries, want)
	}
	if want := []accounts.ID{unknown, series, episode}; !slices.Equal(got.pending["alice"], want) {
		t.Errorf("Alice's waiting titles: %v, want %v", got.pending["alice"], want)
	}
	if want := append(slices.Clone(bobs), movie); len(got.pending["bob"]) != many || !slices.Equal(got.entries["bob"], want[many:]) {
		t.Errorf("Bob: %d waiting, entries %v", len(got.pending["bob"]), got.entries["bob"])
	}
	if got.ledger != 9+many {
		t.Errorf("%d rows in the ledger, want %d", got.ledger, 9+many)
	}
	var mixItems []accounts.ID
	rows, _ := pool.Query(ctx, "SELECT item_id FROM playlist_entries WHERE playlist_id = $1 AND title_id IS NULL ORDER BY position", mix)
	if mixItems, err = pgx.CollectRows(rows, pgx.RowTo[accounts.ID]); err != nil || !slices.Equal(mixItems, []accounts.ID{later, old}) {
		t.Errorf("Alice's playlist: %v %v", mixItems, err)
	}
	var images int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM personal_images").Scan(&images); err != nil || images != 0 {
		t.Errorf("%d images of the view left, %v", images, err)
	}

	// A new user gets a watchlist; an image of a playlist outlasts the
	// migration run again, which changes nothing else.
	if _, err := pool.Exec(ctx, "INSERT INTO users (name, password_hash) VALUES ('dave', 'x')"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO personal_images (user_id, view_id, playlist_id, name, titles, posters) VALUES ($1, $2, $2, 'Mix', '{}', '{}')`,
		users["bob"], mix); err != nil {
		t.Fatal(err)
	}
	before := readWatchlistPlaylists(t, pool)
	if before.watchlists["dave"] != 1 {
		t.Errorf("a new user's watchlists: %d", before.watchlists["dave"])
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, migrations[at].sql)
		return err
	}); err != nil {
		t.Fatalf("run again: %v", err)
	}
	again := readWatchlistPlaylists(t, pool)
	if !slices.Equal(again.entries["alice"], before.entries["alice"]) || !slices.Equal(again.pending["alice"], before.pending["alice"]) ||
		len(again.pending["bob"]) != many || again.ledger != before.ledger || again.playlists != before.playlists ||
		!maps.Equal(again.watchlists, before.watchlists) {
		t.Errorf("run again: %+v, before %+v", again, before)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM personal_images").Scan(&images); err != nil || images != 1 {
		t.Errorf("%d images after running again, %v", images, err)
	}

	// Bob's series get their episodes as they become known, in the
	// background, the latest added first.
	known := &episodesKnown{series: map[accounts.ID][]accounts.ID{}}
	for _, s := range bobs {
		known.air(s, nil)
	}
	began := time.Now()
	now := began
	store := watchlist.New(pool).WithEntries(known.entries).WithClock(func() time.Time { return now })
	if err := store.ExpandAll(ctx); err != nil {
		t.Fatal(err)
	}
	episodesOf := func(series accounts.ID) []accounts.ID {
		ids := make([]accounts.ID, 2)
		for n := range ids {
			ids[n] = series
			ids[n][0], ids[n][1] = 0xe0, byte(n+1)
		}
		return ids
	}
	anHourLater := func() { now = now.Add(61 * time.Minute) }
	for _, s := range bobs[:many/2] {
		known.air(s, episodesOf(s))
	}
	anHourLater()
	if err := store.ExpandAll(ctx); err != nil {
		t.Fatal(err)
	}
	var want []accounts.ID
	for i := many/2 - 1; i >= 0; i-- {
		want = append(want, episodesOf(bobs[i])...)
	}
	want = append(want, movie)
	if entries, err := store.Titles(ctx, users["bob"]); err != nil || !slices.Equal(entries, want) {
		t.Fatalf("Bob's entries once half known: %d, %v", len(entries), err)
	}
	for _, s := range bobs[many/2:] {
		known.air(s, episodesOf(s))
	}
	anHourLater()
	if err := store.ExpandAll(ctx); err != nil {
		t.Fatal(err)
	}
	var rest []accounts.ID
	for i := many - 1; i >= many/2; i-- {
		rest = append(rest, episodesOf(bobs[i])...)
	}
	if entries, err := store.Titles(ctx, users["bob"]); err != nil || !slices.Equal(entries, append(rest, want...)) {
		t.Fatalf("Bob's entries once all known: %d, %v", len(entries), err)
	}
	if after := readWatchlistPlaylists(t, pool); len(after.pending["bob"]) != 0 {
		t.Errorf("Bob still waits for %d titles", len(after.pending["bob"]))
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Errorf("expanding took %v", took)
	}
}

// episodesKnown tells what titles make, as the library does: a series the
// episodes aired, nothing while none is known; anything else itself.
type episodesKnown struct {
	mu     sync.Mutex
	series map[accounts.ID][]accounts.ID
}

func (k *episodesKnown) air(series accounts.ID, episodes []accounts.ID) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.series[series] = episodes
}

func (k *episodesKnown) entries(_ context.Context, _, title accounts.ID) (watchlist.Made, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	episodes, isSeries := k.series[title]
	switch {
	case !isSeries:
		return watchlist.Made{Items: []accounts.ID{title}, Known: true}, nil
	case len(episodes) == 0:
		return watchlist.Made{}, nil
	}
	return watchlist.Made{Items: episodes, Series: title, Known: true}, nil
}
