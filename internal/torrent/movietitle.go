package torrent

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// How a file's name holds a movie's title (see titleIn).
const (
	titleAbsent = iota
	// titleNamed: the name holds the title's words, and no number other
	// than a year comes right after them.
	titleNamed
	// titleDated: the movie's year, or one next to it, comes right after
	// the title's words.
	titleDated
)

// titleIn tells how the base name of the file at filePath holds a movie's
// title, given as its words (see nameWords), and its year, 0 when unknown.
// The title's words must come one after another, as a release's name
// writes them. A number right after them that is not a year tells another
// movie of the same name, a sequel ("Alien 3" for "Alien"), so it does not
// count; a year right after them tells the movie of that year, which a
// remake shares the name of, and a year within one of the movie's own
// counts as its own, as release dates differ from one country to another.
// Of all the places the title's words are found, the best counts.
func titleIn(filePath string, title []string, year int) int {
	if len(title) == 0 {
		return titleAbsent
	}
	words := nameWords(path.Base(strings.ReplaceAll(filePath, `\`, "/")))
	found := titleAbsent
	for start := 0; start+len(title) <= len(words); start++ {
		if !slices.Equal(words[start:start+len(title)], title) {
			continue
		}
		if start+len(title) == len(words) {
			found = max(found, titleNamed)
			continue
		}
		after := words[start+len(title)]
		named, number := wordNumber(after)
		switch {
		case !number:
			found = max(found, titleNamed)
		case named < 1900 || named > 2099 || len(after) != 4:
			// Another movie of the same name: a sequel's number.
		case year > 0 && named >= year-1 && named <= year+1:
			return titleDated
		default:
			found = max(found, titleNamed)
		}
	}
	return found
}

// nameWords splits a title or a file's name into its words: runs of
// letters and digits, in lower case. An apostrophe inside a word is
// dropped, as release names drop it ("Ocean's" and "Oceans" are one word).
func nameWords(name string) []string {
	var words []string
	var word strings.Builder
	end := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(unicode.ToLower(r))
		case (r == '\'' || r == '\u2019') && word.Len() > 0:
		default:
			end()
		}
	}
	end()
	return words
}

// wordNumber reads word as a number when it holds only ASCII digits; one
// too long to read is the largest number.
func wordNumber(word string) (int, bool) {
	for _, r := range word {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, _ := strconv.Atoi(word)
	return n, true
}
