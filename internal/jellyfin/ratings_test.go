package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/ratings"
)

// fakeMDBList answers MDBList's batch lookup as its documentation shows
// it, for the browsing addon's titles: Movie 0 (tt0000) has an IMDb rating
// of 8.1, a critics' score of 97 and an audience score of 60, Movie 1
// (tt0001) 6.2, 40 and 90, the show (tt0100) 8.9, 88 and 91; MDBList does
// not know Movie 2 (tt0002). It counts the titles it is asked about, and
// keeps its answers while held (see hold).
type fakeMDBList struct {
	mu    sync.Mutex
	asked []string
	url   string
	held  chan struct{}
}

func newFakeMDBList(t *testing.T) *fakeMDBList {
	f := &fakeMDBList{}
	known := map[string][3]float64{"tt0000": {8.1, 97, 60}, "tt0001": {6.2, 40, 90}, "tt0100": {8.9, 88, 91}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ IDs []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		for _, id := range body.IDs {
			f.asked = append(f.asked, r.Method+" "+r.URL.Path+" "+id+" "+r.URL.Query().Get("apikey"))
		}
		held := f.held
		f.mu.Unlock()
		if held != nil {
			select {
			case <-held:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("X-RateLimit-Limit", "1000")
		w.Header().Set("X-RateLimit-Remaining", "998")
		var items []string
		for _, id := range body.IDs {
			if scores, ok := known[id]; ok {
				items = append(items, fmt.Sprintf(`{"title": "Title", "year": 2020, "released": "2020-04-10", "type": "movie",
					"ids": {"imdb": %q, "tmdb": 10, "trakt": 1, "tvdb": null, "mal": null, "mdblist": "x"},
					"ratings": [{"source": "imdb", "value": %v, "score": 0, "votes": 100, "url": null},
						{"source": "metacritic", "value": 12, "score": 12, "votes": 3, "url": null},
						{"source": "tomatoes", "value": %v, "score": %[3]v, "votes": 20, "url": null},
						{"source": "popcorn", "value": %v, "score": %[4]v, "votes": 2000, "url": null}]}`, id, scores[0], scores[1], scores[2]))
			}
		}
		_, _ = io.WriteString(w, "["+strings.Join(items, ",")+"]")
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

func (f *fakeMDBList) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// hold has MDBList keep its answers, as a slow one would, until answer
// is called, which the test's end does at the latest.
func (f *fakeMDBList) hold(t *testing.T) (answer func()) {
	held := make(chan struct{})
	f.mu.Lock()
	f.held = held
	f.mu.Unlock()
	answer = sync.OnceFunc(func() { close(held) })
	t.Cleanup(answer)
	return answer
}

// rated is the browsing server with its ratings looked up on a fake
// MDBList.
type rated struct {
	testServer
	token   string
	ids     map[string]string
	mdblist *fakeMDBList
}

func newRated(t *testing.T, key string) rated {
	t.Helper()
	s, token, ids := browsing(t)
	f := newFakeMDBList(t)
	lookups := ratings.New(s.pool, f.url, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), s.store.Settings)
	t.Cleanup(lookups.Close)
	s.library.UseRatings(lookups)
	if key != "" {
		s.setting(t, func(settings *accounts.Settings) { settings.MDBListRatingsKey = key })
	}
	return rated{testServer: s, token: token, ids: ids, mdblist: f}
}

// scores describes the ratings of a library's titles, by name: their
// CommunityRating and CriticRating, "-" for none.
func (r rated) scores(t *testing.T, query string) map[string]string {
	t.Helper()
	var page QueryResult
	if status := r.get(t, "/Items?"+query, r.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", query, status)
	}
	result := map[string]string{}
	for _, item := range page.Items {
		result[item.Name] = describeScores(item)
	}
	return result
}

func describeScores(item BaseItemDto) string {
	number := func(value *float64) string {
		if value == nil {
			return "-"
		}
		return fmt.Sprint(*value)
	}
	return number(item.CommunityRating) + " " + number(item.CriticRating)
}

// With the server's MDBList key, a listing shows what is kept at once and
// has the rest asked about in the background, in one request: the next
// listing, and the title's details, show the IMDb rating as
// CommunityRating, in place of the addon's, and the critics' score as
// CriticRating, or the audience score once Critic rating shows says so.
// A title MDBList does not know keeps its addon's rating. The web player's
// script reads the three scores of a title from /Polyfin/Items/{id}/Ratings.
func TestTitlesShowTheirRatingsFromMDBList(t *testing.T) {
	t.Parallel()
	r := newRated(t, "server-key")
	answer := r.mdblist.hold(t)
	top := "ParentId=" + r.ids["Top"]
	if got := r.scores(t, top); got["Movie 0"] != "7.5 -" {
		t.Fatalf("the first listing, before MDBList answered: %v", got)
	}
	answer()
	want := map[string]string{"Movie 0": "8.1 97", "Movie 1": "6.2 40", "Movie 2": "7.5 -"}
	eventually(t, "rated", func() bool { return fmt.Sprint(r.scores(t, top)) == fmt.Sprint(want) })
	if asked := r.mdblist.requests(); !slices.Equal(asked, []string{
		"POST /imdb/movie tt0000 server-key", "POST /imdb/movie tt0001 server-key", "POST /imdb/movie tt0002 server-key",
	}) {
		t.Errorf("MDBList was asked %q", asked)
	}
	var member UserDto
	r.get(t, "/Users/Me", r.token, &member)
	var details BaseItemDto
	r.get(t, "/Users/"+member.Id+"/Items/"+r.itemID(t, "Movie 0"), r.token, &details)
	if got := describeScores(details); got != "8.1 97" {
		t.Errorf("the title's details: %s", got)
	}
	var three TitleRatings
	r.get(t, "/Polyfin/Items/"+details.Id+"/Ratings", r.token, &three)
	if got := describeThree(three); got != "8.1 97 60 false" {
		t.Errorf("the three scores: %s", got)
	}
	// The show, listed, is asked about too.
	shows := "ParentId=" + r.ids["Shows"]
	eventually(t, "the show rated", func() bool { return r.scores(t, shows)["Show"] == "8.9 88" })

	r.setting(t, func(settings *accounts.Settings) { settings.CriticRatingShows = "audience" })
	if got := r.scores(t, top); got["Movie 0"] != "8.1 60" || got["Movie 1"] != "6.2 90" {
		t.Errorf("with the audience score: %v", got)
	}
	r.get(t, "/Polyfin/Items/"+details.Id+"/Ratings", r.token, &three)
	if got := describeThree(three); got != "8.1 97 60 false" {
		t.Errorf("the three scores with the audience score: %s", got)
	}
	if n := len(r.mdblist.requests()); n != 4 {
		t.Errorf("%d titles asked about, want 4: kept ratings are not asked again", n)
	}
}

func describeThree(three TitleRatings) string {
	number := func(value *float64) string {
		if value == nil {
			return "-"
		}
		return fmt.Sprint(*value)
	}
	return fmt.Sprintf("%s %s %s %v", number(three.CommunityRating), number(three.CriticsScore), number(three.AudienceScore), three.Pending)
}

// itemID is the identifier of a title of the Top library, by name.
func (r rated) itemID(t *testing.T, name string) string {
	t.Helper()
	var page QueryResult
	r.get(t, "/Items?ParentId="+r.ids["Top"], r.token, &page)
	for _, item := range page.Items {
		if item.Name == name {
			return item.Id
		}
	}
	t.Fatalf("no %s in %v", name, itemNames(page.Items))
	return ""
}

// Without the server's key, MDBList is never asked: titles show their
// addon's rating, and no critic rating.
func TestWithoutAKeyMDBListIsNeverAsked(t *testing.T) {
	t.Parallel()
	r := newRated(t, "")
	top := "ParentId=" + r.ids["Top"]
	id := r.itemID(t, "Movie 0")
	var member UserDto
	r.get(t, "/Users/Me", r.token, &member)
	var details BaseItemDto
	for range 3 {
		r.scores(t, top)
		r.get(t, "/Users/"+member.Id+"/Items/"+id, r.token, &details)
		time.Sleep(100 * time.Millisecond)
	}
	if got := r.scores(t, top); got["Movie 0"] != "7.5 -" || describeScores(details) != "7.5 -" {
		t.Errorf("without a key: listed %v, details %s", got, describeScores(details))
	}
	var three TitleRatings
	r.get(t, "/Polyfin/Items/"+id+"/Ratings", r.token, &three)
	if got := describeThree(three); got != "- - - false" {
		t.Errorf("the three scores without a key: %s", got)
	}
	if asked := r.mdblist.requests(); len(asked) > 0 {
		t.Errorf("MDBList was asked without a key: %q", asked)
	}
}

// An administrator's edit of a title's community or critic rating wins
// over MDBList's, for every user, the three scores of the title's page
// included; an edit that leaves them as MDBList gives them keeps none.
// Refresh metadata forgets them, so that MDBList is asked again.
func TestAnAdministratorsEditWinsOverMDBList(t *testing.T) {
	t.Parallel()
	r := newRated(t, "server-key")
	if _, err := r.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	admin := r.signIn("admin", "dashboard")
	top := "ParentId=" + r.ids["Top"]
	r.scores(t, top)
	eventually(t, "rated", func() bool { return r.scores(t, top)["Movie 0"] == "8.1 97" })
	id := r.itemID(t, "Movie 0")
	parsed, _ := accounts.ParseID(id)

	// Saved as shown, the ratings keep following MDBList.
	if status, body := r.call(http.MethodPost, "/Items/"+id, app("dashboard", admin),
		map[string]any{"Name": "Movie 0", "CommunityRating": 8.1, "CriticRating": 97}); status != http.StatusNoContent {
		t.Fatalf("saving as shown: %d %s", status, body)
	}
	if o := r.library.Overrides(parsed); o.CommunityRating != nil || o.CriticRating != nil {
		t.Errorf("saving MDBList's ratings as shown kept %v and %v", o.CommunityRating, o.CriticRating)
	}

	if status, body := r.call(http.MethodPost, "/Items/"+id, app("dashboard", admin),
		map[string]any{"Name": "Movie 0", "CommunityRating": 5, "CriticRating": 55}); status != http.StatusNoContent {
		t.Fatalf("editing: %d %s", status, body)
	}
	if got := r.scores(t, top)["Movie 0"]; got != "5 55" {
		t.Errorf("listed after the edit: %s", got)
	}
	var three TitleRatings
	r.get(t, "/Polyfin/Items/"+id+"/Ratings", r.token, &three)
	if got := describeThree(three); got != "5 55 60 false" {
		t.Errorf("the three scores after the edit: %s", got)
	}

	before := len(r.mdblist.requests())
	if status, body := r.call(http.MethodPost, "/Items/"+id+"/Refresh", app("dashboard", admin), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	r.scores(t, top)
	eventually(t, "asked again after a refresh", func() bool { return len(r.mdblist.requests()) == before+1 })
}

// SortBy=CriticRating sorts a listing read whole, as CommunityRating does:
// the titles of a collection, those without a critic rating first.
func TestCollectionsSortByCriticRating(t *testing.T) {
	t.Parallel()
	r := newRated(t, "server-key")
	var collections QueryResult
	r.get(t, "/Items?ParentId="+r.ids["Groups"], r.token, &collections)
	if len(collections.Items) != 1 {
		t.Fatalf("collections: %v", itemNames(collections.Items))
	}
	titles := "ParentId=" + collections.Items[0].Id + "&Recursive=true&IncludeItemTypes=Movie,Series"
	r.scores(t, titles)
	eventually(t, "rated", func() bool { return r.scores(t, titles)["Movie 0"] == "8.1 97" })
	for _, tc := range []struct {
		order string
		want  []string
	}{
		{"&SortBy=CriticRating,SortName&SortOrder=Descending,Ascending", []string{"Movie 0", "Movie 1", "Movie 2"}},
		{"&SortBy=CriticRating&SortOrder=Ascending", []string{"Movie 2", "Movie 1", "Movie 0"}},
	} {
		var page QueryResult
		r.get(t, "/Items?"+titles+tc.order, r.token, &page)
		if got := itemNames(page.Items); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.order, got, tc.want)
		}
	}
	r.setting(t, func(settings *accounts.Settings) { settings.CriticRatingShows = "audience" })
	var page QueryResult
	r.get(t, "/Items?"+titles+"&SortBy=CriticRating&SortOrder=Descending", r.token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Movie 1", "Movie 0", "Movie 2"}) {
		t.Errorf("by audience score: %v", got)
	}
}
