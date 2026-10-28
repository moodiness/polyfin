package jellyfin

import (
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// A version jellyfin-web plays as it is says SupportsTranscoding, which
// shows its Quality menu, as Jellyfin 12.4 says it, with no
// TranscodingUrl, once its keyframe index is known, as preparing the play
// reads it: the play does not wait to read it. Picking a lower quality
// asks again with that bitrate, and the version then plays converted down
// to it. A version that could not be converted over HLS, as one without a
// keyframe index, or a user who may not have video converted, keeps the
// menu off.
func TestAVersionPlayedAsItIsOffersALowerQuality(t *testing.T) {
	t.Parallel()
	p := playing(t)
	analysis := p.remuxable(t)
	analysis.Bitrate = 8_000_000
	p.storeAnalysis(t, p.versions[0], analysis)
	needsEncoders(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	ask := func(version string, more map[string]any) MediaSourceInfo {
		t.Helper()
		body := map[string]any{"MediaSourceId": version}
		maps.Copy(body, more)
		answer := p.ask(t, p.token, p.movie, chrome, body)
		if len(answer.MediaSources) != 1 {
			t.Fatalf("answer: %+v", answer)
		}
		return answer.MediaSources[0]
	}
	if unprepared := ask(p.movie, nil); !unprepared.SupportsDirectPlay || unprepared.SupportsTranscoding {
		t.Fatalf("played as it is before its index was read: %+v", unprepared)
	}
	for _, version := range p.versions {
		_, _ = p.handler.Playback.Plan(t.Context(), version)
	}
	played := ask(p.movie, nil)
	if !played.SupportsDirectPlay || !played.SupportsTranscoding || played.TranscodingUrl != "" {
		t.Fatalf("played as it is: %+v", played)
	}
	lower := ask(p.movie, map[string]any{"MaxStreamingBitrate": 3_000_000})
	if lower.SupportsDirectPlay || !lower.SupportsTranscoding || !strings.Contains(lower.TranscodingUrl, "ContainerBitrateExceedsLimit") ||
		!strings.HasSuffix(lower.TranscodingUrl, "&allowVideoStreamCopy=false") {
		t.Fatalf("at a lower quality: %+v", lower)
	}
	if status, _, master := fetchText(t, p.url+lower.TranscodingUrl); status != http.StatusOK || !strings.Contains(master, "avc1.") {
		t.Errorf("master playlist at a lower quality: %d %s", status, master)
	}
	// The second version's file has no keyframe index: it plays as it is,
	// but could not be cut into HLS segments to be converted.
	if unindexed := ask(p.versions[1].ID.String(), nil); !unindexed.SupportsDirectPlay || unindexed.SupportsTranscoding {
		t.Errorf("a version without a keyframe index: %+v", unindexed)
	}
	p.permit(t, p.user, false, true, true)
	if refused := ask(p.movie, nil); !refused.SupportsDirectPlay || refused.SupportsTranscoding {
		t.Errorf("for a user who may not have video converted: %+v", refused)
	}
}

// A channel's stream jellyfin-web plays as it is offers a lower quality
// too, which converts it, though its bitrate is not known, as a live HLS
// stream's is not: it counts as the usual bitrate of its height.
func TestALiveStreamPlayedAsItIsOffersALowerQuality(t *testing.T) {
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
	analysis.Format, analysis.Duration, analysis.Remote, analysis.Bitrate = "hls", 0, true, 0
	for i := range analysis.Streams {
		analysis.Streams[i].Bitrate = 0
	}
	s.storeAnalysis(t, versions[0], analysis)
	needsEncoders(t)
	chrome, err := os.ReadFile(playbackFixtures + "/profiles/jellyfin-web-chrome.json")
	if err != nil {
		t.Fatal(err)
	}
	played := s.ask(t, token, channel, chrome, nil)
	if len(played.MediaSources) != 1 || !played.MediaSources[0].SupportsDirectPlay || !played.MediaSources[0].SupportsTranscoding ||
		played.MediaSources[0].TranscodingUrl != "" {
		t.Fatalf("played as it is: %+v", played)
	}
	// The stream is 360p, counted as 1 Mbit/s: within 500 kbit/s, its
	// video is converted.
	lower := s.ask(t, token, channel, chrome, map[string]any{"MaxStreamingBitrate": 500_000})
	if len(lower.MediaSources) != 1 || lower.MediaSources[0].SupportsDirectPlay ||
		!strings.Contains(lower.MediaSources[0].TranscodingUrl, "&TranscodeReasons=ContainerBitrateExceedsLimit&allowVideoStreamCopy=false") {
		t.Fatalf("at a lower quality: %+v", lower)
	}
	s.limit(t, user, func(c *accounts.UserChanges) { c.VideoTranscoding = new(false) })
	if refused := s.ask(t, token, channel, chrome, nil); len(refused.MediaSources) != 1 || refused.MediaSources[0].SupportsTranscoding {
		t.Errorf("for a user who may not have video converted: %+v", refused)
	}
}
