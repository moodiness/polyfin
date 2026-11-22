package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/userdata"
)

// userIDOf is the identifier of the user signed in with token.
func (s testServer) userIDOf(t *testing.T, token string) accounts.ID {
	t.Helper()
	var me UserDto
	s.get(t, "/Users/Me", token, &me)
	id, err := accounts.ParseID(me.Id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// A row shown to chosen users is a suggestion: GET /Polyfin/HomeRows gives
// its row's identifier, its note and the name of the administrator who
// signs it, and tells it New until the user sees it. Seen and Hidden take
// only a row shown to the user, Seen only one shown to chosen users; a row
// hidden leaves their home page until shown again.
func TestSuggestedRowsAreSeenAndHidden(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, _ := homeRowsServer(t)
	memberID, adminID := s.userIDOf(t, member), s.userIDOf(t, admin)
	suggested := s.addRow(t, homerows.Row{Name: "For you", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Users: []accounts.ID{memberID}, Note: "Watch it this weekend.", NoteBy: adminID})
	everyone := s.addRow(t, homerows.Row{Name: "Everyone", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true, Note: "Unsigned."})
	other := s.addRow(t, homerows.Row{Name: "For the admin", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Users: []accounts.ID{adminID}})
	rows := s.customRowsOf(t, member)
	if len(rows) != 2 {
		t.Fatalf("rows: %q", rowSummary(rows))
	}
	if got := rows[0]; got.RowId != suggested.ID.String() || got.Note != "Watch it this weekend." || got.NoteBy != "admin" || !got.Suggested ||
		!got.New {
		t.Errorf("the suggestion: %+v", got)
	}
	if got := rows[1]; got.RowId != everyone.ID.String() || got.Note != "Unsigned." || got.NoteBy != "" || got.Suggested || got.New {
		t.Errorf("the row of everyone: %+v", got)
	}
	call := func(method, path string) int {
		status, _ := s.call(method, path, app("tv", member), nil)
		return status
	}
	for path, want := range map[string]int{
		"/Polyfin/HomeRows/" + everyone.ID.String() + "/Seen":  http.StatusNotFound,
		"/Polyfin/HomeRows/" + other.ID.String() + "/Seen":     http.StatusNotFound,
		"/Polyfin/HomeRows/" + other.ID.String() + "/Hidden":   http.StatusNotFound,
		"/Polyfin/HomeRows/not-a-row/Seen":                     http.StatusNotFound,
		"/Polyfin/HomeRows/" + suggested.ID.String() + "/Seen": http.StatusNoContent,
	} {
		if got := call(http.MethodPost, path); got != want {
			t.Errorf("POST %s: %d, want %d", path, got, want)
		}
	}
	if rows := s.customRowsOf(t, member); rows[0].New {
		t.Errorf("seen, still New: %+v", rows[0])
	}
	if got := call(http.MethodPost, "/Polyfin/HomeRows/"+suggested.ID.String()+"/Hidden"); got != http.StatusNoContent {
		t.Fatalf("hiding: %d", got)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Everyone @Bottom: Movie 0, Movie 1, Movie 2"}) {
		t.Errorf("hidden: %q", got)
	}
	off, err := homerows.New(s.pool).Off(t.Context(), memberID)
	if err != nil || !off[suggested.ID] {
		t.Errorf("turned off in My account: %v, %v", off, err)
	}
	if got := call(http.MethodDelete, "/Polyfin/HomeRows/"+suggested.ID.String()+"/Hidden"); got != http.StatusNoContent {
		t.Fatalf("showing again: %d", got)
	}
	if got := len(s.customRowsOf(t, member)); got != 2 {
		t.Errorf("shown again: %d rows", got)
	}
}

// A row that leaves out the titles played leaves out, for each user, those
// they started or played, on their home page and in the row's list; other
// users and other rows keep them.
func TestRowsLeaveOutTheTitlesPlayed(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, movies := homeRowsServer(t)
	memberID := s.userIDOf(t, member)
	row := s.addRow(t, homerows.Row{Name: "Unplayed", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true, LeavePlayed: true})
	s.addRow(t, homerows.Row{Name: "All", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true})
	if _, err := s.handler.UserData.Change(t.Context(), memberID, []userdata.Item{{ID: mustID(t, movies[0].Id)}}, func(d *userdata.Data) {
		d.Played = true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.handler.UserData.Change(t.Context(), memberID, []userdata.Item{{ID: mustID(t, movies[2].Id)}}, func(d *userdata.Data) {
		d.Position = 600_000_000_000
	}); err != nil {
		t.Fatal(err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Unplayed @Top: Movie 1", "All @Bottom: Movie 0, Movie 1, Movie 2"}) {
		t.Errorf("the member's rows: %q", got)
	}
	if got := rowSummary(s.customRowsOf(t, admin)); !slices.Equal(got, []string{"Unplayed @Top: Movie 0, Movie 1, Movie 2",
		"All @Bottom: Movie 0, Movie 1, Movie 2"}) {
		t.Errorf("the administrator's rows: %q", got)
	}
	var list QueryResult
	s.get(t, "/Items?ParentId="+customRowID(row.ID, accounts.ID{}).String(), member, &list)
	if got := itemNames(list.Items); !slices.Equal(got, []string{"Movie 1"}) {
		t.Errorf("the row's list: %q", got)
	}
}
