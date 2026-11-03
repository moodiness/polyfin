package silo

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// otherVersion is a second version of the movie markedVersions answers.
const otherVersion = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// markedVersions answers the Jellyfin API for a movie of two versions:
// the first with a recap rated by its database, an intro from another
// one, two credits its chapters name, a preview found in its audio and a
// commercial break; the second with only an intro, of its chapters.
func markedVersions(w http.ResponseWriter, c call) {
	source := func(id string) map[string]any {
		return map[string]any{"Id": id, "Container": "mkv", "RunTimeTicks": int64(7200) * 10_000_000,
			"MediaStreams": []any{map[string]any{"Type": "Video", "Index": 0, "Codec": "h264", "Height": 1080}}}
	}
	second := func(s float64) int64 { return int64(s * 10_000_000) }
	switch c.path {
	case "/Items/" + testVersion:
		jsonAnswer(w, map[string]any{"Id": testVersion, "Name": "Movie", "Type": "Movie",
			"MediaSources": []any{source(testVersion), source(otherVersion)}})
	case "/Users/Me":
		jsonAnswer(w, map[string]any{"Configuration": map[string]any{}})
	case "/Polyfin/MediaSegments/" + testVersion:
		jsonAnswer(w, map[string]any{"Items": []any{
			map[string]any{"Type": "Recap", "StartTicks": 0, "EndTicks": second(20), "Found": "database", "Database": "skipdb",
				"Confidence": 0.9},
			map[string]any{"Type": "Intro", "StartTicks": second(30.5), "EndTicks": second(90), "Found": "database",
				"Database": "theintrodb"},
			map[string]any{"Type": "Commercial", "StartTicks": second(600), "EndTicks": second(660)},
			map[string]any{"Type": "Outro", "StartTicks": second(6900), "EndTicks": second(7000), "Found": "chapters"},
			map[string]any{"Type": "Outro", "StartTicks": second(7060), "EndTicks": second(7150), "Found": "chapters"},
			map[string]any{"Type": "Preview", "StartTicks": second(7150), "EndTicks": second(7199), "Found": "audio"},
		}})
	case "/Polyfin/MediaSegments/" + otherVersion:
		jsonAnswer(w, map[string]any{"Items": []any{
			map[string]any{"Type": "Intro", "StartTicks": second(10), "EndTicks": second(70), "Found": "chapters"}}})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type testMarker struct {
	Start      float64  `json:"start_seconds"`
	End        float64  `json:"end_seconds"`
	Source     string   `json:"source"`
	Provider   string   `json:"provider"`
	Algorithm  string   `json:"algorithm"`
	Confidence *float64 `json:"confidence"`
}

type testMarkers struct {
	FileID   string      `json:"file_id"`
	Intro    *testMarker `json:"intro"`
	Credits  *testMarker `json:"credits"`
	Recap    *testMarker `json:"recap"`
	Preview  *testMarker `json:"preview"`
	Segments []struct {
		Kind  string  `json:"kind"`
		Start float64 `json:"start_seconds"`
	} `json:"marker_segments"`
}

// The watch detail gives each version's skip markers, every one in
// marker_segments and the first of each kind with where it comes from,
// and the detail those of the version file_id names, else each kind's
// first among the versions.
func TestWatchDetailGivesSkipMarkers(t *testing.T) {
	t.Parallel()
	h := newHarness(t, markedVersions)
	read := func(query string) (testMarkers, []testMarkers) {
		t.Helper()
		w := h.do(http.MethodGet, "/api/v2/watch/"+testVersion+query, nil)
		var detail struct {
			testMarkers
			Versions []testMarkers `json:"versions"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Versions) != 2 {
			t.Fatalf("watch: %d %s", w.Code, w.Body)
		}
		if err := silotest.CheckWatchMarkers(w.Body.Bytes()); err != nil {
			t.Errorf("watch%s: %v", query, err)
		}
		return detail.testMarkers, detail.Versions
	}
	detail, versions := read("")
	first := versions[0]
	var kinds []string
	for _, segment := range first.Segments {
		kinds = append(kinds, segment.Kind)
	}
	if want := []string{"recap", "intro", "credits", "credits", "preview"}; !slices.Equal(kinds, want) {
		t.Errorf("marker_segments %v, want %v", kinds, want)
	}
	for name, test := range map[string]struct {
		got  *testMarker
		want testMarker
	}{
		"recap":   {first.Recap, testMarker{0, 20, "online", "skipdb", "polyfin.database:v1", new(0.9)}},
		"intro":   {first.Intro, testMarker{30.5, 90, "online", "theintrodb", "polyfin.database:v1", nil}},
		"credits": {first.Credits, testMarker{6900, 7000, "scanner", "", "polyfin.chapters:v1", nil}},
		"preview": {first.Preview, testMarker{7150, 7199, "scanner", "", "polyfin.audio:v1", nil}},
	} {
		got := test.got
		if got == nil || got.Start != test.want.Start || got.End != test.want.End || got.Source != test.want.Source ||
			got.Provider != test.want.Provider || got.Algorithm != test.want.Algorithm ||
			(got.Confidence == nil) != (test.want.Confidence == nil) || got.Confidence != nil && *got.Confidence != *test.want.Confidence {
			t.Errorf("%s: %+v, want %+v", name, got, test.want)
		}
	}
	if second := versions[1]; second.Intro == nil || second.Intro.Start != 10 || second.Credits != nil || len(second.Segments) != 1 {
		t.Errorf("second version: %+v", second)
	}
	// The detail's intro is the first version's.
	if detail.Intro == nil || detail.Intro.Start != 30.5 || detail.Credits == nil || detail.Recap == nil || detail.Preview == nil ||
		detail.Segments != nil {
		t.Errorf("detail: %+v", detail)
	}
	// Asked for the second version, the detail has its markers alone.
	detail, _ = read("?file_id=" + versions[1].FileID)
	if detail.Intro == nil || detail.Intro.Start != 10 || detail.Credits != nil || detail.Recap != nil || detail.Preview != nil {
		t.Errorf("detail of the second version: %+v", detail)
	}
}

// A version whose markers cannot be read still plays, without any.
func TestWatchDetailWithoutMarkers(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		if c.path == "/Polyfin/MediaSegments/"+testVersion {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		markedVersions(w, c)
	})
	w := h.do(http.MethodGet, "/api/v2/watch/"+testVersion, nil)
	var detail struct {
		testMarkers
		Versions []testMarkers `json:"versions"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Versions) != 2 {
		t.Fatalf("watch: %d %s", w.Code, w.Body)
	}
	if first := detail.Versions[0]; first.Segments == nil || len(first.Segments) != 0 || first.Intro != nil {
		t.Errorf("first version: %+v", first)
	}
	if detail.Intro == nil || detail.Intro.Start != 10 {
		t.Errorf("detail: %+v", detail.testMarkers)
	}
	if err := silotest.CheckWatchMarkers(w.Body.Bytes()); err != nil {
		t.Error(err)
	}
}
