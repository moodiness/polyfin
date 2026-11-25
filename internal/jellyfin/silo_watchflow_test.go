package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/preferences"
)

// Silo's apps get a version's video tracks, and its scrubbing thumbnails
// as seek-bar previews: the tiles of its first version, at addresses that
// work without credentials.
func TestSiloAppsGetVideoTracksAndSeekBarPreviews(t *testing.T) {
	t.Parallel()
	s := withThumbnails(t)
	allowSilo(t, s.testServer)
	a := servingSilo(t, s.testServer)
	token, profile := a.signIn(t, "member")
	var detail struct {
		Versions []struct {
			FileID      string `json:"file_id"`
			Trickplay   bool   `json:"trickplay_available"`
			VideoTracks []struct {
				Codec  string `json:"codec"`
				Height int    `json:"height"`
			} `json:"video_tracks"`
		} `json:"versions"`
	}
	a.get(t, "/api/v2/watch/"+s.movie, token, profile, &detail)
	if len(detail.Versions) != 2 || !detail.Versions[0].Trickplay || detail.Versions[1].Trickplay {
		t.Fatalf("versions: %+v", detail.Versions)
	}
	// The second version was analyzed, as ffprobe found it.
	if tracks := detail.Versions[1].VideoTracks; len(tracks) != 1 || tracks[0].Codec != "h264" || tracks[0].Height == 0 {
		t.Errorf("the analyzed version's video tracks: %+v", tracks)
	}
	var previews struct {
		ThumbnailWidth int `json:"thumbnail_width"`
		ThumbnailCount int `json:"thumbnail_count"`
		Sheets         []struct {
			URL string `json:"url"`
		} `json:"sheets"`
	}
	a.get(t, "/api/v2/watch/"+s.movie+"/trickplay?file_id="+detail.Versions[0].FileID, token, profile, &previews)
	if previews.ThumbnailWidth != 320 || previews.ThumbnailCount != 150 || len(previews.Sheets) != len(s.tiles) {
		t.Fatalf("previews: %+v", previews)
	}
	for n, sheet := range previews.Sheets {
		if status, data := a.do(t, http.MethodGet, sheet.URL, "", "", nil); status != http.StatusOK || string(data) != string(s.tiles[n]) {
			t.Errorf("sheet %d: %d %q", n, status, data)
		}
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/watch/"+s.movie+"/trickplay?file_id="+detail.Versions[1].FileID, token, profile,
		nil); status != http.StatusNotFound || problemCode(data) != "not_found" {
		t.Errorf("a version without previews: %d %s", status, data)
	}
	// Turned off, a version has none.
	s.setting(t, func(settings *accounts.Settings) { settings.Trickplay = false })
	if status, data := a.do(t, http.MethodGet, previews.Sheets[0].URL, "", "", nil); status != http.StatusNotFound {
		t.Errorf("a sheet with thumbnails turned off: %d %s", status, data)
	}
}

// The audio and subtitles Silo's apps keep on a series or a movie are the
// track choices Jellyfin apps' plays follow: the Jellyfin API sees each
// change at once, and the apps read them back.
func TestSiloAppsRememberTracksOnATitle(t *testing.T) {
	t.Parallel()
	p := playing(t)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	movie, err := accounts.ParseID(p.movie)
	if err != nil {
		t.Fatal(err)
	}
	remembered := func() preferences.TrackChoice {
		t.Helper()
		choices, err := p.handler.userTrackChoices(t.Context(), p.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		return choices[movie]
	}
	// Read once, the choices are kept in memory.
	if got := remembered(); got != (preferences.TrackChoice{}) {
		t.Fatalf("before: %+v", got)
	}
	send := func(method, path string, body any) {
		t.Helper()
		if status, data := a.do(t, method, path, token, profile, body); status != http.StatusNoContent {
			t.Fatalf("%s %s: %d %s", method, path, status, data)
		}
	}
	audio, subtitles := "/api/v2/audio-prefs/"+p.movie, "/api/v2/subtitle-prefs/"+p.movie
	send(http.MethodPut, audio, map[string]any{"audio_track_index": 1, "audio_language": "fre"})
	send(http.MethodPut, subtitles, map[string]any{"subtitle_track_index": 0, "subtitle_language": "eng", "subtitle_mode": "always",
		"track_signature": map[string]any{"forced": true}})
	if got := remembered(); got != (preferences.TrackChoice{AudioLanguage: "fr", Subtitle: preferences.SubtitleChoice{Language: "en", Forced: true}}) {
		t.Errorf("kept: %+v", got)
	}
	var kept struct {
		AudioLanguage    string `json:"audio_language"`
		SubtitleLanguage string `json:"subtitle_language"`
		UpdatedAt        string `json:"updated_at"`
	}
	a.get(t, audio, token, profile, &kept)
	a.get(t, subtitles, token, profile, &kept)
	if kept.AudioLanguage != "fr" || kept.SubtitleLanguage != "en" || kept.UpdatedAt == "" {
		t.Errorf("read back: %+v", kept)
	}
	send(http.MethodDelete, audio, nil)
	send(http.MethodDelete, subtitles, nil)
	if got := remembered(); got != (preferences.TrackChoice{}) {
		t.Errorf("forgotten: %+v", got)
	}
	if status, data := a.do(t, http.MethodGet, audio, token, profile, nil); status != http.StatusNotFound || problemCode(data) != "not_found" {
		t.Errorf("none: %d %s", status, data)
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/subtitle-prefs/"+p.remote, token, profile, nil); status != http.StatusNotFound {
		t.Errorf("another title: %d %s", status, data)
	}
}
