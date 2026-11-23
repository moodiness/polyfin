package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// cardsOf reads the cards of a page, or of a For You answer's rows, as
// they come, and checks their not_interested against marked.
func (r siloRated) cardsOf(t *testing.T, path, token, profile string, marked ...string) []string {
	t.Helper()
	status, data := r.do(t, http.MethodGet, path, token, profile, nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
		t.Fatalf("%s: %d %s", path, status, data)
	}
	cards := page.Items
	if path == "/api/v2/recommendations/discover" {
		cards = nil
		for _, row := range page.Items {
			for _, card := range row["items"].([]any) {
				cards = append(cards, card.(map[string]any))
			}
		}
	}
	if err := silotest.CheckNotInterested(cards, marked); err != nil {
		t.Errorf("%s: %v", path, err)
	}
	var titles []string
	for _, card := range cards {
		titles = append(titles, card["title"].(string))
	}
	return titles
}

// browse reads the first page of each of the user's libraries, as the
// apps open them, checking their cards against marked.
func (r siloRated) browse(t *testing.T, token, profile string, marked ...string) []string {
	t.Helper()
	var titles []string
	for _, library := range r.libraries(t, token, profile) {
		titles = append(titles, r.cardsOf(t, "/api/v2/catalog?library_id="+library.ID, token, profile, marked...)...)
	}
	return titles
}

// mark marks a title Not interested, or clears its mark, as the Apple app
// does.
func (r siloRated) mark(t *testing.T, method, id string) {
	t.Helper()
	status, data := r.do(t, method, "/api/v2/not-interested/"+id, r.token, r.profile, nil)
	if err := silotest.CheckMarkAnswer(status); err != nil {
		t.Fatalf("%s %s: %v: %s", method, id, err, data)
	}
}

// A user marks a movie or a series Not interested from Silo's apps: it
// leaves their similar titles and For You at once, while their library,
// searches and the title's page still show it, marked, and Jellyfin apps
// see nothing of it. The list of marks lists it, the latest first, and
// clearing it brings it back. An episode takes no mark of its own, another
// user's titles are not marked, and a title the user may no longer see
// leaves the list and is not found.
func TestSiloAppsMarkTitlesNotInterested(t *testing.T) {
	t.Parallel()
	r := newSiloRated(t)
	allowed, restricted, show := r.ids["Allowed"], r.ids["Restricted"], r.ids["Show"]
	var capability map[string]any
	r.get(t, "/api/v2/capabilities/not-interested", r.token, r.profile, &capability)
	if capability["state"] != "available" || capability["allowed"] != true || capability["revision"] == "" {
		t.Errorf("the capability: %v", capability)
	}
	// A title's page reads its cast, which similar titles share; played,
	// Allowed makes For You, with the titles similar to it.
	var detail map[string]any
	r.get(t, "/api/v2/catalog/items/"+restricted, r.token, r.profile, &detail)
	if status, _ := r.call(http.MethodPost, "/UserPlayedItems/"+allowed, app("tv", r.jellyfin), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	similar := "/api/v2/recommendations/similar/" + allowed + "?limit=10"
	if titles := r.cardsOf(t, similar, r.token, r.profile); !slices.Contains(titles, "Restricted") {
		t.Fatalf("similar titles before any mark: %v", titles)
	}
	if titles := r.cardsOf(t, "/api/v2/recommendations/discover", r.token, r.profile); !slices.Contains(titles, "Restricted") {
		t.Fatalf("For You before any mark: %v", titles)
	}
	if status, _ := r.do(t, http.MethodGet, "/api/v2/not-interested/"+restricted, r.token, r.profile, nil); status != http.StatusNotFound {
		t.Errorf("a mark before any: %d", status)
	}

	r.mark(t, http.MethodPut, restricted)
	r.mark(t, http.MethodPut, show)
	// Twice changes nothing.
	r.mark(t, http.MethodPut, restricted)
	for path, want := range map[string]bool{similar: false, "/api/v2/recommendations/discover": false,
		"/api/v2/catalog?source=query&q=Restricted": true} {
		if titles := r.cardsOf(t, path, r.token, r.profile, restricted, show); slices.Contains(titles, "Restricted") != want {
			t.Errorf("%s once marked: %v", path, titles)
		}
	}
	if titles := r.browse(t, r.token, r.profile, restricted, show); !slices.Contains(titles, "Restricted") || !slices.Contains(titles, "Show") {
		t.Errorf("the libraries once marked: %v", titles)
	}
	detail = nil
	r.get(t, "/api/v2/catalog/items/"+restricted, r.token, r.profile, &detail)
	if err := silotest.CheckNotInterested([]map[string]any{detail}, []string{restricted}); err != nil {
		t.Errorf("the page: %v", err)
	}
	var entry struct {
		ItemID   string `json:"item_id"`
		MarkedAt string `json:"marked_at"`
	}
	r.get(t, "/api/v2/not-interested/"+restricted, r.token, r.profile, &entry)
	if entry.ItemID != restricted || entry.MarkedAt == "" {
		t.Errorf("the mark: %+v", entry)
	}
	if titles := r.cardsOf(t, "/api/v2/not-interested", r.token, r.profile, restricted, show); !slices.Equal(titles, []string{"Show", "Restricted"}) {
		t.Errorf("the marks: %v", titles)
	}
	var page siloPage
	r.get(t, "/api/v2/not-interested?limit=1", r.token, r.profile, &page)
	if titles := cardTitles(page.Items); !slices.Equal(titles, []string{"Show"}) || !page.Page.HasMore {
		t.Errorf("a page of one mark: %v, more %v", titles, page.Page.HasMore)
	}
	r.get(t, "/api/v2/not-interested?limit=1&cursor="+page.Page.NextCursor, r.token, r.profile, &page)
	if titles := cardTitles(page.Items); !slices.Equal(titles, []string{"Restricted"}) || page.Page.HasMore {
		t.Errorf("the next page: %v, more %v", titles, page.Page.HasMore)
	}

	// Jellyfin apps see the title as before.
	var recommended []RecommendationDto
	r.testServer.get(t, "/Movies/Recommendations", r.jellyfin, &recommended)
	var listed []string
	for _, category := range recommended {
		listed = append(listed, itemNames(category.Items)...)
	}
	if !slices.Contains(listed, "Restricted") {
		t.Errorf("Jellyfin's recommendations: %v", listed)
	}

	// An episode takes no mark: its series does.
	var episodes QueryResult
	r.testServer.get(t, "/Shows/"+show+"/Episodes", r.jellyfin, &episodes)
	if len(episodes.Items) == 0 {
		t.Fatal("Show has no episodes")
	}
	episode := episodes.Items[0].Id
	if status, data := r.do(t, http.MethodPut, "/api/v2/not-interested/"+episode, r.token, r.profile, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an episode: %d %s", status, data)
	}
	detail = nil
	r.get(t, "/api/v2/catalog/items/"+episode, r.token, r.profile, &detail)
	if err := silotest.CheckNotInterested([]map[string]any{detail}, []string{restricted, show}); err != nil {
		t.Errorf("the episode's page: %v", err)
	}

	// Another user's titles are not marked.
	r.testServer.user("other", nil)
	token, profile := r.signIn(t, "other")
	r.browse(t, token, profile)
	if titles := r.cardsOf(t, "/api/v2/not-interested", token, profile); len(titles) != 0 {
		t.Errorf("the other user's marks: %v", titles)
	}

	// A title the user may no longer see leaves the list, and is not found.
	r.limit(t, r.member, func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	if titles := r.cardsOf(t, "/api/v2/not-interested", r.token, r.profile, restricted, show); len(titles) != 0 {
		t.Errorf("the marks under parental control: %v", titles)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if status, _ := r.do(t, method, "/api/v2/not-interested/"+restricted, r.token, r.profile, nil); status != http.StatusNotFound {
			t.Errorf("%s of a title out of reach: %d", method, status)
		}
	}
	r.limit(t, r.member, func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{} })

	// Cleared, twice, the title comes back.
	r.mark(t, http.MethodDelete, restricted)
	r.mark(t, http.MethodDelete, restricted)
	if titles := r.cardsOf(t, similar, r.token, r.profile, show); !slices.Contains(titles, "Restricted") {
		t.Errorf("similar titles once cleared: %v", titles)
	}
	if titles := r.cardsOf(t, "/api/v2/not-interested", r.token, r.profile, show); !slices.Equal(titles, []string{"Show"}) {
		t.Errorf("the marks once cleared: %v", titles)
	}
}
