package admin

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// A member reads their own statistics, history and CSV only, whatever
// user they name; an administrator reads everyone's, or one user's.
func TestMembersReadOnlyTheirOwnPlaybacks(t *testing.T) {
	var history *plays.History
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		history = plays.NewHistory(deps.pool, o.Accounts.Settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
		o.Plays = history
	})
	administrator := api.signedIn("admin", true)
	member := api.signedIn("member", false)
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]accounts.ID{}
	for _, u := range users {
		ids[u.Name] = u.ID
	}
	start := time.Now().Add(-2 * time.Hour)
	for i, who := range []string{"admin", "member"} {
		history.Record(t.Context(), plays.Change{Kind: plays.Stopped, Playback: plays.Playback{User: ids[who], UserName: who,
			Title: plays.Title{Item: accounts.ID{byte(i + 1)}, Kind: plays.Movie, Name: "Movie of " + who}, App: "Web", DeviceName: "Phone",
			Started: start, LastReport: start.Add(time.Hour), Ended: start.Add(time.Hour), Played: time.Duration(i+1) * time.Hour}})
	}

	other := "?user=" + ids["admin"].String()
	_, own, _ := member.call(http.MethodGet, "/account/statistics"+other, nil)
	if own["played"] != float64(2*3600) || len(own["users"].([]any)) != 1 {
		t.Errorf("the member's statistics: %v played by %v, want 7200 by the member alone", own["played"], own["users"])
	}
	_, page, _ := member.call(http.MethodGet, "/account/history"+other, nil)
	items, _ := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["item"].(map[string]any)["name"] != "Movie of member" {
		t.Errorf("the member's history: %v, want their movie alone", page)
	}
	csv := download(t, member, "/account/history/export"+other)
	if !strings.Contains(csv, "Movie of member") || strings.Contains(csv, "Movie of admin") {
		t.Errorf("the member's CSV:\n%s\nwant their movie alone", csv)
	}
	for _, path := range []string{"/statistics", "/history", "/history/export"} {
		if status, _, _ := member.call(http.MethodGet, path, nil); status != http.StatusForbidden {
			t.Errorf("a member reading %s: %d, want 403", path, status)
		}
	}

	_, all, _ := administrator.call(http.MethodGet, "/statistics?period=7d", nil)
	if all["played"] != float64(3*3600) || len(all["users"].([]any)) != 2 {
		t.Errorf("everyone's statistics: %v by %v, want 10800 by both", all["played"], all["users"])
	}
	_, one, _ := administrator.call(http.MethodGet, "/statistics?user="+ids["member"].String(), nil)
	if one["played"] != float64(2*3600) {
		t.Errorf("the member's statistics read by an administrator: %v, want 7200", one["played"])
	}
	if csv := download(t, administrator, "/history/export"); !strings.Contains(csv, "Movie of member") || !strings.Contains(csv, "Movie of admin") {
		t.Errorf("everyone's CSV:\n%s\nwant both movies", csv)
	}
}

// download reads path as text.
func download(t *testing.T, b browser, path string) string {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, b.api.url+"/admin/api"+path, nil)
	response, err := b.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d %s", path, response.StatusCode, body)
	}
	return string(body)
}
