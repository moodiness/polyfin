// Package plainsearch understands searches written in plain words, such as
// "80s sci-fi under 2 hours" or « comédies des années 80 de moins de 2
// heures », in English, French, German, Spanish, Italian and Portuguese.
//
// A search is understood when each of its words is: a decade or a year; a
// genre of the user's libraries; a runtime ("under 2 hours", "short"); a
// rating ("highly rated"); a language; movies or series; or a word that
// only links the others ("of", "the"). All its words are of one language,
// but numbers. It then stands for the rules of a smart list (see package
// smartlists), and names at least two of these: one word alone, such as
// "Drama" or "1917", is more often a title. Any other search is not
// understood.
//
// The words are those of the tables of words.go, compared without regard
// to letter case or accents. Nothing is asked of any other service.
package plainsearch

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/moodiness/polyfin/internal/smartlists"
)

const (
	// MaxLength bounds a search understood, in characters: it names the
	// collection it stands for, which may be saved as a smart list.
	MaxLength = smartlists.MaxNameLength
	// minCriteria is how many things a search understood names at least.
	minCriteria = 2
	// shortRuntime is the runtime "short" titles stay under, in minutes.
	shortRuntime = 90
	// highRating is the lowest IMDb rating of "highly rated" titles.
	highRating = 7.5
	// firstYear and lastYear bound the years a number stands for.
	firstYear, lastYear = 1900, 2099
)

// Query is what a search understood stands for: Rules, a group of rules
// all of which a title matches, and Type, "movie" or "series" when the
// search names one, else empty.
type Query struct {
	Rules smartlists.Rule
	Type  string
}

// Parse returns what text stands for, given the genres of the user's
// libraries, as their catalogs name them, and whether it was understood.
func Parse(text string, genres []string) (Query, bool) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > MaxLength {
		return Query{}, false
	}
	words := tokens(text)
	if len(words) < minCriteria {
		return Query{}, false
	}
	known := knownGenres(genres)
	named := genreEntries(known)
	for _, t := range tables() {
		if q, ok := t.parse(words, known, named); ok {
			return q, true
		}
	}
	return Query{}, false
}

// What a word or a phrase means.
type sense int8

const (
	link sense = iota
	movies
	series
	short
	rated
	language
	genre
	year
	decade
	runtime
)

// meaning is what a phrase means: the language code of a language, the
// concept of a genre (see concepts) or the library genres it names, the
// first year of a decade, a year, or a runtime in minutes.
type meaning struct {
	sense  sense
	value  string
	genres []string
	amount int
}

// What a slot of a phrase takes: one word, a number of the language,
// digits of a decade, or digits of a year.
type slotKind int8

const (
	literal slotKind = iota
	number
	decadeDigits
	yearDigits
)

type slot struct {
	kind slotKind
	word string
}

// entry is a phrase: its slots and what it means. A runtime's unit tells
// the minutes of its number.
type entry struct {
	slots []slot
	means []meaning
	unit  float64
}

// table is a language's phrases, its number words, and its genre concepts'
// phrases.
type table struct {
	entries []entry
	numbers map[string]float64
}

// match tells how many words from words the entry takes, and what they
// mean, 0 when it does not match them.
func (t *table) match(e entry, words []string, known map[string]string) (int, []meaning) {
	if len(e.slots) > len(words) {
		return 0, nil
	}
	means := slices.Clone(e.means)
	var amount float64
	for i, s := range e.slots {
		w := words[i]
		switch s.kind {
		case literal:
			if w != s.word {
				return 0, nil
			}
		case number:
			n, ok := t.numbers[w]
			if !ok {
				parsed, err := strconv.ParseFloat(w, 64)
				if err != nil || parsed <= 0 || parsed > 100 {
					return 0, nil
				}
				n = parsed
			}
			amount = n
		case decadeDigits:
			first, ok := decadeOf(w)
			if !ok {
				return 0, nil
			}
			means[0].amount = first
		case yearDigits:
			y, err := strconv.Atoi(w)
			if err != nil || len(w) != 4 || y < firstYear || y > lastYear {
				return 0, nil
			}
			means[0].amount = y
		}
	}
	for i, m := range means {
		switch {
		case m.sense == runtime:
			means[i].amount = int(math.Round(amount * e.unit))
		case m.sense == genre && m.genres == nil:
			means[i].genres = resolve(m.value, known)
			if len(means[i].genres) == 0 {
				return 0, nil
			}
		}
	}
	return len(e.slots), means
}

// decadeOf returns the first year of the decade digits stand for: the
// tens of a year from 1900 to 2090 as 1980, or as 80 for the 1980s, 00 and
// 10 for the 2000s and 2010s.
func decadeOf(w string) (int, bool) {
	n, err := strconv.Atoi(w)
	if err != nil || n%10 != 0 {
		return 0, false
	}
	switch {
	case len(w) == 2 && n < 20:
		return 2000 + n, true
	case len(w) == 2:
		return 1900 + n, true
	case len(w) == 4 && n >= firstYear && n <= lastYear:
		return n, true
	}
	return 0, false
}

// parse reads words with the table's phrases, the genres named, then the
// years, taking at each word the longest phrase, the first of the longest.
func (t *table) parse(words []string, known map[string]string, named []entry) (Query, bool) {
	var found []meaning
	for i := 0; i < len(words); {
		taken, means := 0, []meaning(nil)
		for _, list := range [][]entry{t.entries, named, universal} {
			for _, e := range list {
				if n, m := t.match(e, words[i:], known); n > taken {
					taken, means = n, m
				}
			}
		}
		if taken == 0 {
			return Query{}, false
		}
		found = append(found, means...)
		i += taken
	}
	return build(found)
}

// build makes the rules of what a search means: each thing once, but
// genres, which titles all have, and a year or a decade, not both.
func build(found []meaning) (Query, bool) {
	var q Query
	var genres [][]string
	var rules []smartlists.Rule
	seen := map[sense]bool{}
	criteria := 0
	for _, m := range found {
		if m.sense == link {
			continue
		}
		criteria++
		key := m.sense
		switch key {
		case series:
			key = movies
		case decade:
			key = year
		case short:
			key = runtime
		}
		if key == genre {
			if !slices.ContainsFunc(genres, func(g []string) bool { return slices.Equal(g, m.genres) }) {
				genres = append(genres, m.genres)
			}
			continue
		}
		if seen[key] {
			return Query{}, false
		}
		seen[key] = true
		switch m.sense {
		case movies:
			q.Type = "movie"
		case series:
			q.Type = "series"
		case short:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldRuntime, Max: new(float64(shortRuntime - 1))})
		case runtime:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldRuntime, Max: new(float64(m.amount - 1))})
		case rated:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldRating, Min: new(highRating)})
		case language:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldLanguage, Value: m.value})
		case year:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldYear, Min: new(float64(m.amount)), Max: new(float64(m.amount))})
		case decade:
			rules = append(rules, smartlists.Rule{Field: smartlists.FieldYear, Min: new(float64(m.amount)), Max: new(float64(m.amount + 9))})
		}
	}
	if criteria < minCriteria {
		return Query{}, false
	}
	var all []smartlists.Rule
	if q.Type != "" {
		all = append(all, smartlists.Rule{Field: smartlists.FieldType, Value: q.Type})
	}
	for _, names := range genres {
		if len(names) == 1 {
			all = append(all, smartlists.Rule{Field: smartlists.FieldGenre, Value: names[0]})
			continue
		}
		group := smartlists.Rule{Field: smartlists.FieldGroup, Match: smartlists.MatchAny}
		for _, name := range names {
			group.Rules = append(group.Rules, smartlists.Rule{Field: smartlists.FieldGenre, Value: name})
		}
		all = append(all, group)
	}
	q.Rules = smartlists.Rule{Field: smartlists.FieldGroup, Match: smartlists.MatchAll, Rules: append(all, rules...)}
	if q.Rules.Validate() != nil {
		return Query{}, false
	}
	return q, true
}

// universal are the phrases of every language: a year.
var universal = []entry{{slots: []slot{{kind: yearDigits}}, means: []meaning{{sense: year}}}}

// knownGenres maps the genres of the libraries, as phrases compare them, to
// the first of their spellings.
func knownGenres(genres []string) map[string]string {
	known := map[string]string{}
	for _, g := range genres {
		key := strings.Join(tokens(g), " ")
		if _, ok := known[key]; !ok && key != "" {
			known[key] = strings.TrimSpace(g)
		}
	}
	return known
}

// genreEntries are the phrases of the libraries' genres as they name them,
// and with an s, as a plural.
func genreEntries(known map[string]string) []entry {
	keys := make([]string, 0, len(known))
	for key := range known {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var entries []entry
	for _, key := range keys {
		words := strings.Fields(key)
		means := []meaning{{sense: genre, genres: []string{known[key]}}}
		entries = append(entries, entry{slots: literals(words), means: means})
		if last := words[len(words)-1]; !strings.HasSuffix(last, "s") {
			plural := append(slices.Clone(words[:len(words)-1]), last+"s")
			entries = append(entries, entry{slots: literals(plural), means: means})
		}
	}
	return entries
}

// resolve lists the libraries' genres a genre concept stands for, in the
// order of the concept's names.
func resolve(concept string, known map[string]string) []string {
	var names []string
	for _, candidate := range concepts[concept] {
		if name, ok := known[strings.Join(tokens(candidate), " ")]; ok && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func literals(words []string) []slot {
	slots := make([]slot, len(words))
	for i, w := range words {
		slots[i] = slot{kind: literal, word: w}
	}
	return slots
}

// tokens splits text into the words phrases compare: in lower case,
// without accents, apart at anything but letters and digits, and where
// letters and digits meet ("80s" gives "80" and "s"). A number keeps its
// decimal point or comma, as a point.
func tokens(text string) []string {
	runes := []rune(fold(text))
	var result []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			result = append(result, string(word))
			word = word[:0]
		}
	}
	for i, r := range runes {
		digit := unicode.IsDigit(r)
		switch {
		case unicode.IsLetter(r) || digit:
			if len(word) > 0 && unicode.IsDigit(word[len(word)-1]) != digit {
				flush()
			}
			word = append(word, r)
		case (r == '.' || r == ',') && len(word) > 0 && unicode.IsDigit(word[len(word)-1]) && i+1 < len(runes) && unicode.IsDigit(runes[i+1]):
			word = append(word, '.')
		default:
			flush()
		}
	}
	flush()
	return result
}

// fold is text in lower case, without accents.
func fold(text string) string {
	var folded strings.Builder
	for _, r := range norm.NFKD.String(text) {
		if !unicode.Is(unicode.Mn, r) {
			folded.WriteRune(unicode.ToLower(r))
		}
	}
	return folded.String()
}

// phrases expands a list of phrases: alternatives apart with "|", a word's
// endings within parentheses, apart with "/", an empty one for the word as
// it stands: "film(/s)" gives "film" and "films". A word within braces
// stays as it is, for a slot (see compile).
func phrases(list string) [][]string {
	var result [][]string
	for alternative := range strings.SplitSeq(list, "|") {
		forms := [][]string{nil}
		for _, word := range strings.Fields(alternative) {
			if strings.HasPrefix(word, "{") {
				for i := range forms {
					forms[i] = append(forms[i], word)
				}
				continue
			}
			stem, endings := word, []string{""}
			if i := strings.IndexByte(word, '('); i >= 0 && strings.HasSuffix(word, ")") {
				stem, endings = word[:i], strings.Split(word[i+1:len(word)-1], "/")
			}
			var next [][]string
			for _, f := range forms {
				for _, ending := range endings {
					next = append(next, append(slices.Clone(f), tokens(stem+ending)...))
				}
			}
			forms = next
		}
		result = append(result, forms...)
	}
	return result
}

// tables are the languages' tables, compiled once, in the order searches
// try them.
var tables = sync.OnceValue(func() []*table {
	compiled := make([]*table, 0, len(languages))
	for _, w := range languages {
		compiled = append(compiled, w.compile())
	}
	return compiled
})

// compile makes a language's table from its words.
func (w words) compile() *table {
	t := &table{numbers: map[string]float64{}}
	for word, n := range w.numbers {
		t.numbers[fold(word)] = n
	}
	add := func(list string, means ...meaning) {
		for _, p := range phrases(list) {
			t.entries = append(t.entries, entry{slots: literals(p), means: means})
		}
	}
	add(w.links, meaning{sense: link})
	add(w.movies, meaning{sense: movies})
	add(w.series, meaning{sense: series})
	add(w.short, meaning{sense: short})
	add(w.rated, meaning{sense: rated})
	for _, code := range sortedKeys(w.languages) {
		add(w.languages[code], meaning{sense: language, value: code})
	}
	for _, concept := range sortedKeys(w.genres) {
		add(w.genres[concept], meaning{sense: genre, value: concept})
	}
	for _, concept := range sortedKeys(w.genreMovies) {
		add(w.genreMovies[concept], meaning{sense: genre, value: concept}, meaning{sense: movies})
	}
	for _, concept := range sortedKeys(w.genreSeries) {
		add(w.genreSeries[concept], meaning{sense: genre, value: concept}, meaning{sense: series})
	}
	for _, under := range phrases(w.under) {
		for _, unit := range []struct {
			list    string
			minutes float64
		}{{w.hours, 60}, {w.minutes, 1}} {
			for _, p := range phrases(unit.list) {
				slots := append(literals(under), slot{kind: number})
				t.entries = append(t.entries, entry{slots: append(slots, literals(p)...), means: []meaning{{sense: runtime}}, unit: unit.minutes})
			}
		}
	}
	for _, p := range phrases(w.decades) {
		// A pattern holds {d}, digits, or {w}, a decade's words.
		if i := slices.Index(p, "{w}"); i >= 0 {
			for _, first := range sortedKeys(w.decadeWords) {
				for _, spelled := range phrases(w.decadeWords[first]) {
					filled := slices.Concat(p[:i], spelled, p[i+1:])
					t.entries = append(t.entries, entry{slots: literals(filled), means: []meaning{{sense: decade, amount: first}}})
				}
			}
			continue
		}
		slots := make([]slot, len(p))
		for i, word := range p {
			slots[i] = slot{kind: literal, word: word}
			if word == "{d}" {
				slots[i] = slot{kind: decadeDigits}
			}
		}
		t.entries = append(t.entries, entry{slots: slots, means: []meaning{{sense: decade}}})
	}
	return t
}

func sortedKeys[K string | int, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
