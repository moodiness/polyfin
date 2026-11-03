package silotest

import (
	"encoding/json"
	"fmt"
	"slices"
)

// markerKinds are the kinds of marker the apps take from marker_segments:
// Apple PlayerMarkerTimeline.Kind (Screens/Player/PlayerMarkerTimeline.swift)
// knows intro, credits and recap, Android MarkerKind
// (domain/player/MarkerTimeline.kt) preview too. Each leaves out the
// kinds it does not know.
var markerKinds = []string{"intro", "credits", "recap", "preview"}

// markerSources are the classes of source the contract gives a
// WatchMarker's source (contracts/api/v2/openapi.json, WatchMarker).
var markerSources = []string{"scanner", "online", "plugin", "s3", "manual"}

// checkMarker checks a singular marker (intro, credits, recap, preview)
// when sent. Its times are required: Apple APIv2CatalogRead.WatchMarker
// (Networking/APIv2/APIv2CatalogReadModels.swift), Android
// decodeWatchDetail's adaptMarkers (network/apiv2/WatchDetailV2Api.kt),
// whose getValue fails the whole detail without them. Its provenance,
// which neither app reads, is as the contract's WatchMarker gives it:
// source one of its classes, provider and algorithm texts, confidence
// from 0 to 1, absent when unrated rather than 0.
func checkMarker(c *checker, o object, kind, where string) (start, end float64, sent bool) {
	value, ok := o[kind]
	if !ok {
		return 0, 0, false
	}
	marker, isObject := value.(object)
	if !isObject {
		c.fail("%s.%s is not an object: %v", where, kind, value)
		return 0, 0, false
	}
	where += "." + kind
	c.has(marker, where, map[string]string{"start_seconds": "n", "end_seconds": "n"})
	if source, ok := marker["source"]; ok {
		if text, _ := source.(string); !slices.Contains(markerSources, text) {
			c.fail("%s.source %v is not one of %q", where, source, markerSources)
		}
	}
	for _, key := range []string{"provider", "algorithm"} {
		if value, ok := marker[key]; ok {
			if text, _ := value.(string); text == "" {
				c.fail("%s.%s is not a text: %v", where, key, value)
			}
		}
	}
	if value, ok := marker["confidence"]; ok {
		if n, number := value.(float64); !number || n < 0 || n > 1 {
			c.fail("%s.confidence %v is not from 0 to 1", where, value)
		}
	}
	start, _ = marker["start_seconds"].(float64)
	end, _ = marker["end_seconds"].(float64)
	return start, end, true
}

// CheckWatchMarkers checks the skip markers of a watch detail
// (GET /api/v2/watch/{id}) as the apps' players take them:
//
//   - The detail's and each version's singular markers (see checkMarker).
//   - Each version's marker_segments, an array: Apple
//     APIv2CatalogRead.WatchFileVersion and Android FileVersion
//     (model/catalog/CatalogModels.kt) decode it, and a present one replaces
//     the singular fields, even empty: Apple PlayerMarkerTimeline.init,
//     Android MarkerTimeline.resolve. So it holds each singular marker the
//     version has, with the same times, or the player never offers it; the
//     singular one is the first of its kind, as Android FileVersion says.
//   - Each occurrence has its kind and times (Apple PlaybackMarkerSegment,
//     Networking/PlaybackMarkerSegment.swift; Android MarkerSegment), a
//     kind the apps know, and a range they keep: finite, from 0, ending
//     after it starts (Apple PlayerMarkerTimeline.isValid, Android
//     MarkerTimeline.resolve), in the order they start, as the contract
//     lists them.
func CheckWatchMarkers(data []byte) error {
	var detail object
	if err := json.Unmarshal(data, &detail); err != nil {
		return err
	}
	c := &checker{}
	for _, kind := range markerKinds {
		checkMarker(c, detail, kind, "detail")
	}
	versions, _ := detail["versions"].([]any)
	for n, value := range versions {
		version, _ := value.(object)
		where := fmt.Sprintf("versions[%d]", n)
		c.has(version, where, map[string]string{"marker_segments": "a"})
		occurrences, _ := version["marker_segments"].([]any)
		type occurrence struct {
			kind       string
			start, end float64
		}
		var listed []occurrence
		for i, value := range occurrences {
			o, _ := value.(object)
			at := fmt.Sprintf("%s.marker_segments[%d]", where, i)
			c.has(o, at, map[string]string{"kind": "s", "start_seconds": "n", "end_seconds": "n"})
			kind, _ := o["kind"].(string)
			start, _ := o["start_seconds"].(float64)
			end, _ := o["end_seconds"].(float64)
			if !slices.Contains(markerKinds, kind) {
				c.fail("%s.kind %q is none the apps know", at, kind)
			}
			if start < 0 || end <= start {
				c.fail("%s runs from %v to %v, which the apps leave out", at, start, end)
			}
			if len(listed) > 0 && start < listed[len(listed)-1].start {
				c.fail("%s starts before the one listed before it", at)
			}
			listed = append(listed, occurrence{kind, start, end})
		}
		for _, kind := range markerKinds {
			start, end, sent := checkMarker(c, version, kind, where)
			first := slices.IndexFunc(listed, func(o occurrence) bool { return o.kind == kind })
			if sent && (first < 0 || listed[first] != occurrence{kind, start, end}) {
				c.fail("%s.%s is not the first %s of its marker_segments, which the apps take in its place", where, kind, kind)
			}
		}
	}
	return c.err()
}
