package jellyfin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// The letters of Silo's alphabetical bar, the pages in title order and
// seek, against libraries whose titles Polyfin stores, sorted whole, and
// addons' catalogs, which keep their order and give no letters (see
// nameorder.go, and letters.go in package silo).

// shelfNames are the titles of a library whose initials are mixed: an
// article, accents, a digit, a symbol, another alphabet.
var shelfNames = []string{"Zulu", "The Hobbit", "Éclair", "apple", "123 Go", "Ωmega", "Echo", "'Salem's Lot", "Ångström"}

// letterProvider is an Xtream account (user / secret) of movies of names,
// in category Action.
func letterProvider(t *testing.T, names []string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "1", "category_name": "Action"}}
		case "get_vod_streams":
			movies := make([]any, len(names))
			for i, name := range names {
				movies[i] = map[string]any{"name": name, "stream_id": 100 + i, "category_id": "1", "container_extension": "mkv",
					"added": strconv.Itoa(1700000000 - i)}
			}
			answer = movies
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// shelf is a Silo app's view of one library: its member signed in from
// Silo's apps and from a Jellyfin app, and the library's number.
type shelf struct {
	siloApps
	member                  accounts.User
	token, profile, account string
	library                 string
}

func newShelf(t *testing.T, s testServer, libraryType string) shelf {
	t.Helper()
	allowSilo(t, s)
	x := shelf{siloApps: servingSilo(t, s), member: s.user("member", nil)}
	x.token, x.profile = x.signIn(t, "member")
	x.account = s.signIn("member", "tv")
	for _, l := range x.libraries(t, x.token, x.profile) {
		if l.Type == libraryType {
			x.library = l.ID
		}
	}
	if x.library == "" {
		t.Fatalf("no %s library: %+v", libraryType, x.libraries(t, x.token, x.profile))
	}
	return x
}

// newStoredShelf serves an Xtream source's movies of names, the library a
// member sees in Silo's apps, at most most of them when most is set.
func newStoredShelf(t *testing.T, names []string, most int) shelf {
	t.Helper()
	s := newTestServer(t, 10)
	on, off := true, false
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: letterProvider(t, names), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if most > 0 {
		if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
			{AddonID: addon.ID, CatalogType: "movie", CatalogID: "movies", MaxItems: &most}}); err != nil {
			t.Fatal(err)
		}
	}
	return newShelf(t, s, "movies")
}

// siloLetters is CatalogLetters.
type siloLetters struct {
	Total   int `json:"total"`
	Letters []struct {
		Letter   string `json:"letter"`
		Count    int    `json:"count"`
		Position int    `json:"position"`
	} `json:"letters"`
	Runs []struct {
		Letter   string `json:"letter"`
		Position int    `json:"position"`
	} `json:"runs"`
}

// lettersBody is the body of the letters of the library in order,
// narrowed by facets, as the apps send it.
func lettersBody(library, order string, facets silotest.Facets) map[string]any {
	body := map[string]any{"library_id": library, "order": order, "match": "all", "groups": []any{}}
	q := silotest.AppleCatalog(library, facets, "", 1)
	body["match"] = q.Get("match")
	if groups := q.Get("groups"); groups != "" {
		body["groups"] = json.RawMessage(groups)
	}
	return body
}

// letters reads the letters of the library in order, narrowed by facets,
// as the apps ask them, once they meet the apps' rules.
func (x shelf) letters(t *testing.T, library, order string, facets silotest.Facets) siloLetters {
	t.Helper()
	status, data := x.do(t, http.MethodPost, "/api/v2/catalog/letters", x.token, x.profile, lettersBody(library, order, facets))
	var letters siloLetters
	if status != http.StatusOK || json.Unmarshal(data, &letters) != nil {
		t.Fatalf("letters %s %s: %d %s", library, order, status, data)
	}
	if err := silotest.CheckLetters(data); err != nil {
		t.Errorf("letters %s %s: %v: %s", library, order, err, data)
	}
	return letters
}

// titleQuery is the Apple query of the library in title order (order asc
// or desc), narrowed by facets.
func titleQuery(library, order string, facets silotest.Facets, limit int) url.Values {
	sort := "title"
	if order == "desc" {
		sort = "-title"
	}
	return silotest.AppleCatalog(library, facets, sort, limit)
}

// all pages through the library in title order with the pages' cursors.
func (x shelf) all(t *testing.T, library, order string, facets silotest.Facets) []string {
	t.Helper()
	q := titleQuery(library, order, facets, 20)
	var titles []string
	for {
		var page siloPage
		x.get(t, "/api/v2/catalog?"+q.Encode(), x.token, x.profile, &page)
		titles = append(titles, cardTitles(page.Items)...)
		if !page.Page.HasMore {
			return titles
		}
		q.Set("cursor", page.Page.NextCursor)
	}
}

// seek reads limit titles from position, as the Apple app jumps (seek in
// the query), and as the Android app does (seek and the first page's
// window cursor in a body), which must agree.
func (x shelf) seek(t *testing.T, library, order string, facets silotest.Facets, position, limit int) []string {
	t.Helper()
	q := titleQuery(library, order, facets, limit)
	q.Set("seek", strconv.Itoa(position))
	var apple siloPage
	x.get(t, "/api/v2/catalog?"+q.Encode(), x.token, x.profile, &apple)
	body := silotest.AndroidCatalog(library, facets, "title", order, limit)
	body["seek"], body["cursor"], body["skip_total"] = position, "c3RhcnQ6MA", true
	status, data := x.do(t, http.MethodPost, "/api/v2/catalog/query", x.token, x.profile, body)
	var page siloPage
	if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
		t.Fatalf("Android seek %d: %d %s", position, status, data)
	}
	if !slices.Equal(cardTitles(apple.Items), cardTitles(page.Items)) {
		t.Errorf("seek %d: Apple %v, Android %v", position, cardTitles(apple.Items), cardTitles(page.Items))
	}
	return cardTitles(apple.Items)
}

// letterOf is the letter a title files under.
func letterOf(name string) string {
	return library.NameLetter(library.FoldName(name))
}

// agree checks that the letters of the library in order, narrowed by
// facets, describe the list its pages give, in title order, and that seek
// lands on the last run's first title, as both apps send it; it returns
// that list.
func (x shelf) agree(t *testing.T, number, order string, facets silotest.Facets) []string {
	t.Helper()
	letters := x.letters(t, number, order, facets)
	titles := x.all(t, number, order, facets)
	if letters.Total != len(titles) {
		t.Fatalf("%s: %d letters for %d titles", order, letters.Total, len(titles))
	}
	keys := make([]string, len(titles))
	for i, title := range titles {
		keys[i] = library.FoldName(title)
	}
	if !slices.IsSortedFunc(keys, func(a, b string) int {
		if order == "desc" {
			return strings.Compare(b, a)
		}
		return strings.Compare(a, b)
	}) {
		t.Errorf("%s: not in title order: %v", order, titles)
	}
	for _, l := range letters.Letters {
		count := 0
		for _, title := range titles {
			if letterOf(title) == l.Letter {
				count++
			}
		}
		if count != l.Count || letterOf(titles[l.Position]) != l.Letter {
			t.Errorf("%s: %s counts %d from %d (%s), the list %d", order, l.Letter, l.Count, l.Position, titles[l.Position], count)
		}
	}
	if last := len(letters.Runs) - 1; last >= 0 {
		position := letters.Runs[last].Position
		if got := x.seek(t, number, order, facets, position, 3); len(got) == 0 || got[0] != titles[position] {
			t.Errorf("%s: seek %d gives %v, the list %s", order, position, got, titles[position])
		}
	}
	return titles
}

// A library whose titles Polyfin stores, an IPTV source's movies, sorts
// whole by title: the letters, the pages and seek follow one order,
// ascending, descending and filtered, within what the user may see.
func TestSiloAppsLettersOfAStoredLibrary(t *testing.T) {
	t.Parallel()
	names := slices.Clone(shelfNames)
	for i := range 30 {
		names = append(names, fmt.Sprintf("Filler %02d", i))
	}
	x := newStoredShelf(t, names, 0)
	letters := x.letters(t, x.library, "asc", silotest.Facets{})
	var got []string
	for _, l := range letters.Letters {
		got = append(got, fmt.Sprintf("%s%d@%d", l.Letter, l.Count, l.Position))
	}
	if want := []string{"#3@0", "A2@2", "E2@4", "F30@6", "T1@36", "Z1@37"}; letters.Total != len(names) || !slices.Equal(got, want) ||
		len(letters.Runs) != 7 || letters.Runs[6].Letter != "#" || letters.Runs[6].Position != 38 {
		t.Errorf("letters: %d %v", letters.Total, got)
	}
	titles := x.agree(t, x.library, "asc", silotest.Facets{})
	if !slices.Equal(titles[:7], []string{"'Salem's Lot", "123 Go", "Ångström", "apple", "Echo", "Éclair", "Filler 00"}) ||
		titles[len(titles)-1] != "Ωmega" {
		t.Errorf("by title: %v", titles)
	}
	if desc := x.agree(t, x.library, "desc", silotest.Facets{}); len(desc) != len(names) || desc[0] != "Ωmega" || desc[1] != "Zulu" {
		t.Errorf("descending: %v", desc)
	}
	if action := x.agree(t, x.library, "asc", silotest.Facets{Genres: []string{"Action"}}); !slices.Equal(action, titles) {
		t.Errorf("filtered to Action: %v", action)
	}
	// A title the user hides leaves the letters and the pages at once.
	var listed QueryResult
	x.testServer.get(t, "/Items?ParentId="+x.viewsByType(t, x.account)["movies"]+"&Recursive=true&IncludeItemTypes=Movie&searchTerm=Echo",
		x.account, &listed)
	if len(listed.Items) == 0 {
		t.Fatal("no Echo")
	}
	if status, _ := x.call(http.MethodPost, "/Polyfin/UserItems/"+listed.Items[0].Id+"/Hidden", app("tv", x.account), nil); status != http.StatusNoContent {
		t.Fatalf("hiding Echo: %d", status)
	}
	if titles := x.agree(t, x.library, "asc", silotest.Facets{}); slices.Contains(titles, "Echo") || len(titles) != len(names)-1 {
		t.Errorf("Echo hidden: %v", titles)
	}
	// Parental control that hides unrated movies hides them all.
	x.limit(t, x.member, func(c *accounts.UserChanges) {
		c.Parental = &accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}}
	})
	if letters := x.letters(t, x.library, "asc", silotest.Facets{}); letters.Total != 0 || len(letters.Letters) != 0 {
		t.Errorf("unrated movies blocked: %+v", letters)
	}
}

// A stored library whose maximum is below its size sorts the titles it
// lists, and keeps that order rather than sorting at every request.
func TestSiloAppsLettersWithinALibraryMaximum(t *testing.T) {
	t.Parallel()
	x := newStoredShelf(t, shelfNames, 5)
	if titles := x.agree(t, x.library, "asc", silotest.Facets{}); len(titles) != 5 {
		t.Errorf("%d titles within a maximum of 5: %v", len(titles), titles)
	}
	if kept := x.handler.nameOrders.Len(); kept != 1 {
		t.Errorf("%d orders kept", kept)
	}
}

// An addon's catalog, filtered or not, and a library of collections, whose
// movies and series its collections gather, keep their order in title
// order and give no letters, so the apps show no bar; seek reads a
// position of their order as a cursor does.
func TestSiloAppsLettersOfAddonCatalogs(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	x := shelf{siloApps: f.siloApps, member: f.member, token: f.token, profile: f.profile, account: f.account}
	films, picks := f.numbers["Films"], f.numbers["Picks"]
	for name, body := range map[string]map[string]any{"Films": lettersBody(films, "asc", silotest.Facets{}),
		"Picks": lettersBody(picks, "desc", silotest.Facets{}), "Action films": lettersBody(films, "asc", silotest.Facets{Genres: []string{"Action"}})} {
		if status, data := x.do(t, http.MethodPost, "/api/v2/catalog/letters", x.token, x.profile, body); status != http.StatusNotImplemented ||
			!strings.Contains(string(data), "capability_unsupported") {
			t.Errorf("letters of %s: %d %s", name, status, data)
		}
	}
	q := titleQuery(films, "asc", silotest.Facets{}, 2)
	var first, second, sought siloPage
	x.get(t, "/api/v2/catalog?"+q.Encode(), x.token, x.profile, &first)
	next := maps.Clone(q)
	next.Set("cursor", first.Page.NextCursor)
	x.get(t, "/api/v2/catalog?"+next.Encode(), x.token, x.profile, &second)
	q.Set("seek", "2")
	x.get(t, "/api/v2/catalog?"+q.Encode(), x.token, x.profile, &sought)
	if first.Total != 5 || len(second.Items) != 2 || !slices.Equal(cardTitles(sought.Items), cardTitles(second.Items)) {
		t.Errorf("seek 2: %v, the cursor's page %v of %d", cardTitles(sought.Items), cardTitles(second.Items), first.Total)
	}
	q.Set("seek", "-1")
	if status, data := x.do(t, http.MethodGet, "/api/v2/catalog?"+q.Encode(), x.token, x.profile, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("seek -1: %d %s", status, data)
	}
	var capabilities struct {
		LetterIndex bool `json:"letter_index"`
	}
	x.get(t, "/api/v2/catalog/search/capabilities", x.token, x.profile, &capabilities)
	if !capabilities.LetterIndex {
		t.Error("the search capabilities do not offer the letter index")
	}
}
