package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/taste"
)

// tasteServer is a server whose taste profiles are kept, with a library
// of movies of a few genres, in the order of their catalog, and a user who
// may edit metadata. It returns the user, their token, and the movies'
// identifiers by name.
func tasteServer(t *testing.T) (s testServer, tastes *taste.Service, user accounts.User, token string, ids map[string]string) {
	t.Helper()
	s = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		tastes = taste.New(pool, o.Accounts, o.UserData, o.Library, o.Logger)
		o.Taste = tastes
	})
	user = s.user("viewer", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	catalog := &arrivingAddon{}
	for i, movie := range []struct{ name, genre string }{
		{"Comedy A", "Comedy"}, {"Horror A", "Horror"}, {"Drama A", "Drama"}, {"Thriller D", "Thriller"}, {"Comedy B", "Comedy"},
		{"Thriller E", "Thriller"}, {"Horror B", "Horror"}, {"Thriller A", "Thriller"}, {"Thriller B", "Thriller"}, {"Thriller C", "Thriller"},
	} {
		catalog.movies = append(catalog.movies, stremio.Meta{ID: "tt80" + string(rune('0'+i)), Type: "movie", Name: movie.name,
			Genres: stremio.Names{movie.genre}})
	}
	server := httptest.NewServer(catalog)
	t.Cleanup(server.Close)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	token = s.signIn("viewer", "tv")
	var views, listed QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &listed)
	ids = map[string]string{}
	for _, item := range listed.Items {
		ids[item.Name] = item.Id
	}
	if len(ids) != 10 {
		t.Fatalf("the library: %v", itemNames(listed.Items))
	}
	return s, tastes, user, token, ids
}

// A user who finished three thrillers gets the other thrillers first in
// Top picks for you, the titles of a genre they disliked two titles of
// left out, and the row reaches other apps as a library, in the same
// order. Before their profile is confident, the row lists the library's
// titles as its catalog gives them. Suggestions put the thrillers first
// too.
func TestTopPicksFollowTheUsersTaste(t *testing.T) {
	t.Parallel()
	s, tastes, user, token, ids := tasteServer(t)
	row := s.addRow(t, homerows.Row{Name: "Top picks for you", Kind: homerows.KindPicks, Place: homerows.PlaceTop, Everyone: true, Library: true})
	for _, name := range []string{"Thriller A", "Thriller B", "Thriller C"} {
		if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+ids[name], app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("played: %d", status)
		}
	}
	for _, name := range []string{"Horror A", "Horror B"} {
		if status, _ := s.call(http.MethodPost, "/UserItems/"+ids[name]+"/Rating?likes=false", app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("disliked: %d", status)
		}
	}
	popular := []string{"Top picks for you @Top: Comedy A, Horror A, Drama A, Thriller D, Comedy B, Thriller E, Horror B"}
	if got := rowSummary(s.customRowsOf(t, token)); !slices.Equal(got, popular) {
		t.Fatalf("before the profile is built: %q", got)
	}
	if _, err := tastes.Build(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	picks := []string{"Thriller D", "Thriller E", "Comedy A", "Drama A", "Comedy B"}
	if got := rowSummary(s.customRowsOf(t, token)); !slices.Equal(got, []string{"Top picks for you @Top: " + strings.Join(picks, ", ")}) {
		t.Errorf("with the profile: %q", got)
	}
	// Other apps list the row among the libraries, with its first titles
	// as its latest items.
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	folder := customRowID(row.ID, accounts.ID{}).String()
	if !slices.ContainsFunc(views.Items, func(view BaseItemDto) bool { return view.Id == folder && view.Name == "Top picks for you" }) {
		t.Errorf("views: %v", itemNames(views.Items))
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?ParentId="+folder+"&Limit=3", token, &latest); status != http.StatusOK ||
		!slices.Equal(itemNames(latest), picks[:3]) {
		t.Errorf("the row's latest items: %d %v", status, itemNames(latest))
	}
	var suggested QueryResult
	if status := s.get(t, "/Items/Suggestions?type=Movie", token, &suggested); status != http.StatusOK || len(suggested.Items) != 10 ||
		!slices.Contains(itemNames(suggested.Items[:5]), "Thriller D") || !slices.Contains(itemNames(suggested.Items[:5]), "Thriller E") ||
		itemNames(suggested.Items)[9] != "Horror B" && itemNames(suggested.Items)[8] != "Horror B" {
		t.Errorf("suggestions: %d %v", status, itemNames(suggested.Items))
	}
}

// A movie's details tell a user above the confidence gate why it is for
// them, before its own taglines, unless they turned taglines off; a new
// user is told nothing. Saving the movie's metadata from the editor, which
// sends the taglines it was shown, keeps the movie's own only.
func TestTaglineTellsWhyATitleIsForTheUser(t *testing.T) {
	t.Parallel()
	s, tastes, user, token, ids := tasteServer(t)
	newcomer := s.user("newcomer", nil)
	newcomerToken := s.signIn("newcomer", "tv")
	for _, name := range []string{"Thriller A", "Thriller B", "Thriller C"} {
		if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+ids[name], app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("played: %d", status)
		}
	}
	if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+ids["Thriller A"], app("tv", newcomerToken), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	for _, u := range []accounts.User{user, newcomer} {
		if _, err := tastes.Build(t.Context(), u); err != nil {
			t.Fatal(err)
		}
	}
	taglines := func(token string, id accounts.ID) []string {
		t.Helper()
		var dto BaseItemDto
		if status := s.get(t, "/Users/"+id.String()+"/Items/"+ids["Thriller D"], token, &dto); status != http.StatusOK || dto.Taglines == nil {
			t.Fatalf("details: %d", status)
		}
		return *dto.Taglines
	}
	shown := taglines(token, user.ID)
	if len(shown) != 1 || !strings.HasSuffix(shown[0], "% for you · because you like Thriller") {
		t.Fatalf("the user's taglines: %q", shown)
	}
	if got := taglines(newcomerToken, newcomer.ID); len(got) != 0 {
		t.Errorf("the new user's taglines: %q", got)
	}
	edit := editorBody(ids["Thriller D"], map[string]any{"Name": "Thriller D", "Genres": []string{"Thriller"}, "Taglines": []string{shown[0], "Its own"}})
	if status, body := s.call(http.MethodPost, "/Items/"+ids["Thriller D"], app("tv", token), edit); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	if got := taglines(token, user.ID); !slices.Equal(got, []string{shown[0], "Its own"}) {
		t.Errorf("after the edit: %q", got)
	}
	if got := taglines(newcomerToken, newcomer.ID); !slices.Equal(got, []string{"Its own"}) {
		t.Errorf("the new user's after the edit: %q", got)
	}
	if err := tastes.SetTaglines(t.Context(), user.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := taglines(token, user.ID); !slices.Equal(got, []string{"Its own"}) {
		t.Errorf("with taglines off: %q", got)
	}
}
