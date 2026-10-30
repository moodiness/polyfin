// Package taste keeps a taste profile for each user, built in the
// background from what they did with movies and series: those they
// finished, watched again or stopped early, liked, disliked or rated,
// marked as favorites and hid. A profile weighs the genres, directors,
// cast, decades and languages of those titles, the latest counting most.
// It ranks the titles of the Top picks for you home row, and reorders
// similar titles, suggestions and movie recommendations, once it is
// confident; until then, the user gets the titles as their catalogs order
// them. A profile stays on the server: only its user sees it, in My
// account.
package taste

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/library"
)

// Facet is what a profile weighs titles by.
type Facet string

const (
	FacetGenre    Facet = "genre"
	FacetDirector Facet = "director"
	FacetCast     Facet = "cast"
	FacetDecade   Facet = "decade"
	FacetLanguage Facet = "language"
)

// Facets are the facets, in the order a reason is looked for.
var Facets = []Facet{FacetGenre, FacetDirector, FacetCast, FacetDecade, FacetLanguage}

// facetWeights tell how much each facet counts in a title's score: its
// genres and directors fully, each of its first actors, its decade and
// its language half.
var facetWeights = map[Facet]float64{FacetGenre: 1, FacetDirector: 1, FacetCast: 0.5, FacetDecade: 0.5, FacetLanguage: 0.5}

const (
	// castCount is how many of a title's actors, from the first billed,
	// it is weighed by.
	castCount = 5
	// maxValues bounds the values a profile keeps of each facet, the
	// weightiest.
	maxValues = 100
	// halfLife is how long evidence takes to count half as much.
	halfLife = 180 * 24 * time.Hour
	// confidence is the positive evidence a profile needs to be
	// confident, from likedTitles titles at least: three movies finished
	// in the last months, say. Below it, rows and lists keep their own
	// order and no tagline shows.
	confidence  = 2.5
	likedTitles = 3
	// vetoBelow is the weight at or under which a genre is vetoed, once
	// vetoTitles titles at least weigh against it: the titles of a genre
	// the user disliked several titles of, without liking others, leave
	// Top picks for you and come last elsewhere.
	vetoBelow  = -1.5
	vetoTitles = 2
	// minMatch and maxMatch bound the percentage a tagline shows.
	minMatch, maxMatch = 60, 98
)

// Title is what a profile weighs of a movie or series.
type Title struct {
	Genres    []string
	Directors []string
	// Cast are the first actors billed, castCount at most.
	Cast     []string
	Year     int
	Language string
}

// TitleOf is what a profile weighs of item.
func TitleOf(item library.Item) Title {
	t := Title{Genres: item.Genres, Year: item.ProductionYear, Language: item.Language}
	for _, person := range item.People {
		switch {
		case person.Type == "Director":
			t.Directors = append(t.Directors, person.Name)
		case person.Type == "Actor" && len(t.Cast) < castCount:
			t.Cast = append(t.Cast, person.Name)
		}
	}
	return t
}

// values lists a title's values of facet, as display names; each value is
// weighed once, by key.
func (t Title) values(facet Facet) []string {
	switch facet {
	case FacetGenre:
		return t.Genres
	case FacetDirector:
		return t.Directors
	case FacetCast:
		return t.Cast[:min(castCount, len(t.Cast))]
	case FacetDecade:
		if t.Year > 0 {
			return []string{strconv.Itoa(t.Year/10*10) + "s"}
		}
	case FacetLanguage:
		if t.Language != "" {
			return []string{t.Language}
		}
	}
	return nil
}

// key is how a value is known whatever its case and spacing.
func key(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// Affinity is the weight of one value of a facet: positive for a value the
// user likes, negative for one they do not.
type Affinity struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	// Against counts the titles that weighed against the value.
	Against int `json:"against,omitempty"`
}

// vetoes reports whether a genre's affinity vetoes its titles.
func (a Affinity) vetoes() bool {
	return a.Weight <= vetoBelow && a.Against >= vetoTitles
}

// Profile is a user's taste. The zero value is a user nothing is known of.
type Profile struct {
	// Affinities are the weights of the values of each facet, by key.
	Affinities map[Facet]map[string]Affinity `json:"affinities,omitempty"`
	// Positive sums the weights of the titles the user liked, after their
	// decay, and Liked counts them; Titles counts the titles weighed.
	Positive float64 `json:"positive"`
	Liked    int     `json:"liked"`
	Titles   int     `json:"titles"`
	// Built is when the profile was built, zero if never.
	Built time.Time `json:"built"`
}

// Evidence is what a user did with a title.
type Evidence struct {
	Title    Title
	Activity Activity
}

// Build builds a profile from evidence at now. The same evidence in the
// same order gives the same profile.
func Build(evidence []Evidence, now time.Time) Profile {
	p := Profile{Affinities: map[Facet]map[string]Affinity{}, Built: now}
	for _, e := range evidence {
		w := e.Activity.Weight(now)
		if w == 0 {
			continue
		}
		p.Titles++
		if w > 0 {
			p.Positive += w
			p.Liked++
		}
		for _, facet := range Facets {
			seen := map[string]bool{}
			for _, name := range e.Title.values(facet) {
				k := key(name)
				if k == "" || seen[k] {
					continue
				}
				seen[k] = true
				values := p.Affinities[facet]
				if values == nil {
					values = map[string]Affinity{}
					p.Affinities[facet] = values
				}
				a := values[k]
				if a.Name == "" {
					a.Name = name
				}
				a.Weight += w
				if w < 0 {
					a.Against++
				}
				values[k] = a
			}
		}
	}
	for facet, values := range p.Affinities {
		p.Affinities[facet] = weightiest(values, maxValues)
	}
	return p
}

// weightiest keeps the n values of the greatest weight, positive or
// negative; those that weigh the same keep in the order of their keys.
func weightiest(values map[string]Affinity, n int) map[string]Affinity {
	if len(values) <= n {
		return values
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(math.Abs(values[b].Weight), math.Abs(values[a].Weight)), strings.Compare(a, b))
	})
	kept := make(map[string]Affinity, n)
	for _, k := range keys[:n] {
		kept[k] = values[k]
	}
	return kept
}

// Confident reports whether the profile knows enough of the user to
// rank titles and show taglines.
func (p Profile) Confident() bool {
	return p.Positive >= confidence && p.Liked >= likedTitles
}

// Certainty tells, from 0 to 1, how much the profile knows of the user:
// one half at the confidence gate, of the evidence or of the titles,
// whichever is further from it.
func (p Profile) Certainty() float64 {
	return min(p.Positive/(p.Positive+confidence), float64(p.Liked)/float64(p.Liked+likedTitles))
}

// weight is the weight of a value of facet, 0 when unknown.
func (p Profile) weight(facet Facet, value string) float64 {
	return p.Affinities[facet][key(value)].Weight
}

// squash brings a weight within (-1, 1): a value liked many times counts
// more than one liked once, but not without bound.
func squash(w float64) float64 {
	return w / (1 + math.Abs(w))
}

// contribution is what a value of facet adds to a title's score.
func (p Profile) contribution(facet Facet, value string) float64 {
	return facetWeights[facet] * squash(p.weight(facet, value))
}

// Leaning is a value of a facet the user leans to or away from, as My
// account shows it: Vetoed tells a genre they dislike (see Vetoed).
type Leaning struct {
	Name   string
	Weight float64
	Vetoed bool
}

// Leanings lists up to n values of facet the user likes, the most liked
// first, and up to n they dislike, the most disliked first; values of the
// same weight come in an order of their keys.
func (p Profile) Leanings(facet Facet, n int) (liked, disliked []Leaning) {
	keys := make([]string, 0, len(p.Affinities[facet]))
	for k := range p.Affinities[facet] {
		keys = append(keys, k)
	}
	values := p.Affinities[facet]
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(values[b].Weight, values[a].Weight), strings.Compare(a, b))
	})
	for _, k := range keys {
		if a := values[k]; a.Weight > 0 && len(liked) < n {
			liked = append(liked, Leaning{Name: a.Name, Weight: a.Weight})
		}
	}
	for _, k := range slices.Backward(keys) {
		if a := values[k]; a.Weight < 0 && len(disliked) < n {
			disliked = append(disliked, Leaning{Name: a.Name, Weight: a.Weight, Vetoed: facet == FacetGenre && a.vetoes()})
		}
	}
	return liked, disliked
}

// Score tells how much the user should like t: the more positive, the
// more; 0 for a title nothing is known of.
func (p Profile) Score(t Title) float64 {
	score := 0.0
	for _, facet := range Facets {
		seen := map[string]bool{}
		for _, value := range t.values(facet) {
			if k := key(value); k != "" && !seen[k] {
				seen[k] = true
				score += p.contribution(facet, value)
			}
		}
	}
	return score
}

// Vetoed reports whether t is of a genre the user dislikes.
func (p Profile) Vetoed(t Title) bool {
	return slices.ContainsFunc(t.Genres, func(genre string) bool { return p.Affinities[FacetGenre][key(genre)].vetoes() })
}

// Match is why a title is for the user, as a tagline tells it.
type Match struct {
	// Percent is how much the title matches, from minMatch to maxMatch.
	Percent int
	// Reason is the value of the title the user likes most, among its
	// genres, directors and cast, empty when none.
	Reason string
}

// Match tells how much t matches a confident profile. It reports false
// below the confidence gate, and for a title vetoed or not matching.
func (p Profile) Match(t Title) (Match, bool) {
	score := p.Score(t)
	if !p.Confident() || score <= 0 || p.Vetoed(t) {
		return Match{}, false
	}
	m := Match{Percent: minMatch + int(math.Round((maxMatch-minMatch)*score/(score+1)))}
	best := 0.0
	for _, facet := range []Facet{FacetGenre, FacetDirector, FacetCast} {
		for _, value := range t.values(facet) {
			if c := p.contribution(facet, value); c > best {
				best, m.Reason = c, value
			}
		}
	}
	return m, true
}

// Order orders items by how much the user should like them, the most
// first, those of a vetoed genre last; items that score the same keep
// their order. Below the confidence gate, only the vetoed move.
func Order[T any](p Profile, items []T, title func(T) Title) []T {
	scores := scored(p, items, title)
	ordered := slices.Clone(items)
	indexes := make([]int, len(items))
	for i := range indexes {
		indexes[i] = i
	}
	slices.SortStableFunc(indexes, func(a, b int) int { return compareScores(p, scores[a], scores[b]) })
	for i, index := range indexes {
		ordered[i] = items[index]
	}
	return ordered
}

// Blend orders items, already ranked by another measure (similar titles
// by how close they are), half by that rank and half by the user's taste:
// by the sum of the two ranks, the first rank breaking ties. Those of a
// vetoed genre come last. Below the confidence gate, only the vetoed move.
func Blend[T any](p Profile, items []T, title func(T) Title) []T {
	scores := scored(p, items, title)
	byTaste := make([]int, len(items))
	for i := range byTaste {
		byTaste[i] = i
	}
	slices.SortStableFunc(byTaste, func(a, b int) int { return compareScores(p, scores[a], scores[b]) })
	tasteRank := make([]int, len(items))
	for rank, index := range byTaste {
		tasteRank[index] = rank
	}
	indexes := make([]int, len(items))
	for i := range indexes {
		indexes[i] = i
	}
	slices.SortStableFunc(indexes, func(a, b int) int {
		if scores[a].vetoed != scores[b].vetoed {
			if scores[a].vetoed {
				return 1
			}
			return -1
		}
		if !p.Confident() {
			return 0
		}
		return cmp.Compare(a+tasteRank[a], b+tasteRank[b])
	})
	ordered := make([]T, len(items))
	for i, index := range indexes {
		ordered[i] = items[index]
	}
	return ordered
}

// Pick keeps the items the user may like, in the order they should like
// them: those of a vetoed genre are left out; below the confidence gate,
// the others keep their order.
func Pick[T any](p Profile, items []T, title func(T) Title) []T {
	kept := slices.DeleteFunc(slices.Clone(items), func(item T) bool { return p.Vetoed(title(item)) })
	return Order(p, kept, title)
}

// score is an item's score, and whether it is vetoed.
type score struct {
	value  float64
	vetoed bool
}

func scored[T any](p Profile, items []T, title func(T) Title) []score {
	scores := make([]score, len(items))
	for i, item := range items {
		t := title(item)
		scores[i] = score{p.Score(t), p.Vetoed(t)}
	}
	return scores
}

// compareScores orders the vetoed last and, above the confidence gate, the
// higher scores first.
func compareScores(p Profile, a, b score) int {
	if a.vetoed != b.vetoed {
		if a.vetoed {
			return 1
		}
		return -1
	}
	if !p.Confident() {
		return 0
	}
	return cmp.Compare(b.value, a.value)
}
