package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// fakeWatching keeps each user's Continue Watching and hidden titles, by
// user name, and records the changes asked.
type fakeWatching struct {
	mu      sync.Mutex
	resume  map[string][]jellyfin.ResumeItem
	hidden  map[string][]jellyfin.HiddenTitle
	changes []string
}

func (f *fakeWatching) ResumeItems(_ context.Context, user accounts.User) ([]jellyfin.ResumeItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resume[user.Name], nil
}

func (f *fakeWatching) RemoveResume(_ context.Context, user accounts.User, id accounts.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, entry := range f.resume[user.Name] {
		if entry.Item.ID == id {
			f.resume[user.Name] = slices.Delete(f.resume[user.Name], i, i+1)
			f.changes = append(f.changes, user.Name+" removed "+entry.Item.Name)
			return nil
		}
	}
	return library.ErrNotFound
}

func (f *fakeWatching) HiddenTitles(_ context.Context, user accounts.User) ([]jellyfin.HiddenTitle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hidden[user.Name], nil
}

func (f *fakeWatching) ShowTitle(_ context.Context, user accounts.User, id accounts.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, user.Name+" showed "+id.String())
	return nil
}

func TestMyAccountListsAndChangesTheUsersOwnTitles(t *testing.T) {
	t.Parallel()
	movie, episode, series := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	played := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	watching := &fakeWatching{
		resume: map[string][]jellyfin.ResumeItem{"member": {
			{Item: library.Item{ID: movie, Kind: library.KindMovie, Name: "Abandoned", ProductionYear: 2021, Runtime: 100 * time.Minute},
				Data: userdata.Data{Position: 10 * time.Minute, PlayCount: 1, LastPlayed: &played}},
			{Item: library.Item{ID: episode, Kind: library.KindEpisode, Name: "Pilot", SeriesID: series, SeriesName: "Show", ParentIndexNumber: 1, IndexNumber: 1},
				Data: userdata.Data{Position: 5 * time.Minute, Runtime: 45 * time.Minute}},
		}},
		hidden: map[string][]jellyfin.HiddenTitle{"member": {{Item: library.Item{ID: series, Kind: library.KindSeries, Name: "Show"}, At: played}}},
	}
	api := newTestAPI(t, 10, func(options *Options, _ testDeps) { options.Watching = watching })
	list := func(b browser, path string, into any) int {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api"+path, nil)
		response, err := b.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(into); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}
	anonymous := api.browser()
	for _, path := range []string{"/account/resume", "/account/hidden"} {
		if status := list(anonymous, path, nil); status != http.StatusUnauthorized {
			t.Errorf("anonymous %s: %d", path, status)
		}
	}

	member := api.signedIn("member", false)
	var resume []resumeJSON
	if status := list(member, "/account/resume", &resume); status != http.StatusOK || len(resume) != 2 {
		t.Fatalf("resume: %d %+v", status, resume)
	}
	if got := resume[0]; got.Item.ID != movie.String() || got.Item.Name != "Abandoned" || got.Position != 600 || got.Runtime != 6000 ||
		got.LastPlayedAt == nil || !got.LastPlayedAt.Equal(played) || got.Played {
		t.Errorf("movie: %+v %+v", got, got.Item)
	}
	if got := resume[1]; got.Item.PosterID != series.String() || got.Item.SeriesName != "Show" || got.Runtime != 2700 || got.LastPlayedAt != nil {
		t.Errorf("episode: %+v %+v", got, got.Item)
	}
	var hidden []hiddenJSON
	if status := list(member, "/account/hidden", &hidden); status != http.StatusOK || len(hidden) != 1 ||
		hidden[0].Item.ID != series.String() || !hidden[0].HiddenAt.Equal(played) {
		t.Fatalf("hidden: %d %+v", status, hidden)
	}

	// Each acts for the user signed in only.
	other := api.signedIn("other", false)
	if status := list(other, "/account/resume", &resume); status != http.StatusOK || len(resume) != 0 {
		t.Errorf("another user's resume: %d %+v", status, resume)
	}
	if status, _, _ := other.call(http.MethodDelete, "/account/resume/"+movie.String(), nil); status != http.StatusNotFound {
		t.Errorf("another user removing: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/resume/"+movie.String(), nil); status != http.StatusNoContent {
		t.Errorf("removing: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/hidden/"+series.String(), nil); status != http.StatusNoContent {
		t.Errorf("showing again: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/resume/not-an-id", nil); status != http.StatusNotFound {
		t.Errorf("a malformed identifier: %d", status)
	}
	// Another site cannot change them.
	if status, _, _ := member.call(http.MethodDelete, "/account/hidden/"+series.String(), nil, "Origin", "https://elsewhere.test"); status != http.StatusForbidden {
		t.Errorf("from another site: %d", status)
	}
	if want := []string{"member removed Abandoned", "member showed " + series.String()}; !slices.Equal(watching.changes, want) {
		t.Errorf("changes: %q", watching.changes)
	}
}
