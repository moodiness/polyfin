package playback

import "strings"

// SubtitleTags is what a subtitle file's name tells past its video's: its
// language as written ("fr", "fre", "French"), its title (the words that
// are neither a language nor a flag), and whether it is forced, for the
// hearing impaired, or the one to show by default. The subtitle files
// beside local files and those inside torrents are read alike.
type SubtitleTags struct {
	Language, Title                  string
	Forced, HearingImpaired, Default bool
}

// ReadSubtitleTags reads the words of a subtitle file's name past its
// video's, separated by dots, in any order and letter case: "forced" or
// "foreign", "sdh", "hi" or "cc" for the hearing impaired, "default", and
// the first word naming a language; the others make its title.
func ReadSubtitleTags(rest string) SubtitleTags {
	var tags SubtitleTags
	var title []string
	for word := range strings.SplitSeq(rest, ".") {
		word = strings.TrimSpace(word)
		switch strings.ToLower(word) {
		case "":
		case "forced", "foreign":
			tags.Forced = true
		case "sdh", "hi", "cc":
			tags.HearingImpaired = true
		case "default":
			tags.Default = true
		default:
			if tags.Language == "" && KnownLanguage(word) {
				tags.Language = word
			} else {
				title = append(title, word)
			}
		}
	}
	tags.Title = strings.Join(title, " ")
	return tags
}

// SubtitleNamed reports whether name, a subtitle file's name without its
// extension, begins with video, a video's name without its extension,
// followed by a dot or nothing, in any letter case, as Jellyfin matches
// the files beside a video; rest is what follows, which ReadSubtitleTags
// reads.
func SubtitleNamed(video, name string) (rest string, ok bool) {
	if len(name) < len(video) || !strings.EqualFold(name[:len(video)], video) {
		return "", false
	}
	rest = name[len(video):]
	if rest != "" && rest[0] != '.' {
		return "", false
	}
	return rest, true
}
