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
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".ts": true, ".m2ts": true, ".mts": true,
	".webm": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".flv": true, ".ogv": true,
}

// Video reports whether name is a video's, by its extension.
func Video(name string) bool {
	return videoExtensions[strings.ToLower(path.Ext(name))]
}

var (
	// Tag matches S01E02, s01e02, S01E01-E02, S01E01E02 and S01E01-02,
	// Extra each episode past the first in its third group; Cross matches
	// 1x02; Alone E02 or Episode 2, in a season folder.
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
		e.Last = e.First
		for _, m := range Extra.FindAllStringSubmatch(stem[loc[6]:loc[7]], -1) {
			if last, _ := strconv.Atoi(m[1]); last > e.Last {
				e.Last = last
			}
		}
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
