package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/media"
)

// The audio track that plays is checked against the app's profile even
// when the app names none and no track is flagged default (#856):
// jellyfin-web names none for a torrent version whose page lists no
// tracks yet, and an AC-3 track Chrome cannot decode played as it is
// gave a picture without sound. An AAC track Chrome plays still plays
// as it is; a flagged track, or one the app names, is checked as before.
func TestTheAudioThatPlaysIsCheckedWhenTheAppNamesNone(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		codec   string
		flagged bool
		named   bool
		direct  bool
	}{
		{"unflagged AC-3", "ac3", false, false, false},
		{"unflagged AAC", "aac", false, false, true},
		{"flagged AC-3", "ac3", true, false, false},
		{"flagged AAC", "aac", true, false, true},
		{"named AC-3", "ac3", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p := playing(t)
			analysis := p.remuxable(t)
			analysis.Format = "matroska,webm"
			analysis.Streams = []media.Stream{
				{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1920, Height: 1080, FrameRate: 24, AverageRate: 24,
					PixelFormat: "yuv420p", BitDepth: 8},
				{Index: 1, Type: "audio", Codec: test.codec, Default: test.flagged, Channels: 6, SampleRate: 48000, Bitrate: 448_000},
			}
			data, _ := json.Marshal(analysis)
			if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[0].ID, data); err != nil {
				t.Fatal(err)
			}
			ask := func(source string, index *int) MediaSourceInfo {
				body := map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": source, "MaxStreamingBitrate": 120_000_000,
					"DeviceProfile": p.profile(t, "jellyfin-web-chrome")}
				if index != nil {
					body["AudioStreamIndex"] = *index
				}
				status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
				var response playbackInfoResponse
				if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
					t.Fatalf("%d %s", status, data)
				}
				return response.MediaSources[0]
			}
			source := ask(p.movie, nil)
			if test.named {
				// An index counts only with the version the app names; the
				// subtitle file beside it is numbered first.
				for _, stream := range source.MediaStreams {
					if stream.Type == "Audio" {
						source = ask(source.Id, new(stream.Index))
						break
					}
				}
			}
			if source.SupportsDirectPlay != test.direct {
				t.Fatalf("direct play %v, want %v: %+v", source.SupportsDirectPlay, test.direct, source)
			}
			if test.direct {
				return
			}
			if source.TranscodingSubProtocol != "hls" || !strings.Contains(transcodeReasons(t, source.TranscodingUrl), "AudioCodecNotSupported") ||
				strings.HasSuffix(source.Name, "(direct play)") {
				t.Errorf("source: %+v", source)
			}
		})
	}
}

// A channel whose audio track is not flagged default has it checked too:
// no app names a channel's track, and an AC-3 channel played as it is in
// Chrome had no sound.
func TestAChannelsUnflaggedAudioIsChecked(t *testing.T) {
	t.Parallel()
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Guide: provider.url + "/epg.xml",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i, codec := range []string{"ac3", "aac"} {
		channel := channels.Items[i].Id
		id, _ := accounts.ParseID(channel)
		versions, err := s.library.Versions(t.Context(), user, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("channel streams: %v %v", versions, err)
		}
		analysis := media.Analysis{Format: "hls", Remote: true, Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1280, Height: 720, FrameRate: 25, AverageRate: 25,
				PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: codec, Channels: 2, SampleRate: 48000},
		}}
		data, _ := json.Marshal(analysis)
		if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[0].ID, data); err != nil {
			t.Fatal(err)
		}
		status, body := s.call(http.MethodPost, "/Items/"+channel+"/PlaybackInfo", app("tv", token),
			map[string]any{"DeviceProfile": json.RawMessage(profile), "IsPlayback": true, "AutoOpenLiveStream": true})
		var info playbackInfoResponse
		if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo of an %s channel: %d %s", codec, status, body)
		}
		source := info.MediaSources[0]
		if codec == "aac" {
			if !source.SupportsDirectPlay {
				t.Errorf("an AAC channel: %+v", source)
			}
			continue
		}
		if source.SupportsDirectPlay || !strings.Contains(transcodeReasons(t, source.TranscodingUrl), "AudioCodecNotSupported") {
			t.Errorf("an AC-3 channel: %+v", source)
		}
	}
}
