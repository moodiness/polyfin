package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// edited is a server with the rated addon's titles, an administrator, a
// member and a child who may see up to PG-13, and the titles' identifiers
// by name, as the administrator lists them.
type edited struct {
	testServer
	admin, member, child string
	memberID, childID    string
	ids                  map[string]string
}

func newEdited(t *testing.T) edited {
	t.Helper()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	member := s.user("member", nil)
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	e := edited{testServer: s, admin: s.signIn("admin", "dashboard"), member: s.signIn("member", "tv"), child: s.signIn("child", "tablet"),
		memberID: member.ID.String(), childID: child.ID.String(), ids: map[string]string{}}
	var views QueryResult
	s.get(t, "/UserViews", e.admin, &views)
	for _, view := range views.Items {
		e.ids[view.Name] = view.Id
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, e.admin, &page)
		for _, item := range page.Items {
			e.ids[item.Name] = item.Id
		}
	}
	if e.ids["Allowed"] == "" || e.ids["Restricted"] == "" || e.ids["Show"] == "" {
		t.Fatalf("titles: %v", e.ids)
	}
	return e
}

// item describes an item as user sees it, with its status.
func (e edited) item(t *testing.T, token, user, id string) (int, BaseItemDto) {
	t.Helper()
	var dto BaseItemDto
	status := e.get(t, "/Users/"+user+"/Items/"+id, token, &dto)
	return status, dto
}

// listed returns the names of a library's titles as user sees them.
func (e edited) listed(t *testing.T, token, library string) []string {
	t.Helper()
	var page QueryResult
	if status := e.get(t, "/Items?ParentId="+e.ids[library]+"&fields=SortName", token, &page); status != http.StatusOK {
		t.Fatalf("listing %s: %d", library, status)
	}
	return itemNames(page.Items)
}
