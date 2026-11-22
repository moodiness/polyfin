package jellyfin

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// A collection holds collections as Jellyfin's box sets hold box sets:
// jellyfin-web adds one to another with Add to collection, and lists them
// among the outer one's titles.
func TestCollectionsHoldCollections(t *testing.T) {
	t.Parallel()
	s, admin, titles := grouping(t)
	avengers := s.createCollection(t, admin, "name=Avengers&ids="+titles.teens+","+titles.adults)
	ironMan := s.createCollection(t, admin, "name=Iron%20Man&ids="+titles.kids)
	animated := s.createCollection(t, admin, "name=Animated&ids="+titles.kids)
	marvel := s.createCollection(t, admin, "name=Marvel")
	s.expectStatus(t, http.StatusNoContent, http.MethodPost,
		"/Collections/"+marvel+"/Items?Ids="+strings.Join([]string{avengers, ironMan, titles.show, animated}, ","), admin, nil)
	s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	member := s.signIn("member", "phone")
	child := s.signIn("child", "tablet")

	children := func(token, id, query string) []BaseItemDto {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+id+query, token, &page); status != http.StatusOK || page.TotalRecordCount != len(page.Items) {
			t.Fatalf("children of %s: %d, %d of %d", id, status, len(page.Items), page.TotalRecordCount)
		}
		return page.Items
	}
	// By premiere date, a collection's being its earliest title's; Iron
	// Man and Animated, of the same title, in the order they were added.
	listed := children(member, marvel, "&Fields=ChildCount,RecursiveItemCount")
	if got := itemNames(listed); !slices.Equal(got, []string{"Avengers", "Iron Man", "Animated", "Show"}) {
		t.Fatalf("Marvel: %v", got)
	}
	if inner := listed[0]; inner.Type != "BoxSet" || !inner.IsFolder || inner.Id != avengers || inner.ChildCount == nil || *inner.ChildCount != 2 ||
		inner.RecursiveItemCount == nil || *inner.RecursiveItemCount != 2 || inner.OfficialRating != "PG-13" {
		t.Errorf("Avengers within Marvel: %+v", inner)
	}
	var detail BaseItemDto
	s.get(t, "/Items/"+marvel, member, &detail)
	if detail.Type != "BoxSet" || detail.ChildCount == nil || *detail.ChildCount != 4 || detail.RecursiveItemCount == nil ||
		*detail.RecursiveItemCount != 6 || detail.OfficialRating != "G" || detail.ProductionYear == nil || *detail.ProductionYear != 1999 ||
		detail.Genres == nil || !slices.Equal(slices.Sorted(slices.Values(*detail.Genres)), []string{"Action", "Animation", "Drama"}) {
		t.Errorf("Marvel: %+v", detail)
	}
	// Played as apps play a collection, the titles of the collections in
	// it come too, each once.
	if got := itemNames(children(member, marvel, "&Recursive=true&Filters=IsNotFolder&MediaTypes=Video")); !slices.Equal(got,
		[]string{"Teens", "Kids", "Adults", "Pilot", "Second"}) {
		t.Errorf("played: %v", got)
	}
	if got := itemNames(children(member, marvel, "&IncludeItemTypes=BoxSet")); !slices.Equal(got, []string{"Avengers", "Iron Man", "Animated"}) {
		t.Errorf("collections only: %v", got)
	}

	// A collection the user is not shown is left out for them: none of
	// Avengers' titles is within the child's reach, nor is Show.
	if got := itemNames(children(child, marvel, "&Fields=ChildCount")); !slices.Equal(got, []string{"Iron Man", "Animated"}) {
		t.Errorf("Marvel for the child: %v", got)
	}
	s.get(t, "/Items/"+marvel, child, &detail)
	if detail.ChildCount == nil || *detail.ChildCount != 2 || detail.RecursiveItemCount == nil || *detail.RecursiveItemCount != 2 {
		t.Errorf("Marvel's counts for the child: %v %v", detail.ChildCount, detail.RecursiveItemCount)
	}
	if status := s.get(t, "/Items/"+avengers, child, nil); status != http.StatusNotFound {
		t.Errorf("Avengers for the child: %d", status)
	}
	view := s.viewIDs(t, child)["Collections"]
	if got := itemNames(children(child, view, "")); !slices.Equal(got, []string{"Animated", "Iron Man", "Marvel"}) {
		t.Errorf("the child's Collections view: %v", got)
	}

	// The poster is that of the first title the user sees, through the
	// collections in it.
	poster := func(token string) string {
		t.Helper()
		var listed QueryResult
		s.get(t, "/Items?ParentId="+s.viewIDs(t, token)["Collections"], token, &listed)
		tag := ""
		for _, item := range listed.Items {
			if item.Id == marvel {
				tag = item.ImageTags["Primary"]
			}
		}
		if tag == "" {
			t.Fatalf("no poster tag for Marvel: %v", itemNames(listed.Items))
		}
		response, err := testClient.Get(s.url + "/Items/" + marvel + "/Images/Primary?tag=" + tag)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("poster: %d %s", response.StatusCode, body)
		}
		return strings.TrimPrefix(string(body), jpegStart)
	}
	if got := poster(member); got != "poster of tt2" {
		t.Errorf("the member's poster: %q", got)
	}
	if got := poster(child); got != "poster of tt1" {
		t.Errorf("the child's poster: %q", got)
	}

	// No collection goes within itself, at any depth, and nesting stops
	// four levels deep. A refused change changes nothing.
	refused := func(method, path string) {
		t.Helper()
		if status, body := s.call(method, path, app("tv", admin), nil); status != http.StatusBadRequest || string(body) != "Error processing request." {
			t.Errorf("%s %s: %d %s", method, path, status, body)
		}
	}
	refused(http.MethodPost, "/Collections/"+marvel+"/Items?ids="+marvel)
	refused(http.MethodPost, "/Collections/"+avengers+"/Items?ids="+titles.kids+","+marvel)
	if got := itemNames(children(member, avengers, "")); !slices.Equal(got, []string{"Teens", "Adults"}) {
		t.Errorf("Avengers after a refused change: %v", got)
	}
	one := s.createCollection(t, admin, "name=One")
	two := s.createCollection(t, admin, "name=Two&ids="+one)
	three := s.createCollection(t, admin, "name=Three&ids="+two)
	four := s.createCollection(t, admin, "name=Four&ids="+three)
	refused(http.MethodPost, "/Collections?name=Five&ids="+four)
	refused(http.MethodPost, "/Collections/"+one+"/Items?ids="+ironMan)
	if got := itemNames(children(admin, one, "")); len(got) != 0 {
		t.Errorf("One after a refused change: %v", got)
	}

	// Deleting a collection takes it out of those that held it; removing
	// one takes it out of one.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+ironMan, admin, nil)
	if got := itemNames(children(member, marvel, "")); !slices.Equal(got, []string{"Avengers", "Animated", "Show"}) {
		t.Errorf("Marvel after deleting Iron Man: %v", got)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Collections/"+marvel+"/Items?ids="+avengers, admin, nil)
	if got := itemNames(children(member, marvel, "")); !slices.Equal(got, []string{"Animated", "Show"}) {
		t.Errorf("Marvel after removing Avengers: %v", got)
	}
	if status := s.get(t, "/Items/"+avengers, member, nil); status != http.StatusOK {
		t.Errorf("Avengers once out of Marvel: %d", status)
	}
}

// Silo's apps cannot open a collection from within another: a
// collection's page there lists its titles, and its count on a library's
// Collections tab counts them, without the collections in it.
func TestSiloAppsLeaveOutCollectionsInACollection(t *testing.T) {
	t.Parallel()
	p := playing(t)
	if _, err := p.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	admin := p.signIn("admin", "tv")
	inner := p.createCollection(t, admin, "name=Inner&ids="+p.remote)
	picks := p.createCollection(t, admin, "name=Picks&ids="+p.movie+","+inner)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	library := a.libraries(t, token, profile)[0].ID

	var tab struct {
		Collections []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"collections"`
	}
	a.get(t, "/api/v2/library/"+library+"/collections", token, profile, &tab)
	counts := map[string]int{}
	for _, c := range tab.Collections {
		counts[c.ID] = c.ItemCount
	}
	if len(counts) != 2 || counts[picks] != 1 || counts[inner] != 1 {
		t.Errorf("the tab: %+v", tab.Collections)
	}
	var page siloPage
	a.get(t, "/api/v2/library/"+library+"/collections/"+picks+"/items", token, profile, &page)
	if got := cardTitles(page.Items); !slices.Equal(got, []string{"Movie"}) || page.Page.HasMore {
		t.Errorf("Picks' page: %v, more: %v", got, page.Page.HasMore)
	}
}

// A smart list shown as a collection goes inside a collection as a box
// set; nothing goes inside a smart list, whose titles are its rules'.
func TestASmartListGoesInsideACollection(t *testing.T) {
	t.Parallel()
	s, admin, titles := grouping(t)
	dramas := s.makeSmartList(t, nil, "Dramas", smartlists.ShownAsCollection, []string{"movie/top", "series/shows"}, smartlists.SortPremiere,
		smartlists.Rule{Field: smartlists.FieldGenre, Value: "Drama"})
	picks := s.createCollection(t, admin, "name=Picks&ids="+titles.kids+","+dramas)
	var page QueryResult
	s.get(t, "/Items?ParentId="+picks, admin, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Dramas", "Kids"}) || page.Items[0].Type != "BoxSet" {
		t.Errorf("Picks: %+v", page.Items)
	}
	if status, body := s.call(http.MethodPost, "/Collections/"+dramas+"/Items?ids="+picks, app("tv", admin), nil); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a collection into a smart list: %d %s", status, body)
	}
}
