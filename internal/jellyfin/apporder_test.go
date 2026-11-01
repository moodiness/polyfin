package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// eac3 stores for a version the analysis of the Matroska file the
// streaming addon and the folder serve, with E-AC-3 audio flagged default,
// which jellyfin-web converts.
func (s testServer) eac3(t *testing.T, version library.Version) {
	t.Helper()
	info, err := os.Stat(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	s.storeAnalysis(t, version, media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond, Size: info.Size(), Bitrate: 25_000,
		Remote: !version.LocalFolder, Streams: []media.Stream{
			{Index: 0, Type: "audio", Codec: "eac3", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1(side)"},
			{Index: 1, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: 64, Height: 64, FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
		}})
}

// etags names media sources by the version each describes.
func etags(sources []MediaSourceInfo) []string {
	tags := make([]string, len(sources))
	for i, source := range sources {
		tags[i] = source.ETag
	}
	return tags
}

// versionIDs are the identifiers of versions, as etags names them.
func versionIDs(versions ...library.Version) []string {
	ids := make([]string, len(versions))
	for i, version := range versions {
		ids[i] = version.ID.String()
	}
	return ids
}

// detailsOn is the media sources item details list for item on device.
func (p playbackSetup) detailsOn(t *testing.T, device, token, item string) []MediaSourceInfo {
	t.Helper()
	var dto BaseItemDto
	status, body := p.call(http.MethodGet, "/Users/"+p.user.ID.String()+"/Items/"+item, app(device, token), nil)
	if status != http.StatusOK {
		t.Fatalf("details on %s: %d %s", device, status, body)
	}
	if err := json.Unmarshal(body, &dto); err != nil || dto.MediaSources == nil {
		t.Fatalf("details on %s: %v %s", device, err, body)
	}
	return *dto.MediaSources
}

// With Prefer versions the app plays without conversion, item details
// list first, for a device whose profile is known, the version it plays
// without conversion: the second, H.264 and AAC in MP4, before the first,
// whose E-AC-3 audio jellyfin-web converts. Play from the page, which
// sends the title's own identifier, starts it, as does the stream of that
// identifier. Another device of the same user, whose profile is not known,
// keeps the server's order, and each keeps its own order while its page
// is open.
func TestPreferDirectPlayOrdersTheMenuForTheApp(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.eac3(t, p.versions[0])
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	chrome := p.profile(t, "jellyfin-web-12.4-chrome")
	web := p.signIn("member", "web")
	served := versionIDs(p.versions[0], p.versions[1])
	forApp := versionIDs(p.versions[1], p.versions[0])

	// Before the device's first play, it is listed the server's order.
	if got := etags(p.detailsOn(t, "tv", p.token, p.movie)); !slices.Equal(got, served) {
		t.Errorf("before the device's first play: %v, want %v", got, served)
	}
	p.ask(t, p.token, p.movie, chrome, nil)
	sources := p.detailsOn(t, "tv", p.token, p.movie)
	if got := etags(sources); !slices.Equal(got, forApp) {
		t.Errorf("once its profile is known: %v, want %v", got, forApp)
	}
	if sources[0].Id != p.movie || !strings.HasSuffix(sources[0].Name, "(direct play)") || !strings.Contains(sources[1].Name, "(converted") {
		t.Errorf("the menu: %s %q, %q", sources[0].Id, sources[0].Name, sources[1].Name)
	}
	if got := etags(p.detailsOn(t, "web", web, p.movie)); !slices.Equal(got, served) {
		t.Errorf("another device of the user: %v, want %v", got, served)
	}
	if got := etags(p.detailsOn(t, "tv", p.token, p.movie)); !slices.Equal(got, forApp) {
		t.Errorf("the first device again, its page open: %v, want %v", got, forApp)
	}

	// Play from the page names the menu's first version by the title's
	// identifier: it plays, as it is.
	played := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie}))
	if played.ETag != p.versions[1].ID.String() || !played.SupportsDirectPlay {
		t.Errorf("Play from the page: %+v", played)
	}
	// The title's identifier streams that version for the device too: the
	// addon serves its bytes, not the Matroska file of the first.
	response, body := p.fetch(t, "/Videos/"+p.movie+"/stream.mp4?static=true&mediaSourceId="+p.movie+"&api_key="+p.token)
	if response.StatusCode != http.StatusOK || !strings.HasSuffix(string(body), " media bytes") {
		t.Errorf("the title's stream: %d, %d bytes", response.StatusCode, len(body))
	}
}

// For a device whose profile is known, the title's own identifier picks
// the first version of its menu, which plays even when it needs a
// conversion and a version Polyfin had not analyzed would play without:
// the menu showed it first. A device whose profile was not known gets
// today's choice, the version that plays without conversion.
func TestAVersionPickedFromTheMenuPlays(t *testing.T) {
	t.Parallel()
	probe := newFakeProbe(t, true)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	remote := p.versionsOf(t, p.remote)
	p.eac3(t, remote[0])
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	chrome := p.profile(t, "jellyfin-web-12.4-chrome")
	p.ask(t, p.token, p.movie, chrome, nil)

	sources := p.detailsOn(t, "tv", p.token, p.remote)
	if got, want := etags(sources), versionIDs(remote...); !slices.Equal(got, want) {
		t.Fatalf("the menu: %v, want %v", got, want)
	}
	picked := firstSource(t, p.ask(t, p.token, p.remote, chrome, map[string]any{"MediaSourceId": sources[0].Id}))
	if picked.ETag != remote[0].ID.String() || picked.TranscodingUrl == "" {
		t.Errorf("the menu's first version, picked: %+v", picked)
	}
	if other := firstSource(t, p.ask(t, p.token, p.remote, chrome, map[string]any{"MediaSourceId": sources[1].Id})); other.ETag != remote[1].ID.String() {
		t.Errorf("the second version, picked: %+v", other)
	}

	web := p.signIn("member", "web")
	status, body := p.call(http.MethodPost, "/Items/"+p.remote+"/PlaybackInfo", app("web", web),
		map[string]any{"MaxStreamingBitrate": 120_000_000, "DeviceProfile": chrome, "MediaSourceId": p.remote})
	var answer playbackAnswer
	if err := json.Unmarshal(body, &answer); status != http.StatusOK || err != nil {
		t.Fatalf("PlaybackInfo on another device: %d %s", status, body)
	}
	if source := firstSource(t, answer); source.ETag != remote[1].ID.String() || !source.SupportsDirectPlay {
		t.Errorf("a device whose profile was not known: %+v", source)
	}
}

// Play local files first wins over Prefer versions the app plays without
// conversion: for its user, a local file that plays, its audio converted,
// comes first and plays, from a card and from the page, before a stream
// the app plays as it is; the other members get the stream first.
func TestLocalFilesFirstBeatsPreferDirectPlay(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	shelf, stream := p.versions[4], p.versions[0]
	local, localToken := p.another(t, "local", "")
	local, err := p.store.UpdateUser(t.Context(), local.ID, accounts.UserChanges{LocalFirst: new(true)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.eac3(t, shelf)
	p.analyzed(t, stream, "h264-aac-mp4")
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	chrome := p.profile(t, "jellyfin-web-12.4-chrome")

	// A card plays the title without naming a version.
	if source := firstSource(t, p.ask(t, localToken, p.movie, chrome, nil)); source.ETag != shelf.ID.String() || source.TranscodingUrl == "" {
		t.Errorf("a card's play, local files first: %+v", source)
	}
	if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil)); source.ETag != stream.ID.String() || !source.SupportsDirectPlay {
		t.Errorf("a card's play, the server's order: %+v", source)
	}
	// Both devices' profiles are known now.
	sources := p.detailed(t, local, localToken)
	if sources[0].ETag != shelf.ID.String() || sources[1].ETag != stream.ID.String() {
		t.Errorf("the menu, local files first: %v", firstWords(sources))
	}
	if source := firstSource(t, p.ask(t, localToken, p.movie, chrome, map[string]any{"MediaSourceId": sources[0].Id})); source.ETag != shelf.ID.String() {
		t.Errorf("Play from the page, local files first: %+v", source)
	}
	if sources := p.detailed(t, p.user, p.token); sources[0].ETag != stream.ID.String() {
		t.Errorf("the menu, the server's order: %v", firstWords(sources))
	}
}

// An open page keeps its versions in place only while what orders them
// stays the same: the quality group and Play local files first included.
func TestKeptInPlaceFollowsTheQualityGroup(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"source"}
		s.VersionSourceOrder = []accounts.ID{p.shelf.ID, p.one.ID, p.two.ID}
	})
	check := func(name string, want ...string) {
		t.Helper()
		if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	group := func(height int) {
		t.Helper()
		p.limit(t, p.user, func(c *accounts.UserChanges) { c.QualityGroup = new(height) })
		p.user.QualityGroup = height
	}
	check("without a group", "Shelf", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish")
	// Every version is taller than 480p: the closest come first.
	group(480)
	check("a 480p group", "TwoFrench", "TwoEnglish", "Shelf", "OneEnglish", "OneFrench")
	group(0)
	check("the group taken away", "Shelf", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish")

	p.setting(t, func(s *accounts.Settings) { s.VersionSourceOrder = []accounts.ID{p.one.ID, p.two.ID, p.shelf.ID} })
	check("the folder last", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish", "Shelf")
	localFirst := func(on bool) {
		t.Helper()
		if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{LocalFirst: new(on)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	localFirst(true)
	check("local files first", "Shelf", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish")
	localFirst(false)
	check("local files first taken away", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish", "Shelf")
}
