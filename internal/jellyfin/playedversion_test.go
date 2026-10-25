package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// jellyfin-web, playing a title's first version as it is, reports the
// title's own identifier as its media source and no play session. Under a
// 1080p quality group the movie's first version, 4K, is left out, and the
// title's identifier stands for its second, 1080p, which PlaybackInfo
// chose: the dashboard's live sessions and the Jellyfin sessions describe
// that version, its picture and its chapters, not the first nor none.
func TestLiveSessionsDescribeTheVersionChosenForATitlePlayedAsItIs(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.sized(t, 0, 3840, 2160)
	p.sized(t, 1, 1920, 1080)
	// The second version has chapters, the first none.
	var played media.Analysis
	if err := p.pool.QueryRow(t.Context(), "SELECT analysis FROM media_analyses WHERE version_id = $1", p.versions[1].ID).Scan(&played); err != nil {
		t.Fatal(err)
	}
	played.Chapters = []media.Chapter{{Start: 0, End: time.Minute, Title: "Opening"}, {Start: time.Minute, End: 2 * time.Minute, Title: "Harbor"}}
	data, _ := json.Marshal(played)
	if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[1].ID, data); err != nil {
		t.Fatal(err)
	}
	p.handler.Playback.ForgetFile(p.versions[1].ID)
	p.group(t, p.user, 1080)

	source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil))
	if source.Id != p.movie || source.ETag != p.versions[1].ID.String() || !source.SupportsDirectPlay {
		t.Fatalf("PlaybackInfo: %+v", source)
	}
	if status, body := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token), map[string]any{"ItemId": p.movie, "MediaSourceId": p.movie,
		"PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("start: %d %s", status, body)
	}

	sessions, err := p.handler.LiveSessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("live sessions: %+v %v", sessions, err)
	}
	if source := sessions[0].Source; source == nil || videoHeight(*source) != 1080 {
		t.Errorf("the live session describes %+v, want the 1080p version", source)
	}

	var infos []SessionInfo
	if status, body := p.call(http.MethodGet, "/Sessions", app("tv", p.token), nil); status != http.StatusOK || json.Unmarshal(body, &infos) != nil {
		t.Fatalf("sessions: %d %s", status, body)
	}
	for _, info := range infos {
		if info.NowPlayingItem == nil {
			continue
		}
		// The media source is the one the app reported, as in Jellyfin.
		if info.PlayState.MediaSourceId != p.movie {
			t.Errorf("media source %q, want the title's %q", info.PlayState.MediaSourceId, p.movie)
		}
		if chapters := info.NowPlayingItem.Chapters; chapters == nil || len(*chapters) != 2 || (*chapters)[1].Name != "Harbor" {
			t.Errorf("chapters %+v, want the 1080p version's", chapters)
		}
		return
	}
	t.Errorf("no session plays: %+v", infos)
}
