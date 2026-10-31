package jellyfin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// mediaAnswer is the status and body of a media request whose token comes
// as the header's, as the ApiKey query parameter, or as api_key.
type mediaAnswer struct {
	status int
	body   string
}

// askSigned requests path with token in the Authorization header (by
// "header"), or as the query parameter named by.
func askSigned(t *testing.T, s testServer, path, by, token string) mediaAnswer {
	t.Helper()
	var header http.Header
	if by == "header" {
		header = http.Header{"Authorization": {app("tv", token)}}
	} else {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		path += separator + by + "=" + token
	}
	response, body := fetchURL(t, s.url+path, header)
	return mediaAnswer{response.StatusCode, body}
}

// An addon subtitle whose URL names the title, not the version, is synced
// for the caller the token names: given as api_key, without legacy
// authorization, as with the header or ApiKey. The streams Jellyfin
// serves to anyone, of videos and audio tracks, take the token so too;
// those it serves only to a signed-in caller, the universal audio stream
// and downloads, keep to Jellyfin's rule and refuse api_key.
func TestMediaRequestsSignedWithApiKey(t *testing.T) {
	t.Parallel()
	p := playing(t)
	movie, _ := accounts.ParseID(p.movie)
	files, err := p.library.Subtitles(t.Context(), p.user, movie)
	if err != nil || len(files) == 0 {
		t.Fatalf("subtitles: %+v %v", files, err)
	}
	// The addon's subtitle, at 1 second, is synced 2 seconds later to
	// every version of the movie.
	for _, version := range p.versions {
		if _, err := p.pool.Exec(t.Context(), `INSERT INTO version_speech (version_id, reference) VALUES ($1, 'track')`, version.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := p.pool.Exec(t.Context(), `INSERT INTO subtitle_syncs (version_id, subtitle_id, ratio, offset_ms) VALUES ($1, $2, 1, 2000)`,
			version.ID, files[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	subtitle := "/Videos/" + p.movie + "/" + p.movie + "/Subtitles/0/0/Stream.srt"
	for _, by := range []string{"header", "ApiKey", "api_key"} {
		if got := askSigned(t, p.testServer, subtitle, by, p.token); got.status != http.StatusOK || !strings.Contains(got.body, "00:00:03,000 --> 00:00:06,000") {
			t.Errorf("the subtitle signed by %s: %d %q", by, got.status, got.body)
		}
	}
	for _, tc := range []struct {
		name, path string
		// status is the answer to ApiKey; apiKey is that to api_key.
		status, apiKey int
	}{
		{"the video stream", "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + p.versions[1].ID.String(), http.StatusOK, http.StatusOK},
		// The movie has no audio track to stream: past the token, it is
		// not found.
		{"the audio stream", "/Audio/" + p.movie + "/stream?static=true", http.StatusNotFound, http.StatusNotFound},
		{"the universal audio stream", "/Audio/" + p.movie + "/universal", http.StatusNotFound, http.StatusUnauthorized},
		{"the download", "/Items/" + p.movie + "/Download", http.StatusOK, http.StatusUnauthorized},
	} {
		keyed := askSigned(t, p.testServer, tc.path, "ApiKey", p.token)
		legacy := askSigned(t, p.testServer, tc.path, "api_key", p.token)
		if keyed.status != tc.status || legacy.status != tc.apiKey || tc.status == tc.apiKey && legacy != keyed {
			t.Errorf("%s: %d with ApiKey, %d with api_key; want %d and %d", tc.name, keyed.status, legacy.status, tc.status, tc.apiKey)
		}
	}
}

// A file attached to a version, a font, comes for a token given as
// api_key as for ApiKey: Jellyfin serves attachments to anyone.
func TestAttachmentsSignedWithApiKey(t *testing.T) {
	t.Parallel()
	s, user, token, id := playingFile(t, subtitledFile)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	font := "/Videos/" + hyphenated(id) + "/" + versions[0].ID.String() + "/Attachments/3"
	keyed := askSigned(t, s, font, "ApiKey", token)
	legacy := askSigned(t, s, font, "api_key", token)
	if keyed.status != http.StatusOK || legacy != keyed {
		t.Errorf("the font: %d with ApiKey, %d with api_key", keyed.status, legacy.status)
	}
}

// A recording under way is streamed for a token given as api_key as for
// ApiKey: past the token, an unknown one is not found.
func TestRecordingStreamsSignedWithApiKey(t *testing.T) {
	t.Parallel()
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	s.user("member", nil)
	token := s.signIn("member", "tv")
	stream := "/LiveTv/LiveRecordings/0123456789abcdef0123456789abcdef/stream"
	keyed := askSigned(t, s, stream, "ApiKey", token)
	legacy := askSigned(t, s, stream, "api_key", token)
	if keyed.status != http.StatusNotFound || legacy.status != keyed.status {
		t.Errorf("the recording: %d with ApiKey, %d with api_key", keyed.status, legacy.status)
	}
}
