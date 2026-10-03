package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// recordedAnswer is an answer of answers.json: status, content type and
// body as Jellyfin 12.1 gave them. The body is JSON when Jellyfin answered
// JSON, else a string of the text.
type recordedAnswer struct {
	Status      int
	ContentType string
	Body        json.RawMessage
}

// text is a body Jellyfin answered as text, or a JSON string's value.
func (a recordedAnswer) text() string {
	var text string
	if err := json.Unmarshal(a.Body, &text); err != nil {
		return string(a.Body)
	}
	return text
}

func recordedAnswers(t *testing.T) map[string]recordedAnswer {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "ass", "answers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var answers map[string]recordedAnswer
	if err := json.Unmarshal(raw, &answers); err != nil {
		t.Fatal(err)
	}
	return answers
}

func TestEncodingConfigurationMatchesJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	answers := recordedAnswers(t)

	// jellyfin-web reads it as any user before rendering ASS subtitles.
	recorded := answers["Encoding configuration as viewer"]
	var want, got map[string]any
	if err := json.Unmarshal(recorded.Body, &want); err != nil {
		t.Fatal(err)
	}
	// The path of Jellyfin's own FFmpeg.
	delete(want, "EncoderAppPathDisplay")
	for _, key := range []string{"encoding", "Encoding"} {
		status, body := s.call(http.MethodGet, "/System/Configuration/"+key, app("tv", token), nil)
		if err := json.Unmarshal(body, &got); status != recorded.Status || err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %d %s\nwant %+v", key, status, body, want)
		}
	}

	unknown := answers["Unknown configuration key"]
	if status, body := s.call(http.MethodGet, "/System/Configuration/unknown", app("tv", token), nil); status != unknown.Status || string(body) != unknown.text() {
		t.Errorf("unknown key: %d %q, want %d %q", status, body, unknown.Status, unknown.text())
	}
	if status, _ := s.call(http.MethodGet, "/System/Configuration/encoding", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without a token: %d", status)
	}
}
