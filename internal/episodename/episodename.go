// Package episodename reads what the names of video files and their
// folders tell of the episodes they hold, as releases and media servers
// write them: S01E02, 1x02, or E02 in a season's folder. Local folders
// read their shows' episodes with it, and torrents find an episode's file
// in a season's pack.
package episodename

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// videoExtensions are the files read as videos, by their extension.
// .m2t, .tp and .trp are MPEG-TS, as .ts, .m2ts and .mts are.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".ts": true, ".m2ts": true, ".mts": true,
	".m2t": true, ".tp": true, ".trp": true,
	".webm": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".vob": true, ".flv": true, ".ogv": true,
}

// Video reports whether name is a video's, by its extension.
func Video(name string) bool {
	return videoExtensions[strings.ToLower(path.Ext(name))]
}

var (
	// Tag matches S01E02, s01e02, S01E01-E02, S01E01E02 and S01E01-02,
	// Extra each episode past the first in its third group, of which Last
	// keeps those that end a range; Cross matches 1x02; Alone E02 or
	// Episode 2, in a season folder.
	Tag   = regexp.MustCompile(`(?i)\bs(\d{1,3})[ ._-]?e(\d{1,4})((?:(?:-e?|[ ._]?-?[ ._]?e)\d{1,4})*)`)
	Extra = regexp.MustCompile(`(?i)(?:-e?|[ ._]?-?[ ._]?e)(\d{1,4})`)
	Cross = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	Alone = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:e|ep|episode)[ ._]?(\d{1,4})\b`)
	// SeasonFolder names a season's folder: Season 01, Saison 1, S01;
	// "Specials" is season 0.
	SeasonFolder = regexp.MustCompile(`(?i)^(?:season|series|saison|staffel|temporada|s)[ ._-]*(\d{1,3})$`)
	// seasonWord is a season told among a folder's words, as a season's
	// pack names it: Show.S01.1080p, Show Season 1.
	seasonWord = regexp.MustCompile(`(?i)\b(?:s|season[ ._-]?)(\d{1,3})\b`)
)

// Episodes are the episodes a file holds: those from First to Last of
// Season.
type Episodes struct {
	Season, First, Last int
}

// Holds reports whether the file holds episode of season.
func (e Episodes) Holds(season, episode int) bool {
	return e.Season == season && episode >= e.First && episode <= e.Last
}

// Last is the last episode of a name whose episode tag (see Tag) tells
// first, its following episodes in name[from:to], Tag's third group. As
// Jellyfin 12.4 reads a range's end
// (Emby.Naming/TV/EpisodePathParser.cs, lines 151-167, and the
// endingepnumber groups of Emby.Naming/Common/NamingOptions.cs, lines
// 771-782), an episode ending it has at most 3 digits, is not followed by
// another digit, nor by a "p" or an "i", as a resolution is, and ends no
// range when lower than first: "S09E14-1080p" and "S01E01-2022", a year,
// hold one episode, "S01E01-E02", "S01E01-02" and "S01E01E02" two.
func Last(name string, first, from, to int) int {
	last := first
	for _, m := range Extra.FindAllStringSubmatchIndex(name[from:to], -1) {
		start, end := from+m[2], from+m[3]
		if end-start > 3 || end < len(name) && strings.ContainsRune("0123456789iIpP", rune(name[end])) {
			continue
		}
		if number, _ := strconv.Atoi(name[start:end]); number > last {
			last = number
		}
	}
	return last
}

// Of reads the episodes a video's path tells: its name's S01E02 or 1x02,
// else its name's E02 or Episode 2 within the season its folder tells
// (Season 01, Specials, or a season's pack, Show.S01); false when it
// tells none.
func Of(rel string) (Episodes, bool) {
	rel = strings.ReplaceAll(rel, `\`, "/")
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if loc := Tag.FindStringSubmatchIndex(stem); loc != nil {
		var e Episodes
		e.Season, _ = strconv.Atoi(stem[loc[2]:loc[3]])
		e.First, _ = strconv.Atoi(stem[loc[4]:loc[5]])
		e.Last = Last(stem, e.First, loc[6], loc[7])
		return e, true
	}
	if m := Cross.FindStringSubmatch(stem); m != nil {
		var e Episodes
		e.Season, _ = strconv.Atoi(m[1])
		e.First, _ = strconv.Atoi(m[2])
		e.Last = e.First
		return e, true
	}
	m := Alone.FindStringSubmatch(stem)
	if m == nil || !strings.Contains(rel, "/") {
		return Episodes{}, false
	}
	season, ok := folderSeason(path.Base(path.Dir(rel)))
	if !ok {
		return Episodes{}, false
	}
	episode, _ := strconv.Atoi(m[1])
	return Episodes{Season: season, First: episode, Last: episode}, true
}

// folderSeason is the season a folder's name tells.
func folderSeason(folder string) (int, bool) {
	if strings.EqualFold(folder, "specials") {
		return 0, true
	}
	m := SeasonFolder.FindStringSubmatch(folder)
	if m == nil {
		m = seasonWord.FindStringSubmatch(folder)
	}
	if m == nil {
		return 0, false
	}
	season, _ := strconv.Atoi(m[1])
	return season, true
}

// An absolute episode number is an episode's place among all the regular
// episodes of its series, specials left out, as anime is often numbered:
// "Show - 41.mkv" is its 41st episode, whatever its season. Jellyfin 12.4
// reads such names with three of its episode expressions
// (Emby.Naming/Common/NamingOptions.cs at v12.4): "Foo Bar 889", lines
// 361-370, "[bar] Foo - 1 [baz]", lines 377-382, and "Name - 101",
// "Name - 101 [720p]" or "Name - 101 (2020)", lines 384-390. Absolute
// reads them so, but:
//   - a number after a dash is read first, so that "Show 2 - 05" is
//     episode 5, where Jellyfin's "Foo Bar 889" reads 2;
//   - "Foo Bar 889" takes the last number of the name, so that a number
//     in the show's title is not the episode's ("Station 100 05" is
//     episode 5), and the words after it may hold an "x", but for "1x02"
//     or "1920x1080";
//   - a year, 1880 to 2100 written with four digits, and a resolution, a
//     height followed by "p" or "i" or, in "Foo Bar 889", 2160, 1440,
//     1080, 720, 576, 480 or 360 alone, are never episodes: Jellyfin
//     reads "Show 2019" as episode 2019 and "Show 1080p" as 1080;
//   - a number told as an episode's, a season's or a part's ("Show
//     Episode 5", "Show S2 - 05", "Show Part 2") is no absolute number,
//     nor is a range ("Show 41-42"): such a file holds one episode;
//   - a name holding a season and an episode, S01E02 or 1x02, has no
//     absolute number, as Jellyfin's "Foo Bar 889" guards (line 364).
var (
	// absoluteDash is a dash and the number after it, "v2" for a
	// release's second version; the dash is between spaces or
	// underscores, as Jellyfin's "Name - 101" reads it, but in a name
	// with a tag between brackets, as its "[bar] Foo - 1 [baz]".
	absoluteDash = regexp.MustCompile(`([ _]*)-([ _]*)(\d{1,4})(?:v\d{1,2})?`)
	// absoluteWord is the number of "Foo Bar 889", a word of its own;
	// titleWord a word of its title, letters and digits as Jellyfin's
	// \w (with the underscore, read as a space here).
	absoluteWord = regexp.MustCompile(`^(\d{1,4})(?:v\d{1,2})?$`)
	titleWord    = regexp.MustCompile(`^[\p{L}\p{M}\p{N}]+$`)
	// numberedWord makes the number after it another than an absolute
	// episode's.
	numberedWord = regexp.MustCompile(`(?i)^(?:e|ep|episode|s|season|series|saison|staffel|temporada|part|pt|vol|volume|disc|disk|cd)$`)
	// seasonEnd is a season ending a title, as in "Show S2 - 05".
	seasonEnd = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:s|season[ ._-]?)\d{1,3}$`)
	// otherCross is "1920x1080" or "1x02" among words.
	otherCross = regexp.MustCompile(`\d[xX]\d`)
	// bracketedTag is a tag between brackets or braces: a release
	// group, a resolution, a checksum.
	bracketedTag = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	// resolutionHeights are the heights a number alone may be.
	resolutionHeights = map[int]bool{2160: true, 1440: true, 1080: true, 720: true, 576: true, 480: true, 360: true}
)

// Absolute reads the absolute episode number a video's name tells, stem
// its name without its extension (see the rules above), with the title
// before it: "Show - 41", "Show 41" and "[Group] Show - 041 [1080p]" are
// the 41st episode of "Show". It is false for a name telling none, or a
// season and an episode.
func Absolute(stem string) (number int, title string, ok bool) {
	if Tag.MatchString(stem) || Cross.MatchString(stem) {
		return 0, "", false
	}
	text := bracketedTag.ReplaceAllString(stem, " ")
	tagged := text != stem
	text = strings.TrimSpace(text)
	for _, m := range absoluteDash.FindAllStringSubmatchIndex(text, -1) {
		title := strings.Trim(strings.ReplaceAll(text[:m[0]], "_", " "), " -")
		spaced := m[3] > m[2] && m[5] > m[4]
		if title == "" || !spaced && (!tagged || strings.ContainsRune("0123456789", rune(text[m[0]-1]))) {
			continue
		}
		// The number ends the name, or a separator follows it: "Show -
		// 1080p", "Show - 41.5" and "Show - 41-42" tell none.
		if end := m[1]; end < len(text) {
			next := text[end]
			digit := end+1 < len(text) && text[end+1] >= '0' && text[end+1] <= '9'
			if !strings.ContainsRune(" _-.", rune(next)) || (next == '.' || next == '-') && digit {
				continue
			}
		}
		digits := text[m[6]:m[7]]
		n, _ := strconv.Atoi(digits)
		if year(digits) || n == 0 {
			continue
		}
		words := strings.Fields(title)
		if seasonEnd.MatchString(title) || numberedWord.MatchString(words[len(words)-1]) {
			return 0, "", false
		}
		return n, title, true
	}
	words := strings.Fields(strings.ReplaceAll(text, "_", " "))
	for i := len(words) - 1; i > 0; i-- {
		m := absoluteWord.FindStringSubmatch(words[i])
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if year(m[1]) || resolutionHeights[n] || n == 0 {
			continue
		}
		if numberedWord.MatchString(words[i-1]) || otherCross.MatchString(strings.Join(words[i+1:], " ")) {
			return 0, "", false
		}
		for _, word := range words[:i] {
			if !titleWord.MatchString(word) {
				return 0, "", false
			}
		}
		return n, strings.Join(words[:i], " "), true
	}
	return 0, "", false
}

// year reports whether digits, an episode's number as written, are a
// year's: four of them, from 1880 to 2100.
func year(digits string) bool {
	n, _ := strconv.Atoi(digits)
	return len(digits) == 4 && n >= 1880 && n <= 2100
}
