package jellyfin

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
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
// its segments out.
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
	if h.Segments == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	title, err := h.segmentTitle(r.Context(), user, item)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	wanted := map[string]bool{}
	included := listQuery(r, "includeSegmentTypes")
	for _, name := range included {
		if kind, ok := segmentTypes[strings.ToLower(name)]; ok {
			wanted[kind] = true
		}
	}
	for _, segment := range h.Segments.Segments(r.Context(), title) {
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
