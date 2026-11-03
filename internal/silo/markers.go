package silo

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/moodiness/polyfin/internal/mediasegments"
)

// watchMarker is a span the apps offer to skip, with where it comes from:
// source is the class of source, "online" for a segment database's and
// "scanner" for one Polyfin found itself; provider the database, empty
// for Polyfin's own; algorithm how Polyfin found it (markerAlgorithms);
// confidence the database's own rating, absent when it gives none, as an
// unrated marker must not read as rated 0.
type watchMarker struct {
	StartSeconds float64  `json:"start_seconds"`
	EndSeconds   float64  `json:"end_seconds"`
	Source       string   `json:"source,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Algorithm    string   `json:"algorithm,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`
}

// markerOccurrence is a marker of marker_segments: every span of a version
// the apps offer to skip, in the order they start. The apps take them in
// place of the singular fields, even when there are none.
type markerOccurrence struct {
	Kind         string  `json:"kind"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
}

// firstMarkers are the first skip marker of each kind, as the watch
// detail and its versions give them.
type firstMarkers struct {
	Intro   *watchMarker `json:"intro,omitempty"`
	Credits *watchMarker `json:"credits,omitempty"`
	Recap   *watchMarker `json:"recap,omitempty"`
	Preview *watchMarker `json:"preview,omitempty"`
}

// markers are the skip markers of a version: the first of each kind, and
// every one.
type markers struct {
	firstMarkers
	Segments []markerOccurrence `json:"marker_segments"`
}

// markerKinds are the apps' kinds of Polyfin's segment types. Commercial
// breaks have none: the apps know no such marker.
var markerKinds = map[mediasegments.Type]string{mediasegments.Intro: "intro", mediasegments.Outro: "credits",
	mediasegments.Recap: "recap", mediasegments.Preview: "preview"}

// markerSources are the apps' classes of source of the ways Polyfin finds
// segments: a database is an online provider, chapters and audio are
// Polyfin's own detection.
var markerSources = map[mediasegments.Finder]string{mediasegments.InDatabase: "online", mediasegments.InChapters: "scanner",
	mediasegments.InAudio: "scanner"}

// markerAlgorithms name how Polyfin found a segment, versioned as the
// apps' algorithms are, so that a change in the way is told apart.
var markerAlgorithms = map[mediasegments.Finder]string{mediasegments.InDatabase: "polyfin.database:v1",
	mediasegments.InChapters: "polyfin.chapters:v1", mediasegments.InAudio: "polyfin.audio:v1"}

// slot is the field of a kind.
func (m *firstMarkers) slot(kind string) **watchMarker {
	switch kind {
	case "intro":
		return &m.Intro
	case "credits":
		return &m.Credits
	case "recap":
		return &m.Recap
	default:
		return &m.Preview
	}
}

// ticksSeconds is a time in ticks in seconds.
func ticksSeconds(ticks int64) float64 {
	return float64(ticks) / 1e7
}

// markersOf reads the skip markers of a version, by its media source's
// identifier, from GET /Polyfin/MediaSegments/{itemId}: the segments
// Jellyfin apps get for it, with where each comes from. None while the
// settings turn the skip buttons off.
func (s *Server) markersOf(r *http.Request, v viewer, source string) (markers, error) {
	m := markers{Segments: []markerOccurrence{}}
	var result struct{ Items []segmentOrigin }
	a, err := s.call(r, v, http.MethodGet, "/Polyfin/MediaSegments/"+url.PathEscape(source), nil, &result)
	if err == nil && !a.ok() {
		err = fmt.Errorf("the Jellyfin API answered %d", a.status)
	}
	if err != nil {
		return m, err
	}
	// Sorted by start, as the apps take them.
	slices.SortStableFunc(result.Items, func(a, b segmentOrigin) int { return cmp.Compare(a.StartTicks, b.StartTicks) })
	for _, segment := range result.Items {
		kind, ok := markerKinds[mediasegments.Type(segment.Type)]
		if !ok || segment.EndTicks <= segment.StartTicks || segment.StartTicks < 0 {
			continue
		}
		start, end := ticksSeconds(segment.StartTicks), ticksSeconds(segment.EndTicks)
		m.Segments = append(m.Segments, markerOccurrence{Kind: kind, StartSeconds: start, EndSeconds: end})
		if slot := m.slot(kind); *slot == nil {
			found := mediasegments.Finder(segment.Found)
			*slot = &watchMarker{StartSeconds: start, EndSeconds: end, Source: markerSources[found], Algorithm: markerAlgorithms[found],
				Confidence: segment.Confidence}
			if found == mediasegments.InDatabase {
				(*slot).Provider = segment.Database
			}
		}
	}
	return m, nil
}

// segmentOrigin is a segment as GET /Polyfin/MediaSegments/{itemId}
// describes it.
type segmentOrigin struct {
	Type       string
	StartTicks int64
	EndTicks   int64
	Found      string
	Database   string
	Confidence *float64
}

// detailMarkers are the markers of a title's watch detail: those of the
// version file_id names when it names one, else each kind's first among
// the versions, in their order, as Silo answers them.
func detailMarkers(r *http.Request, versions []watchVersion) firstMarkers {
	if asked := r.URL.Query().Get("file_id"); asked != "" {
		for _, version := range versions {
			if version.FileID == asked {
				return version.firstMarkers
			}
		}
	}
	var m firstMarkers
	for _, version := range versions {
		for _, kind := range []string{"intro", "credits", "recap", "preview"} {
			if slot := m.slot(kind); *slot == nil {
				*slot = *version.slot(kind)
			}
		}
	}
	return m
}
