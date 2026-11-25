package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// The detailed log tells each PlaybackInfo, stream, subtitle and download
// request as the app sent it: its route and values, its query with the
// credentials' values left out, and the PlaybackInfo's body, device
// profile included.
func TestPlaybackRequestsAreLoggedAsSent(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.warnings.all.Store(true)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	item, version := "a1f6d7d2b0f04f2a9d1e3c4b5a697887", "0b2c4d6e8f1a4c3e9b7d5f3a1c2e4d6f"

	profile := map[string]any{"DeviceProfile": map[string]any{"Name": "Test profile",
		"SubtitleProfiles": []map[string]string{{"Format": "srt", "Method": "External"}}}}
	s.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo?UserId=u1&StartTimeTicks=0", app("tv", token), profile)
	s.call(http.MethodGet, "/Videos/"+item+"/stream?Static=true&MediaSourceId="+version+"&api_key="+token+"&PlaySessionId=signed", "", nil)
	s.call(http.MethodGet, "/Videos/"+item+"/"+version+"/Subtitles/3/0/Stream.srt?ApiKey="+token, "", nil)
	s.call(http.MethodGet, "/Items/"+item+"/Download?"+grantParameter+"=granted", "", nil)

	got := s.warnings.attrsOf("An app sent a playback request")
	if len(got) != 4 {
		t.Fatalf("%d lines, want 4: %v", len(got), got)
	}
	want := []map[string]string{
		{"app": "Test App", "app_version": "1.0.0", "route": "/Items/{itemId}/PlaybackInfo", "itemId": item, "query": "StartTimeTicks=0&UserId=u1",
			"body": `{"DeviceProfile":{"Name":"Test profile","SubtitleProfiles":[{"Format":"srt","Method":"External"}]}}`},
		{"app": "", "app_version": "", "route": "/Videos/{itemId}/{file}", "itemId": item, "file": "stream",
			"query": "MediaSourceId=" + version + "&PlaySessionId=sent&Static=true&api_key=sent"},
		{"app": "", "app_version": "", "route": "/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{startTicks}/{file}", "itemId": item,
			"mediaSourceId": version, "index": "3", "startTicks": "0", "file": "Stream.srt", "query": "ApiKey=sent"},
		{"app": "", "app_version": "", "route": "/Items/{itemId}/Download", "itemId": item, "query": grantParameter + "=sent"},
	}
	for i, line := range got {
		for key, value := range want[i] {
			if line[key] != value {
				t.Errorf("line %d: %s=%q, want %q", i, key, line[key], value)
			}
		}
		if len(line) != len(want[i]) {
			t.Errorf("line %d: %v, want only %v", i, line, want[i])
		}
		for key, value := range line {
			if strings.Contains(value, token) || strings.Contains(value, "signed") || strings.Contains(value, "granted") {
				t.Errorf("line %d: %s holds a credential: %q", i, key, value)
			}
		}
	}
}
