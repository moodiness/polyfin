package taste

import (
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)

// pick is a candidate title, by name.
type pick struct {
	name  string
	title Title
}

func titleOf(p pick) Title { return p.title }

func names(picks []pick) []string {
	result := []string{}
	for _, p := range picks {
		result = append(result, p.name)
	}
	return result
}

// finished is a movie of genres finished at.
func finished(at time.Time, genres ...string) Evidence {
	return Evidence{Title: Title{Genres: genres}, Activity: Activity{Finished: 1, At: at}}
}

// disliked is a movie of genres disliked at, not finished.
func disliked(at time.Time, genres ...string) Evidence {
	return Evidence{Title: Title{Genres: genres}, Activity: Activity{Rating: new(1.0), At: at}}
}

// candidates are titles in the order their catalog gives them, the most
// popular first.
var candidates = []pick{
	{"Comedy", Title{Genres: []string{"Comedy"}}},
	{"Drama", Title{Genres: []string{"Drama"}}},
	{"Thriller", Title{Genres: []string{"Thriller"}}},
	{"Horror", Title{Genres: []string{"Horror"}}},
	{"Thriller drama", Title{Genres: []string{"thriller", "Drama"}}},
}

// A user who finished three thrillers gets the thrillers first, whatever
// their catalog's order; the titles their profile ranks the same keep it.
func TestThreeThrillersFinishedComeFirst(t *testing.T) {
	t.Parallel()
	profile := Build([]Evidence{
		finished(now.Add(-72*time.Hour), "Thriller"),
		finished(now.Add(-48*time.Hour), "Thriller", "Crime"),
		finished(now.Add(-24*time.Hour), "Thriller"),
	}, now)
	if !profile.Confident() {
		t.Fatalf("not confident: %+v", profile)
	}
	if got := names(Pick(profile, candidates, titleOf)); !slices.Equal(got, []string{"Thriller", "Thriller drama", "Comedy", "Drama", "Horror"}) {
		t.Errorf("top picks: %q", got)
	}
	if got := names(Order(profile, candidates, titleOf)); got[0] != "Thriller" || got[1] != "Thriller drama" {
		t.Errorf("suggestions: %q", got)
	}
}

// A genre two titles the user disliked weigh against is vetoed: its
// titles leave Top picks for you, and come last elsewhere, even below the
// confidence gate. One disliked title vetoes none of its genres, and a
// genre the user also liked titles of is not vetoed.
func TestDislikedGenreIsVetoed(t *testing.T) {
	t.Parallel()
	evidence := []Evidence{
		finished(now.Add(-72*time.Hour), "Thriller"),
		finished(now.Add(-48*time.Hour), "Thriller"),
		finished(now.Add(-24*time.Hour), "Thriller", "Drama"),
		disliked(now.Add(-2*time.Hour), "Horror", "Drama"),
	}
	once := Build(evidence, now)
	if !once.Confident() {
		t.Fatalf("not confident: %+v", once)
	}
	if got := names(Pick(once, candidates, titleOf)); !slices.Equal(got, []string{"Thriller", "Thriller drama", "Comedy", "Drama", "Horror"}) {
		t.Errorf("top picks after one disliked horror drama: %q", got)
	}
	twice := Build(append(evidence, disliked(now.Add(-time.Hour), "Horror")), now)
	if got := names(Pick(twice, candidates, titleOf)); !slices.Equal(got, []string{"Thriller", "Thriller drama", "Comedy", "Drama"}) {
		t.Errorf("top picks after two disliked horror titles: %q", got)
	}
	if got := names(Order(twice, candidates, titleOf)); got[len(got)-1] != "Horror" {
		t.Errorf("suggestions: %q", got)
	}
	if got := names(Blend(twice, []pick{candidates[3], candidates[2], candidates[0]}, titleOf)); !slices.Equal(got, []string{"Thriller", "Comedy", "Horror"}) {
		t.Errorf("similar titles: %q", got)
	}
	if twice.Vetoed(candidates[1].title) {
		t.Error("drama, of one disliked title and of a thriller finished, is vetoed")
	}
	// A new user who disliked one horror movie, then another.
	newcomer := Build([]Evidence{disliked(now.Add(-2*time.Hour), "Horror")}, now)
	if newcomer.Confident() || newcomer.Vetoed(candidates[3].title) {
		t.Fatalf("after one disliked horror movie: confident %v, vetoed %v", newcomer.Confident(), newcomer.Vetoed(candidates[3].title))
	}
	newcomer = Build([]Evidence{disliked(now.Add(-2*time.Hour), "Horror"), disliked(now.Add(-time.Hour), "Horror")}, now)
	if got := names(Pick(newcomer, candidates, titleOf)); !slices.Equal(got, []string{"Comedy", "Drama", "Thriller", "Thriller drama"}) {
		t.Errorf("a new user's top picks: %q", got)
	}
}

// Below the confidence gate, nothing is reordered and no title matches;
// above it, a title matches by what the user likes most of it.
func TestConfidenceGateHidesTheMatch(t *testing.T) {
	t.Parallel()
	thriller := Title{Genres: []string{"Thriller"}, Directors: []string{"Some Director"}}
	var nobody Profile
	if _, ok := nobody.Match(thriller); ok {
		t.Error("a user nothing is known of matches")
	}
	// One movie finished and liked weighs enough, but is one title.
	liked := Evidence{Title: thriller, Activity: Activity{Finished: 1, Rating: new(10.0), At: now.Add(-time.Hour)}}
	newcomer := Build([]Evidence{liked}, now)
	if _, ok := newcomer.Match(thriller); ok || newcomer.Confident() || newcomer.Certainty() >= 0.5 {
		t.Errorf("a new user: confident %v, certainty %v", newcomer.Confident(), newcomer.Certainty())
	}
	if got := names(Order(newcomer, candidates, titleOf)); !slices.Equal(got, names(candidates)) {
		t.Errorf("a new user's suggestions: %q", got)
	}
	regular := Build([]Evidence{finished(now.Add(-3*time.Hour), "Thriller"), finished(now.Add(-2*time.Hour), "Thriller"), liked}, now)
	match, ok := regular.Match(thriller)
	if !ok || match.Percent < minMatch || match.Percent > maxMatch || match.Reason != "Thriller" || regular.Certainty() < 0.5 {
		t.Errorf("match: %+v %v, certainty %v", match, ok, regular.Certainty())
	}
	if _, ok := regular.Match(Title{Genres: []string{"Comedy"}}); ok {
		t.Error("a title of nothing the user likes matches")
	}
}

// Evidence counts less as it ages: comedies finished yesterday come
// before thrillers finished two years ago, as many times.
func TestOlderEvidenceCountsLess(t *testing.T) {
	t.Parallel()
	profile := Build([]Evidence{
		finished(now.AddDate(-2, 0, 0), "Thriller"),
		finished(now.AddDate(-2, 0, 0), "Thriller"),
		finished(now.Add(-24*time.Hour), "Comedy"),
		finished(now.Add(-24*time.Hour), "Comedy"),
		finished(now.Add(-24*time.Hour), "Drama"),
	}, now)
	if got := names(Pick(profile, candidates[:3], titleOf)); !slices.Equal(got, []string{"Comedy", "Drama", "Thriller"}) {
		t.Errorf("top picks: %q", got)
	}
}

// What a user did weighs as Activity.Weight tells: a movie watched again
// counts more, an early stop counts against once a few days passed, and a
// hidden title counts against.
func TestActivityWeights(t *testing.T) {
	t.Parallel()
	weekAgo := now.AddDate(0, 0, -7)
	for _, c := range []struct {
		name     string
		activity Activity
		want     float64
	}{
		{"finished", Activity{Finished: 1}, 1},
		{"watched three times", Activity{Finished: 3}, 2},
		{"watched five times", Activity{Finished: 5}, 2},
		{"seven episodes", Activity{Series: true, Finished: 7}, 2.5},
		{"stopped early today", Activity{Early: true, At: now.Add(-time.Hour)}, 0},
		{"stopped early a week ago", Activity{Early: true, At: weekAgo}, -Activity{Finished: 1, At: weekAgo}.Weight(now)},
		{"liked", Activity{Rating: new(10.0)}, 2},
		{"disliked", Activity{Rating: new(1.0)}, -1.6},
		{"favorite", Activity{Favorite: true}, 1.5},
		{"hidden", Activity{Hidden: true}, -1},
		{"half a year ago", Activity{Finished: 1, At: now.Add(-halfLife)}, 0.5},
	} {
		if got := c.activity.Weight(now); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// A play that stopped before a tenth of the title, or before the resume
// point Jellyfin keeps, stopped early.
func TestEarly(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		position, runtime time.Duration
		want              bool
	}{
		{0, 0, true},
		{0, time.Hour, true},
		{5 * time.Minute, time.Hour, true},
		{10 * time.Minute, time.Hour, false},
		{5 * time.Minute, 0, false},
	} {
		if got := Early(c.position, c.runtime); got != c.want {
			t.Errorf("Early(%v, %v) = %v", c.position, c.runtime, got)
		}
	}
}
