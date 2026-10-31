package jellyfin

import (
	"bytes"
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// A movie's or an episode's credits may hold a scene, a stinger: during
// them, between two parts of the credits, or after them. Credits that run
// over it would have Skip jump past it. Once a version is analyzed, its
// credits are split around the stingers its chapters or its dialogue show
// (see splitAtStingers), so that skipping them lands on each stinger, and
// jellyfin-web offers the credits after it again as playback enters them.
//
// A chapter right after credits chapters needs no rule of its own: the
// version's credits chapters give its credits, in place of the databases',
// and they end where the next chapter starts (see chapterSegments).
const (
	// Dialogue resuming in the credits after dialogueGap without any is a
	// stinger, of minStingerCues lines at least, lasting maxDialogueStinger
	// at most: longer is the title itself, under credits timed too early.
	// It starts dialogueLead before its first line, as a scene starts
	// before its first word.
	dialogueGap        = 20 * time.Second
	minStingerCues     = 3
	maxDialogueStinger = 10 * time.Minute
	dialogueLead       = 5 * time.Second
	// minDialogueCues is how many lines an addon subtitle needs to give
	// the dialogue, as a track inside the file (see
	// playback.Service.KeptDialogue).
	minDialogueCues = 20
)

// stingerNames are the chapter names that mark a scene during or after
// the credits, as whole words in lower case and without accents, with
// their French: "Post-Credits Scene", "Mid-credits", "Scène post-générique".
var stingerNames = []string{
	"post credits", "post credit", "postcredits", "postcredit",
	"mid credits", "mid credit", "midcredits", "midcredit",
	"after credits", "after credit", "after the credits", "stinger",
	"post generique", "postgenerique", "mi generique", "apres le generique", "apres generique",
}

// stingerText reports whether the words of a chapter's name (see
// chapterWords) name a scene during or after the credits.
func stingerText(text string) bool {
	return slices.ContainsFunc(stingerNames, func(n string) bool { return strings.Contains(text, " "+n+" ") })
}

// stinger is a scene within credits, not to be skipped.
type stinger struct {
	Start, End time.Duration
}

// chapterStingers are the stingers a version's chapters name (see
// stingerNames), each to the next chapter's start, the last to the
// version's end.
func chapterStingers(analysis media.Analysis) []stinger {
	var found []stinger
	for i, chapter := range analysis.Chapters {
		if _, text := chapterWords(chapter.Title); stingerText(text) {
			if end := chapterEnd(analysis, i); end > chapter.Start {
				found = append(found, stinger{chapter.Start, end})
			}
		}
	}
	return found
}

// dialogueStingers are the stingers within credits that dialogue, cues
// sorted by start, shows: lines resuming in the credits after dialogueGap
// without any since the credits started or the lines before, at least
// minStingerCues of them with no such gap between them, maybe running past
// the credits' end, and lasting at most maxDialogueStinger. Cues that are
// not dialogue, as lyrics, are left out (see dialogueCue): credits over a
// song are not a stinger.
func dialogueStingers(cues []subtitles.Cue, credits mediasegments.Segment) []stinger {
	var found []stinger
	last := credits.Start
	var from time.Duration
	var cluster []subtitles.Cue
	flush := func() {
		if len(cluster) >= minStingerCues && cluster[0].Start-from >= dialogueGap && last-cluster[0].Start <= maxDialogueStinger {
			found = append(found, stinger{max(cluster[0].Start-dialogueLead, from), last})
		}
		cluster = nil
	}
	for _, cue := range cues {
		if cue.End <= credits.Start || cue.End <= cue.Start || !dialogueCue(cue) {
			continue
		}
		if cue.Start < credits.Start {
			// Dialogue running into the credits.
			last = max(last, cue.End)
			continue
		}
		if len(cluster) == 0 || cue.Start-last >= dialogueGap {
			if cue.Start >= credits.End {
				break
			}
			flush()
			from = last
		}
		cluster = append(cluster, cue)
		last = max(last, cue.End)
	}
	flush()
	return found
}

// musicMarks mark lyrics in subtitles.
const musicMarks = "♪♫♬♩"

// dialogueCue reports whether a cue is dialogue: not lyrics, marked with a
// note or with "#" as subtitles mark them, and with words left once its
// tags and its descriptions in brackets, such as "[music]" or "(singing)",
// are taken out.
func dialogueCue(cue subtitles.Cue) bool {
	text := strings.TrimSpace(strings.Join(cue.Lines, "\n"))
	if strings.ContainsAny(text, musicMarks) || strings.HasPrefix(text, "#") || strings.HasSuffix(text, "#") {
		return false
	}
	depth := 0
	for _, r := range text {
		switch {
		case r == '<' || r == '[' || r == '(' || r == '{':
			depth++
		case r == '>' || r == ']' || r == ')' || r == '}':
			depth = max(depth-1, 0)
		case depth == 0 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			return true
		}
	}
	return false
}

// splitCredits takes stingers out of the credits among segments: credits
// running over a stinger end where it starts, and those it ends within
// start again where it ends. Parts shorter than minSkippable, which
// jellyfin-web ignores, are left out.
func splitCredits(segments []mediasegments.Segment, stingers []stinger) []mediasegments.Segment {
	split := make([]mediasegments.Segment, 0, len(segments)+len(stingers))
	for _, segment := range segments {
		if segment.Type != mediasegments.Outro {
			split = append(split, segment)
			continue
		}
		parts := []mediasegments.Segment{segment}
		for _, s := range stingers {
			var next []mediasegments.Segment
			for _, part := range parts {
				if s.End <= part.Start || s.Start >= part.End {
					next = append(next, part)
					continue
				}
				if s.Start > part.Start {
					next = append(next, mediasegments.Segment{Type: part.Type, Start: part.Start, End: s.Start})
				}
				if s.End < part.End {
					next = append(next, mediasegments.Segment{Type: part.Type, Start: s.End, End: part.End})
				}
			}
			parts = next
		}
		for _, part := range parts {
			if part.End-part.Start >= minSkippable {
				split = append(split, part)
			}
		}
	}
	return split
}

// splitAtStingers splits the credits among the segments of a version,
// shaped for it (see versionSegments), around the stingers its chapters
// show (see chapterStingers) and those its dialogue shows (see
// dialogueStingers), then fits them to the version again (see
// fitSegments), so that the last credits still end creditsLead before its
// end. listed is the runtime the title is listed with.
func (h *Handler) splitAtStingers(ctx context.Context, segments []mediasegments.Segment, version accounts.ID, analysis media.Analysis, listed time.Duration) []mediasegments.Segment {
	if !hasSegment(segments, mediasegments.Outro) {
		return segments
	}
	found := chapterStingers(analysis)
	if cues := h.dialogue(ctx, version, analysis); len(cues) > 0 {
		for _, segment := range segments {
			if segment.Type == mediasegments.Outro {
				found = append(found, dialogueStingers(cues, segment)...)
			}
		}
	}
	if len(found) == 0 {
		return segments
	}
	return fitSegments(splitCredits(segments, found), analysis.Duration, listed)
}

// dialogue is a version's dialogue already at hand, by start, nothing
// being read or synced for it: a text track inside its file (see
// playback.Service.KeptDialogue), else the addon subtitle synced to it
// (see playback.Service.SyncSubtitle) read lately with the most lines,
// timed as synced. None when neither is at hand.
func (h *Handler) dialogue(ctx context.Context, version accounts.ID, analysis media.Analysis) []subtitles.Cue {
	cues, ok := h.Playback.KeptDialogue(ctx, version, analysis)
	if !ok {
		timings := h.Playback.SubtitleTimings(ctx, version)
		ids := slices.SortedFunc(maps.Keys(timings), func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })
		var most []subtitles.Cue
		for _, id := range ids {
			if text, read := h.subtitleCache.Get(id); read && len(text.cues) >= minDialogueCues && len(text.cues) > len(most) {
				most, cues = text.cues, timings[id].Apply(text.cues)
			}
		}
	}
	return slices.SortedStableFunc(slices.Values(cues), func(a, b subtitles.Cue) int { return cmp.Compare(a.Start, b.Start) })
}
