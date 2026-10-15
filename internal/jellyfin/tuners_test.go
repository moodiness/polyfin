package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/iptv/tunertest"
)

// tunerGuide serves an XMLTV guide whose channels are named by the tuner's
// numbers: "5.1" alone, and "7.2" before a name.
func tunerGuide(t *testing.T) string {
	t.Helper()
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `<tv><channel id="g1"><display-name>5.1</display-name></channel>
			<channel id="g2"><display-name>7.2 Heron Hills</display-name></channel>
			<programme channel="g1" start="%[1]s" stop="%[2]s"><title>Morning Ridge</title></programme>
			<programme channel="g2" start="%[1]s" stop="%[2]s"><title>Evening Lark</title></programme></tv>`, at(-time.Hour), at(time.Hour))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/guide.xml"
}

// Jellyfin's tuner routes show the server's HDHomeRun tuners: the type is
// listed, the tuners of the local network are found, an administrator adds
// one found and one by address, which list their channels but the
// protected ones, with the guide's channels named by their numbers, and
// /System/Configuration/livetv lists them. A member adds none.
func TestTunersAreFoundAddedAndListed(t *testing.T) {
	t.Parallel()
	s, admin, member := administrated(t)
	found := tunertest.Start(t, tunertest.Options{DeviceID: 0x1234ABCD, Name: "Attic Tuner", Model: "HDTC-2US", Tuners: 2,
		Channels: []tunertest.Channel{{Number: "5.1", Name: "KSTL"}, {Number: "7.2", Name: "HRNH"}, {Number: "9.1", Name: "Locked", DRM: true}}})
	typed := tunertest.Start(t, tunertest.Options{DeviceID: 0x5678EF01, Name: "Den Tuner", Model: "HDHR5-4US", Tuners: 4,
		Channels: []tunertest.Channel{{Number: "11.3", Name: "OWLT", Favorite: true}, {Number: "12.1", Name: "Plain"}}})
	// Discovery asks the tuner found, never the local network.
	s.iptv.UseTunerDiscovery(0, found.Discovery)

	status, body := s.call(http.MethodGet, "/LiveTv/TunerHosts/Types", member, nil)
	if status != http.StatusOK || string(body) != `[{"Name":"HD Homerun","Id":"hdhomerun"}]`+"\n" {
		t.Errorf("tuner types: %d %s", status, body)
	}
	matchesFixture(t, "tuner-host-types", body, shapeRules{})

	var hosts []TunerHostInfo
	status, body = s.call(http.MethodGet, "/LiveTv/Tuners/Discvover?newDevicesOnly=true", admin, nil)
	if err := json.Unmarshal(body, &hosts); status != http.StatusOK || err != nil || len(hosts) != 1 {
		t.Fatalf("tuners found: %d %s", status, body)
	}
	if h := hosts[0]; h.Id != nil || h.Type != "hdhomerun" || h.Url != found.URL || h.DeviceId != "1234ABCD" || h.FriendlyName != "Attic Tuner" ||
		h.TunerCount != 2 || !h.AllowStreamSharing {
		t.Errorf("tuner found: %s", body)
	}
	if status, body := s.call(http.MethodGet, "/LiveTv/Tuners/Discover", member, nil); status != http.StatusForbidden {
		t.Errorf("tuners found for a member: %d %s", status, body)
	}

	// A member adds no tuner; an administrator adds the one found.
	host := map[string]any{"Type": "hdhomerun", "Url": hosts[0].Url, "DeviceId": hosts[0].DeviceId}
	if status, body := s.call(http.MethodPost, "/LiveTv/TunerHosts", member, host); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("a member's tuner: %d %s", status, body)
	}
	var added TunerHostInfo
	status, body = s.call(http.MethodPost, "/LiveTv/TunerHosts", admin, host)
	if err := json.Unmarshal(body, &added); status != http.StatusOK || err != nil || added.Id == nil || added.FriendlyName != "Attic Tuner" ||
		added.TunerCount != 2 || added.DeviceId != "1234ABCD" {
		t.Fatalf("tuner added: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodPost, "/LiveTv/TunerHosts", admin, host); status != http.StatusBadRequest {
		t.Errorf("the same tuner added twice: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/LiveTv/TunerHosts", admin, map[string]string{"Type": "m3u", "Url": found.URL}); status != http.StatusNotFound {
		t.Errorf("another type of tuner: %d", status)
	}
	if status, body := s.call(http.MethodGet, "/LiveTv/Tuners/Discover?newDevicesOnly=true", admin, nil); status != http.StatusOK || string(body) != "[]\n" {
		t.Errorf("new tuners found once added: %d %s", status, body)
	}
	// The other, by its address, with its favorite channels only, and the
	// guide named by numbers.
	address := strings.TrimPrefix(typed.URL, "http://")
	favorites := true
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Account: iptv.Account{Kind: addons.KindHDHomeRun, URL: address},
		Options: &iptv.OptionsPatch{FavoritesOnly: &favorites}}, false); err != nil {
		t.Fatal(err)
	}
	foundID := *added.Id
	id, err := accounts.ParseID(foundID)
	if err != nil {
		t.Fatal(err)
	}
	key := addons.LibraryKey{AddonID: id, CatalogType: "tv", CatalogID: "channels"}
	if _, err := s.addons.SetGuide(t.Context(), addons.Shared(), key, tunerGuide(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}

	var options LiveTvOptions
	status, body = s.call(http.MethodGet, "/System/Configuration/livetv", admin, nil)
	if err := json.Unmarshal(body, &options); status != http.StatusOK || err != nil || len(options.TunerHosts) != 2 {
		t.Fatalf("Live TV configuration: %d %s", status, body)
	}
	byURL := map[string]TunerHostInfo{}
	for _, h := range options.TunerHosts {
		byURL[h.Url] = h
	}
	if h := byURL[found.URL]; h.Id == nil || *h.Id != foundID || h.ImportFavoritesOnly {
		t.Errorf("the tuner found: %+v", h)
	}
	if h := byURL[typed.URL]; h.FriendlyName != "Den Tuner" || h.TunerCount != 4 || !h.ImportFavoritesOnly {
		t.Errorf("the tuner added by address: %+v", h)
	}
	if status, body := s.call(http.MethodGet, "/System/Configuration/livetv", member, nil); status != http.StatusOK ||
		!strings.Contains(string(body), `"TunerHosts":[]`) {
		t.Errorf("Live TV configuration for a member: %d %s", status, body)
	}

	// The tuners' channels are listed with the others: protected ones and
	// those not favorite left out, each guide channel on its number.
	var channels QueryResult
	_, body = s.call(http.MethodGet, "/LiveTv/Channels", member, nil)
	if err := json.Unmarshal(body, &channels); err != nil {
		t.Fatal(err)
	}
	var listed []string
	programmes := map[string]string{}
	for _, c := range channels.Items {
		listed = append(listed, c.Number+" "+c.Name)
		if c.CurrentProgram != nil {
			programmes[c.Number] = c.CurrentProgram.Name
		}
	}
	slices.Sort(listed)
	if !slices.Equal(listed, []string{"11.3 OWLT", "5.1 KSTL", "7.2 HRNH"}) {
		t.Errorf("channels: %q", listed)
	}
	if programmes["5.1"] != "Morning Ridge" || programmes["7.2"] != "Evening Lark" {
		t.Errorf("guide channels by number: %v", programmes)
	}
	// Each tuner plays as many channels as it has tuners.
	if limit, err := s.iptv.Connections(t.Context(), id); err != nil || limit != (iptv.Limit{Connections: 2, Tuners: true}) {
		t.Errorf("the tuner's limit: %+v %v", limit, err)
	}

	// Removed, it is no longer listed.
	if status, _ := s.call(http.MethodDelete, "/LiveTv/TunerHosts?id="+foundID, member, nil); status != http.StatusForbidden {
		t.Errorf("removed by a member: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/LiveTv/TunerHosts?id="+foundID, admin, nil); status != http.StatusNoContent {
		t.Errorf("removed: %d", status)
	}
	_, body = s.call(http.MethodGet, "/System/Configuration/livetv", admin, nil)
	if err := json.Unmarshal(body, &options); err != nil || len(options.TunerHosts) != 1 || options.TunerHosts[0].Url != typed.URL {
		t.Errorf("after the removal: %s", body)
	}
}

// A recording that takes the tuner a viewer's channel played on refuses
// the viewer's stream, a normal event: the viewer's app hears a 503, which
// tells it to ask again later, and nothing is logged as a failure.
func TestARecordingTakingTheViewersTunerAnswers503(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(ffprobe); err != nil {
		t.Skip("no ffprobe next to POLYFIN_TEST_FFMPEG")
	}
	// The tuner starts first, to stop last: after the streams read from it.
	tuner := tunertest.Start(t, tunertest.Options{DeviceID: 0x2468ACE0, Name: "Porch Tuner", Model: "HDHR5-2US", Tuners: 1,
		Channels: []tunertest.Channel{{Number: "5.1", Name: "KSTL"}, {Number: "7.2", Name: "HRNH"}}})
	s := newProbingServer(t, 10, ffprobe)
	s.handler.Playback.LiveSources(s.iptv.Connections, nil)
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{
		Account: iptv.Account{Kind: addons.KindHDHomeRun, URL: strings.TrimPrefix(tuner.URL, "http://")}}, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	byNumber := map[string]string{}
	for _, c := range channels.Items {
		byNumber[c.Number] = c.Id
	}
	if len(byNumber) != 2 {
		t.Fatalf("channels: %v", byNumber)
	}

	// The viewer watches 5.1, converted by FFmpeg.
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "swiftfin-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Items/"+byNumber["5.1"]+"/PlaybackInfo", app("tv", token),
		map[string]any{"DeviceProfile": json.RawMessage(profile), "IsPlayback": true, "AutoOpenLiveStream": true})
	var info playbackInfoResponse
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 || info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo of the channel: %d %s", status, body)
	}
	transcoding := info.MediaSources[0].TranscodingUrl
	playlist := transcoding[:strings.Index(transcoding, "master.m3u8")] + "live.m3u8" + transcoding[strings.Index(transcoding, "?"):]
	if status, body := s.call(http.MethodGet, transcoding, "", nil); status != http.StatusOK {
		t.Fatalf("master playlist: %d %s", status, body)
	}
	if status, body := s.call(http.MethodGet, playlist, "", nil); status != http.StatusOK || !strings.Contains(string(body), "#EXTINF") {
		t.Fatalf("live playlist: %d %s", status, body)
	}

	// A recording of 7.2 takes the only tuner.
	id, _ := accounts.ParseID(byNumber["7.2"])
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("the recorded channel's streams: %v %v", versions, err)
	}
	ctx, stop := context.WithCancel(t.Context())
	recorded := make(chan error, 1)
	go func() {
		recorded <- s.handler.Playback.Record(ctx, user.ID, versions[0], "porch", filepath.Join(t.TempDir(), "recording.ts"))
	}()
	defer func() {
		stop()
		<-recorded
	}()

	// The viewer's app keeps asking for its playlist: once the segments
	// FFmpeg made before are no longer listed, it hears the refusal.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		status, body = s.call(http.MethodGet, playlist, "", nil)
		if status != http.StatusOK || time.Now().After(deadline) {
			break
		}
		select {
		case err := <-recorded:
			recorded <- err
			t.Fatalf("the recording ended: %v", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
	if status != http.StatusServiceUnavailable {
		t.Errorf("the playlist once the tuner was taken: %d %s, want 503", status, body)
	}
	if logged := s.warnings.logged(); len(logged) > 0 {
		t.Errorf("warnings logged for a refused stream: %q", logged)
	}
}
