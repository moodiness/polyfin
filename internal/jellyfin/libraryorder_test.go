package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// jellyfin-web's Settings › Home saves the order of every library each
// time it is saved. An order saved as the server lists it, by the user or
// by an administrator for them, is not kept: the user goes on following
// the order of Content › Libraries, libraries added later at their place.
// An order the user changed is kept and wins, a library added later
// coming after the ones ordered, as in Jellyfin.
func TestLibraryOrderSavedAsListedFollowsTheServer(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	users := map[string]accounts.User{}
	for _, name := range []string{"saver", "other", "own"} {
		users[name] = s.user(name, nil)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), catalogAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	library := func(id string) addons.LibraryChoice {
		types := map[string]string{"top": "movie", "shows": "series", "groups": "collection"}
		return addons.LibraryChoice{AddonID: addon.ID, CatalogType: types[id], CatalogID: id}
	}
	order := func(ids ...string) {
		t.Helper()
		var choices []addons.LibraryChoice
		for _, id := range ids {
			choices = append(choices, library(id))
		}
		if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
			t.Fatal(err)
		}
	}
	order("top", "shows")
	tokens := map[string]string{}
	for _, name := range []string{"saver", "other", "own", "admin"} {
		tokens[name] = s.signIn(name, "tv")
	}
	listed := func(name string) (names, ids []string) {
		t.Helper()
		var page QueryResult
		s.get(t, "/UserViews?includeHidden=true", tokens[name], &page)
		return itemNames(page.Items), itemIDs(page.Items)
	}
	names, ids := listed("saver")
	if !slices.Equal(names, []string{"Top", "Shows"}) {
		t.Fatalf("libraries before: %v", names)
	}
	save := func(path, token string, ordered []string) {
		t.Helper()
		if status := s.configure(t, path, token, map[string]any{"OrderedViews": ordered}); status != http.StatusNoContent {
			t.Fatalf("saving %s: %d", path, status)
		}
	}
	// As Settings › Home saves it unchanged, and as an administrator saves it
	// for another user; the third user moves Shows first.
	save("/Users/Configuration", tokens["saver"], ids)
	save("/Users/"+users["other"].ID.String()+"/Configuration", tokens["admin"], ids)
	save("/Users/Configuration", tokens["own"], []string{ids[1], ids[0]})
	for name, want := range map[string]int{"saver": 0, "other": 0, "own": 2} {
		if dto, _ := s.me(t, tokens[name]); len(dto.Configuration.OrderedViews) != want {
			t.Errorf("%s keeps %v, want %d", name, dto.Configuration.OrderedViews, want)
		}
	}

	// The administrator adds Groups first and moves Top last.
	order("groups", "shows", "top")
	for name, want := range map[string][]string{
		"saver": {"Groups", "Shows", "Top"},
		"other": {"Groups", "Shows", "Top"},
		"own":   {"Shows", "Top", "Groups"},
	} {
		if got, _ := listed(name); !slices.Equal(got, want) {
			t.Errorf("%s after the administrator's order: %v, want %v", name, got, want)
		}
	}
}
