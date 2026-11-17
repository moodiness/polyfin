package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// entries is a JSON array of n copies of entry.
func entries(entry string, n int) string {
	return "[" + strings.TrimSuffix(strings.Repeat(entry+",", n), ",") + "]"
}

// A device profile's lists are read to their bounds and one entry more,
// which tells them over it, the entries past it skipped; within their
// bounds, they are read whole, by Jellyfin's rules, an empty one staying
// empty.
func TestProfileListsAreReadToTheirBounds(t *testing.T) {
	t.Parallel()
	read := func(profile string) *playback.DeviceProfile {
		t.Helper()
		var request playbackInfoRequest
		if err := unmarshalBody([]byte(`{"DeviceProfile":`+profile+`}`), &request); err != nil || request.DeviceProfile == nil {
			t.Fatalf("%.80s: %v", profile, err)
		}
		return request.DeviceProfile
	}
	bound, conditions := playback.MaxProfileEntries, playback.MaxProfileConditions
	within := read(`{"DirectPlayProfiles":` + entries("{}", bound) + `,"TranscodingProfiles":` + entries(`{"Conditions":`+entries("{}", conditions)+`}`, 2) +
		`,"ContainerProfiles":` + entries("{}", bound) + `,"CodecProfiles":` + entries(`{"ApplyConditions":`+entries("{}", conditions)+`}`, bound) +
		`,"subtitleprofiles":` + entries(`{"Format":2}`, bound) + `}`)
	if !within.Bounded() || len(within.DirectPlayProfiles) != bound || len(within.TranscodingProfiles[1].Conditions) != conditions ||
		len(within.CodecProfiles) != bound || len(within.CodecProfiles[bound-1].ApplyConditions) != conditions ||
		len(within.SubtitleProfiles) != bound || within.SubtitleProfiles[bound-1].Format != "2" {
		t.Errorf("within the bounds: %v, %d direct play, %d subtitle profiles", within.Bounded(), len(within.DirectPlayProfiles), len(within.SubtitleProfiles))
	}
	if empty := read(`{"CodecProfiles":[],"SubtitleProfiles":null}`); empty.CodecProfiles == nil || len(empty.CodecProfiles) != 0 || empty.SubtitleProfiles != nil {
		t.Errorf("empty lists: %+v", empty)
	}
	for name, tc := range map[string]struct {
		profile string
		length  func(p *playback.DeviceProfile) int
		want    int
	}{
		"direct play": {`{"DirectPlayProfiles":` + entries("{}", 10*bound) + `}`,
			func(p *playback.DeviceProfile) int { return len(p.DirectPlayProfiles) }, bound + 1},
		"transcoding": {`{"TranscodingProfiles":` + entries("{}", bound+1) + `}`,
			func(p *playback.DeviceProfile) int { return len(p.TranscodingProfiles) }, bound + 1},
		"container": {`{"ContainerProfiles":` + entries(`{"Type":"Video"}`, 10*bound) + `}`,
			func(p *playback.DeviceProfile) int { return len(p.ContainerProfiles) }, bound + 1},
		"codec": {`{"CodecProfiles":` + entries("{}", 10*bound) + `}`,
			func(p *playback.DeviceProfile) int { return len(p.CodecProfiles) }, bound + 1},
		"subtitle": {`{"SubtitleProfiles":` + entries(`{"Format":"srt","Method":"External"}`, 10*bound) + `}`,
			func(p *playback.DeviceProfile) int { return len(p.SubtitleProfiles) }, bound + 1},
		"transcoding condition": {`{"TranscodingProfiles":[{"Conditions":` + entries("{}", 10*conditions) + `}]}`,
			func(p *playback.DeviceProfile) int { return len(p.TranscodingProfiles[0].Conditions) }, conditions + 1},
		"container condition": {`{"ContainerProfiles":[{"Conditions":` + entries("{}", conditions+1) + `}]}`,
			func(p *playback.DeviceProfile) int { return len(p.ContainerProfiles[0].Conditions) }, conditions + 1},
		"codec condition": {`{"CodecProfiles":[{"Conditions":` + entries("{}", 10*conditions) + `}]}`,
			func(p *playback.DeviceProfile) int { return len(p.CodecProfiles[0].Conditions) }, conditions + 1},
		"codec apply condition": {`{"CodecProfiles":[{"ApplyConditions":` + entries("{}", 10*conditions) + `}]}`,
			func(p *playback.DeviceProfile) int { return len(p.CodecProfiles[0].ApplyConditions) }, conditions + 1},
	} {
		if over := read(tc.profile); over.Bounded() || tc.length(over) != tc.want {
			t.Errorf("%s over its bound: bounded %v, %d read, want %d", name, over.Bounded(), tc.length(over), tc.want)
		}
	}
}

// The device profiles kept in memory stay within their bound in bytes,
// the least recently used going first.
func TestDevicePlaysStayWithinTheirBytes(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	profile, _ := json.Marshal(playback.DeviceProfile{Name: strings.Repeat("x", accounts.MaxDevicePlayProfile-200)})
	play := storedPlay(accounts.DevicePlay{Profile: profile})
	if play.profile == nil || play.size() < 3*(accounts.MaxDevicePlayProfile-200) {
		t.Fatalf("a large profile: %d bytes", play.size())
	}
	devices := 2 * playsBytes / play.size()
	for i := range devices {
		s.handler.plays.Put(accounts.ID{byte(i), byte(i >> 8), 1}, play)
	}
	if kept, n := s.handler.plays.Bytes(), s.handler.plays.Len(); kept > playsBytes || kept <= playsBytes-play.size() || n >= devices {
		t.Errorf("%d profiles of %d bytes kept %d bytes in %d entries, bound %d", devices, play.size(), kept, n, playsBytes)
	}
	if _, ok := s.handler.plays.Get(accounts.ID{0, 0, 1}); ok {
		t.Error("the least recently used profile is kept")
	}
	if _, ok := s.handler.plays.Get(accounts.ID{byte(devices - 1), byte((devices - 1) >> 8), 1}); !ok {
		t.Error("the last profile is not kept")
	}
}

// memoryPlay is what memory keeps of how a user's device plays.
func (s testServer) memoryPlay(t *testing.T, user, device string) (devicePlay, bool) {
	t.Helper()
	var id accounts.ID
	err := s.pool.QueryRow(t.Context(), "SELECT d.id FROM devices d JOIN users u ON u.id = d.user_id WHERE u.name = $1 AND d.device_id = $2",
		user, device+"-id").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return s.handler.plays.Get(id)
}

// A profile too large to keep, or with a list over its bound, is kept
// neither in memory nor in the database, and the one kept before is
// forgotten: item details then name versions as before any play. The
// PlaybackInfo that sent it answers as for any profile. One at the
// bounds is kept.
func TestProfilesOverTheirBoundsAreNotKept(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	plain := p.detailsNames(t, p.user, p.token, p.movie)
	chrome := p.profile(t, "jellyfin-web-chrome")
	changed := func(change func(profile map[string]any)) json.RawMessage {
		var profile map[string]any
		if err := json.Unmarshal(chrome, &profile); err != nil {
			t.Fatal(err)
		}
		change(profile)
		encoded, _ := json.Marshal(profile)
		return encoded
	}
	subtitles := func(n int) func(map[string]any) {
		return func(profile map[string]any) {
			profile["SubtitleProfiles"] = json.RawMessage(entries(`{"Format":"vtt","Method":"External"}`, n))
		}
	}
	conditions := func(n int) func(map[string]any) {
		return func(profile map[string]any) {
			profile["CodecProfiles"] = json.RawMessage(`[{"Type":"Video","Codec":"h264","Conditions":` +
				entries(`{"Condition":"LessThanEqual","Property":"VideoLevel","Value":"52"}`, n) + `}]`)
		}
	}
	// The profile kept, in memory and in the database.
	kept := func(what string) {
		t.Helper()
		if _, ok := p.keptPlay(t, "member", "tv"); !ok {
			t.Errorf("%s: none kept in the database", what)
		}
		if play, ok := p.memoryPlay(t, "member", "tv"); !ok || play.profile == nil {
			t.Errorf("%s: none kept in memory", what)
		}
	}
	p.ask(t, p.token, p.movie, changed(subtitles(playback.MaxProfileEntries)), nil)
	kept("subtitle profiles at their bound")
	p.ask(t, p.token, p.movie, changed(conditions(playback.MaxProfileConditions)), nil)
	kept("conditions at their bound")
	for name, profile := range map[string]json.RawMessage{
		"too large":                  changed(func(profile map[string]any) { profile["Name"] = strings.Repeat("x", accounts.MaxDevicePlayProfile) }),
		"too many subtitle profiles": changed(subtitles(playback.MaxProfileEntries + 1)),
		"too many conditions":        changed(conditions(10 * playback.MaxProfileConditions)),
	} {
		p.ask(t, p.token, p.movie, chrome, nil)
		kept(name + ", before")
		if answer := p.ask(t, p.token, p.movie, profile, nil); len(answer.MediaSources) != 2 || answer.PlaySessionId == "" {
			t.Errorf("%s: PlaybackInfo answered %+v", name, answer)
		}
		if got := p.detailsNames(t, p.user, p.token, p.movie); !slices.Equal(got, plain) {
			t.Errorf("%s: details name %q, want %q", name, got, plain)
		}
		if kept, ok := p.keptPlay(t, "member", "tv"); ok {
			t.Errorf("%s: kept in the database: %+v", name, kept)
		}
		if play, ok := p.memoryPlay(t, "member", "tv"); ok && play.profile != nil {
			t.Errorf("%s: kept in memory, size %d", name, play.size())
		}
	}
	// A device's capabilities keep none either.
	status, _ := p.call(http.MethodPost, "/Sessions/Capabilities/Full", app("tv", p.token),
		map[string]any{"PlayableMediaTypes": []string{"Video"}, "DeviceProfile": changed(subtitles(10 * playback.MaxProfileEntries))})
	if _, ok := p.keptPlay(t, "member", "tv"); status != http.StatusNoContent || ok {
		t.Errorf("capabilities over a bound: %d, kept %v", status, ok)
	}
	if play, ok := p.memoryPlay(t, "member", "tv"); ok && play.profile != nil {
		t.Error("capabilities over a bound kept in memory")
	}
}
