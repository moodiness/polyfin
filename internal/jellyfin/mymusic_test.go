package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/eclipse"
)

func TestMyMusicListsWhatEachUserPlayedOrMarkedFavorite(t *testing.T) {
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, "")
	addon.rowless = true
	token, _, views := listening(t, s, addon.url)
	// An addon without rows gets My music as its library, named in the
	// server's language.
	mine := views[eclipse.MyMusicName]
	if len(views) != 1 || mine == "" {
		t.Fatalf("views: %v", views)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Language = "fr" })
	var french QueryResult
	if s.get(t, "/UserViews", token, &french); len(french.Items) != 1 || french.Items[0].Name != "Ma musique" {
		t.Errorf("in French: %+v", french.Items)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Language = "en" })
	list := func(token, path string) []string {
		t.Helper()
		var result QueryResult
		s.get(t, path, token, &result)
		return itemNames(result.Items)
	}
	ids := func(path string) map[string]string {
		t.Helper()
		var result QueryResult
		s.get(t, path, token, &result)
		found := map[string]string{}
		for _, item := range result.Items {
			found[item.Name] = item.Id
		}
		return found
	}
	if got := list(token, "/Items?ParentId="+mine); len(got) != 0 {
		t.Errorf("before listening: %v", got)
	}
	// The music is found through the addon's search.
	artists := ids("/Artists?searchTerm=tone")
	albums := ids("/Items?ParentId=" + artists["Tone Quartet"])
	tracks := ids("/Items?ParentId=" + albums["Sine Studies"])
	mark := func(path string) {
		t.Helper()
		if status, body := s.call(http.MethodPost, path, app("web", token), nil); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	mark("/UserFavoriteItems/" + tracks["First Light"])
	mark("/UserPlayedItems/" + tracks["Second Wind"])
	mark("/UserFavoriteItems/" + albums["Loud Songs"])

	// The played track comes first, then the favorite track and album.
	if got, want := list(token, "/Items?ParentId="+mine), []string{"Second Wind", "First Light", "Loud Songs"}; !slices.Equal(got, want) {
		t.Errorf("My music: %v, want %v", got, want)
	}
	if got, want := list(token, "/Items?ParentId="+mine+"&IncludeItemTypes=MusicAlbum&Recursive=true"), []string{"Loud Songs", "Sine Studies"}; !slices.Equal(got, want) {
		t.Errorf("its albums: %v, want %v", got, want)
	}
	if got, want := list(token, "/Items?ParentId="+mine+"&IncludeItemTypes=Audio&Recursive=true"), []string{"Second Wind", "First Light", "Rude Words"}; !slices.Equal(got, want) {
		t.Errorf("its songs: %v, want %v", got, want)
	}

	// Another user has listened to nothing.
	s.user("other", nil)
	other := s.signIn("other", "web")
	for _, path := range []string{"/Items?ParentId=" + mine, "/Items?ParentId=" + mine + "&IncludeItemTypes=MusicAlbum&Recursive=true"} {
		if got := list(other, path); len(got) != 0 {
			t.Errorf("another user's %s: %v", path, got)
		}
	}
	if n := addon.catalogs.Load(); n != 0 {
		t.Errorf("the addon was asked for %d catalogs", n)
	}

	// An addon with rows offers My music without enabling it.
	installed, err := s.addons.Install(t.Context(), addons.Shared(), newFakeEclipse(t, "").url, false)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	var enabled []string
	offered := false
	for _, l := range libraries {
		if l.AddonID != installed.ID {
			continue
		}
		offered = offered || l.Catalog.ID == eclipse.MyMusic
		if l.Enabled {
			enabled = append(enabled, l.Catalog.ID)
		}
	}
	if !offered || !slices.Equal(enabled, []string{"new", "top"}) {
		t.Errorf("an addon with rows: offered %v, enabled %v", offered, enabled)
	}
}
