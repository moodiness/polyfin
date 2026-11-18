package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playlists"
)

// refused expects the request to be refused as one that does not validate,
// its answer naming why.
func (s testServer) refused(t *testing.T, method, path, token string, body any, why string) {
	t.Helper()
	status, response := s.call(method, path, app("tv", token), body)
	if status != http.StatusBadRequest || !strings.Contains(string(response), why) {
		t.Errorf("%s %.80s: %d %.300s, want 400 naming %q", method, path, status, response, why)
	}
}

// playlistRows counts the playlists owner has, their watchlist aside, and
// the entries of the playlist id, in the database.
func (s testServer) playlistRows(t *testing.T, owner accounts.ID, id string) (lists, entries int) {
	t.Helper()
	playlist := accounts.ID{}
	if id != "" {
		playlist = mustID(t, id)
	}
	if err := s.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM playlists WHERE owner_id = $1 AND NOT watchlist),
		(SELECT count(*) FROM playlist_entries WHERE playlist_id = $2)`, owner, playlist).Scan(&lists, &entries); err != nil {
		t.Fatal(err)
	}
	return lists, entries
}

// fillPlaylist gives the playlist id count entries, of the titles of
// pattern over and over, as a playlist made before the bounds may hold.
func (s testServer) fillPlaylist(t *testing.T, id string, count int, pattern ...string) {
	t.Helper()
	items := make([]accounts.ID, count)
	for i := range items {
		items[i] = mustID(t, pattern[i%len(pattern)])
	}
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO playlist_entries (playlist_id, item_id, position)
		SELECT $1, item_id, n - 1 FROM unnest($2::uuid[]) WITH ORDINALITY AS i(item_id, n)`, mustID(t, id), items); err != nil {
		t.Fatal(err)
	}
}

// A playlist's name and entries are bounded at each route that sets them,
// from the body and from the query: a longer name, and the entries that
// would pass the bound, a series' episodes among them, are refused as a
// request that does not validate, and nothing of the request is kept.
func TestPlaylistsRefuseANameOrEntriesPastTheirBounds(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	const nameBound, entryBound = "maximum length of 500", "at most 10000 entries"
	// The bound counts characters, not bytes.
	whole := strings.Repeat("é", playlists.MaxNameLength)
	long := whole + "é"
	movie := titles.movies[0]

	s.refused(t, http.MethodPost, "/Playlists", token, map[string]any{"Name": long, "Ids": []string{movie}}, nameBound)
	s.refused(t, http.MethodPost, "/Playlists?name="+url.QueryEscape(long)+"&ids="+movie, token, nil, nameBound)
	over := make([]string, playlists.MaxEntries+1)
	for i := range over {
		over[i] = movie
	}
	s.refused(t, http.MethodPost, "/Playlists", token, map[string]any{"Name": "Long", "Ids": over}, entryBound)
	s.refused(t, http.MethodPost, "/Playlists?name=Long&ids="+strings.Join(over, ","), token, nil, entryBound)
	if lists, _ := s.playlistRows(t, member.ID, ""); lists != 0 {
		t.Fatalf("the refused requests created %d playlists", lists)
	}

	// A name of the bound's length is kept. With room for one entry, two
	// movies are refused whole, and so are a series' three episodes.
	id := s.createPlaylist(t, token, map[string]any{"Name": whole})
	s.fillPlaylist(t, id, playlists.MaxEntries-1, movie)
	s.refused(t, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Name": long, "IsPublic": true}, nameBound)
	s.refused(t, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[1]+","+titles.movies[2], token, nil, entryBound)
	s.refused(t, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.series+"&position=0", token, nil, entryBound)
	s.refused(t, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Name": "Other", "Ids": over}, entryBound)
	var name string
	var open bool
	if err := s.pool.QueryRow(t.Context(), "SELECT name, open_access FROM playlists WHERE id = $1", mustID(t, id)).Scan(&name, &open); err != nil {
		t.Fatal(err)
	}
	if _, entries := s.playlistRows(t, member.ID, id); name != whole || open || entries != playlists.MaxEntries-1 {
		t.Errorf("after the refusals: the name kept is %v, open %v, %d entries, want %d", name == whole, open, entries, playlists.MaxEntries-1)
	}
	// The entry that fits is added, the next one is not.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[1], token, nil)
	s.refused(t, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[2], token, nil, entryBound)
	if _, entries := s.playlistRows(t, member.ID, id); entries != playlists.MaxEntries {
		t.Errorf("the playlist holds %d entries, want %d", entries, playlists.MaxEntries)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Name": "Short", "Ids": []string{titles.series}})
	if got := itemNames(s.playlistEntries(t, token, id)); !slices.Equal(got, []string{"Pilot", "Second", "Return"}) {
		t.Errorf("the playlist replaced by a series: %v", got)
	}
}

// A user owns playlists.MaxPerUser playlists at most, their Watchlist
// aside: one more is refused, whoever asks it for them, until they
// delete one. A user who had more before the bound still lists them all.
func TestPlaylistsOfAUserStopAtTheirBound(t *testing.T) {
	t.Parallel()
	s, token, _ := browsing(t)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	s.user("other", nil)
	other := s.signIn("other", "phone")
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	root := s.signIn("root", "dashboard")
	const countBound = "at most 200 playlists"
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO playlists (owner_id, name) SELECT $1, 'List ' || n FROM generate_series(1, $2) n",
		member.ID, playlists.MaxPerUser+1); err != nil {
		t.Fatal(err)
	}
	var listed QueryResult
	s.get(t, "/Items?includeItemTypes=Playlist&recursive=true&limit=1", token, &listed)
	if listed.TotalRecordCount != playlists.MaxPerUser+2 {
		t.Errorf("the member is listed %d playlists, want %d and the Watchlist", listed.TotalRecordCount, playlists.MaxPerUser+1)
	}
	body := map[string]any{"Name": "One more"}
	s.refused(t, http.MethodPost, "/Playlists", token, body, countBound)
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+s.playlistsOf(t, token)[0].Id, token, nil)
	s.refused(t, http.MethodPost, "/Playlists", token, body, countBound)
	s.refused(t, http.MethodPost, "/Playlists?userId="+member.ID.String(), root, body, countBound)
	if lists, _ := s.playlistRows(t, member.ID, ""); lists != playlists.MaxPerUser {
		t.Fatalf("the member has %d playlists, want %d", lists, playlists.MaxPerUser)
	}
	s.createPlaylist(t, other, body)
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+s.playlistsOf(t, token)[0].Id, token, nil)
	s.createPlaylist(t, token, body)
	s.refused(t, http.MethodPost, "/Playlists", token, body, countBound)
}

// A playlist that held more entries before the bound is opened whole, in
// order, by its owner and by the users who see it, and shortened; it
// takes no entry besides until it is under the bound.
func TestAPlaylistOverItsBoundIsReadAndShortened(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	s.user("viewer", nil)
	viewer := s.signIn("viewer", "phone")
	const held = playlists.MaxEntries + 3
	id := s.createPlaylist(t, token, map[string]any{"Name": "Old", "IsPublic": true})
	s.fillPlaylist(t, id, held, titles.movies[0], titles.movies[1], titles.movies[2])

	var described PlaylistDto
	if status := s.get(t, "/Playlists/"+id, viewer, &described); status != http.StatusOK || len(described.ItemIds) != held {
		t.Fatalf("the playlist opened: %d, %d entries, want %d", status, len(described.ItemIds), held)
	}
	for i, item := range described.ItemIds {
		if want := mustID(t, titles.movies[i%3]).String(); item != want {
			t.Fatalf("entry %d is %s, want %s: the entries are not in order", i, item, want)
		}
	}
	// Its first entries and its last, a page each.
	var entries, end QueryResult
	s.get(t, "/Playlists/"+id+"/Items?Limit=3", viewer, &entries)
	s.get(t, fmt.Sprintf("/Playlists/%s/Items?StartIndex=%d", id, held-2), viewer, &end)
	if entries.TotalRecordCount != held || !slices.Equal(itemNames(entries.Items), []string{"Movie 0", "Movie 1", "Movie 2"}) ||
		!slices.Equal(itemNames(end.Items), []string{fmt.Sprintf("Movie %d", (held-2)%3), fmt.Sprintf("Movie %d", (held-1)%3)}) {
		t.Fatalf("the playlist lists %d entries, want %d: the first %v, the last %v", entries.TotalRecordCount, held,
			itemNames(entries.Items), itemNames(end.Items))
	}
	listed := s.playlistsOf(t, viewer)
	if at := slices.IndexFunc(listed, func(dto BaseItemDto) bool { return dto.Id == id }); at < 0 || *listed[at].ChildCount != held {
		t.Errorf("the playlist is not listed with its %d entries: %v", held, itemNames(listed))
	}

	const entryBound = "at most 10000 entries"
	s.refused(t, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[0], token, nil, entryBound)
	// Removing leaves it over the bound: it still takes none, and a
	// replacement by what it held before is one that grows it.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete,
		"/Playlists/"+id+"/Items?entryIds="+entries.Items[1].PlaylistItemId+","+entries.Items[2].PlaylistItemId, token, nil)
	if _, kept := s.playlistRows(t, member.ID, id); kept != held-2 {
		t.Fatalf("%d entries after two were removed, want %d", kept, held-2)
	}
	s.refused(t, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[0], token, nil, entryBound)
	same := make([]string, held)
	for i := range same {
		same[i] = titles.movies[2]
	}
	s.refused(t, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Ids": same}, entryBound)
	// Replaced by fewer, it is under the bound and takes entries again.
	// (The playlists package's tests replace one by as many as it holds.)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token,
		map[string]any{"Name": "Older", "Ids": []string{titles.movies[1]}})
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[0], token, nil)
	if got := itemNames(s.playlistEntries(t, token, id)); !slices.Equal(got, []string{"Movie 1", "Movie 0"}) {
		t.Errorf("the shortened playlist: %v", got)
	}
}

// entryReads counts the entries of playlists a pool reads: the rows
// answered to its statements that list them.
type entryReads struct {
	rows atomic.Int64
}

type entryReadKey struct{}

// listsEntries matches a statement that lists rows of playlist_entries,
// not one that counts them or asks whether a playlist has any.
var listsEntries = regexp.MustCompile(`(?s)^\s*SELECT [^()]* FROM playlist_entries\b`)

func (c *entryReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, entryReadKey{}, listsEntries.MatchString(data.SQL))
}

func (c *entryReads) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if lists, _ := ctx.Value(entryReadKey{}).(bool); lists {
		c.rows.Add(data.CommandTag.RowsAffected())
	}
}

// A listing of playlists reads the entries of the playlists it answers,
// not of every playlist the user may see: those of its page for the
// Playlists view, of the playlists found for a search, of the user's
// favorites for a listing of favorites. The playlists another user opened
// to everyone cost it a row each. Opening one playlist still gives every
// entry, in order.
func TestAListingOfPlaylistsReadsTheEntriesOfThoseItAnswers(t *testing.T) {
	t.Parallel()
	reads := &entryReads{}
	s, member, views := browsingOn(t, newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		config := pool.Config()
		config.ConnConfig.Tracer = reads
		counted, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(counted.Close)
		o.Playlists = playlists.New(counted)
	}))
	titles := titlesForPlaylists(t, s, member, views)
	s.user("viewer", nil)
	viewer := s.signIn("viewer", "phone")
	entries := []string{titles.movies[1], titles.movies[0], titles.movies[1], titles.movies[2]}
	open := map[string]string{}
	for _, name := range []string{"Open 1", "Open 2", "Open 3"} {
		open[name] = s.createPlaylist(t, member, map[string]any{"Name": name, "Ids": entries, "IsPublic": true})
	}
	s.createPlaylist(t, member, map[string]any{"Name": "Open 0, kept private", "Ids": entries})

	// listing answers the viewer path, with the entries read for it.
	listing := func(path string) (QueryResult, int64) {
		t.Helper()
		reads.rows.Store(0)
		var page QueryResult
		if status := s.get(t, path, viewer, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return page, reads.rows.Load()
	}
	// counts are the entries each playlist listed shows.
	counts := func(page QueryResult) []int {
		result := make([]int, len(page.Items))
		for i, item := range page.Items {
			result[i] = *item.ChildCount
		}
		return result
	}
	view := "/Items?ParentId=" + views["Playlists"] + "&SortBy=SortName"
	// The viewer sees the three open playlists and their own Watchlist.
	page, read := listing(view + "&Limit=1")
	if page.TotalRecordCount != 4 || !slices.Equal(itemNames(page.Items), []string{"Open 1"}) || !slices.Equal(counts(page), []int{4}) {
		t.Fatalf("the first page: %v with %v entries, of %d playlists", itemNames(page.Items), counts(page), page.TotalRecordCount)
	}
	if read != 4 {
		t.Errorf("a page of one playlist of 4 entries read %d entries", read)
	}
	page, read = listing(view + "&StartIndex=1&Limit=2")
	if !slices.Equal(itemNames(page.Items), []string{"Open 2", "Open 3"}) || !slices.Equal(counts(page), []int{4, 4}) || read != 8 {
		t.Errorf("the second page: %v with %v entries, %d entries read, want 8", itemNames(page.Items), counts(page), read)
	}
	page, read = listing(view)
	if len(page.Items) != 4 || !slices.Equal(counts(page)[:3], []int{4, 4, 4}) || read != 12 {
		t.Errorf("the whole view: %v with %v entries, %d entries read, want 12", itemNames(page.Items), counts(page), read)
	}
	page, read = listing("/Items?searchTerm=open+2&includeItemTypes=Playlist&recursive=true&limit=25")
	if !slices.Equal(itemNames(page.Items), []string{"Open 2"}) || !slices.Equal(counts(page), []int{4}) || read != 4 {
		t.Errorf("the search: %v with %v entries, %d entries read, want 4", itemNames(page.Items), counts(page), read)
	}
	if status, body := s.call(http.MethodPost, "/UserFavoriteItems/"+open["Open 3"], app("phone", viewer), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d %s", status, body)
	}
	page, read = listing("/Items?Filters=IsFavorite&Recursive=true&IncludeItemTypes=Playlist&Limit=20")
	if !slices.Equal(itemNames(page.Items), []string{"Open 3"}) || !slices.Equal(counts(page), []int{4}) || read != 4 {
		t.Errorf("the favorites: %v with %v entries, %d entries read, want 4", itemNames(page.Items), counts(page), read)
	}

	// Opening a playlist reads it whole.
	reads.rows.Store(0)
	var described PlaylistDto
	want := make([]string, len(entries))
	for i, entry := range entries {
		want[i] = mustID(t, entry).String()
	}
	status, body := s.call(http.MethodGet, "/Playlists/"+open["Open 2"], app("phone", viewer), nil)
	if status != http.StatusOK || json.Unmarshal(body, &described) != nil || !slices.Equal(described.ItemIds, want) {
		t.Errorf("the playlist opened: %d %s, want the entries %v", status, body, want)
	}
	if read := reads.rows.Load(); read != 4 {
		t.Errorf("opening a playlist of 4 entries read %d entries", read)
	}
	if got := itemNames(s.playlistEntries(t, viewer, open["Open 2"])); !slices.Equal(got, []string{"Movie 1", "Movie 0", "Movie 1", "Movie 2"}) {
		t.Errorf("the playlist's entries: %v", got)
	}
}
