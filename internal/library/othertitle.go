package library

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// seasonWords name a season before its number, and episodeWords an
// episode, in the languages releases are named in.
var (
	seasonWords = map[string]bool{"season": true, "seasons": true, "saison": true, "saisons": true, "temporada": true,
		"temporadas": true, "stagione": true, "stagioni": true, "staffel": true, "seizoen": true}
	episodeWords = map[string]bool{"episode": true, "episodes": true, "ep": true, "épisode": true, "episodio": true,
		"folge": true, "aflevering": true}
)

// OtherTitle reads whether the labels of a stream, an addon's name, title,
// description or file name, each line of them apart, name another title
// than the one the stream is listed for, and returns the marker naming
// it; empty when none does. name is the movie's or the series' name.
//
// For an episode, of season and episode, episode above 0: a line naming
// seasons or episodes after the series' own words (see episodeSpans), as
// S01E02, 1x02, S01E01-E04, S01, S01-S03 or Season 2, names another title
// when none of these holds the episode. A season pack or a range holding
// it, where the file played is picked inside the torrent, is kept, as is
// a line naming no season.
//
// For a movie of year, above 0: a line naming the movie, whose next word
// is a year more than one year from it, names another movie of the same
// name. A line not naming the movie tells no year: its first words may be
// another title, such as a translated one, holding a year of its own.
func OtherTitle(name string, season, episode, year int, labels ...string) string {
	title := lowerWords(name)
	for _, label := range labels {
		for line := range strings.Lines(label) {
			words := releaseWords(line)
			start := wordsAfter(words, title)
			switch {
			case episode > 0:
				spans := episodeSpans(words[max(start, 0):])
				if len(spans) > 0 && !slices.ContainsFunc(spans, func(s span) bool { return s.holds(season, episode) }) {
					return spans[0].String()
				}
			case year > 0 && start >= 0 && start < len(words):
				if named, ok := number(words[start].text, 4); ok && named >= 1900 && named <= 2099 && (named < year-1 || named > year+1) {
					return strconv.Itoa(named)
				}
			}
		}
	}
	return ""
}

// releaseWord is a word of a release's name, its letters and digits in
// lower case, and ranged when a dash or a tilde comes before it, joining
// it to the word before as the end of a range (S01-S03, E01~E04).
type releaseWord struct {
	text   string
	ranged bool
}

// releaseWords splits a line of a release's name into its words.
func releaseWords(line string) []releaseWord {
	var words []releaseWord
	var word strings.Builder
	ranged := false
	for _, r := range line + " " {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
			continue
		}
		if word.Len() > 0 {
			words = append(words, releaseWord{strings.ToLower(word.String()), ranged})
			word.Reset()
			ranged = false
		}
		if r == '-' || r == '~' || r == '–' {
			ranged = true
		}
	}
	return words
}

// wordsAfter is where the words of title, in lower case, end among words,
// where they first come; -1 when they do not, or title has none.
func wordsAfter(words []releaseWord, title []string) int {
	if len(title) == 0 {
		return -1
	}
	for i := 0; i+len(title) <= len(words); i++ {
		if slices.EqualFunc(words[i:i+len(title)], title, func(w releaseWord, t string) bool { return w.text == t }) {
			return i + len(title)
		}
	}
	return -1
}

// span is what a marker of a release's name holds: the seasons from
// season to lastSeason, and in a single season the episodes from episode
// to lastEpisode, each of them for episode 0.
type span struct {
	season, lastSeason   int
	episode, lastEpisode int
}

// holds reports whether the span holds an episode.
func (s span) holds(season, episode int) bool {
	return season >= s.season && season <= s.lastSeason && (s.episode == 0 || episode >= s.episode && episode <= s.lastEpisode)
}

// String writes the span as releases do: S01E02, S01E01-E04, S02 or
// S01-S03.
func (s span) String() string {
	text := fmt.Sprintf("S%02d", s.season)
	if s.episode > 0 {
		text += fmt.Sprintf("E%02d", s.episode)
	}
	switch {
	case s.lastSeason != s.season:
		text += fmt.Sprintf("-S%02d", s.lastSeason)
	case s.lastEpisode != s.episode:
		text += fmt.Sprintf("-E%02d", s.lastEpisode)
	}
	return text
}

// episodeSpans reads the seasons and episodes the words of a release's
// name name: S01E02, S01E02E03, 1x02, S01, Season 1, each with what
// follows it: its episode (S01 E02, S01.E02, Season 1 Episode 2), the end
// of its range after a dash (S01E01-E04, S01E01-04, 1x01-1x04, S01-S03,
// S01-03, Season 1-3), and more episodes (S01E01 E02).
func episodeSpans(words []releaseWord) []span {
	var spans []span
	for i := 0; i < len(words); i++ {
		var s span
		if season, first, last, ok := seasonMarker(words[i].text); ok {
			s = span{season, season, first, last}
		} else if season, episode, ok := crossMarker(words[i].text); ok {
			s = span{season, season, episode, episode}
		} else if seasonWords[words[i].text] && i+1 < len(words) {
			season, ok := number(words[i+1].text, 2)
			if !ok {
				continue
			}
			i++
			s = span{season, season, 0, 0}
		} else {
			continue
		}
		for i+1 < len(words) {
			next := words[i+1]
			if s.episode == 0 && s.lastSeason == s.season && !next.ranged {
				if episode, ok := episodeMarker(next.text); ok {
					s.episode, s.lastEpisode = episode, episode
					i++
					continue
				}
				if episodeWords[next.text] && i+2 < len(words) {
					if episode, ok := number(words[i+2].text, 4); ok {
						s.episode, s.lastEpisode = episode, episode
						i += 2
						continue
					}
				}
			}
			if !s.extend(next) {
				break
			}
			i++
		}
		spans = append(spans, s)
	}
	return spans
}

// extend extends a span to the word after it, when that word ends its
// range, or names another of its episodes, and reports whether it did.
func (s *span) extend(next releaseWord) bool {
	end, ok := 0, false
	if s.episode > 0 {
		end, ok = episodeMarker(next.text)
		if !ok && next.ranged {
			end, ok = number(next.text, 4)
			if season, first, last, marked := seasonMarker(next.text); !ok && marked && season == s.season && first > 0 {
				end, ok = last, true
			}
			if season, episode, crossed := crossMarker(next.text); !ok && crossed && season == s.season {
				end, ok = episode, true
			}
		}
		if !ok || end < s.episode {
			return false
		}
		s.lastEpisode = max(s.lastEpisode, end)
		return true
	}
	if !next.ranged {
		return false
	}
	end, ok = number(next.text, 2)
	if season, first, _, marked := seasonMarker(next.text); !ok && marked && first == 0 {
		end, ok = season, true
	}
	if !ok || end < s.season {
		return false
	}
	s.lastSeason = end
	return true
}

// seasonMarker reads a season, S01, or its episodes, S01E02 or
// S01E02E03, from the first to the last; first is 0 for a season.
func seasonMarker(word string) (season, first, last int, ok bool) {
	rest, found := strings.CutPrefix(word, "s")
	if !found {
		return 0, 0, 0, false
	}
	seasonText, episodes, numbered := strings.Cut(rest, "e")
	if season, ok = number(seasonText, 2); !ok || !numbered {
		return season, 0, 0, ok
	}
	for i, text := range strings.Split(episodes, "e") {
		episode, ok := number(text, 4)
		if !ok {
			return 0, 0, 0, false
		}
		if i == 0 {
			first = episode
		}
		last = max(last, episode)
	}
	return season, first, last, true
}

// crossMarker reads an episode written 1x02.
func crossMarker(word string) (season, episode int, ok bool) {
	seasonText, episodeText, found := strings.Cut(word, "x")
	if !found {
		return 0, 0, false
	}
	season, seasonOK := number(seasonText, 2)
	episode, episodeOK := number(episodeText, 3)
	return season, episode, seasonOK && episodeOK
}

// episodeMarker reads an episode written E02.
func episodeMarker(word string) (int, bool) {
	text, found := strings.CutPrefix(word, "e")
	if !found {
		return 0, false
	}
	return number(text, 4)
}

// number reads a word of 1 to digits digits.
func number(word string, digits int) (int, bool) {
	if word == "" || len(word) > digits || strings.ContainsFunc(word, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, false
	}
	n, _ := strconv.Atoi(word)
	return n, true
}
