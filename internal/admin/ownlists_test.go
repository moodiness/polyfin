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

// fakeWatching keeps each user's Continue Watching, Next Up and hidden
// titles, by user name, and records the changes asked.
type fakeWatching struct {
	mu      sync.Mutex
	resume  map[string][]jellyfin.ResumeItem
	nextUp  map[string][]library.Item
	removed map[string][]jellyfin.NextUpRemoval
	hidden  map[string][]jellyfin.HiddenTitle
	changes []string
}

func (f *fakeWatching) NextUpItems(_ context.Context, user accounts.User) ([]library.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nextUp[user.Name], nil
}

func (f *fakeWatching) NextUpRemovals(_ context.Context, user accounts.User) ([]jellyfin.NextUpRemoval, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removed[user.Name], nil
}

func (f *fakeWatching) RemoveFromNextUp(_ context.Context, user accounts.User, id accounts.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, item := range f.nextUp[user.Name] {
		if item.ID == id {
			f.nextUp[user.Name] = slices.Delete(f.nextUp[user.Name], i, i+1)
			f.changes = append(f.changes, user.Name+" took off Next Up "+item.Name)
			return nil
		}
	}
	return library.ErrNotFound
}

func (f *fakeWatching) PutBackOnNextUp(_ context.Context, user accounts.User, id accounts.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, user.Name+" put back on Next Up "+id.String())
	return nil
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

func TestMyAccountListsAndChangesTheUsersNextUp(t *testing.T) {
	t.Parallel()
	episode, series, other := accounts.ID{4}, accounts.ID{5}, accounts.ID{6}
	removedAt := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	watching := &fakeWatching{
		nextUp: map[string][]library.Item{"member": {
			{ID: episode, Kind: library.KindEpisode, Name: "Second", SeriesID: series, SeriesName: "Show", ParentIndexNumber: 1, IndexNumber: 2},
		}},
		removed: map[string][]jellyfin.NextUpRemoval{"member": {{Item: library.Item{ID: other, Kind: library.KindSeries, Name: "Other"}, At: removedAt}}},
	}
	api := newTestAPI(t, 10, func(options *Options, _ testDeps) { options.Watching = watching })
	nextUp := func(b browser) (int, nextUpJSON) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api/account/next-up", nil)
		response, err := b.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result nextUpJSON
		if response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode, result
	}
	if status, _ := nextUp(api.browser()); status != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", status)
	}

	member := api.signedIn("member", false)
	status, got := nextUp(member)
	if status != http.StatusOK || len(got.Items) != 1 || got.Items[0].ID != episode.String() || got.Items[0].SeriesName != "Show" ||
		len(got.Removed) != 1 || got.Removed[0].Item.ID != other.String() || !got.Removed[0].RemovedAt.Equal(removedAt) || got.WithResume {
		t.Fatalf("next up: %d %+v", status, got)
	}

	// Each acts for the user signed in only.
	stranger := api.signedIn("stranger", false)
	if status, got := nextUp(stranger); status != http.StatusOK || len(got.Items) != 0 || len(got.Removed) != 0 {
		t.Errorf("another user's next up: %d %+v", status, got)
	}
	if status, _, _ := stranger.call(http.MethodDelete, "/account/next-up/"+episode.String(), nil); status != http.StatusNotFound {
		t.Errorf("another user removing: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/next-up/"+episode.String(), nil); status != http.StatusNoContent {
		t.Errorf("removing: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/next-up/removed/"+other.String(), nil); status != http.StatusNoContent {
		t.Errorf("putting back: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/next-up/not-an-id", nil); status != http.StatusNotFound {
		t.Errorf("a malformed identifier: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/next-up/removed/"+other.String(), nil, "Origin", "https://elsewhere.test"); status != http.StatusForbidden {
		t.Errorf("from another site: %d", status)
	}
	if want := []string{"member took off Next Up Second", "member put back on Next Up " + other.String()}; !slices.Equal(watching.changes, want) {
		t.Errorf("changes: %q", watching.changes)
	}

	// Remove from Continue Watching taking the series off Next Up is the
	// user's own choice, kept with their account.
	if status, body, _ := member.call(http.MethodPut, "/account/next-up/settings", map[string]bool{"withResume": true}); status != http.StatusOK ||
		body["withResume"] != true {
		t.Fatalf("turning it on: %d %v", status, body)
	}
	if _, got := nextUp(member); !got.WithResume {
		t.Errorf("after turning it on: %+v", got)
	}
	if _, got := nextUp(stranger); got.WithResume {
		t.Errorf("another user after turning it on: %+v", got)
	}
	if status, _, _ := member.call(http.MethodPut, "/account/next-up/settings", map[string]any{}); status != http.StatusBadRequest {
		t.Errorf("without a value: %d", status)
	}
	if status, _, _ := member.call(http.MethodPost, "/account/preferences/reset", nil); status != http.StatusNoContent {
		t.Fatalf("reset: %d", status)
	}
	if _, got := nextUp(member); got.WithResume {
		t.Errorf("after a reset: %+v", got)
	}
}
