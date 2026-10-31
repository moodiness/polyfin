package library

import (
	"strconv"
	"strings"
)

// Tiers are the source tiers labels tell, of Labels.Tier, best first: a
// disc's full copy ("disc"), an encode of a Blu-ray disc ("bluray"), a
// copy of a web stream ("web"), an encode of a DVD ("dvd") and a capture
// of a TV broadcast ("tv").
var Tiers = []string{"disc", "bluray", "web", "dvd", "tv"}

// AudioFormat is what labels or an analysis tell of a version's best audio
// track: lossless (Dolby TrueHD, DTS-HD Master Audio, FLAC, PCM), object
// based (Dolby Atmos, DTS:X), and its channels, 0 when unknown. The zero
// value tells nothing.
type AudioFormat struct {
	Lossless bool
	Object   bool
	Channels int
}

// Rank is the format's class for the audio criterion: 2 lossless and
// object based, 1 one of them, 0 neither.
func (a AudioFormat) Rank() int {
	rank := 0
	if a.Lossless {
		rank++
	}
	if a.Object {
		rank++
	}
	return rank
}

// Better reports whether a is a better format than b: of a higher class
// (see Rank), else with more channels.
func (a AudioFormat) Better(b AudioFormat) bool {
	if a.Rank() != b.Rank() {
		return a.Rank() > b.Rank()
	}
	return a.Channels > b.Channels
}

// tierWords are the words naming a source tier, in lower case, by their
// tier of Tiers; tierPairs, by two words in a row ("WEB-DL", "Blu-ray",
// "COMPLETE BLURAY"). A full disc copy's own word is its release's.
var (
	tierWords = map[string]string{
		"remux": "disc", "bdremux": "disc", "bdmv": "disc", "bd25": "disc", "bd50": "disc", "bd66": "disc", "bd100": "disc",
		"dvd5": "disc", "dvd9": "disc", "dvdr": "disc",
		"bluray": "bluray", "bdrip": "bluray", "brrip": "bluray",
		"web": "web", "webdl": "web", "webrip": "web",
		"dvd": "dvd", "dvdrip": "dvd",
		"hdtv": "tv", "pdtv": "tv", "sdtv": "tv", "tvrip": "tv", "satrip": "tv", "dsr": "tv", "dsrip": "tv",
	}
	tierPairs = map[[2]string]string{
		{"complete", "bluray"}: "disc", {"complete", "uhd"}: "disc", {"blu", "ray"}: "bluray", {"web", "dl"}: "web", {"web", "rip"}: "web",
	}
)

// losslessWords and objectWords name lossless and object-based audio, in
// lower case; losslessRuns and objectRuns, by words in a row ("DTS-HD
// MA", "True HD", "DTS:X"). DTS-HD without MA is lossy.
var (
	losslessWords = map[string]bool{"truehd": true, "flac": true, "lpcm": true, "pcm": true, "alac": true, "dtsma": true}
	losslessRuns  = [][]string{{"dts", "hd", "ma"}, {"dtshd", "ma"}, {"true", "hd"}}
	objectWords   = map[string]bool{"atmos": true, "dtsx": true}
	objectRuns    = [][]string{{"dts", "x"}}
)

// editionWords are the words naming an edition of accounts.VersionEditions,
// in lower case; a director's cut is named by two or three words
// ("Director's Cut", "Directors.Cut").
var editionWords = map[string]string{"theatrical": "theatrical", "extended": "extended", "imax": "imax"}

// ReadRelease reads what the labels of a stream tell of its release: its
// source tier, of Tiers, the best named, empty when they name none; its
// audio format, the best named; and its edition, of
// accounts.VersionEditions, the first named, empty when they name none.
// titles are as ReleaseKinds reads them: the words of a line naming the
// title are the title's, so that a movie named "Extended" is no extended
// cut.
func ReadRelease(titles []string, labels ...string) (tier string, audio AudioFormat, edition string) {
	var named [][]string
	for _, title := range titles {
		if words := lowerWords(title); len(words) > 0 {
			named = append(named, words)
		}
	}
	best := len(Tiers)
	for _, label := range labels {
		for line := range strings.Lines(label) {
			words := lowerWords(line)
			words = words[titleEnd(words, named):]
			for i, word := range words {
				next := ""
				if i+1 < len(words) {
					next = words[i+1]
				}
				found, ok := tierWords[word]
				if pair, paired := tierPairs[[2]string{word, next}]; paired {
					found, ok = pair, true
				}
				if ok {
					if rank := tierRank(found); rank < best {
						best = rank
					}
				}
				if losslessWords[word] || runAt(words, i, losslessRuns) {
					audio.Lossless = true
				}
				if objectWords[word] || runAt(words, i, objectRuns) {
					audio.Object = true
				}
				if channels := labelChannels(word, next); channels > audio.Channels {
					audio.Channels = channels
				}
				if edition == "" {
					edition = editionAt(words, i)
				}
			}
		}
	}
	if best < len(Tiers) {
		tier = Tiers[best]
	}
	return tier, audio, edition
}

// tierRank is a tier's place in Tiers, len(Tiers) for none.
func tierRank(tier string) int {
	for i, known := range Tiers {
		if known == tier {
			return i
		}
	}
	return len(Tiers)
}

// runAt reports whether one of runs starts at words[i].
func runAt(words []string, i int, runs [][]string) bool {
	for _, run := range runs {
		if i+len(run) > len(words) {
			continue
		}
		matched := true
		for j, word := range run {
			if words[i+j] != word {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// labelChannels reads a channel layout from two words in a row: "5" and
// "1" from "5.1", "DDP5" and "1" from "DDP5.1", "AAC2" and "0" from
// "AAC2.0"; 6, 6 and 2 channels, 0 for none. The first word ends with one
// digit, after letters or none, and the second is 0 or 1, the
// low-frequency channel: a year or "1080" is none.
func labelChannels(word, next string) int {
	if next != "0" && next != "1" || word == "" {
		return 0
	}
	last := word[len(word)-1]
	if last < '1' || last > '9' {
		return 0
	}
	for _, r := range word[:len(word)-1] {
		if r < 'a' || r > 'z' {
			return 0
		}
	}
	main, _ := strconv.Atoi(string(last))
	if next == "1" {
		return main + 1
	}
	return main
}

// editionAt is the edition the words starting at words[i] name, empty for
// none.
func editionAt(words []string, i int) string {
	if edition, ok := editionWords[words[i]]; ok {
		return edition
	}
	if runAt(words, i, [][]string{{"directors", "cut"}, {"director", "s", "cut"}, {"director", "cut"}}) {
		return "directors"
	}
	return ""
}
