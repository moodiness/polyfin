package localfiles

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/episodename"
	"github.com/moodiness/polyfin/internal/iptv"
)

// videoFile reports whether name is a video a scan lists, a sample's
// included: a scan reading extras reads it as one (see splitExtras).
func videoFile(name string) bool {
	return episodename.Video(name)
}

// sideExtensions are the files a scan lists beside the videos, by their
// extension: NFO files, which may tell a title's identifiers, and subtitle
// files (see subtitleFormats), with the .sub files of VobSub pairs.
var sideExtensions = map[string]bool{".nfo": true, ".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sup": true, ".idx": true,
	".sub": true}

// sideFile reports whether name is a side file a scan lists.
func sideFile(name string) bool {
	return sideExtensions[strings.ToLower(path.Ext(name))]
}

// audioContainers are the audio files a music folder's scan lists, by
// their extension, with the container apps know them as.
var audioContainers = map[string]string{
	".mp3": "mp3", ".flac": "flac", ".m4a": "m4a", ".m4b": "m4a", ".alac": "m4a", ".aac": "aac", ".ogg": "ogg", ".oga": "ogg",
	".opus": "ogg", ".wav": "wav", ".aif": "aiff", ".aiff": "aiff", ".wma": "asf", ".ape": "ape", ".wv": "wv", ".dsf": "dsf",
}

// audioFile reports whether name is an audio file a music folder's scan
// lists.
func audioFile(name string) bool {
	return audioContainers[strings.ToLower(path.Ext(name))] != ""
}

// folderImageNames are the images beside an album's tracks that may be its
// artwork, and artistImageNames those in an artist's folder that may be
// their picture (see artistFolders), in the order they are preferred.
var (
	folderImageNames = []string{"cover", "folder", "front"}
	artistImageNames = []string{"artist", "folder", "photo"}
)

// imageExtensions are those of the images a scan reads, in the order they
// are preferred for a same name: JPEG, PNG and WebP.
var imageExtensions = []string{".jpg", ".jpeg", ".png", ".webp"}

// imageSide reports whether name is an image, by its extension (see
// imageExtensions).
func imageSide(name string) bool {
	return slices.Contains(imageExtensions, strings.ToLower(path.Ext(name)))
}

// musicImage reports whether name is an image a music folder's scan lists
// beside the tracks: an album's artwork or an artist's picture, by one of
// folderImageNames or artistImageNames.
func musicImage(name string) bool {
	if !imageSide(name) {
		return false
	}
	s := strings.ToLower(stem(name))
	return slices.Contains(folderImageNames, s) || slices.Contains(artistImageNames, s)
}

// lyricsFile reports whether name is a lyrics file, LRC, which gives the
// lyrics of the tracks of the same name beside it.
func lyricsFile(name string) bool { return strings.EqualFold(path.Ext(name), ".lrc") }

// m3uFile reports whether name is a playlist a music folder's scan reads:
// an M3U file, .m3u or .m3u8.
func m3uFile(name string) bool {
	extension := strings.ToLower(path.Ext(name))
	return extension == ".m3u" || extension == ".m3u8"
}

// listed tells which files a scan of a folder of kind lists, by name: the
// files it reads (videos, or a music folder's audio files), and the side
// files beside them (NFO and subtitle files and images, which may be a
// title's artwork whatever their name, as one named after its video; or a
// music folder's images, lyrics files and playlists).
func listed(kind, name string) (file, side bool) {
	if kind == KindMusic {
		return audioFile(name), musicImage(name) || lyricsFile(name) || m3uFile(name)
	}
	return videoFile(name), sideFile(name) || imageSide(name)
}

// skippedFolder reports whether a scan leaves a folder out: hidden ones,
// those of system files, and a "sample" folder. The folders of extras
// are walked and watched, the scan taking their files apart (see
// splitExtras).
func skippedFolder(name string) bool {
	return systemFolder(name) || strings.EqualFold(name, "sample")
}

// systemFolder reports whether a folder is hidden, or one of system files:
// a NAS's (@eaDir, #recycle), Linux's lost+found, and those Windows makes
// at a drive's root ($RECYCLE.BIN, System Volume Information).
func systemFolder(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#") ||
		strings.EqualFold(name, "lost+found") || strings.EqualFold(name, "$RECYCLE.BIN") || strings.EqualFold(name, "System Volume Information")
}

// skippedIn reports whether a scan of a folder of kind leaves a folder
// out: in a music folder, hidden folders and those of system files only,
// as an album may be named "Other" or "Interviews"; in a home videos
// folder, the folders of extras too, which it does not read.
func skippedIn(kind, name string) bool {
	switch kind {
	case KindMusic:
		return systemFolder(name)
	case KindHomeVideos:
		return skippedFolder(name) || formerlySkipped[strings.ToLower(name)]
	}
	return skippedFolder(name)
}

// name is what a file's or folder's name tells: the title and year it
// names, the IMDb ("tt…") and TMDB identifiers written in it, the season
// and episodes of an episode (lastEpisode, the last of a file holding
// several, is episode for one holding one), the absolute number of an
// episode named by it (see episodeName), and its video's height.
type name struct {
	title                        string
	year                         int
	imdb, tmdb                   string
	season, episode, lastEpisode int
	absolute                     int
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
	// The episodes names tell, as the episodename package reads them.
	episodeTag   = episodename.Tag
	crossTag     = episodename.Cross
	episodeOnly  = episodename.Alone
	seasonFolder = episodename.SeasonFolder
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
		n.lastEpisode = episodename.Last(text, n.episode, loc[6], loc[7])
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
// name, completed by its folder's (see completedName).
func movieName(rel string) name {
	return completedName(parseName(stem(rel)), path.Dir(rel))
}

// completedName completes what a movie's own name tells by its folder's,
// dir (a folder per movie), whose identifiers and year count when the
// movie's name has none; dir is "." for no folder.
func completedName(file name, dir string) name {
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
// episode numbered alone. A name telling an absolute number alone (see
// episodename.Absolute) is numbered so in the specials' folder, as
// Jellyfin reads it; elsewhere it tells its absolute number, the episode
// being listed, until it is matched to the catalog's (see
// absolutePlace), in its season folder's season, else the first, as
// Jellyfin's TMDB provider reads an episode without a season
// (MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbEpisodeProvider.cs, lines
// 99-100 at v12.4).
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
	if !episode.episodic {
		if number, title, ok := episodename.Absolute(stem(rel)); ok {
			episode.episodic = true
			episode.season, episode.episode, episode.lastEpisode = season, number, number
			if season != 0 {
				episode.season, episode.absolute = max(season, 1), number
			}
			episode.title, episode.year = titleAndYear(title)
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

// Why the name of a show's file tells no episode, the reason Unmatched
// gives such a file of a show in place of reasonUnreadableName, with how
// to name it (see episodeReason). A name telling an absolute number alone
// tells its episode (see absolute.go).
const (
	// reasonNoSeason: an episode's number alone, as "E05" or "Episode
	// 5", outside a season's folder.
	reasonNoSeason = "no_season"
	// reasonAirDate: an air date, as "2024-03-15", which local folders
	// do not read.
	reasonAirDate = "air_date"
	// reasonNoEpisodeNumber: no number at all.
	reasonNoEpisodeNumber = "no_episode_number"
)

// airDate is the date of a daily show's episode, "2024-03-15" or
// "15.03.2024", as Jellyfin reads it
// (Emby.Naming/Common/NamingOptions.cs, lines 332-351 at v12.4).
var airDate = regexp.MustCompile(`(?:^|\D)(?:(?:19|20)\d{2}[._ -](?:0[1-9]|1[0-2])[._ -](?:0[1-9]|[12]\d|3[01])|` +
	`(?:0[1-9]|[12]\d|3[01])[._ -](?:0[1-9]|1[0-2])[._ -](?:19|20)\d{2})(?:\D|$)`)

// episodeReason is why the name of the file at rel, of a show, tells no
// episode (see episodeName).
func episodeReason(rel string) string {
	text := stem(rel)
	if episodeOnly.MatchString(text) {
		return reasonNoSeason
	}
	if airDate.MatchString(text) {
		return reasonAirDate
	}
	return reasonNoEpisodeNumber
}
