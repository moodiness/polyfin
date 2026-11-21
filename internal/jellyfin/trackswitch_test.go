package jellyfin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// A device playing a title asks PlaybackInfo again with the title's own
// identifier and the track it switches to, as jellyfin-web does: the play
// goes on from the version it plays, with that track, even when the
// title's first version is another by then, here pinned meanwhile.
// Another device, which plays nothing, gets the first version.
func TestATrackSwitchStaysOnTheVersionPlayed(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.waitForEveryVersion()
	p.remuxable(t, media.Stream{Index: 2, Type: "audio", Codec: "opus", Channels: 1, SampleRate: 8000, ChannelLayout: "mono", Language: "fre"})
	profile := p.profile(t, "jellyfin-web-chrome")
	played := p.versions[0]
	started := p.ask(t, p.token, p.movie, profile, map[string]any{"MediaSourceId": played.ID.String()})
	source := firstSource(t, started)
	// The track switched to: the version's second audio track.
	switched := -1
	for _, stream := range source.MediaStreams {
		if stream.Type == "Audio" && (source.DefaultAudioStreamIndex == nil || stream.Index != *source.DefaultAudioStreamIndex) {
			switched = stream.Index
		}
	}
	if switched < 0 {
		t.Fatalf("the version played has no second audio track: %+v", source.MediaStreams)
	}
	// The device plays the version as the title, its play session naming
	// the version.
	if status, body := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token), map[string]any{"ItemId": p.movie,
		"MediaSourceId": p.movie, "PlaySessionId": started.PlaySessionId, "PositionTicks": 0}); status != http.StatusNoContent {
		t.Fatalf("start report: %d %s", status, body)
	}
	movie, _ := accounts.ParseID(p.movie)
	if err := p.handler.PinVersion(t.Context(), p.user, p.user, movie, p.versions[1].ID); err != nil {
		t.Fatal(err)
	}
	asked := map[string]any{"MediaSourceId": p.movie, "AudioStreamIndex": switched}
	again := firstSource(t, p.ask(t, p.token, p.movie, profile, asked))
	if again.ETag != played.ID.String() || again.DefaultAudioStreamIndex == nil || *again.DefaultAudioStreamIndex != switched {
		t.Errorf("the track switch played %s with audio %v, want %s with %d", again.ETag, deref(again.DefaultAudioStreamIndex), played.ID, switched)
	}
	phone := p.signIn("member", "phone")
	if other := firstSource(t, p.ask(t, phone, p.movie, profile, map[string]any{"MediaSourceId": p.movie})); other.ETag != p.versions[1].ID.String() {
		t.Errorf("another device played %s, want the pinned %s", other.ETag, p.versions[1].ID)
	}
}
