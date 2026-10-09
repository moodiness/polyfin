package localfiles

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/iptv"
)

// videoExtensions are the files a scan lists, by their extension.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".ts": true, ".m2ts": true, ".mts": true,
	".webm": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".flv": true, ".ogv": true,
}

// videoFile reports whether name is a video a scan lists: not a sample.
func videoFile(name string) bool {
	extension := strings.ToLower(path.Ext(name))
	if !videoExtensions[extension] {
		return false
	}
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	return stem != "sample" && !strings.HasSuffix(stem, "-sample") && !strings.HasSuffix(stem, ".sample")
}

// skippedFolder reports whether a scan leaves a folder out: hidden ones,
// those of system files, and the extras of a title, which are not one of
// its versions.
func skippedFolder(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#") {
		return true
	}
	switch strings.ToLower(name) {
	case "extras", "featurettes", "behind the scenes", "deleted scenes", "interviews", "scenes", "shorts", "trailers", "samples",
		"sample", "other", "lost+found":
		return true
	}
	return false
}

// name is what a file's or folder's name tells: the title and year it
// names, the IMDb ("tt…") and TMDB identifiers written in it, the season
// and episodes of an episode (lastEpisode, the last of a file holding
// several, is episode for one holding one), and its video's height.
type name struct {
	title                        string
	year                         int
	imdb, tmdb                   string
	season, episode, lastEpisode int
	episodic                     bool
	height                       int
}

var (
	// imdbTag and tmdbTag are the identifiers written in names, as
	// {imdb-tt0063350}, [imdbid-tt0063350], {tmdb-10331} or [tmdbid-10331].
	imdbTag = regexp.MustCompile(`(?i)[\[{]imdb(?:id)?[-=: ](tt\d{5,12})[\]}]`)
	tmdbTag = regexp.MustCompile(`(?i)[\[{]tmdb(?:id)?[-=: ](\d{1,10})[\]}]`)
	// bracketed are the other tags between brackets or braces, such as a
	// release group or an edition: they are not the title.
	bracketed = regexp.MustCompile(`\[[^\]]*\]|\{[^}]*\}`)
	// episodeTag matches S01E02, s01e02, S01E01-E02, S01E01E02 and
	// S01E01-02; crossTag 1x02; episodeOnly E02 or Episode 2, in a season
	// folder.
	episodeTag  = regexp.MustCompile(`(?i)\bs(\d{1,3})[ ._-]?e(\d{1,4})((?:(?:-e?|[ ._]?-?[ ._]?e)\d{1,4})*)`)
	extraEpisod = regexp.MustCompile(`(?i)(?:-e?|[ ._]?-?[ ._]?e)(\d{1,4})`)
	crossTag    = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	episodeOnly = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:e|ep|episode)[ ._]?(\d{1,4})\b`)
	// seasonFolder names a season's folder: Season 01, Saison 1, S01,
	// Specials (season 0).
	seasonFolder = regexp.MustCompile(`(?i)^(?:season|series|saison|staffel|temporada|s)[ ._-]*(\d{1,3})$`)
	// yearInParentheses is the year of "Title (1968)"; yearToken one among
	// the words of "Title.1968.1080p".
	yearInParentheses = regexp.MustCompile(`\((\d{4})\)`)
	yearToken         = regexp.MustCompile(`^(?:19|20)\d{2}$`)
	heightTag         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(2160|1440|1080|720|576|480|360)[pi](?:$|[^a-z0-9])`)
	uhdTag            = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:4k|uhd)(?:$|[^a-z0-9])`)
)

// releaseWords end the title of a name that has no year: what follows
// describes the release.
var releaseWords = map[string]bool{
	"2160p": true, "1440p": true, "1080p": true, "1080i": true, "720p": true, "576p": true, "480p": true, "360p": true, "4k": true,
	"uhd": true, "bluray": true, "blu-ray": true, "bdrip": true, "brrip": true, "web": true, "web-dl": true, "webdl": true, "webrip": true,
	"hdtv": true, "dvdrip": true, "dvd": true, "remux": true, "x264": true, "x265": true, "h264": true, "h265": true, "hevc": true,
	"avc": true, "xvid": true, "divx": true, "10bit": true, "hdr": true, "hdr10": true, "dv": true, "proper": true, "repack": true,
	"extended": true, "unrated": true, "remastered": true, "multi": true, "dts": true, "ac3": true, "aac": true,
}

// parseName reads a file's name without its extension, or a folder's.
func parseName(text string) name {
	var n name
	if m := imdbTag.FindStringSubmatch(text); m != nil {
		n.imdb = strings.ToLower(m[1])
	}
	if m := tmdbTag.FindStringSubmatch(text); m != nil {
		n.tmdb = strings.TrimLeft(m[1], "0")
	}
	n.height = height(text)
	text = bracketed.ReplaceAllString(text, " ")
	if loc := episodeTag.FindStringSubmatchIndex(text); loc != nil {
		n.episodic = true
		n.season, _ = strconv.Atoi(text[loc[2]:loc[3]])
		n.episode, _ = strconv.Atoi(text[loc[4]:loc[5]])
		n.lastEpisode = n.episode
		for _, m := range extraEpisod.FindAllStringSubmatch(text[loc[6]:loc[7]], -1) {
			if last, _ := strconv.Atoi(m[1]); last > n.lastEpisode {
				n.lastEpisode = last
			}
		}
		text = text[:loc[0]]
	} else if loc := crossTag.FindStringSubmatchIndex(text); loc != nil {
		n.episodic = true
		n.season, _ = strconv.Atoi(text[loc[2]:loc[3]])
		n.episode, _ = strconv.Atoi(text[loc[4]:loc[5]])
		n.lastEpisode = n.episode
		text = text[:loc[0]]
	}
	n.title, n.year = titleAndYear(text)
	return n
}

// height is the video height a name tells, 0 when it tells none.
func height(text string) int {
	if m := heightTag.FindStringSubmatch(text); m != nil {
		h, _ := strconv.Atoi(m[1])
		return h
	}
	if uhdTag.MatchString(text) {
		return 2160
	}
	return 0
}

// titleAndYear reads "Title (1968)", "Title.1968.1080p.BluRay" or "Title"
// alone.
func titleAndYear(text string) (string, int) {
	if loc := yearInParentheses.FindStringSubmatchIndex(text); loc != nil {
		if year, _ := strconv.Atoi(text[loc[2]:loc[3]]); year >= 1880 && year <= 2100 {
			if title := cleanTitle(text[:loc[0]]); title != "" {
				return title, year
			}
		}
	}
	words := strings.Fields(separate(text))
	// The last year after the first word is the title's: "1917 2019" is
	// 1917 of 2019, "2001 A Space Odyssey 1968" is of 1968.
	for i := len(words) - 1; i > 0; i-- {
		if yearToken.MatchString(strings.Trim(words[i], "()")) {
			year, _ := strconv.Atoi(strings.Trim(words[i], "()"))
			return cleanTitle(strings.Join(words[:i], " ")), year
		}
	}
	for i, word := range words {
		if i > 0 && releaseWords[strings.ToLower(word)] {
			words = words[:i]
			break
		}
	}
	return cleanTitle(strings.Join(words, " ")), 0
}

// separate turns the dots and underscores separating the words of a name
// into spaces: all of them in a name without spaces, as in
// "Night.of.the.Living.Dead", those between words otherwise, so that
// "Mr. Smith" keeps its dot.
func separate(text string) string {
	text = strings.ReplaceAll(text, "_", " ")
	if !strings.Contains(strings.TrimSpace(text), " ") {
		return strings.ReplaceAll(text, ".", " ")
	}
	return text
}

// cleanTitle trims what surrounds a title in a name.
func cleanTitle(text string) string {
	text = strings.Join(strings.Fields(separate(text)), " ")
	return strings.Trim(text, " -–_,([")
}

// fold is a title as matching compares it: lower case, without accents
// nor punctuation, "&" read as "and".
func fold(title string) string {
	folded := iptv.Fold(strings.ReplaceAll(title, "&", " and "))
	var b strings.Builder
	space := false
	for _, r := range folded {
		switch {
		case r == '\'' || r == '’':
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127:
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

// movieName is what a movie file's path, in its folder, tells: its own
// name, completed by its folder's (a folder per movie), whose identifiers
// and year count when the file's name has none.
func movieName(rel string) name {
	file := parseName(stem(rel))
	dir := path.Dir(rel)
	if dir == "." {
		return file
	}
	folder := parseName(path.Base(dir))
	if file.imdb == "" {
		file.imdb = folder.imdb
	}
	if file.tmdb == "" {
		file.tmdb = folder.tmdb
	}
	if file.title == "" || file.year == 0 && folder.year != 0 && folder.title != "" {
		file.title, file.year = folder.title, folder.year
	}
	if file.height == 0 {
		file.height = folder.height
	}
	file.episodic = false
	return file
}

// episodeName is what an episode file's path, in a shows folder, tells:
// the show (its folder, the first of the path, else what the file's name
// says before the episode), the unit matched to a title (the show's
// folder, or the file at the folder's top), and the episode's season and
// numbers, from its name, its season folder giving the season of an
// episode numbered alone.
func episodeName(rel string) (show name, episode name, unit string) {
	episode = parseName(stem(rel))
	parts := strings.Split(rel, "/")
	season := -1
	if len(parts) >= 2 {
		if m := seasonFolder.FindStringSubmatch(parts[len(parts)-2]); m != nil {
			season, _ = strconv.Atoi(m[1])
		} else if strings.EqualFold(parts[len(parts)-2], "specials") {
			season = 0
		}
	}
	if !episode.episodic && season >= 0 {
		if m := episodeOnly.FindStringSubmatch(stem(rel)); m != nil {
			episode.episodic = true
			episode.season = season
			episode.episode, _ = strconv.Atoi(m[1])
			episode.lastEpisode = episode.episode
		}
	}
	top := parts[0]
	if len(parts) >= 2 && !seasonFolder.MatchString(top) && !strings.EqualFold(top, "specials") {
		show = parseName(top)
		show.episodic = false
		return show, episode, top
	}
	show = name{title: episode.title, year: episode.year, imdb: episode.imdb, tmdb: episode.tmdb}
	return show, episode, rel
}

// stem is a path's last element without its extension.
func stem(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}
