package jellyfin

import (
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Home's row of the Collections view lists the titles of its collections,
// each once, in their order, as the Jellyfin API lists them, and reads
// only the collections that hold one: as many reads however many empty
// collections a user made.
func TestSiloHomeReadsNoEmptyCollection(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.testServer.user("curator", func(c *accounts.UserChanges) { c.CollectionManagement = new(true) })
	curator := p.signIn("curator", "tv")
	var mu sync.Mutex
	made := map[string]bool{}
	create := func(query string) {
		t.Helper()
		id := p.createCollection(t, curator, query)
		mu.Lock()
		made[id] = true
		mu.Unlock()
	}
	create("name=Picks&ids=" + p.movie + "," + p.remote)
	create("name=Sequels&ids=" + p.remote)
	allowSilo(t, p.testServer)
	var reads atomic.Int64
	a, _ := countedSilo(t, p.testServer, func(r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/Items" && made[r.URL.Query().Get("ParentId")] {
			reads.Add(1)
		}
	})
	token, profile := a.signIn(t, "member")
	// The titles of the view's collections, as Jellyfin apps list them.
	var view QueryResult
	p.get(t, "/Items?ParentId="+collectionsViewID.String(), p.token, &view)
	var want []string
	for _, collection := range view.Items {
		var listed QueryResult
		p.get(t, "/Items?ParentId="+collection.Id+"&IncludeItemTypes=Movie,Series", p.token, &listed)
		for _, title := range listed.Items {
			if !slices.Contains(want, title.Name) {
				want = append(want, title.Name)
			}
		}
	}
	if len(view.Items) != 2 || len(want) != 2 {
		t.Fatalf("the view lists %d collections of %q", len(view.Items), want)
	}
	row := func() []string {
		t.Helper()
		var home struct {
			Sections []struct {
				Type  string     `json:"section_type"`
				Title string     `json:"title"`
				Items []siloCard `json:"items"`
			} `json:"sections"`
		}
		a.get(t, "/api/v2/home/sections", token, profile, &home)
		for _, section := range home.Sections {
			if section.Title == "Collections" && section.Type == "collection" {
				return cardTitles(section.Items)
			}
		}
		return nil
	}
	empty := 0
	for _, more := range []int{4, 20} {
		for range more {
			create("name=Empty+" + strconv.Itoa(empty))
			empty++
		}
		reads.Store(0)
		if titles := row(); !slices.Equal(titles, want) {
			t.Errorf("%d empty collections: the row lists %q, want %q", empty, titles, want)
		}
		if read := reads.Load(); read != 2 {
			t.Errorf("%d empty collections: %d collections read for Home, want the 2 with titles", empty, read)
		}
	}
}
