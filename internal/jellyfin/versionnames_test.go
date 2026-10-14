package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Version names end with how the device asking plays each version known:
// PlaybackInfo names every version it lists for the profile sent; item
// details name them for the device's last PlaybackInfo, and not for a
// device that never played. Ids and ETags stay. The names follow the
// server language, conversions the user may not have, and the setting.
func TestVersionNamesSayHowTheyPlay(t *testing.T) {
	t.Parallel()
	p := playing(t)
	// The first version plays with its AC3 audio converted for Chrome; the
	// second, H.264 and AAC in MP4, as it is.
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	userItem := "/Users/" + p.user.ID.String() + "/Items/" + p.movie
	names := func(sources []MediaSourceInfo) []string {
		result := make([]string, len(sources))
		for i, source := range sources {
			result[i] = source.Name
		}
		return result
	}
	details := func(what, token string, want ...string) []MediaSourceInfo {
		t.Helper()
		var dto BaseItemDto
		if status := p.get(t, userItem, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("%s: %d", what, status)
		}
		if got := names(*dto.MediaSources); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
		return *dto.MediaSources
	}
	first, second := "Source 2160p · REMUX · HEVC", "Source 1080p · WEB-DL"
	before := details("before any play", p.token, first, second)

	chrome := p.profile(t, "jellyfin-web-chrome")
	answer := p.ask(t, p.token, p.movie, chrome, nil)
	if got := names(answer.MediaSources); !slices.Equal(got, []string{first + " (converted: audio)", second + " (direct play)"}) {
		t.Errorf("PlaybackInfo: %q", got)
	}
	after := details("after a play", p.token, first+" (converted: audio)", second+" (direct play)")
	for i := range after {
		if after[i].Id != before[i].Id || after[i].ETag != before[i].ETag {
			t.Errorf("source %d: %s %s, before %s %s", i, after[i].Id, after[i].ETag, before[i].Id, before[i].ETag)
		}
	}
	phone := p.signIn("member", "phone")
	details("another device", phone, first, second)

	p.setting(t, func(s *accounts.Settings) { s.Language = "fr" })
	details("in French", p.token, first+" (converti\u00a0: audio)", second+" (lecture directe)")
	p.setting(t, func(s *accounts.Settings) { s.Language, s.Transcoding = "en", false })
	details("without conversion", p.token, first+" (cannot play here)", second+" (direct play)")
	p.setting(t, func(s *accounts.Settings) { s.Transcoding, s.NameHowVersionsPlay = true, false })
	details("the setting off", p.token, first, second)
	if answer := p.ask(t, p.token, p.movie, chrome, nil); !slices.Equal(names(answer.MediaSources), []string{first, second}) {
		t.Errorf("PlaybackInfo with the setting off: %q", names(answer.MediaSources))
	}

	// Taller than the user's quality group, a 1080p version is converted
	// down to it.
	needsEncoders(t)
	q := playing(t)
	q.sized(t, 1, 1920, 1080)
	q.indexed(t)
	q.group(t, q.user, 720)
	q.ask(t, q.token, q.movie, chrome, nil)
	var dto BaseItemDto
	q.get(t, "/Users/"+q.user.ID.String()+"/Items/"+q.movie, q.token, &dto)
	at := slices.IndexFunc(*dto.MediaSources, func(s MediaSourceInfo) bool { return s.ETag == q.versions[1].ID.String() })
	if at < 0 || (*dto.MediaSources)[at].Name != second+" (converted: quality limit)" {
		t.Errorf("over the quality group: %q", names(*dto.MediaSources))
	}
}
