package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// soundSource stores for the movie's first version a Matroska file of
// H.264 and AAC in 5.1, which jellyfin-web in Chrome plays as it is, with
// its keyframe index, which a remux needs.
func (p playbackSetup) soundSource(t *testing.T) {
	t.Helper()
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15 * time.Second, Size: 21, Bitrate: 25_000, Remote: true,
		Streams: []media.Stream{
			{Index: 0, Type: "audio", Codec: "aac", Profile: "LC", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1"},
			{Index: 1, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: 64, Height: 64, FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
		}}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	p.keyframed(t, p.versions[0])
}

// withChannels is a recorded device profile whose video transcoding
// profiles take up to channels audio channels.
func withChannels(t *testing.T, profile json.RawMessage, channels string) json.RawMessage {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(profile, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, raw := range decoded["TranscodingProfiles"].([]any) {
		if transcoding := raw.(map[string]any); transcoding["Type"] == "Video" {
			transcoding["MaxAudioChannels"] = channels
		}
	}
	data, _ := json.Marshal(decoded)
	return data
}

// For a user who has their sound converted, a version jellyfin-web plays
// as it is is streamed over HLS instead, its video copied and its audio
// converted with the app's channels, 5.1 included, and its name says so.
// The HLS request evens the audio and limits its peaks whatever the
// server's normalization setting, for that user alone. Others, and a user
// who may not have audio converted, play it as it is.
func TestConvertedSoundCopiesTheVideo(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.soundSource(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	asked := map[string]any{"MediaSourceId": p.versions[0].ID.String()}
	if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, asked)); !source.SupportsDirectPlay || source.TranscodingUrl != "" {
		t.Fatalf("for others: %+v", source)
	}
	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{ConvertSound: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	user, err := p.store.User(t.Context(), p.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	opened, _ := accounts.ParseID(p.movie)
	remux := func(who accounts.User, address string) remuxRequest {
		t.Helper()
		req, ok := p.handler.remuxFor(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, address, nil), who, opened, p.versions[0].ID, "session")
		if !ok {
			t.Fatalf("the HLS request %s was refused", address)
		}
		return req
	}
	const evened = "loudnorm=I=-16:TP=-1.5:LRA=11,aresample=48000,alimiter=limit=0.841395:level=disabled"
	for _, tc := range []struct {
		name     string
		profile  json.RawMessage
		channels int
		filter   string
	}{
		{"stereo", chrome, 2, "volume=2," + evened},
		{"5.1", withChannels(t, chrome, "6"), 6, evened},
	} {
		source := firstSource(t, p.ask(t, p.token, p.movie, tc.profile, asked))
		if source.SupportsDirectPlay || source.TranscodingSubProtocol != "hls" || !strings.Contains(source.TranscodingUrl, "allowAudioStreamCopy=false") ||
			strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
			t.Fatalf("%s: %+v", tc.name, source)
		}
		if !strings.HasSuffix(source.Name, "(converted: audio)") {
			t.Errorf("%s: name %q", tc.name, source.Name)
		}
		played := remux(user, source.TranscodingUrl).remux
		if played.ConvertVideo != nil || played.ConvertAudio == nil || played.ConvertAudio.Codec != "aac" ||
			played.ConvertAudio.Channels != tc.channels || !strings.HasSuffix(played.ConvertAudio.Filter, tc.filter) || !played.Normalize {
			t.Errorf("%s: the HLS request converts %+v %+v", tc.name, played.ConvertVideo, played.ConvertAudio)
		}
		// The same request for a user who has not asked converts the audio
		// as the settings say, without evening it.
		if other := remux(p.user, source.TranscodingUrl).remux; other.ConvertAudio == nil || strings.Contains(other.ConvertAudio.Filter, "loudnorm") || other.Normalize {
			t.Errorf("%s: for another user, %+v", tc.name, other.ConvertAudio)
		}
	}

	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{AudioTranscoding: new(false), VideoTranscoding: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, asked)); !source.SupportsDirectPlay || !strings.HasSuffix(source.Name, "(direct play)") {
		t.Errorf("for a user who may not have audio converted: %+v", source)
	}
}

// Converting the sound never costs a video conversion: with Prefer
// versions the app plays without conversion, a user who has their sound
// converted plays the version whose video is copied, the second, rather
// than the first, whose video the minimal profile takes only converted.
func TestConvertedSoundKeepsTheVersionsWithoutVideoConversion(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts with the encoders FFmpeg has")
	}
	p := playing(t)
	p.analyzed(t, p.versions[0], "av1-opus-webm")
	p.keyframed(t, p.versions[0])
	p.keyframed(t, p.versions[1])
	minimal := p.profile(t, "minimal")
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil)); source.ETag != p.versions[0].ID.String() || !strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("without the setting, the first version plays, its video converted: %+v", source)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil)); source.ETag != p.versions[1].ID.String() || !source.SupportsDirectPlay {
		t.Fatalf("for others: %+v", source)
	}
	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{ConvertSound: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil))
	if source.ETag != p.versions[1].ID.String() || source.SupportsDirectPlay || !strings.Contains(source.TranscodingUrl, "allowAudioStreamCopy=false") ||
		strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Errorf("for the user: %+v", source)
	}
}

// Channels follow: a channel's stream jellyfin-web plays as it is is
// converted into HLS for a user who has their sound converted, its video
// copied and its audio converted.
func TestConvertedSoundReachesChannels(t *testing.T) {
	t.Parallel()
	s, token, user := tuned(t, newTVAddon(t, false, ""))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	id, _ := accounts.ParseID(channel)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("channel streams: %v %v", versions, err)
	}
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Format, analysis.Duration, analysis.Remote = "hls", 0, true
	s.storeAnalysis(t, versions[0], analysis)
	chrome, err := os.ReadFile(playbackFixtures + "/profiles/jellyfin-web-chrome.json")
	if err != nil {
		t.Fatal(err)
	}
	if played := s.ask(t, token, channel, chrome, nil); len(played.MediaSources) != 1 || !played.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("for others: %+v", played)
	}
	s.limit(t, user, func(c *accounts.UserChanges) { c.ConvertSound = new(true) })
	played := s.ask(t, token, channel, chrome, nil)
	if len(played.MediaSources) != 1 || played.MediaSources[0].SupportsDirectPlay ||
		!strings.Contains(played.MediaSources[0].TranscodingUrl, "allowAudioStreamCopy=false") ||
		strings.Contains(played.MediaSources[0].TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Errorf("for the user: %+v", played)
	}
}

// For a user who boosts the volume, the direct plays of a device whose web
// player runs Polyfin's script, which reads the user's sound, are relayed
// and say so in their address: the script then knows it may hook them.
// Other devices, and users who do not boost, keep the usual redirect.
func TestBoostedDirectPlaysAreRelayed(t *testing.T) {
	t.Parallel()
	p := playing(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	asked := map[string]any{"MediaSourceId": p.versions[1].ID.String()}
	path := func(token string) string {
		t.Helper()
		source := firstSource(t, p.ask(t, token, p.movie, chrome, asked))
		if !source.SupportsDirectPlay {
			t.Fatalf("not played as it is: %+v", source)
		}
		return source.Path
	}
	if status := p.get(t, "/Polyfin/Sound", p.token, nil); status != http.StatusOK {
		t.Fatalf("reading the sound: %d", status)
	}
	if got := path(p.token); strings.Contains(got, "PolyfinRelay=") {
		t.Errorf("without the boost: %s", got)
	}
	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{BoostVolume: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if got := path(p.token); !strings.Contains(got, "PolyfinRelay=1") {
		t.Errorf("boosted, on the script's device: %s", got)
	}
	if got := path(p.signIn("member", "phone")); strings.Contains(got, "PolyfinRelay=") {
		t.Errorf("boosted, on a device without the script: %s", got)
	}
}

// Polyfin's script reads the user's sound, and turns Boost the volume on
// and off from the player's menu; a member who follows their group keeps
// the group's.
func TestTheScriptReadsAndBoostsTheSound(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	token := s.signIn("member", "web")
	var sound SoundDto
	if status := s.get(t, "/Polyfin/Sound", token, &sound); status != http.StatusOK || sound != (SoundDto{}) {
		t.Fatalf("a new user's: %d %+v", status, sound)
	}
	status, data := s.call(http.MethodPost, "/Polyfin/Sound", app("web", token), map[string]any{"BoostVolume": true})
	if err := json.Unmarshal(data, &sound); status != http.StatusOK || err != nil || !sound.BoostVolume || sound.ConvertSound {
		t.Fatalf("boosting: %d %s", status, data)
	}
	if saved, err := s.store.User(t.Context(), user.ID); err != nil || !saved.BoostVolume {
		t.Errorf("saved: %+v %v", saved, err)
	}
	if status, data := s.call(http.MethodPost, "/Polyfin/Sound", app("web", token), map[string]any{}); status != http.StatusBadRequest {
		t.Errorf("no choice: %d %s", status, data)
	}

	group, err := s.store.CreateGroup(t.Context(), "Quiet", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.UpdateUser(t.Context(), user.ID, accounts.UserChanges{Group: new(group.ID)}, nil); err != nil {
		t.Fatal(err)
	}
	if status := s.get(t, "/Polyfin/Sound", token, &sound); status != http.StatusOK || sound != (SoundDto{FollowsGroup: true}) {
		t.Errorf("a following member's: %d %+v", status, sound)
	}
	if status, data := s.call(http.MethodPost, "/Polyfin/Sound", app("web", token), map[string]any{"BoostVolume": true}); status != http.StatusConflict {
		t.Errorf("a following member's change: %d %s", status, data)
	}
}
