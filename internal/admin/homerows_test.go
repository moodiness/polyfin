package admin

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// fixedChoices offers one collection and one playlist for home rows.
type fixedChoices struct{}

func (fixedChoices) HomeRowCollections(context.Context, accounts.User) ([]jellyfin.HomeRowCollection, error) {
	id, _ := accounts.ParseID("0123456789abcdef0123456789abcdef")
	return []jellyfin.HomeRowCollection{{ID: id, Name: "Picks", Source: "users"}}, nil
}

func (fixedChoices) HomeRowPlaylists(context.Context) ([]jellyfin.HomeRowPlaylist, error) {
	id, _ := accounts.ParseID("fedcba9876543210fedcba9876543210")
	return []jellyfin.HomeRowPlaylist{{ID: id, Name: "Friday night", OwnerName: "member", Access: "shared"}}, nil
}

// rowNames lists the names of the rows of an answer.
func rowNames(answer map[string]any) []string {
	var result []string
	rows, _ := answer["rows"].([]any)
	for _, row := range rows {
		result = append(result, row.(map[string]any)["name"].(string))
	}
	return result
}

// Administrators add, change, order and delete the web player's home rows,
// choosing among the catalogs of the server's addons and the collections;
// rows that cannot work are refused. Members may not.
func TestHomeRowRoutes(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.HomeRows = homerows.New(d.pool)
		o.HomeRowChoices = fixedChoices{}
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	if status, _, _ := member.call(http.MethodGet, "/home-rows", nil); status != http.StatusForbidden {
		t.Errorf("a member's list: %d", status)
	}
	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": listAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}
	status, choices, _ := administrator.call(http.MethodGet, "/home-rows/choices", nil)
	catalogs, _ := choices["catalogs"].([]any)
	collections, _ := choices["collections"].([]any)
	if status != http.StatusOK || len(catalogs) != 1 || catalogs[0].(map[string]any)["catalogId"] != "top" || len(collections) != 1 ||
		collections[0].(map[string]any)["name"] != "Picks" {
		t.Fatalf("choices: %d %v", status, choices)
	}

	catalog := map[string]any{"name": "Top movies", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie",
		"catalogId": "top", "everyone": true}
	for _, refused := range []map[string]any{
		{"name": "", "kind": "new", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "trending", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "new", "place": "middle", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "new", "place": "top", "addonId": addon["id"], "catalogType": "movie", "catalogId": "top", "everyone": true},
		{"name": "Rows", "kind": "collection", "place": "top", "collectionId": "not an identifier", "everyone": true},
		{"name": "Rows", "kind": "because", "place": "top", "count": 11, "everyone": true},
		// Catalogs the choices do not offer, which no row could draw: one
		// that cannot be browsed, one the addon does not have, and one of
		// no addon installed.
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie", "catalogId": "search", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "collection", "catalogId": "sets", "everyone": true},
		{"name": "Rows", "kind": "catalog", "place": "top", "addonId": "0123456789abcdef0123456789abcdef", "catalogType": "movie", "catalogId": "top",
			"everyone": true},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/home-rows", refused); status != http.StatusBadRequest || body["error"] != "invalid_home_row" {
			t.Errorf("%v: %d %v", refused, status, body)
		}
	}
	status, created, _ := administrator.call(http.MethodPost, "/home-rows", catalog)
	if status != http.StatusCreated || created["kind"] != "catalog" || created["addonId"] != addon["id"] || created["library"] != false || created["count"] != 3.0 {
		t.Fatalf("created: %d %v", status, created)
	}
	_, because, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "Because you watched", "kind": "because",
		"place": "resume", "count": 2, "everyone": true})
	var me map[string]any
	_, me, _ = member.call(http.MethodGet, "/session", nil)
	memberID := me["user"].(map[string]any)["id"].(string)
	_, chosen, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "For the member", "kind": "collection",
		"place": "nextup", "collectionId": "0123456789abcdef0123456789abcdef", "everyone": false, "users": []string{memberID}, "library": true})
	if users, _ := chosen["users"].([]any); len(users) != 1 || users[0] != memberID || chosen["library"] != true {
		t.Fatalf("a row for the member: %v", chosen)
	}
	_, list, _ := administrator.call(http.MethodGet, "/home-rows", nil)
	if got := rowNames(list); !slices.Equal(got, []string{"Top movies", "Because you watched", "For the member"}) {
		t.Errorf("rows: %v", got)
	}
	status, list, _ = administrator.call(http.MethodPut, "/home-rows/order", map[string]any{"ids": []any{chosen["id"], created["id"]}})
	if got := rowNames(list); status != http.StatusOK || !slices.Equal(got, []string{"For the member", "Top movies", "Because you watched"}) {
		t.Errorf("ordered: %d %v", status, got)
	}
	catalog["place"], catalog["name"] = "bottom", "Top of the week"
	if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+created["id"].(string), catalog); status != http.StatusOK ||
		body["place"] != "bottom" || body["name"] != "Top of the week" {
		t.Errorf("changed: %d %v", status, body)
	}
	unbrowsable := map[string]any{"name": "Search", "kind": "catalog", "place": "top", "addonId": addon["id"], "catalogType": "movie",
		"catalogId": "search", "everyone": true}
	if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+created["id"].(string), unbrowsable); status != http.StatusBadRequest ||
		body["error"] != "invalid_home_row" {
		t.Errorf("changed to a catalog not offered: %d %v", status, body)
	}

	// The member sees the rows shown to them, and turns one off.
	status, own, _ := member.call(http.MethodGet, "/account/home-rows", nil)
	if got := rowNames(own); status != http.StatusOK || !slices.Equal(got, []string{"For the member", "Top of the week", "Because you watched"}) {
		t.Fatalf("the member's rows: %d %v", status, own)
	}
	if status, _, _ := member.call(http.MethodPut, "/account/home-rows/"+because["id"].(string), map[string]bool{"off": true}); status != http.StatusNoContent {
		t.Errorf("turning off: %d", status)
	}
	_, own, _ = member.call(http.MethodGet, "/account/home-rows", nil)
	rows, _ := own["rows"].([]any)
	if len(rows) != 3 || rows[2].(map[string]any)["off"] != true || rows[0].(map[string]any)["off"] != false {
		t.Errorf("after turning off: %v", own)
	}
	// The administrator does not see the row for the member, and cannot
	// turn it off.
	_, own, _ = administrator.call(http.MethodGet, "/account/home-rows", nil)
	if got := rowNames(own); !slices.Equal(got, []string{"Top of the week", "Because you watched"}) {
		t.Errorf("the administrator's rows: %v", got)
	}
	if status, _, _ := administrator.call(http.MethodPut, "/account/home-rows/"+chosen["id"].(string), map[string]bool{"off": true}); status != http.StatusNotFound {
		t.Errorf("turning off a row not shown: %d", status)
	}

	if status, _, _ := administrator.call(http.MethodDelete, "/home-rows/"+because["id"].(string), nil); status != http.StatusNoContent {
		t.Errorf("deleting: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/home-rows/"+because["id"].(string), nil); status != http.StatusNotFound {
		t.Errorf("deleting again: %d", status)
	}
	for range homerows.MaxRows - 2 {
		if status, body, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "New", "kind": "new", "place": "top",
			"everyone": true}); status != http.StatusCreated {
			t.Fatalf("adding: %d %v", status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "New", "kind": "new", "place": "top",
		"everyone": true}); status != http.StatusConflict || body["error"] != "too_many_home_rows" {
		t.Errorf("one row too many: %d %v", status, body)
	}
}

// A row keeps the schedule it is given, its days in order, and tells
// whether it shows now; schedules that cannot be are refused.
func TestHomeRowSchedules(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.HomeRows = homerows.New(d.pool)
		o.HomeRowChoices = fixedChoices{}
	})
	administrator := api.signedIn("administrator", true)
	start, end := time.Now().AddDate(0, 0, -20).Format(time.DateOnly), time.Now().AddDate(0, 0, -10).Format(time.DateOnly)
	status, created, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "Past", "kind": "new", "place": "top",
		"everyone": true, "schedule": map[string]any{"startDate": start, "endDate": end, "days": []int{5, 1, 5}, "startHour": 22, "endHour": 2}})
	schedule, _ := created["schedule"].(map[string]any)
	if status != http.StatusCreated || schedule["startDate"] != start || schedule["endDate"] != end || !slices.Equal(schedule["days"].([]any), []any{1.0, 5.0}) ||
		schedule["startHour"] != 22.0 || schedule["endHour"] != 2.0 || created["shownNow"] != false {
		t.Fatalf("a row with a schedule: %d %v", status, created)
	}
	_, always, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "Always", "kind": "new", "place": "top",
		"everyone": true})
	schedule, _ = always["schedule"].(map[string]any)
	if schedule["startDate"] != "" || len(schedule["days"].([]any)) != 0 || schedule["startHour"] != nil || always["shownNow"] != true {
		t.Errorf("a row without a schedule: %v", always)
	}
	for _, refused := range []map[string]any{
		{"startDate": "2026-13-01"},
		{"startDate": "2026-12-02", "endDate": "2026-12-01"},
		{"startHour": 22},
		{"days": []int{7}},
		{"startHour": 8, "endHour": 8},
		{"startHour": 20, "endHour": 0},
	} {
		if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+always["id"].(string), map[string]any{"name": "Always",
			"kind": "new", "place": "top", "everyone": true, "schedule": refused}); status != http.StatusBadRequest || body["error"] != "invalid_home_row" {
			t.Errorf("%v: %d %v", refused, status, body)
		}
	}
}

// fixedPreviews previews a row of an episode and a movie, for whoever is
// asked, and remembers who was.
type fixedPreviews struct {
	mu    sync.Mutex
	asked []accounts.User
}

func (p *fixedPreviews) PreviewHomeRows(_ context.Context, user accounts.User) ([]jellyfin.HomeRowPreview, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, user)
	id := func(raw string) accounts.ID {
		parsed, _ := accounts.ParseID(raw)
		return parsed
	}
	return []jellyfin.HomeRowPreview{{ID: id("10000000000000000000000000000000"), Name: "Because you watched Show",
		Row: homerows.Row{ID: id("20000000000000000000000000000000"), Kind: homerows.KindBecause, Place: homerows.PlaceResume},
		Titles: []library.Item{
			{ID: id("30000000000000000000000000000000"), Kind: library.KindEpisode, Name: "Pilot", SeriesID: id("40000000000000000000000000000000")},
			{ID: id("50000000000000000000000000000000"), Kind: library.KindMovie, Name: "Movie", ProductionYear: 2001},
		}}}, nil
}

func (*fixedPreviews) WatchlistRowName(context.Context, accounts.User) (string, error) {
	return "Mine to watch", nil
}

// Rows of the Watchlist, of favorite playlists and of a chosen playlist
// are added with the playlist field, refused when their fields do not go
// together or name no playlist, and deleted with their playlist. A member
// sees a Watchlist row by their own Watchlist's name.
func TestPlaylistHomeRowRoutes(t *testing.T) {
	t.Parallel()
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.HomeRows = homerows.New(d.pool)
		o.HomeRowChoices = fixedChoices{}
		o.HomeRowPreviews = &fixedPreviews{}
		pool = d.pool
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, me, _ := member.call(http.MethodGet, "/session", nil)
	memberID := me["user"].(map[string]any)["id"].(string)
	status, choices, _ := administrator.call(http.MethodGet, "/home-rows/choices", nil)
	lists, _ := choices["playlists"].([]any)
	if status != http.StatusOK || len(lists) != 1 || lists[0].(map[string]any)["ownerName"] != "member" || lists[0].(map[string]any)["access"] != "shared" {
		t.Fatalf("choices: %d %v", status, choices)
	}
	playlist := "fedcba9876543210fedcba9876543210"
	if _, err := pool.Exec(t.Context(), "INSERT INTO playlists (id, owner_id, name) VALUES ($1, $2, 'Friday night')", playlist, memberID); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []map[string]any{
		{"name": "Rows", "kind": "playlist", "place": "top", "everyone": true},
		{"name": "Rows", "kind": "playlist", "place": "top", "playlistId": "not an identifier", "everyone": true},
		{"name": "Rows", "kind": "playlist", "place": "top", "playlistId": "0123456789abcdef0123456789abcdef", "everyone": true},
		{"name": "Rows", "kind": "watchlist", "place": "top", "playlistId": playlist, "everyone": true},
		{"name": "Rows", "kind": "playlists", "place": "top", "playlistId": playlist, "everyone": true},
		{"name": "Rows", "kind": "playlists", "place": "top", "library": true, "everyone": true},
		{"name": "Rows", "kind": "playlists", "place": "top", "count": 11, "everyone": true},
		{"name": "Rows", "kind": "playlists", "place": "top", "count": -1, "everyone": true},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/home-rows", refused); status != http.StatusBadRequest || body["error"] != "invalid_home_row" {
			t.Errorf("%v: %d %v", refused, status, body)
		}
	}
	status, watchlist, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "À regarder", "kind": "watchlist",
		"place": "top", "everyone": true, "library": true})
	if status != http.StatusCreated || watchlist["kind"] != "watchlist" || watchlist["playlistId"] != "" || watchlist["library"] != true {
		t.Fatalf("a Watchlist row: %d %v", status, watchlist)
	}
	status, favorites, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "My playlists", "kind": "playlists",
		"place": "bottom", "count": 10, "everyone": true})
	if status != http.StatusCreated || favorites["count"] != 10.0 {
		t.Fatalf("a My playlists row: %d %v", status, favorites)
	}
	status, chosen, _ := administrator.call(http.MethodPost, "/home-rows", map[string]any{"name": "Friday", "kind": "playlist",
		"place": "nextup", "playlistId": playlist, "everyone": true, "library": true})
	if status != http.StatusCreated || chosen["playlistId"] != playlist || chosen["library"] != true {
		t.Fatalf("a playlist row: %d %v", status, chosen)
	}
	gone := map[string]any{"name": "Friday", "kind": "playlist", "place": "nextup", "playlistId": "0123456789abcdef0123456789abcdef", "everyone": true}
	if status, body, _ := administrator.call(http.MethodPatch, "/home-rows/"+chosen["id"].(string), gone); status != http.StatusBadRequest ||
		body["error"] != "invalid_home_row" {
		t.Errorf("changed to a playlist that does not exist: %d %v", status, body)
	}
	_, own, _ := member.call(http.MethodGet, "/account/home-rows", nil)
	if got := rowNames(own); !slices.Equal(got, []string{"Mine to watch", "My playlists", "Friday"}) {
		t.Errorf("the member's rows: %v", got)
	}
	if _, err := pool.Exec(t.Context(), "DELETE FROM playlists WHERE id = $1", playlist); err != nil {
		t.Fatal(err)
	}
	_, list, _ := administrator.call(http.MethodGet, "/home-rows", nil)
	if got := rowNames(list); !slices.Equal(got, []string{"À regarder", "My playlists"}) {
		t.Errorf("after the playlist is deleted: %v", got)
	}
}

// Preview as asks for the rows of the user chosen, as their account is,
// and answers each row with its titles, an episode's poster being its
// series'. A user that does not exist is not found; members may not.
func TestHomeRowPreview(t *testing.T) {
	t.Parallel()
	previews := &fixedPreviews{}
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.HomeRows = homerows.New(d.pool)
		o.HomeRowPreviews = previews
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	var me map[string]any
	_, me, _ = member.call(http.MethodGet, "/session", nil)
	memberID := me["user"].(map[string]any)["id"].(string)
	id, _ := accounts.ParseID(memberID)
	if _, err := api.store.UpdateUser(t.Context(), id, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := member.call(http.MethodGet, "/home-rows/preview/"+memberID, nil); status != http.StatusForbidden {
		t.Errorf("a member's preview: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodGet, "/home-rows/preview/0123456789abcdef0123456789abcdef", nil); status != http.StatusNotFound {
		t.Errorf("a user that does not exist: %d", status)
	}
	status, answer, _ := administrator.call(http.MethodGet, "/home-rows/preview/"+memberID, nil)
	rows, _ := answer["rows"].([]any)
	if status != http.StatusOK || len(rows) != 1 {
		t.Fatalf("the preview: %d %v", status, answer)
	}
	row := rows[0].(map[string]any)
	titles := row["titles"].([]any)
	if row["name"] != "Because you watched Show" || row["kind"] != "because" || row["place"] != "resume" ||
		row["rowId"] != "20000000000000000000000000000000" || len(titles) != 2 ||
		titles[0].(map[string]any)["posterId"] != "40000000000000000000000000000000" ||
		titles[1].(map[string]any)["posterId"] != "50000000000000000000000000000000" || titles[1].(map[string]any)["year"] != 2001.0 {
		t.Errorf("the preview's row: %v", row)
	}
	previews.mu.Lock()
	defer previews.mu.Unlock()
	if len(previews.asked) != 1 || previews.asked[0].ID != id || previews.asked[0].Parental.MaxRating == nil || *previews.asked[0].Parental.MaxRating != 13 {
		t.Errorf("asked for %+v", previews.asked)
	}
}
