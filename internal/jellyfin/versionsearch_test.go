package jellyfin

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// searchAnswer is the answer of POST /Polyfin/Items/{id}/Versions/Search.
type searchAnswer struct {
	status     int
	progress   VersionProgress
	retryAfter string
}

// search asks for the versions of the item id again, as token.
func (h heldSetup) search(t *testing.T, id, token string) searchAnswer {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+"/Polyfin/Items/"+id+"/Versions/Search", nil)
	if token != "" {
		request.Header.Set("Authorization", app("tv", token))
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	a := searchAnswer{status: response.StatusCode, retryAfter: response.Header.Get("Retry-After")}
	if a.status == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&a.progress); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// A title no addon had a stream for has its addons asked again on demand:
// their stream lists are dropped and they are asked in the background, at
// most once every 20 seconds for a user's title, with the checks of its
// details.
func TestATitlesAddonsAreAskedAgainOnDemand(t *testing.T) {
	addon := newScriptedAddon(t, "Gathering")
	s := newTestServer(t, 10)
	now := time.Now()
	var clock atomic.Pointer[time.Time]
	clock.Store(&now)
	s.handler.now = func() time.Time { return *clock.Load() }
	h := heldOn(t, s, addon.url)

	h.sources(t)
	addon.replies <- 0
	h.reaches(t, VersionProgress{Pending: 0, Count: 1, Known: 0}, "Movie")
	// Opened again, the title's empty list is known: its addon is not asked.
	h.sources(t)
	if got := h.progress(t); got != (VersionProgress{Pending: 0, Count: 1, Known: 0}) || addon.asked.Load() != 1 {
		t.Fatalf("reopened: %+v, addon asked %d times", got, addon.asked.Load())
	}

	// Asked again, the addon is asked anew, and the answer tells it.
	if got := h.search(t, h.movie, h.token); got.status != http.StatusOK || got.progress != (VersionProgress{Pending: 1, Count: 1, Known: 0}) {
		t.Fatalf("asked again: %+v", got)
	}
	eventually(t, "the addon to be asked again", func() bool { return addon.asked.Load() == 2 })
	addon.replies <- 2
	h.reaches(t, VersionProgress{Pending: 0, Count: 2, Known: 2}, "Gathering", "Gathering")

	// Again within 20 seconds, nothing is asked.
	later := now.Add(15 * time.Second)
	clock.Store(&later)
	got := h.search(t, h.movie, h.token)
	if got.status != http.StatusTooManyRequests || got.retryAfter != "5" {
		t.Errorf("again 15 seconds later: %+v", got)
	}
	// Another user may, at once.
	h.testServer.user("other", nil)
	other := h.signIn("other", "tv")
	if got := h.search(t, h.movie, other); got.status != http.StatusOK || got.progress.Pending != 1 {
		t.Errorf("asked again by another user: %+v", got)
	}
	eventually(t, "the addon to be asked for the other user", func() bool { return addon.asked.Load() == 3 })
	addon.replies <- 1
	eventually(t, "the other user's answer", func() bool { return h.progress(t) == VersionProgress{Pending: 0, Count: 1, Known: 1} })

	// 20 seconds after the first time, the user may again.
	later = now.Add(20 * time.Second)
	clock.Store(&later)
	if got := h.search(t, h.movie, h.token); got.status != http.StatusOK || got.progress != (VersionProgress{Pending: 1, Count: 1, Known: 0}) {
		t.Errorf("20 seconds later: %+v", got)
	}
	eventually(t, "the addon to be asked a fourth time", func() bool { return addon.asked.Load() == 4 })
	addon.replies <- 3
	h.reaches(t, VersionProgress{Pending: 0, Count: 3, Known: 3}, "Gathering", "Gathering", "Gathering")

	// A user who may not see the title can neither follow it nor have it
	// asked again; nor can anyone signed out, for any item.
	h.testServer.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{BlockUnrated: []string{"Movie"}} })
	child := h.signIn("child", "tv")
	if got := h.search(t, h.movie, child); got.status != http.StatusNotFound {
		t.Errorf("asked again by a user blocking the title: %+v", got)
	}
	if status := h.get(t, "/Polyfin/Items/"+h.movie+"/Versions", child, nil); status != http.StatusNotFound {
		t.Errorf("followed by a user blocking the title: %d", status)
	}
	if got := h.search(t, h.movie, ""); got.status != http.StatusUnauthorized {
		t.Errorf("without credentials: %+v", got)
	}
	if got := h.search(t, "0123456789abcdef0123456789abcdef", h.token); got.status != http.StatusNotFound {
		t.Errorf("unknown item: %+v", got)
	}
	if got := addon.asked.Load(); got != 4 {
		t.Errorf("addon asked %d times, want 4", got)
	}
}

// Asked again while one of the title's addons is still answering, Polyfin
// leaves that one to answer and asks the others again.
func TestAskingAgainLeavesAnAddonStillAnswering(t *testing.T) {
	slow, empty := newScriptedAddon(t, "Slow"), newScriptedAddon(t, "Empty")
	h := heldOn(t, newTestServer(t, 10), slow.url, empty.url)

	h.sources(t)
	eventually(t, "both addons to be asked", func() bool { return slow.asked.Load() == 1 && empty.asked.Load() == 1 })
	empty.replies <- 0
	h.reaches(t, VersionProgress{Pending: 1, Count: 1, Known: 0}, "Movie")

	if got := h.search(t, h.movie, h.token); got.status != http.StatusOK || got.progress != (VersionProgress{Pending: 2, Count: 1, Known: 0}) {
		t.Fatalf("asked again: %+v", got)
	}
	eventually(t, "the addon that answered to be asked again", func() bool { return empty.asked.Load() == 2 })
	empty.replies <- 1
	slow.replies <- 1
	h.reaches(t, VersionProgress{Pending: 0, Count: 2, Known: 2}, "Slow", "Empty")
	if got := slow.asked.Load(); got != 1 {
		t.Errorf("the addon still answering was asked %d times", got)
	}
}
