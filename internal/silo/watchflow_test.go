package silo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// The routes a Silo client asks while it opens a movie or an episode,
// beside the watch detail: the versions' video tracks, the skip markers of
// a version, its seek-bar previews, and the series' remembered tracks.

// hiddenTitle is a title the Jellyfin API does not show the user.
const hiddenTitle = "dddddddddddddddddddddddddddddddd"

// unserved reports whether the API logged an operation it does not serve.
func (h *harness) unserved() bool {
	return strings.Contains(h.log.String(), "does not serve")
}

// A version lists its video tracks, as the contract describes them: a
// client takes a version listing audio tracks and no video track for
// audio alone, and plays it without a picture.
func TestVersionsListTheirVideoTracks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		switch c.path {
		case "/Items/" + testVersion:
			jsonAnswer(w, map[string]any{"Id": testVersion, "Name": "Episode", "Type": "Episode", "MediaSources": []any{map[string]any{
				"Id": testVersion, "Container": "mkv", "MediaStreams": []any{
					map[string]any{"Type": "Video", "Index": 0, "Codec": "hevc", "Profile": "Main 10", "Level": 150.0, "Width": 3840,
						"Height": 2160, "IsInterlaced": false, "RealFrameRate": 23.976, "BitDepth": 10, "BitRate": 15_000_000,
						"VideoRange": "HDR", "VideoRangeType": "DOVIWithHDR10", "DvProfile": 8, "DvLevel": 6,
						"DvBlSignalCompatibilityId": 1, "RpuPresentFlag": 1, "BlPresentFlag": 1, "ElPresentFlag": 0,
						"ColorTransfer": "smpte2084", "ColorPrimaries": "bt2020"},
					map[string]any{"Type": "Audio", "Index": 1, "Codec": "eac3", "Language": "eng"}}}}})
		case "/Users/Me":
			jsonAnswer(w, map[string]any{"Configuration": map[string]any{}})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	read := func(path, list string) []map[string]any {
		t.Helper()
		w := h.do(http.MethodGet, path, nil)
		var answer map[string]json.RawMessage
		var versions []struct {
			VideoTracks []map[string]any `json:"video_tracks"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &answer) != nil || json.Unmarshal(answer[list], &versions) != nil ||
			len(versions) != 1 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return versions[0].VideoTracks
	}
	for path, list := range map[string]string{"/api/v2/watch/" + testVersion: "versions", "/api/v2/catalog/items/" + testVersion + "/versions": "items"} {
		tracks := read(path, list)
		if len(tracks) != 1 {
			t.Fatalf("%s: video tracks %v", path, tracks)
		}
		got := tracks[0]
		want := map[string]any{"codec": "hevc", "profile": "Main 10", "level": 150.0, "width": 3840.0, "height": 2160.0, "interlaced": false,
			"frame_rate": "23.976", "bit_depth": 10.0, "bitrate": 15000.0, "video_range": "HDR", "video_range_type": "DOVIWithHDR10",
			"dv_profile": 8.0, "dv_level": 6.0, "dv_bl_compat_id": 1.0, "dv_config_present": true, "dv_bl_compat_id_present": true,
			"dv_rpu_present": true, "dv_bl_present": true, "dv_el_present": false, "color_transfer": "smpte2084", "color_primaries": "bt2020"}
		for key, value := range want {
			if got[key] != value {
				t.Errorf("%s: %s is %v, want %v", path, key, got[key], value)
			}
		}
	}
}

// fileNumber is the number the apps know a version by.
func (h *harness) fileNumber(id string) string {
	h.t.Helper()
	parsed, err := accounts.ParseID(id)
	if err != nil {
		h.t.Fatal(err)
	}
	number, err := h.server.numbers.number(h.t.Context(), parsed)
	if err != nil {
		h.t.Fatal(err)
	}
	return number
}

// markedTitle answers markedVersions' movie by either of its versions, as
// the Jellyfin API describes a title opened by one, a series, and nothing
// of a hidden title.
func markedTitle(w http.ResponseWriter, c call) {
	switch c.path {
	case "/Items/" + otherVersion:
		c.path = "/Items/" + testVersion
	case "/Items/" + hiddenTitle:
		w.WriteHeader(http.StatusNotFound)
		return
	case "/Items/" + seriesTitle:
		jsonAnswer(w, map[string]any{"Id": seriesTitle, "Name": "Series", "Type": "Series"})
		return
	}
	markedVersions(w, c)
}

// The skip markers of a title's first version, or of a version by its
// number, are those of the watch detail: the first of each kind with
// where it comes from, an empty object for a kind it has none of, and
// every one.
func TestMarkersOfATitleAndOfAVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t, markedTitle)
	read := func(path string) (testMarkers, map[string]json.RawMessage) {
		t.Helper()
		w := h.do(http.MethodGet, path, nil)
		var m testMarkers
		var raw map[string]json.RawMessage
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &m) != nil || json.Unmarshal(w.Body.Bytes(), &raw) != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return m, raw
	}
	first, _ := read("/api/v2/markers/items/" + testVersion)
	if first.FileID != h.fileNumber(testVersion) || len(first.Segments) != 5 || first.Recap == nil || first.Recap.Provider != "skipdb" ||
		first.Intro == nil || first.Intro.Start != 30.5 || first.Credits == nil || first.Credits.End != 7000 || first.Preview == nil ||
		first.Preview.Algorithm != "polyfin.audio:v1" {
		t.Errorf("the title's: %+v", first)
	}
	byNumber, _ := read("/api/v2/markers/files/" + h.fileNumber(testVersion))
	if byNumber.FileID != first.FileID || len(byNumber.Segments) != 5 {
		t.Errorf("the first version's: %+v", byNumber)
	}
	second, raw := read("/api/v2/markers/files/" + h.fileNumber(otherVersion))
	if second.FileID != h.fileNumber(otherVersion) || second.Intro == nil || second.Intro.Start != 10 || len(second.Segments) != 1 {
		t.Errorf("the second version's: %+v", second)
	}
	for _, kind := range []string{"credits", "recap", "preview"} {
		if string(raw[kind]) != "{}" {
			t.Errorf("the second version's %s: %s, want {}", kind, raw[kind])
		}
	}
	if h.unserved() {
		t.Errorf("logged as not served: %s", h.log)
	}
}

// A version without skip markers has an empty object for each kind and
// no segment, never not found.
func TestMarkersOfAVersionWithoutAny(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		if strings.HasPrefix(c.path, "/Polyfin/MediaSegments/") {
			jsonAnswer(w, map[string]any{"Items": []any{}})
			return
		}
		markedVersions(w, c)
	})
	for _, path := range []string{"/api/v2/markers/items/" + testVersion, "/api/v2/markers/files/" + h.fileNumber(testVersion)} {
		w := h.do(http.MethodGet, path, nil)
		want := `{"file_id":"` + h.fileNumber(testVersion) + `","intro":{},"credits":{},"recap":{},"preview":{},"marker_segments":[]}`
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != want {
			t.Errorf("%s: %d %s, want %s", path, w.Code, w.Body, want)
		}
	}
}

// seriesTitle is a series the user sees.
const seriesTitle = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// The markers keep to what the user may see and play: a title hidden from
// them, a number no version was given, a series, and a profile that is
// not theirs are refused.
func TestMarkersKeepToTheUsersAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t, markedTitle)
	for name, test := range map[string]struct {
		path   string
		header []string
		status int
	}{
		"a hidden title":                {"/api/v2/markers/items/" + hiddenTitle, nil, http.StatusNotFound},
		"a version of a hidden title":   {"/api/v2/markers/files/" + h.fileNumber(hiddenTitle), nil, http.StatusNotFound},
		"an unknown number":             {"/api/v2/markers/files/999999", nil, http.StatusNotFound},
		"a series":                      {"/api/v2/markers/items/" + seriesTitle, nil, http.StatusNotFound},
		"another profile":               {"/api/v2/markers/items/" + testVersion, []string{"X-Profile-Id", accounts.ID{9}.String()}, http.StatusForbidden},
		"another profile, by a version": {"/api/v2/markers/files/1", []string{"X-Profile-Id", accounts.ID{9}.String()}, http.StatusForbidden},
	} {
		if w := h.do(http.MethodGet, test.path, nil, test.header...); w.Code != test.status {
			t.Errorf("%s: %d %s, want %d", name, w.Code, w.Body, test.status)
		}
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v2/markers/items/"+testVersion, nil)
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d %s", w.Code, w.Body)
	}
}

// ownVersion is the version the movie's first media source stands for,
// as its ETag names it.
const ownVersion = "cccccccccccccccccccccccccccccccc"

// previewed answers a movie of two versions, the first with scrubbing
// thumbnails at two widths, of 150 thumbnails in tiles of 10 by 10.
func previewed(w http.ResponseWriter, c call) {
	switch c.path {
	case "/Items/" + testVersion:
		source := func(id, etag string) map[string]any {
			return map[string]any{"Id": id, "ETag": etag, "Container": "mkv",
				"MediaStreams": []any{map[string]any{"Type": "Video", "Index": 0, "Codec": "h264", "Height": 1080}}}
		}
		info := func(width int) map[string]any {
			return map[string]any{"Width": width, "Height": width * 9 / 16, "TileWidth": 10, "TileHeight": 10, "ThumbnailCount": 150,
				"Interval": 10000}
		}
		jsonAnswer(w, map[string]any{"Id": testVersion, "Name": "Movie", "Type": "Movie",
			"MediaSources": []any{source(testVersion, ownVersion), source(otherVersion, otherVersion)},
			"Trickplay":    map[string]any{testVersion: map[string]any{"320": info(320), "160": info(160)}}})
	case "/Items/" + hiddenTitle:
		w.WriteHeader(http.StatusNotFound)
	case "/Users/Me":
		jsonAnswer(w, map[string]any{"Configuration": map[string]any{}})
	case "/Polyfin/MediaSegments/" + testVersion, "/Polyfin/MediaSegments/" + otherVersion:
		jsonAnswer(w, map[string]any{"Items": []any{}})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type testTrickplay struct {
	FileID          string `json:"file_id"`
	IntervalMS      int    `json:"interval_ms"`
	ThumbnailWidth  int    `json:"thumbnail_width"`
	ThumbnailHeight int    `json:"thumbnail_height"`
	TileColumns     int    `json:"tile_columns"`
	TileRows        int    `json:"tile_rows"`
	ThumbnailCount  int    `json:"thumbnail_count"`
	Sheets          []struct {
		Index int    `json:"index"`
		URL   string `json:"url"`
	} `json:"sheets"`
	ExpiresAt string `json:"expires_at"`
}

// fetch loads an address as an image, without credentials.
func (h *harness) fetch(address string) *httptest.ResponseRecorder {
	h.t.Helper()
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, httptest.NewRequestWithContext(h.t.Context(), http.MethodGet, address, nil))
	return w
}

// A version's scrubbing thumbnails are its seek-bar previews: the watch
// detail tells which versions have them, and they describe the widest
// thumbnails, with an address for each tile that works without
// credentials until it expires.
func TestTrickplayOfAVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t, previewed)
	var mu sync.Mutex
	var asked []string
	h.server.options.Tiles = func(_ context.Context, version accounts.ID, width, index int) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, version.String())
		if version.String() != ownVersion || width != 320 || index > 1 {
			return nil, thumbnails.ErrNotFound
		}
		return []byte("tile " + string(rune('0'+index))), nil
	}
	w := h.do(http.MethodGet, "/api/v2/watch/"+testVersion, nil)
	var detail struct {
		Versions []struct {
			FileID    string `json:"file_id"`
			Trickplay bool   `json:"trickplay_available"`
		} `json:"versions"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Versions) != 2 ||
		!detail.Versions[0].Trickplay || detail.Versions[1].Trickplay {
		t.Fatalf("watch: %d %s", w.Code, w.Body)
	}
	w = h.do(http.MethodGet, "/api/v2/watch/"+testVersion+"/trickplay?file_id="+detail.Versions[0].FileID, nil)
	var previews testTrickplay
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &previews) != nil {
		t.Fatalf("trickplay: %d %s", w.Code, w.Body)
	}
	if previews.FileID != detail.Versions[0].FileID || previews.IntervalMS != 10000 || previews.ThumbnailWidth != 320 ||
		previews.ThumbnailHeight != 180 || previews.TileColumns != 10 || previews.TileRows != 10 || previews.ThumbnailCount != 150 ||
		len(previews.Sheets) != 2 || previews.Sheets[1].Index != 1 || previews.ExpiresAt != instant(h.clock.Add(time.Hour)) {
		t.Fatalf("trickplay: %s", w.Body)
	}
	for n, sheet := range previews.Sheets {
		if w := h.fetch(sheet.URL); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" ||
			w.Body.String() != "tile "+string(rune('0'+n)) {
			t.Errorf("sheet %d: %d %s", n, w.Code, w.Body)
		}
	}
	if len(asked) != 2 || asked[0] != ownVersion {
		t.Errorf("tiles asked of %v, want the version's own", asked)
	}
	first := previews.Sheets[0].URL
	for name, test := range map[string]struct {
		address string
		status  int
	}{
		"another tile":       {strings.Replace(first, "/0.jpg", "/5.jpg", 1), http.StatusNotFound},
		"another width":      {strings.Replace(first, "/320/", "/160/", 1), http.StatusForbidden},
		"another version":    {strings.Replace(first, ownVersion, otherVersion, 1), http.StatusForbidden},
		"a signature forged": {first[:strings.Index(first, "&sig=")] + "&sig=forged", http.StatusForbidden},
		"no signature":       {first[:strings.Index(first, "?")], http.StatusForbidden},
	} {
		if w := h.fetch(test.address); w.Code != test.status {
			t.Errorf("%s: %d %s, want %d", name, w.Code, w.Body, test.status)
		}
	}
	*h.clock = h.clock.Add(time.Hour)
	if w := h.fetch(first); w.Code != http.StatusForbidden {
		t.Errorf("expired: %d %s", w.Code, w.Body)
	}
	if h.unserved() {
		t.Errorf("logged as not served: %s", h.log)
	}
}

// A version without scrubbing thumbnails has no previews, as Silo answers
// it: not found, from a route the API serves. The previews keep to what
// the user may see.
func TestTrickplayOfAVersionWithoutAnyAndAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t, previewed)
	second := h.fileNumber(otherVersion)
	for name, test := range map[string]struct {
		path   string
		header []string
		status int
		code   string
	}{
		"a version without previews": {"/api/v2/watch/" + testVersion + "/trickplay?file_id=" + second, nil, http.StatusNotFound, "not_found"},
		"no file named":              {"/api/v2/watch/" + testVersion + "/trickplay", nil, http.StatusUnprocessableEntity, "validation_failed"},
		"a file of another title":    {"/api/v2/watch/" + testVersion + "/trickplay?file_id=999999", nil, http.StatusNotFound, "not_found"},
		"a hidden title":             {"/api/v2/watch/" + hiddenTitle + "/trickplay?file_id=" + second, nil, http.StatusNotFound, "not_found"},
		"another profile": {"/api/v2/watch/" + testVersion + "/trickplay?file_id=" + second, []string{"X-Profile-Id", accounts.ID{9}.String()},
			http.StatusForbidden, "permission_denied"},
	} {
		w := h.do(http.MethodGet, test.path, nil, test.header...)
		var p problemJSON
		if w.Code != test.status || json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Type != problemTypes+test.code {
			t.Errorf("%s: %d %s, want %d %s", name, w.Code, w.Body, test.status, test.code)
		}
	}
	if h.unserved() {
		t.Errorf("logged as not served: %s", h.log)
	}
}

// fakeChoices keeps track choices in memory.
type fakeChoices struct {
	mu      sync.Mutex
	choices map[[2]accounts.ID]preferences.TrackChoice
	at      time.Time
}

func (f *fakeChoices) TrackChoice(_ context.Context, user, title accounts.ID) (preferences.TrackChoice, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	choice, ok := f.choices[[2]accounts.ID{user, title}]
	if !ok {
		return preferences.TrackChoice{}, time.Time{}, nil
	}
	return choice, f.at, nil
}

func (f *fakeChoices) PutTrackChoice(_ context.Context, user, title accounts.ID, choice preferences.TrackChoice) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if choice.Empty() {
		delete(f.choices, [2]accounts.ID{user, title})
	} else {
		f.choices[[2]accounts.ID{user, title}] = choice
	}
	return nil
}

func (f *fakeChoices) of(user, title accounts.ID) (preferences.TrackChoice, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	choice, ok := f.choices[[2]accounts.ID{user, title}]
	return choice, ok
}

// remembering answers a series, an episode and a hidden series, and the
// user's configuration, remembering the selections remember says.
func remembering(remember *map[string]any) func(w http.ResponseWriter, c call) {
	return func(w http.ResponseWriter, c call) {
		switch c.path {
		case "/Items/" + seriesTitle:
			jsonAnswer(w, map[string]any{"Id": seriesTitle, "Name": "Series", "Type": "Series"})
		case "/Items/" + testVersion:
			jsonAnswer(w, map[string]any{"Id": testVersion, "Name": "Episode", "Type": "Episode"})
		case "/Items/" + hiddenTitle:
			w.WriteHeader(http.StatusNotFound)
		case "/Users/Me":
			jsonAnswer(w, map[string]any{"Configuration": *remember})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func choicesHarness(t *testing.T) (*harness, *fakeChoices, accounts.ID, *map[string]any) {
	t.Helper()
	remember := &map[string]any{"RememberAudioSelections": true, "RememberSubtitleSelections": true}
	h := newHarness(t, remembering(remember))
	choices := &fakeChoices{choices: map[[2]accounts.ID]preferences.TrackChoice{}, at: time.Date(2026, 10, 20, 8, 30, 0, 0, time.UTC)}
	h.server.options.TrackChoices = choices
	series, err := accounts.ParseID(seriesTitle)
	if err != nil {
		t.Fatal(err)
	}
	return h, choices, series, remember
}

// A series' remembered audio and subtitles are the track choices Polyfin
// remembers on it: a language, in its regional variant, and subtitles in
// a language with their flags, or off. The apps keep them, and forget
// them; a series without one has none, as Silo answers it: not found.
func TestSeriesTrackPreferences(t *testing.T) {
	t.Parallel()
	h, choices, series, _ := choicesHarness(t)
	audio, subtitles := "/api/v2/audio-prefs/"+seriesTitle, "/api/v2/subtitle-prefs/"+seriesTitle
	answer := func(path string, status int) map[string]any {
		t.Helper()
		w := h.do(http.MethodGet, path, nil)
		var got map[string]any
		if w.Code != status || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("%s: %d %s, want %d", path, w.Code, w.Body, status)
		}
		return got
	}
	put := func(path string, body map[string]any) {
		t.Helper()
		if w := h.do(http.MethodPut, path, body); w.Code != http.StatusNoContent {
			t.Fatalf("PUT %s: %d %s", path, w.Code, w.Body)
		}
	}
	if got := answer(audio, http.StatusNotFound); got["type"] != problemTypes+"not_found" {
		t.Errorf("no audio: %v", got)
	}
	answer(subtitles, http.StatusNotFound)

	put(audio, map[string]any{"audio_track_index": 1, "audio_language": "por_br", "track_signature": map[string]any{"codec": "aac"}})
	put(subtitles, map[string]any{"subtitle_track_index": 2, "subtitle_language": "ENG", "subtitle_mode": "always",
		"track_signature": map[string]any{"forced": false, "hearing_impaired": true}})
	if got, _ := choices.of(h.accounts.user.ID, series); got != (preferences.TrackChoice{AudioLanguage: "pt-br", AudioVariant: "pt-BR",
		Subtitle: preferences.SubtitleChoice{Language: "en", HearingImpaired: true}}) {
		t.Errorf("kept %+v", got)
	}
	updated := instant(choices.at)
	got := answer(audio, http.StatusOK)
	if got["profile_id"] != h.profile || got["series_id"] != seriesTitle || got["audio_track_index"] != 0.0 || got["audio_language"] != "pt-BR" ||
		got["updated_at"] != updated || got["track_signature"].(map[string]any)["language"] != "pt-BR" {
		t.Errorf("audio: %v", got)
	}
	got = answer(subtitles, http.StatusOK)
	signature, _ := got["track_signature"].(map[string]any)
	if got["subtitle_track_index"] != 0.0 || got["subtitle_language"] != "en" || got["subtitle_mode"] != "always" ||
		got["updated_at"] != updated || signature["language"] != "en" || signature["hearing_impaired"] != true || signature["forced"] != false {
		t.Errorf("subtitles: %v", got)
	}

	put(subtitles, map[string]any{"subtitle_track_index": -1, "subtitle_language": "", "subtitle_mode": "off"})
	got = answer(subtitles, http.StatusOK)
	if got["subtitle_track_index"] != -1.0 || got["subtitle_language"] != "" || got["subtitle_mode"] != "off" || got["track_signature"] != nil {
		t.Errorf("subtitles off: %v", got)
	}

	if w := h.do(http.MethodDelete, subtitles, nil); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE subtitles: %d %s", w.Code, w.Body)
	}
	answer(subtitles, http.StatusNotFound)
	if answer(audio, http.StatusOK)["audio_language"] != "pt-BR" {
		t.Error("forgetting the subtitles forgot the audio")
	}
	put(audio, map[string]any{"audio_track_index": -1})
	answer(audio, http.StatusNotFound)
	if w := h.do(http.MethodDelete, audio, nil); w.Code != http.StatusNoContent {
		t.Errorf("DELETE of none: %d %s", w.Code, w.Body)
	}
	if got, ok := choices.of(h.accounts.user.ID, series); ok {
		t.Errorf("left %+v", got)
	}
	if h.unserved() {
		t.Errorf("logged as not served: %s", h.log)
	}
}

// A kind of selection the user's configuration does not remember is
// neither answered nor kept, as Jellyfin apps' choices are not.
func TestSeriesTrackPreferencesKeepToWhatIsRemembered(t *testing.T) {
	t.Parallel()
	h, choices, series, remember := choicesHarness(t)
	user := h.accounts.user.ID
	choices.choices[[2]accounts.ID{user, series}] = preferences.TrackChoice{AudioLanguage: "jpn", Subtitle: preferences.SubtitleChoice{Off: true}}
	*remember = map[string]any{"RememberAudioSelections": false, "RememberSubtitleSelections": true}
	if w := h.do(http.MethodGet, "/api/v2/audio-prefs/"+seriesTitle, nil); w.Code != http.StatusNotFound {
		t.Errorf("audio not remembered: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodGet, "/api/v2/subtitle-prefs/"+seriesTitle, nil); w.Code != http.StatusOK {
		t.Errorf("subtitles remembered: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodPut, "/api/v2/audio-prefs/"+seriesTitle, map[string]any{"audio_track_index": 0, "audio_language": "fr"}); w.Code !=
		http.StatusNoContent {
		t.Errorf("PUT audio: %d %s", w.Code, w.Body)
	}
	if got, _ := choices.of(user, series); got.AudioLanguage != "jpn" {
		t.Errorf("kept %+v while audio is not remembered", got)
	}
}

// The remembered tracks keep to what the user may see, and to a series or
// a movie; what the apps send is checked.
func TestSeriesTrackPreferencesAccessAndBodies(t *testing.T) {
	t.Parallel()
	h, choices, _, _ := choicesHarness(t)
	for name, test := range map[string]struct {
		method, path string
		body         map[string]any
		header       []string
		status       int
	}{
		"a hidden series":      {http.MethodGet, "/api/v2/audio-prefs/" + hiddenTitle, nil, nil, http.StatusNotFound},
		"kept on a hidden one": {http.MethodPut, "/api/v2/subtitle-prefs/" + hiddenTitle, map[string]any{"subtitle_track_index": -1}, nil, http.StatusNotFound},
		"an episode":           {http.MethodGet, "/api/v2/subtitle-prefs/" + testVersion, nil, nil, http.StatusNotFound},
		"another profile": {http.MethodGet, "/api/v2/audio-prefs/" + seriesTitle, nil, []string{"X-Profile-Id", accounts.ID{9}.String()},
			http.StatusForbidden},
		"no profile":       {http.MethodDelete, "/api/v2/audio-prefs/" + seriesTitle, nil, []string{"X-Profile-Id", ""}, http.StatusUnprocessableEntity},
		"no track":         {http.MethodPut, "/api/v2/audio-prefs/" + seriesTitle, map[string]any{"audio_language": "en"}, nil, http.StatusUnprocessableEntity},
		"a place below -1": {http.MethodPut, "/api/v2/subtitle-prefs/" + seriesTitle, map[string]any{"subtitle_track_index": -2}, nil, http.StatusUnprocessableEntity},
		"not a language": {http.MethodPut, "/api/v2/audio-prefs/" + seriesTitle, map[string]any{"audio_track_index": 0, "audio_language": "not one"},
			nil, http.StatusUnprocessableEntity},
	} {
		if w := h.do(test.method, test.path, test.body, test.header...); w.Code != test.status {
			t.Errorf("%s: %d %s, want %d", name, w.Code, w.Body, test.status)
		}
	}
	if len(choices.choices) != 0 {
		t.Errorf("kept %v", choices.choices)
	}
}
