package jellyfin

import (
	"net/http"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// A PlaybackInfo that plays nothing tells PlayFailed who tried what, on
// which app and device, and why: no version the app plays, or the user's
// limit of playbacks at once.
func TestFailedPlaysAreTold(t *testing.T) {
	t.Parallel()
	p := playing(t)
	var mu sync.Mutex
	var told []plays.Failure
	p.handler.PlayFailed = func(f plays.Failure) {
		mu.Lock()
		defer mu.Unlock()
		told = append(told, f)
	}
	last := func() plays.Failure {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if len(told) == 0 {
			t.Fatal("no failure was told")
		}
		return told[len(told)-1]
	}

	// A version the title does not have plays nowhere.
	answer := p.playbackInfo(t, "tv", p.token, p.movie, map[string]any{"MediaSourceId": "0123456789abcdef0123456789abcdef"})
	if answer.ErrorCode != "NoCompatibleStream" {
		t.Fatalf("answer: %+v", answer)
	}
	f := last()
	if f.Reason != plays.FailedNoStream || f.User != p.user.ID || f.UserName != "member" || f.App != "Test App" || f.DeviceName != "tv" ||
		f.Title.Item.String() != p.movie || f.Title.Name == "" || f.Title.Kind != plays.Movie {
		t.Errorf("failure: %+v", f)
	}

	// One playback at most, another device playing: the limit refuses it.
	phone := p.signIn("member", "phone")
	body := map[string]any{"ItemId": p.movie, "MediaSourceId": p.versions[1].ID.String(), "PositionTicks": 0, "PlayMethod": "DirectPlay"}
	if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("phone", phone), body); status != http.StatusNoContent {
		t.Fatalf("playing from the phone: %d %s", status, data)
	}
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	if answer := p.playbackInfo(t, "tv", p.token, p.movie, nil); answer.ErrorCode != "RateLimitExceeded" {
		t.Fatalf("answer at the limit: %+v", answer)
	}
	if f := last(); f.Reason != plays.FailedLimit || f.Title.Item.String() != p.movie || f.DeviceName != "tv" {
		t.Errorf("failure at the limit: %+v", f)
	}

	// A play that works tells nothing.
	mu.Lock()
	before := len(told)
	mu.Unlock()
	if answer := p.playbackInfo(t, "phone", phone, p.movie, nil); answer.ErrorCode != "" {
		t.Fatalf("answer on the phone: %+v", answer)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(told) != before {
		t.Errorf("a play that worked was told as failed: %+v", told[before:])
	}
}
