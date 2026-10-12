package jellyfin

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/mediasegments"
)

// MediaSegmentDto is a part of an item that apps offer to skip.
type MediaSegmentDto struct {
	Id         string
	ItemId     string
	Type       string
	StartTicks int64
	EndTicks   int64
}

// MediaSegmentDtoQueryResult lists an item's segments.
type MediaSegmentDtoQueryResult struct {
	Items            []MediaSegmentDto
	TotalRecordCount int
	StartIndex       int
}

// segmentTypes are the types Jellyfin names, by their lower-case names.
var segmentTypes = map[string]string{
	"unknown": "Unknown", "commercial": "Commercial", "preview": "Preview",
	"recap": "Recap", "outro": "Outro", "intro": "Intro",
}

// mediaSegments answers the segments of a movie or an episode, opened by
// its own identifier or by one of its versions'. Other items have none.
// The segments come from community databases; one that fails only leaves
// its segments out. They are then shaped for the version asked, the first
// for the title's own identifier, once it is analyzed, and that version's
// chapters may give its own (see versionSegments). A version not analyzed
// yet is never analyzed for this. With the skip buttons turned off in the
// settings, no database is asked and every item has none, as on a Jellyfin
// server without a segment provider.
func (h *Handler) mediaSegments(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	opened := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	user := callerFrom(r.Context()).User
	result := MediaSegmentDtoQueryResult{Items: []MediaSegmentDto{}}
	item, err := h.title(r.Context(), user, opened)
	if errors.Is(err, library.ErrNotFound) {
		// Jellyfin answers an empty list for an item without segments,
		// such as a series, and Not Found for an unknown one.
		if _, err := h.Library.Item(r.Context(), user, opened); err != nil {
			h.browseError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	if !h.Accounts.Settings().SkipButtons {
		writeJSON(w, http.StatusOK, result)
		return
	}
	var segments []mediasegments.Segment
	if h.Segments.Asks() {
		title, err := h.segmentTitle(r.Context(), user, item)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		segments = h.Segments.Segments(r.Context(), title)
	}
	if analysis, ok := h.segmentAnalysis(r.Context(), user, item, opened); ok {
		segments = versionSegments(segments, analysis, item.Runtime)
	}
	wanted := map[string]bool{}
	included := listQuery(r, "includeSegmentTypes")
	for _, name := range included {
		if kind, ok := segmentTypes[strings.ToLower(name)]; ok {
			wanted[kind] = true
		}
	}
	for _, segment := range segments {
		if len(included) > 0 && !wanted[string(segment.Type)] {
			continue
		}
		result.Items = append(result.Items, MediaSegmentDto{
			Id:         segmentID(opened, segment).String(),
			ItemId:     opened.String(),
			Type:       string(segment.Type),
			StartTicks: int64(segment.Start / 100),
			EndTicks:   int64(segment.End / 100),
		})
	}
	result.TotalRecordCount = len(result.Items)
	writeJSON(w, http.StatusOK, result)
}

// segmentAnalysis is the analysis of the version a movie's or an episode's
// segments are asked for, if it was analyzed: the version of the identifier
// asked, or, for the title's own identifier, its first version listed, which
// media sources name after the title (see sourceID).
func (h *Handler) segmentAnalysis(ctx context.Context, user accounts.User, item library.Item, opened accounts.ID) (media.Analysis, bool) {
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
		return media.Analysis{}, false
	}
	version := opened
	if opened == item.ID {
		versions := h.cachedPlayable(ctx, user, item).versions
		if len(versions) == 0 {
			return media.Analysis{}, false
		}
		version = versions[0].ID
	}
	return h.Playback.Analyzed(ctx, version)
}

// Credits ending within creditsAtEnd of a version's end run to its end.
// Those jellyfin-web is to offer to skip end creditsLead before it, and
// only when they still last minSkippable, as it ignores shorter segments.
const (
	creditsAtEnd = 2 * time.Second
	creditsLead  = time.Second
	minSkippable = 3 * time.Second
)

// versionSegments shapes a title's segments for a version of it, from the
// version's analysis, as jellyfin-web plays them. listed is the runtime the
// title is listed with, which jellyfin-web queues it with.
//
// The version's own chapters named as an intro, a recap, a preview or
// credits (see chapterSegments) give its segments of those types in place
// of the databases', which may be timed on another cut. No segment passes
// the version's end: one starting at or after it is left out, and the
// others end at the latest there.
//
// jellyfin-web shows no skip button for credits ending at or after the
// listed runtime when a next item is queued: its video page shows its "Up
// Next" card instead, but only for credits ending at or after the version's
// own length. Credits running to the end of a version that the listed
// runtime does not fall short of thus end creditsLead before the end, so
// that jellyfin-web offers to skip them and skipping still ends the
// version; the others end exactly at the end, so that it shows its card.
func versionSegments(segments []mediasegments.Segment, analysis media.Analysis, listed time.Duration) []mediasegments.Segment {
	own := chapterSegments(analysis)
	shaped := make([]mediasegments.Segment, 0, len(segments)+len(own))
	for _, segment := range segments {
		if !slices.ContainsFunc(own, func(o mediasegments.Segment) bool { return o.Type == segment.Type }) {
			shaped = append(shaped, segment)
		}
	}
	shaped = append(shaped, own...)
	if end := analysis.Duration; end > 0 {
		shaped = slices.DeleteFunc(shaped, func(s mediasegments.Segment) bool { return s.Start >= end })
		for i := range shaped {
			segment := &shaped[i]
			segment.End = min(segment.End, end)
			if segment.Type != mediasegments.Outro || end-segment.End > creditsAtEnd {
				continue
			}
			segment.End = end
			// Compared in ticks, as jellyfin-web compares them.
			if listed/100 >= end/100 && end-creditsLead-segment.Start >= minSkippable {
				segment.End = end - creditsLead
			}
		}
	}
	slices.SortStableFunc(shaped, func(a, b mediasegments.Segment) int { return cmp.Compare(a.Start, b.Start) })
	return shaped
}

// chapterSegments are the segments a version's chapters name (see
// chapterKind), as Jellyfin's Chapter Segments Provider makes them: each
// runs from its chapter's start to the next chapter's, the last to the
// version's end. Chapters of the same kind in a row make one segment.
func chapterSegments(analysis media.Analysis) []mediasegments.Segment {
	chapters := analysis.Chapters
	var segments []mediasegments.Segment
	var previous mediasegments.Type
	for i, chapter := range chapters {
		end := chapter.End
		if i+1 < len(chapters) {
			end = chapters[i+1].Start
		} else if analysis.Duration > 0 {
			end = analysis.Duration
		}
		kind := chapterKind(chapter.Title)
		switch {
		case kind == "" || end <= chapter.Start:
			previous = ""
		case kind == previous:
			last := &segments[len(segments)-1]
			last.End = max(last.End, end)
		default:
			segments = append(segments, mediasegments.Segment{Type: kind, Start: chapter.Start, End: end})
			previous = kind
		}
	}
	return segments
}

// chapterNames are the chapter names that mark each type of segment, in
// the order they are tried, as whole words in lower case and without
// accents: the default patterns of Jellyfin's Chapter Segments Provider,
// but its commercial breaks, with their French. "Credits" covers end and
// closing credits, and "crédits".
var chapterNames = []struct {
	kind  mediasegments.Type
	names []string
}{
	{mediasegments.Intro, []string{"intro", "introduction", "opening",
		"generique de debut", "generique d ouverture", "ouverture"}},
	{mediasegments.Preview, []string{"preview", "previews", "next time on", "next on", "sneak peek", "next episode",
		"apercu", "prochainement", "prochain episode", "a suivre"}},
	{mediasegments.Recap, []string{"recap", "previously", "last time on", "last on",
		"recapitulatif", "resume", "precedemment", "episode precedent"}},
	{mediasegments.Outro, []string{"outro", "closing", "credits", "ending",
		"generique de fin", "generique final"}},
}

// chapterKind tells the segment a chapter's name marks: Intro, Preview,
// Recap or Outro, matching its whole words in any letter case, with or
// without accents; none for other names, such as "Chapter 2". The types are
// tried in that order, as Jellyfin's plugin tries them, so that "Opening
// Credits" is an intro. The short forms OP and ED count only as the whole
// name (see shortName), as words such as "Ed" in "Ed Wood" are often
// something else.
func chapterKind(name string) mediasegments.Type {
	words := strings.FieldsFunc(iptv.Fold(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	text := " " + strings.Join(words, " ") + " "
	for _, kind := range chapterNames {
		if slices.ContainsFunc(kind.names, func(n string) bool { return strings.Contains(text, " "+n+" ") }) ||
			kind.kind == mediasegments.Intro && shortName(words, "op") ||
			kind.kind == mediasegments.Outro && shortName(words, "ed") {
			return kind.kind
		}
	}
	return ""
}

// shortName reports whether a name's words are only a short form, maybe
// followed by a number, with or without a space or a sign before it: "OP",
// "op 1", "ED2", "ED - 2".
func shortName(words []string, short string) bool {
	switch len(words) {
	case 1:
		rest, ok := strings.CutPrefix(words[0], short)
		return ok && (rest == "" || decimalDigits(rest))
	case 2:
		return words[0] == short && decimalDigits(words[1])
	}
	return false
}

// segmentTitle identifies a movie or an episode to the segment databases:
// an episode by its series' identifiers and its numbers.
func (h *Handler) segmentTitle(ctx context.Context, user accounts.User, item library.Item) (mediasegments.Title, error) {
	title := mediasegments.Title{Item: item.ID, Runtime: item.Runtime}
	ids := item.ProviderIDs
	if item.Kind == library.KindEpisode {
		series, err := h.Library.Item(ctx, user, item.SeriesID)
		if err != nil {
			return title, err
		}
		ids = series.ProviderIDs
		title.Season, title.Episode = item.ParentIndexNumber, item.IndexNumber
	}
	title.IMDb, title.TMDB = ids["Imdb"], ids["Tmdb"]
	return title, nil
}

// segmentID derives a segment's identifier from the item opened and the
// segment, so that it stays the same from one request to the next.
func segmentID(item accounts.ID, segment mediasegments.Segment) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:segment:" + item.String() + "|" + string(segment.Type) + "|" +
		strconv.FormatInt(int64(segment.Start), 10)))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}
