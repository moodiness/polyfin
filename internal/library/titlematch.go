package library

import (
	"path"
	"slices"
	"strings"
	"unicode"
)

// withoutMismatched leaves out, when Settings.ExactTitleMatch is on, the
// versions of a movie or an episode whose file names a longer title
// holding the movie's or the series' own: a search of Friends finds
// Smiling Friends' releases too. Versions without a file name, or whose title
// cannot be read from it, are kept.
func (s *Service) withoutMismatched(t target, versions []Version) []Version {
	if !s.settings().ExactTitleMatch || t.name == "" || (t.kind != KindMovie && t.kind != KindEpisode) {
		return versions
	}
	want := matchKey(t.name)
	if want == "" {
		return versions
	}
	return slices.DeleteFunc(versions, func(v Version) bool {
		got := releaseTitle(v.Filename)
		return got != "" && otherTitleWith(got, want)
	})
}

// otherTitleWith reports whether a release's title is another title that
// holds the title's words, as Smiling Friends holds Friends'. A release
// named otherwise, in another language say, is not told apart.
func otherTitleWith(release, title string) bool {
	if sameReleaseTitle(release, title) {
		return false
	}
	bare := func(text string) string {
		text = strings.TrimPrefix(text, "the ")
		fields := strings.Fields(text)
		if n := len(fields); n > 1 && isYear(fields[n-1]) {
			fields = fields[:n-1]
		}
		return " " + strings.Join(fields, " ") + " "
	}
	return strings.Contains(bare(release), bare(title))
}

// sameReleaseTitle compares a release's title with the title's own, read with
// matchKey: equal, or equal once a leading "the" or a trailing year is
// left out of either.
func sameReleaseTitle(release, title string) bool {
	if release == title {
		return true
	}
	trim := func(text string) string {
		text = strings.TrimPrefix(text, "the ")
		fields := strings.Fields(text)
		if n := len(fields); n > 1 && isYear(fields[n-1]) {
			fields = fields[:n-1]
		}
		return strings.Join(fields, " ")
	}
	return trim(release) == trim(title)
}

// matchKey reads a name for comparison: in lower case, "&" as "and",
// letters and digits only, apostrophes dropped, the rest as single spaces.
func matchKey(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, "&", " and "))
	var b strings.Builder
	space := false
	for _, r := range name {
		switch {
		case r == '\'' || r == '’':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// releaseTitle reads the title a release's file name starts with, as
// matchKey reads it: the words before its year, its season and episode,
// its resolution or its first tag in brackets; a leading [group] is left
// out. Empty when there is no such word to end it, or nothing before it.
func releaseTitle(filename string) string {
	name := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	if filename == "" {
		return ""
	}
	name = strings.TrimSpace(name)
	for strings.HasPrefix(name, "[") {
		end := strings.Index(name, "]")
		if end < 0 {
			break
		}
		name = strings.TrimSpace(name[end+1:])
	}
	if i := strings.IndexAny(name, "[("); i > 0 {
		inside := name[i+1:]
		if j := strings.IndexAny(inside, "])"); j >= 0 && isYear(strings.TrimSpace(inside[:j])) {
			name = name[:i]
		}
	}
	words := strings.Fields(matchKey(name))
	for i, word := range words {
		if i > 0 && endsTitle(word) {
			return strings.Join(words[:i], " ")
		}
	}
	if len(words) < len(strings.Fields(matchKey(strings.TrimSuffix(path.Base(filename), path.Ext(filename))))) {
		return strings.Join(words, " ")
	}
	return ""
}

func isYear(word string) bool {
	return len(word) == 4 && (strings.HasPrefix(word, "19") || strings.HasPrefix(word, "20")) && allDigits(word)
}

func allDigits(word string) bool {
	for _, r := range word {
		if r < '0' || r > '9' {
			return false
		}
	}
	return word != ""
}

// endsTitle reports whether a word of a release name follows its title.
func endsTitle(word string) bool {
	if isYear(word) {
		return true
	}
	switch word {
	case "480p", "576p", "720p", "1080p", "1080i", "2160p", "4k", "uhd", "complete", "season", "web", "webrip", "webdl",
		"bluray", "bdrip", "brrip", "hdtv", "dvdrip", "remux", "x264", "x265", "h264", "h265", "hevc", "av1", "proper", "repack",
		"hdr", "hdr10", "dv", "sdr", "imax", "extended", "unrated", "uncut", "remastered", "directors", "theatrical", "internal", "limited",
		"multi", "dual", "dubbed", "subbed", "english", "eng", "french", "truefrench", "vff", "vf", "vostfr", "german", "ger", "spanish",
		"esp", "latino", "italian", "ita", "portuguese", "por", "russian", "rus", "hindi", "japanese", "jpn", "korean", "kor", "chinese",
		"nordic", "swedish", "danish", "norwegian", "finnish", "dutch", "polish", "turkish", "arabic":
		return true
	}
	// S01, S01E01, S01E01E02, 1x01.
	if len(word) >= 3 && word[0] == 's' && allDigits(word[1:3]) {
		return len(word) == 3 || word[3] == 'e' || allDigits(word[1:])
	}
	if x := strings.IndexByte(word, 'x'); x > 0 && x < len(word)-1 && allDigits(word[:x]) && allDigits(word[x+1:]) {
		return true
	}
	return false
}
