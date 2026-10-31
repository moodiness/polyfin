package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// orderedShow is a server whose addon lists one series, Show (TMDB 77),
// aired as S01E01 to S01E03, S02E01, S02E02 and a special, S00E01. A fake
// TMDB gives it a DVD episode group: Volume 1 holds S01E02, S01E01 and
// S02E01, Volume 2 holds S01E03; it leaves S02E02 and the special out.
type orderedShow struct {
	testServer
	admin     string
	adminUser accounts.User
	series    string
	// episodes are the series' episodes by their aired place, "S01E02".
	episodes map[string]string
	mu       sync.Mutex
	streams  []string
}

func newOrderedShow(t *testing.T) *orderedShow {
	t.Helper()
	o := &orderedShow{episodes: map[string]string{}}
	episode := func(season, number int) map[string]any {
		return map[string]any{"show_id": 77, "season_number": season, "episode_number": number}
	}
	at := func(e map[string]any, order int) map[string]any {
		e["order"] = order
		return e
	}
	answers := map[string]any{
		"/3/tv/77/episode_groups": map[string]any{"results": []map[string]any{
			{"id": "story", "name": "Story", "type": 5, "episode_count": 5, "group_count": 1},
			{"id": "dvd", "name": "DVD", "type": 3, "episode_count": 4, "group_count": 2},
		}},
		"/3/tv/episode_group/dvd": map[string]any{"id": "dvd", "name": "DVD", "type": 3, "groups": []map[string]any{
			{"name": "Volume 2", "order": 2, "episodes": []any{at(episode(1, 3), 0)}},
			{"name": "Volume 1", "order": 1, "episodes": []any{at(episode(2, 1), 2), at(episode(1, 2), 0), at(episode(1, 1), 1)}},
		}},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[r.URL.Path]
		if !ok || r.URL.Query().Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(api.Close)
	video := func(season, number int, name string) stremio.Video {
		return stremio.Video{ID: "tt20:" + itoa(season) + ":" + itoa(number), Title: name, Season: stremio.Number(season),
			Episode: stremio.Number(number), Released: "2020-01-0" + itoa(season*3+number) + "T00:00:00Z"}
	}
	show := stremio.Meta{ID: "tt20", Type: "series", Name: "Show", TmdbID: "77", Videos: []stremio.Video{
		video(1, 1, "One"), video(1, 2, "Two"), video(1, 3, "Three"), video(2, 1, "Four"), video(2, 2, "Five"), video(0, 1, "Special"),
	}}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shows", Name: "Shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Catalogs: []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}}})
		case strings.HasPrefix(path, "/catalog/"):
			listed := show
			listed.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{listed}})
		case path == "/meta/series/tt20":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		case strings.HasPrefix(path, "/stream/series/"):
			o.mu.Lock()
			o.streams = append(o.streams, strings.TrimPrefix(path, "/stream/series/"))
			o.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{URL: "https://media.example/" + path + ".mkv"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	o.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		options.Library.UseTMDB(tmdb.New(api.URL+"/3", api.URL+"/t/p/", "test", options.Accounts.Settings))
	})
	o.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = "k3y" })
	o.adminUser = o.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	if _, err := o.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	o.admin = o.signIn("admin", "tv")
	for _, id := range o.viewIDs(t, o.admin) {
		var page QueryResult
		o.get(t, "/Items?ParentId="+id, o.admin, &page)
		for _, item := range page.Items {
			if item.Name == "Show" {
				o.series = item.Id
			}
		}
	}
	if o.series == "" {
		t.Fatal("the series is not listed")
	}
	for _, e := range o.list(t, "") {
		o.episodes[place(e)] = e.Id
	}
	return o
}

func itoa(n int) string { return string(rune('0' + n)) }

// place is where an episode shows, "S01E02".
func place(e BaseItemDto) string {
	if e.ParentIndexNumber == nil || e.IndexNumber == nil {
		return "?"
	}
	return "S0" + itoa(*e.ParentIndexNumber) + "E0" + itoa(*e.IndexNumber)
}

// list lists the series' episodes, of a season when set.
func (o *orderedShow) list(t *testing.T, season string) []BaseItemDto {
	t.Helper()
	path := "/Shows/" + o.series + "/Episodes?userId=" + o.adminUser.ID.String()
	if season != "" {
		path += "&seasonId=" + season
	}
	var page QueryResult
	if status := o.get(t, path, o.admin, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return page.Items
}

// seasons lists the series' seasons.
func (o *orderedShow) seasons(t *testing.T) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := o.get(t, "/Shows/"+o.series+"/Seasons?userId="+o.adminUser.ID.String(), o.admin, &page); status != http.StatusOK {
		t.Fatalf("seasons: %d", status)
	}
	return page.Items
}

// order sets the series' DisplayOrder as jellyfin-web's metadata editor
// does, with its name.
func (o *orderedShow) order(t *testing.T, displayOrder string) {
	t.Helper()
	status, body := o.call(http.MethodPost, "/Items/"+o.series, app("tv", o.admin), map[string]any{"Name": "Show", "DisplayOrder": displayOrder})
	if status != http.StatusNoContent {
		t.Fatalf("display order %q: %d %s", displayOrder, status, body)
	}
}

// shown lists episodes by their places and names.
func shown(episodes []BaseItemDto) []string {
	var result []string
	for _, e := range episodes {
		result = append(result, place(e)+" "+e.Name)
	}
	return result
}

func TestDisplayOrderShowsTheEpisodeGroupsSeasons(t *testing.T) {
	t.Parallel()
	o := newOrderedShow(t)
	o.order(t, "dvd")

	var series BaseItemDto
	if o.get(t, "/Items/"+o.series, o.admin, &series); series.DisplayOrder != "dvd" {
		t.Errorf("display order: %q", series.DisplayOrder)
	}
	seasons := o.seasons(t)
	var names []string
	for _, s := range seasons {
		names = append(names, itoa(*s.IndexNumber)+" "+s.Name)
	}
	// The special and S02E02, which the group leaves out, keep a place.
	if want := []string{"0 Specials", "1 Volume 1", "2 Volume 2", "3 Other episodes"}; !slices.Equal(names, want) {
		t.Fatalf("seasons: %v, want %v", names, want)
	}
	volume := o.list(t, seasons[1].Id)
	if got, want := shown(volume), []string{"S01E01 Two", "S01E02 One", "S01E03 Four"}; !slices.Equal(got, want) {
		t.Errorf("volume 1: %v, want %v", got, want)
	}
	for _, e := range volume {
		if e.SeasonId != seasons[1].Id || e.SeasonName != "Volume 1" {
			t.Errorf("%s: season %s %q", e.Name, e.SeasonId, e.SeasonName)
		}
	}
	if got, want := shown(o.list(t, "")), []string{"S00E01 Special", "S01E01 Two", "S01E02 One", "S01E03 Four", "S02E01 Three",
		"S03E01 Five"}; !slices.Equal(got, want) {
		t.Errorf("episodes: %v, want %v", got, want)
	}
	// The group's season opens by its identifier, with its episodes.
	var season BaseItemDto
	if status := o.get(t, "/Items/"+seasons[2].Id, o.admin, &season); status != http.StatusOK || season.Name != "Volume 2" {
		t.Errorf("season: %d %q", status, season.Name)
	}
	var children QueryResult
	if o.get(t, "/Items?ParentId="+seasons[2].Id, o.admin, &children); !slices.Equal(shown(children.Items), []string{"S02E01 Three"}) {
		t.Errorf("its children: %v", shown(children.Items))
	}
	// The episode itself shows where the order puts it.
	var episode BaseItemDto
	if o.get(t, "/Items/"+o.episodes["S02E01"], o.admin, &episode); place(episode) != "S01E03" || episode.SeasonId != seasons[1].Id {
		t.Errorf("S02E01 as DVD: %s in %s", place(episode), episode.SeasonId)
	}

	// Back as aired, the addon's seasons come back.
	o.order(t, "")
	names = nil
	for _, s := range o.seasons(t) {
		names = append(names, s.Name)
	}
	if want := []string{"Specials", "Season 1", "Season 2"}; !slices.Equal(names, want) {
		t.Errorf("seasons as aired: %v", names)
	}
}

func TestEpisodeOrderPlaysAndMarksTheAiredEpisode(t *testing.T) {
	t.Parallel()
	o := newOrderedShow(t)
	o.order(t, "dvd")
	seasons := o.seasons(t)
	third := o.list(t, seasons[1].Id)[2]
	if place(third) != "S01E03" || third.Id != o.episodes["S02E01"] {
		t.Fatalf("DVD S01E03: %s %s", place(third), third.Id)
	}
	// Playing DVD S01E03 asks for the aired S02E01's streams.
	status, body := o.call(http.MethodPost, "/Items/"+third.Id+"/PlaybackInfo?UserId="+o.adminUser.ID.String(), app("tv", o.admin), map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("playback info: %d %s", status, body)
	}
	o.mu.Lock()
	asked := slices.Clone(o.streams)
	o.mu.Unlock()
	if !slices.Contains(asked, "tt20:2:1") || slices.Contains(asked, "tt20:1:3") {
		t.Errorf("streams asked: %v", asked)
	}
	if status, _ := o.call(http.MethodPost, "/UserPlayedItems/"+third.Id, app("tv", o.admin), nil); status != http.StatusOK {
		t.Fatalf("mark played: %d", status)
	}
	// Its played mark shows in the DVD order, and counts in its season.
	for _, e := range o.list(t, seasons[1].Id) {
		if e.UserData.Played != (e.Id == third.Id) {
			t.Errorf("DVD %s played: %v", place(e), e.UserData.Played)
		}
	}
	var season BaseItemDto
	o.get(t, "/Items/"+seasons[1].Id+"?userId="+o.adminUser.ID.String(), o.admin, &season)
	if season.UserData.UnplayedItemCount == nil || *season.UserData.UnplayedItemCount != 2 {
		t.Errorf("Volume 1 unplayed: %+v", season.UserData)
	}
	// And as aired.
	o.order(t, "")
	for _, e := range o.list(t, "") {
		if e.UserData.Played != (place(e) == "S02E01") {
			t.Errorf("aired %s played: %v", place(e), e.UserData.Played)
		}
	}
}

func TestNextUpFollowsTheEpisodeOrder(t *testing.T) {
	t.Parallel()
	o := newOrderedShow(t)
	o.order(t, "dvd")
	// DVD S01E01 is the aired S01E02.
	if status, _ := o.call(http.MethodPost, "/UserPlayedItems/"+o.episodes["S01E02"], app("tv", o.admin), nil); status != http.StatusOK {
		t.Fatalf("mark played: %d", status)
	}
	next := func() []string {
		var page QueryResult
		o.get(t, "/Shows/NextUp?userId="+o.adminUser.ID.String(), o.admin, &page)
		return shown(page.Items)
	}
	if got := next(); !slices.Equal(got, []string{"S01E02 One"}) {
		t.Errorf("next up as DVD: %v", got)
	}
	o.order(t, "")
	if got := next(); !slices.Equal(got, []string{"S01E03 Three"}) {
		t.Errorf("next up as aired: %v", got)
	}
}

func TestEpisodeOrderChosenInTheAdminApp(t *testing.T) {
	t.Parallel()
	o := newOrderedShow(t)
	series, err := accounts.ParseID(o.series)
	if err != nil {
		t.Fatal(err)
	}
	lib := o.handler.Library
	if err := lib.SetEpisodeOrder(t.Context(), o.adminUser, series, "nope", ""); err == nil {
		t.Error("a group the series does not have was chosen")
	}
	// The DVD group, with the specials hidden.
	if err := lib.SetEpisodeOrder(t.Context(), o.adminUser, series, "dvd", library.SpecialsHidden); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range o.seasons(t) {
		names = append(names, s.Name)
	}
	if want := []string{"Volume 1", "Volume 2", "Other episodes"}; !slices.Equal(names, want) {
		t.Errorf("seasons: %v, want %v", names, want)
	}
	var item BaseItemDto
	if o.get(t, "/Items/"+o.series, o.admin, &item); item.DisplayOrder != "dvd" {
		t.Errorf("display order: %q", item.DisplayOrder)
	}
	// jellyfin-web's editor saving the same order keeps the group and
	// where the specials show.
	o.order(t, "dvd")
	_, order, err := lib.EpisodeOrderOf(t.Context(), o.adminUser, series)
	if err != nil || order.Group != "dvd" || order.Specials != library.SpecialsHidden || len(order.Groups) != 2 {
		t.Errorf("order: %+v %v", order, err)
	}
	// Aired, with the specials last.
	if err := lib.SetEpisodeOrder(t.Context(), o.adminUser, series, "", library.SpecialsLast); err != nil {
		t.Fatal(err)
	}
	names = nil
	for _, s := range o.seasons(t) {
		names = append(names, s.Name)
	}
	if want := []string{"Season 1", "Season 2", "Specials"}; !slices.Equal(names, want) {
		t.Errorf("seasons as aired: %v, want %v", names, want)
	}
	if got := shown(o.list(t, "")); got[len(got)-1] != "S00E01 Special" {
		t.Errorf("episodes as aired: %v", got)
	}
}

// A type TMDB gives the series no group of, or none of TMDB's, shows the
// series as aired.
func TestDisplayOrderWithoutGroupShowsAsAired(t *testing.T) {
	t.Parallel()
	o := newOrderedShow(t)
	for _, order := range []string{"absolute", "alternate"} {
		o.order(t, order)
		var names []string
		for _, s := range o.seasons(t) {
			names = append(names, s.Name)
		}
		if want := []string{"Specials", "Season 1", "Season 2"}; !slices.Equal(names, want) {
			t.Errorf("%s: %v", order, names)
		}
	}
}
