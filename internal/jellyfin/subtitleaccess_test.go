package jellyfin

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A subtitle file is served for the user its address names. A caller
// Polyfin does not know gets the answer of a title nobody opened, status
// and body alike, once a member opened the title and played it too.
func TestSubtitleFilesAnswerTheSameWithoutCredentialsOnceTheTitleWasOpened(t *testing.T) {
	t.Parallel()
	p := playing(t)
	file := "/Videos/" + p.movie + "/" + p.movie + "/Subtitles/0/Stream.srt"
	addresses := map[string]string{
		"the title's file":         file,
		"from a start position":    "/Videos/" + p.movie + "/" + p.movie + "/Subtitles/0/0/Stream.vtt",
		"a version's file":         "/Videos/" + p.movie + "/" + p.versions[1].ID.String() + "/Subtitles/0/0/Stream.js",
		"a token nobody was given": file + "?ApiKey=" + strings.Repeat("ab", 16),
		"a play session not ours":  file + "?playSessionId=" + strings.Repeat("ab", 16),
	}
	ask := func() map[string]mediaAnswer {
		t.Helper()
		answers := map[string]mediaAnswer{}
		for name, address := range addresses {
			response, body := fetchURL(t, p.url+address, nil)
			answers[name] = mediaAnswer{response.StatusCode, response.Header.Get("Content-Type") + "\n" + body}
		}
		return answers
	}
	before := ask()
	for name, answer := range before {
		if answer.status != http.StatusNotFound {
			t.Fatalf("%s, before anyone opened the title: %d %q", name, answer.status, answer.body)
		}
	}
	// The member opens the title's page, then plays it: the file is kept
	// for them, and served to them.
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, nil); status != http.StatusOK {
		t.Fatalf("the member's details: %d", status)
	}
	p.playbackInfo(t, "tv", p.token, p.movie, nil)
	if got := askSigned(t, p.testServer, file, "ApiKey", p.token); got.status != http.StatusOK || !strings.Contains(got.body, "Bonjour") {
		t.Fatalf("the member's own request: %d %q", got.status, got.body)
	}
	for name, answer := range ask() {
		if answer != before[name] {
			t.Errorf("%s, once the title was opened: %d %q; before, %d %q", name, answer.status, answer.body, before[name].status, before[name].body)
		}
	}
}

// The address PlaybackInfo gives for a subtitle file names the device that
// asked. A player that follows it gets the file, an addon's as one a
// member added: as it is, with the format swapped as jellyfin-web does to
// draw the text itself, or with the token in the header instead, as apps
// that sign every request do. The address without what names the caller
// gets nothing.
func TestPlayersGetSubtitleFilesAtTheAddressPlaybackInfoGave(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.SubtitleManagement = new(true) })
	added := map[string]any{"Language": "eng", "Format": "SRT", "IsForced": false, "IsHearingImpaired": false,
		"Data": base64.StdEncoding.EncodeToString([]byte("1\r\n00:00:01,000 --> 00:00:03,000\r\nHello there\r\n"))}
	if status, body := p.call(http.MethodPost, "/Videos/"+p.movie+"/Subtitles", app("tv", p.token), added); status != http.StatusNoContent {
		t.Fatalf("adding a subtitle file: %d %s", status, body)
	}
	answer := p.playbackInfo(t, "tv", p.token, p.movie, map[string]any{"UserId": p.user.ID.String(),
		"MediaSourceId": p.versions[1].ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	if len(answer.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	texts := map[string]string{"eng": "Hello there", "fra": "Bonjour"}
	var served []string
	for _, stream := range answer.MediaSources[0].MediaStreams {
		text, wanted := texts[stream.Language]
		if !stream.IsExternal || !wanted {
			continue
		}
		if stream.DeliveryMethod != "External" || !strings.Contains(stream.DeliveryUrl, "/Stream.vtt?") {
			t.Fatalf("the %s file's delivery: %s %s", stream.Language, stream.DeliveryMethod, stream.DeliveryUrl)
		}
		served = append(served, stream.Language)
		if status, _, body := fetchText(t, p.url+stream.DeliveryUrl); status != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") || !strings.Contains(body, text) {
			t.Errorf("the %s file at the address given: %d %q", stream.Language, status, body)
		}
		if status, _, body := fetchText(t, p.url+strings.Replace(stream.DeliveryUrl, ".vtt", ".js", 1)); status != http.StatusOK || !strings.Contains(body, `"Text":"`+text+`"`) {
			t.Errorf("the %s file as jellyfin-web reads it: %d %q", stream.Language, status, body)
		}
		bare, _, _ := strings.Cut(stream.DeliveryUrl, "?")
		if response, body := fetchURL(t, p.url+bare, http.Header{"Authorization": {app("tv", p.token)}}); response.StatusCode != http.StatusOK || !strings.Contains(body, text) {
			t.Errorf("the %s file with the token in the header: %d %q", stream.Language, response.StatusCode, body)
		}
		if status, _, body := fetchText(t, p.url+bare); status != http.StatusNotFound || body != "Error processing request." {
			t.Errorf("the %s file without what names the caller: %d %q", stream.Language, status, body)
		}
	}
	if strings.Join(served, " ") != "eng fra" {
		t.Errorf("subtitle files offered: %v; want the member's, then the addon's", served)
	}
}
