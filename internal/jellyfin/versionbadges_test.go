package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// badgeTexts are the texts of badges, joined by spaces.
func badgeTexts(badges []library.Badge) string {
	texts := make([]string, len(badges))
	for i, badge := range badges {
		texts[i] = badge.Text
	}
	return strings.Join(texts, " ")
}

// /Polyfin/Items/{itemId}/Badges lists a title's versions as its details
// name their sources, each with the addon's own name and its badges: from
// its labels, or from its analysis once there, in the language asked, the
// server's otherwise. Opened as a version, that version carries the
// identifier asked. Turned off, the badges are still listed, Show telling
// the web player to draw none.
func TestItemBadgesListEachVersion(t *testing.T) {
	t.Parallel()
	p := playing(t)
	badges := func(item, query string) itemBadgesJSON {
		t.Helper()
		var answer itemBadgesJSON
		if status := p.get(t, "/Polyfin/Items/"+item+"/Badges"+query, p.token, &answer); status != http.StatusOK {
			t.Fatalf("badges of %s: %d", item, status)
		}
		return answer
	}
	answer := badges(p.movie, "")
	if !answer.Show || len(answer.Versions) != 2 {
		t.Fatalf("badges: %+v", answer)
	}
	first, second := answer.Versions[0], answer.Versions[1]
	if first.Id != p.movie || first.ETag != p.versions[0].ID.String() || second.Id != p.versions[1].ID.String() ||
		second.ETag != p.versions[1].ID.String() {
		t.Errorf("identifiers: %+v", answer.Versions)
	}
	if first.Name != "Source 2160p · REMUX · HEVC" || second.Name != "Source 1080p · WEB-DL" {
		t.Errorf("names: %q, %q", first.Name, second.Name)
	}
	// The first from its labels; the second from its analysis, a 640×360
	// H.264 video with mono English audio, though its labels say 1080p.
	if got := badgeTexts(first.Badges); got != "4K HEVC" {
		t.Errorf("first version's badges: %q", got)
	}
	if got := badgeTexts(second.Badges); got != "SD H.264 Mono EN" {
		t.Errorf("second version's badges: %q", got)
	}
	if label := second.Badges[3].Label; label != "Audio: English" {
		t.Errorf("label in English: %q", label)
	}
	if label := badges(p.movie, "?language=fr").Versions[1].Badges[3].Label; label != "Audio\u202f: anglais" {
		t.Errorf("label in French: %q", label)
	}
	p.setting(t, func(s *accounts.Settings) { s.Language = "de" })
	if label := badges(p.movie, "").Versions[1].Badges[1].Label; label != "H.264-Video" {
		t.Errorf("label in the server language: %q", label)
	}

	opened := badges(p.versions[1].ID.String(), "")
	if len(opened.Versions) != 2 || opened.Versions[0].Id != p.versions[0].ID.String() || opened.Versions[1].Id != p.versions[1].ID.String() {
		t.Errorf("opened as the second version: %+v", opened.Versions)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionBadges = false })
	if off := badges(p.movie, ""); off.Show || len(off.Versions) != 2 || len(off.Versions[0].Badges) == 0 {
		t.Errorf("turned off: %+v", off)
	}
	if status := p.get(t, "/Polyfin/Items/"+strings.Repeat("0", 32)+"/Badges", p.token, nil); status != http.StatusNotFound {
		t.Errorf("an unknown item: %d", status)
	}
}

// /Polyfin/Playback/Badges tells the badges of the version the caller's
// own device plays: none before it reports a play, nor for another device.
func TestPlayingBadgesFollowTheDevice(t *testing.T) {
	t.Parallel()
	p := playing(t)
	playing := func(token, device string) playingBadgesJSON {
		t.Helper()
		status, body := p.call(http.MethodGet, "/Polyfin/Playback/Badges", app(device, token), nil)
		var answer playingBadgesJSON
		if status != http.StatusOK || json.Unmarshal(body, &answer) != nil {
			t.Fatalf("playing badges from %s: %d %s", device, status, body)
		}
		return answer
	}
	if before := playing(p.token, "tv"); !before.Show || before.ItemId != "" || before.Badges == nil || len(before.Badges) != 0 {
		t.Errorf("before a play: %+v", before)
	}
	body := map[string]any{"ItemId": p.movie, "MediaSourceId": p.versions[1].ID.String(), "PositionTicks": 0, "PlayMethod": "DirectPlay"}
	if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token), body); status != http.StatusNoContent {
		t.Fatalf("playing: %d %s", status, data)
	}
	now := playing(p.token, "tv")
	if now.ItemId != p.movie || now.Name != "Source 1080p · WEB-DL" || badgeTexts(now.Badges) != "SD H.264 Mono EN" {
		t.Errorf("while playing: %+v", now)
	}
	phone := p.signIn("member", "phone")
	if other := playing(phone, "phone"); other.ItemId != "" || len(other.Badges) != 0 {
		t.Errorf("another device: %+v", other)
	}
}

// With Version names on Short, details and PlaybackInfo name versions from
// their badges, their size and their addon, in the server language, and
// still end with how they play when that setting is on. Two versions alike
// never share a name. The badges endpoint keeps the addon's names.
func TestShortVersionNames(t *testing.T) {
	t.Parallel()
	p := playing(t)
	userItem := "/Users/" + p.user.ID.String() + "/Items/" + p.movie
	names := func(sources []MediaSourceInfo) []string {
		result := make([]string, len(sources))
		for i, source := range sources {
			result[i] = source.Name
		}
		return result
	}
	details := func(what string, want ...string) {
		t.Helper()
		var dto BaseItemDto
		if status := p.get(t, userItem, p.token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("%s: %d", what, status)
		}
		if got := names(*dto.MediaSources); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
	}
	details("as the addon writes them", "Source 2160p · REMUX · HEVC", "Source 1080p · WEB-DL")
	p.setting(t, func(s *accounts.Settings) { s.VersionNames = "short" })
	// The first from its labels and the addon's size; the second from its
	// analysis.
	second := "SD · H.264 · Mono · EN · 1 MB · Streams"
	details("short", "4K · HEVC · 1 MB · Streams", second)
	p.setting(t, func(s *accounts.Settings) { s.Language = "fr" })
	details("short in French", "4K · HEVC · 1 Mo · Streams", "SD · H.264 · Mono · EN · 1 Mo · Streams")
	p.setting(t, func(s *accounts.Settings) { s.Language = "en" })

	// Analyzed, the first is named anew; it plays with its AC3 audio
	// converted for Chrome.
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	first := "SD · H.264 · 5.1 · EN FR · 2 MB · Streams"
	details("analyzed", first, second)
	chrome := p.profile(t, "jellyfin-web-chrome")
	if got := names(p.ask(t, p.token, p.movie, chrome, nil).MediaSources); !slices.Equal(got, []string{first + " (converted: audio)", second + " (direct play)"}) {
		t.Errorf("PlaybackInfo: %q", got)
	}
	details("after a play", first+" (converted: audio)", second+" (direct play)")
	p.setting(t, func(s *accounts.Settings) { s.NameHowVersionsPlay = false })
	if got := names(p.ask(t, p.token, p.movie, chrome, nil).MediaSources); !slices.Equal(got, []string{first, second}) {
		t.Errorf("PlaybackInfo without how they play: %q", got)
	}
	var answer itemBadgesJSON
	p.get(t, "/Polyfin/Items/"+p.movie+"/Badges", p.token, &answer)
	if len(answer.Versions) != 2 || answer.Versions[0].Name != "Source 2160p · REMUX · HEVC" {
		t.Errorf("the badges' names: %+v", answer.Versions)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionNames = "addon" })
	details("as the addon writes them again", "Source 2160p · REMUX · HEVC", "Source 1080p · WEB-DL")
}

// Two versions whose short names would be the same get different ones:
// the later ends with its rank among them. Details, PlaybackInfo and a
// version opened as an item agree.
func TestShortVersionNamesNeverRepeat(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.analyzed(t, p.versions[0], "h264-aac-mp4")
	p.setting(t, func(s *accounts.Settings) { s.VersionNames, s.NameHowVersionsPlay = "short", false })
	want := []string{"SD · H.264 · Mono · EN · 1 MB · Streams", "SD · H.264 · Mono · EN · 1 MB · Streams · 2"}
	for _, opened := range []string{p.movie, p.versions[1].ID.String()} {
		var dto BaseItemDto
		if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+opened, p.token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("details of %s: %d", opened, status)
		}
		got := []string{(*dto.MediaSources)[0].Name, (*dto.MediaSources)[1].Name}
		if !slices.Equal(got, want) {
			t.Errorf("details of %s: %q, want %q", opened, got, want)
		}
	}
	answer := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil)
	if len(answer.MediaSources) != 2 || answer.MediaSources[0].Name != want[0] || answer.MediaSources[1].Name != want[1] {
		t.Errorf("PlaybackInfo: %+v", answer.MediaSources)
	}
}

// Short names put the audio format with its channels and the languages
// together, then the size and the addon, those unknown left out; sizes,
// stereo and mono read as the server language writes them, and MULTI
// shows only when no language is named.
func TestShortVersionNameParts(t *testing.T) {
	t.Parallel()
	version := library.Version{Height: 2160, Labels: library.ReadLabels("Movie", "4K DV HEVC IMAX 🇫🇷 🇬🇧 Multi")}
	version.Labels.Tier, version.Labels.Audio, version.Labels.Edition = library.ReadRelease([]string{"Movie"}, "TrueHD Atmos 7.1 IMAX")
	if got := shortVersionName(library.VersionBadges(version, "en"), 58_200_000_000, "Addon", "en"); got != "4K · DV · HEVC · Atmos 7.1 · IMAX · FR EN · 58 GB · Addon" {
		t.Errorf("a full name: %q", got)
	}
	stereo := library.Version{Height: 1080, Labels: library.ReadLabels("Movie", "1080p x264 MULTi")}
	stereo.Labels.Tier, stereo.Labels.Audio, stereo.Labels.Edition = library.ReadRelease([]string{"Movie"}, "AAC 2.0")
	for language, want := range map[string]string{"en": "1080p · H.264 · Stereo · MULTI · 4.7 GB · Addon", "fr": "1080p · H.264 · Stéréo · MULTI · 4,7 Go · Addon"} {
		if got := shortVersionName(library.VersionBadges(stereo, language), 4_700_000_000, "Addon", language); got != want {
			t.Errorf("stereo in %s: %q, want %q", language, got, want)
		}
	}
	for _, tc := range []struct {
		size     int64
		language string
		want     string
	}{
		{4_700_000_000, "en", "4.7 GB"},
		{4_700_000_000, "de", "4,7 GB"},
		{4_700_000_000, "fr", "4,7 Go"},
		{700_000_000, "fr", "700 Mo"},
		{12_400_000_000, "it", "12 GB"},
	} {
		if got := sizeName(tc.size, tc.language); got != tc.want {
			t.Errorf("%d in %s: %q, want %q", tc.size, tc.language, got, tc.want)
		}
	}
	if got := shortVersionName(nil, 0, " ", "en"); got != "" {
		t.Errorf("nothing known: %q", got)
	}
}
