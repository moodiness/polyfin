package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// tracking browses the catalog addon as a member and finds its first movie
// and its series' episodes, in order.
type tracking struct {
	testServer
	token, user   string
	views         map[string]string
	movie, series string
	episodes      []string
	seasons       []string
}

func newTracking(t *testing.T) tracking {
	t.Helper()
	s, token, views := browsing(t)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	tr := tracking{testServer: s, token: token, user: member.ID.String(), views: views}
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &page)
	tr.movie = page.Items[0].Id
	s.get(t, "/Items?ParentId="+views["Shows"], token, &page)
	tr.series = page.Items[0].Id
	s.get(t, "/Shows/"+tr.series+"/Episodes", token, &page)
	for _, episode := range page.Items {
		tr.episodes = append(tr.episodes, episode.Id)
	}
	s.get(t, "/Shows/"+tr.series+"/Seasons", token, &page)
	for _, season := range page.Items {
		tr.seasons = append(tr.seasons, season.Id)
	}
	return tr
}

func (tr tracking) report(t *testing.T, path string, body map[string]any) {
	t.Helper()
	if status, data := tr.call(http.MethodPost, path, app("tv", tr.token), body); status != http.StatusNoContent {
		t.Fatalf("%s: %d %s", path, status, data)
	}
}

func (tr tracking) userData(t *testing.T, id string) UserItemData {
	t.Helper()
	var item BaseItemDto
	if status := tr.get(t, "/Users/"+tr.user+"/Items/"+id, tr.token, &item); status != http.StatusOK {
		t.Fatalf("item %s: %d", id, status)
	}
	return item.UserData
}

func (tr tracking) mark(t *testing.T, method, path string) UserItemData {
	t.Helper()
	var data UserItemData
	if status := tr.send(t, method, path, &data); status != http.StatusOK {
		t.Fatalf("%s %s: %d", method, path, status)
	}
	return data
}

func (tr tracking) send(t *testing.T, method, path string, into any) int {
	t.Helper()
	status, body := tr.call(method, path, app("tv", tr.token), nil)
	if into != nil && status == http.StatusOK {
		if err := json.Unmarshal(body, into); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return status
}

func (tr tracking) list(t *testing.T, path string) []string {
	t.Helper()
	var page QueryResult
	if status := tr.get(t, path, tr.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return itemIDs(page.Items)
}

func itemIDs(items []BaseItemDto) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Id)
	}
	return ids
}

func TestPlaybackReportsMoveTheResumePoint(t *testing.T) {
	tr := newTracking(t)
	// The movie runs 1h30, 54 000 000 000 ticks.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})
	if data := tr.userData(t, tr.movie); data.PlayCount != 1 || data.LastPlayedDate == nil || data.Played || data.PlaybackPositionTicks != 0 {
		t.Errorf("after starting: %+v", data)
	}
	tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.movie, "PositionTicks": 27_000_000_000})
	data := tr.userData(t, tr.movie)
	if data.PlaybackPositionTicks != 27_000_000_000 || data.PlayedPercentage == nil || *data.PlayedPercentage != 50 {
		t.Errorf("halfway: %+v", data)
	}
	if resume := tr.list(t, "/UserItems/Resume?mediaTypes=Video"); !slices.Equal(resume, []string{tr.movie}) {
		t.Errorf("resume: %v", resume)
	}
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": 52_000_000_000})
	if data := tr.userData(t, tr.movie); !data.Played || data.PlaybackPositionTicks != 0 || data.PlayCount != 1 || data.PlayedPercentage != nil {
		t.Errorf("stopped near the end: %+v", data)
	}
	if resume := tr.list(t, "/UserItems/Resume"); len(resume) != 0 {
		t.Errorf("resume after the end: %v", resume)
	}
	// What one user did is theirs only.
	tr.user2(t)
}

// user2 checks that another user of the server sees none of the member's
// data.
func (tr tracking) user2(t *testing.T) {
	t.Helper()
	tr.testServer.user("other", nil)
	token := tr.signIn("other", "phone")
	var item BaseItemDto
	tr.get(t, "/Items/"+tr.movie, token, &item)
	if item.UserData.Played || item.UserData.PlayCount != 0 || item.UserData.LastPlayedDate != nil {
		t.Errorf("another user sees %+v", item.UserData)
	}
}

func TestMarkingASeriesPlayedMarksItsEpisodes(t *testing.T) {
	tr := newTracking(t)
	data := tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.series)
	if !data.Played || data.UnplayedItemCount == nil || *data.UnplayedItemCount != 0 || data.ItemId != tr.series {
		t.Errorf("series marked played: %+v", data)
	}
	var page QueryResult
	tr.get(t, "/Shows/"+tr.series+"/Episodes", tr.token, &page)
	for _, episode := range page.Items {
		if !episode.UserData.Played {
			t.Errorf("episode %s not played", episode.Name)
		}
	}
	// Unmarking one episode leaves its season and series with one to play.
	tr.mark(t, http.MethodDelete, "/UserPlayedItems/"+tr.episodes[0])
	tr.get(t, "/Shows/"+tr.series+"/Seasons", tr.token, &page)
	first := page.Items[0].UserData
	if first.Played || first.UnplayedItemCount == nil || *first.UnplayedItemCount != 1 || first.PlayedPercentage == nil || *first.PlayedPercentage != 50 {
		t.Errorf("first season: %+v", first)
	}
	if series := tr.userData(t, tr.series); series.Played || *series.UnplayedItemCount != 1 {
		t.Errorf("series: %+v", series)
	}
	data = tr.mark(t, http.MethodDelete, "/Users/"+tr.user+"/PlayedItems/"+tr.seasons[1])
	if data.Played || *data.UnplayedItemCount != 1 {
		t.Errorf("second season unmarked: %+v", data)
	}
}

func TestNextUpFollowsTheFurthestPlayedEpisode(t *testing.T) {
	tr := newTracking(t)
	nextUp := func(query string) []string { return tr.list(t, "/Shows/NextUp?"+query) }
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("before watching: %v", got)
	}
	// A series asked for opens on its first episode.
	if got := nextUp("seriesId=" + tr.series); !slices.Equal(got, tr.episodes[:1]) {
		t.Errorf("series never watched: %v", got)
	}
	// The last episode of a season leads to the next season, even when an
	// earlier episode is played afterwards.
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[1])
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])
	if got := nextUp(""); !slices.Equal(got, tr.episodes[2:3]) {
		t.Errorf("after the first season: %v", got)
	}
	// Only libraries narrow Next Up down: a series does not.
	if got := nextUp("parentId=" + tr.views["Shows"]); !slices.Equal(got, tr.episodes[2:3]) {
		t.Errorf("in the series library: %v", got)
	}
	for _, parent := range []string{tr.views["Top"], tr.series} {
		if got := nextUp("parentId=" + parent); len(got) != 0 {
			t.Errorf("under %s: %v", parent, got)
		}
	}
	// jellyfin-web's home page sends a date only; series played before it
	// are left out.
	if got := nextUp("nextUpDateCutoff=2099-01-01"); len(got) != 0 {
		t.Errorf("after the cutoff: %v", got)
	}
	if got := nextUp("nextUpDateCutoff=2020-01-01&enableTotalRecordCount=false"); !slices.Equal(got, tr.episodes[2:3]) {
		t.Errorf("before the cutoff: %v", got)
	}
	var page QueryResult
	tr.get(t, "/Shows/NextUp?enableTotalRecordCount=false", tr.token, &page)
	if page.TotalRecordCount != 0 {
		t.Errorf("total without counting: %d", page.TotalRecordCount)
	}
	// An episode under way is next up, unless resumable ones are left out:
	// then the series is.
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.episodes[2], "PositionTicks": 10_000_000_000})
	if got := nextUp("enableResumable=false"); len(got) != 0 {
		t.Errorf("without resumable episodes: %v", got)
	}
	if got := nextUp(""); !slices.Equal(got, tr.episodes[2:3]) {
		t.Errorf("with the episode under way: %v", got)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[2])
	if got := nextUp(""); len(got) != 0 {
		t.Errorf("after the last episode: %v", got)
	}
}

func TestPlayedMarksCountPlays(t *testing.T) {
	tr := newTracking(t)
	first := tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie)
	again := tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie)
	if first.PlayCount != 1 || again.PlayCount != 1 || first.LastPlayedDate == nil || again.LastPlayedDate == nil ||
		time.Time(*again.LastPlayedDate) != time.Time(*first.LastPlayedDate) {
		t.Errorf("undated marks: %+v then %+v", first, again)
	}
	// jellyfin-web dates its marks: each one counts.
	dated := tr.mark(t, http.MethodPost, "/Users/"+tr.user+"/PlayedItems/"+tr.movie+"?DatePlayed=2021-05-06")
	if dated.PlayCount != 2 || time.Time(*dated.LastPlayedDate) != time.Date(2021, 5, 6, 0, 0, 0, 0, time.UTC) {
		t.Errorf("dated mark: %+v", dated)
	}
	if unplayed := tr.mark(t, http.MethodDelete, "/UserPlayedItems/"+tr.movie); unplayed.Played || unplayed.PlayCount != 0 || unplayed.LastPlayedDate != nil {
		t.Errorf("unmarked: %+v", unplayed)
	}
}

func TestFavoritesAreListed(t *testing.T) {
	tr := newTracking(t)
	if data := tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+tr.movie); !data.IsFavorite {
		t.Errorf("favorite: %+v", data)
	}
	tr.mark(t, http.MethodPost, "/Users/"+tr.user+"/FavoriteItems/"+tr.series)
	all := "/Items?Filters=IsFavorite&Recursive=true"
	if got := tr.list(t, all+"&IncludeItemTypes=Movie"); !slices.Equal(got, []string{tr.movie}) {
		t.Errorf("favorite movies: %v", got)
	}
	if got := tr.list(t, all+"&IncludeItemTypes=Series&ParentId="+tr.views["Shows"]); !slices.Equal(got, []string{tr.series}) {
		t.Errorf("favorite series of the series library: %v", got)
	}
	if got := tr.list(t, all+"&ParentId="+tr.views["Top"]); !slices.Equal(got, []string{tr.movie}) {
		t.Errorf("favorites of the movie library: %v", got)
	}
	tr.mark(t, http.MethodDelete, "/UserFavoriteItems/"+tr.movie)
	if got := tr.list(t, "/Items?isFavorite=true&Recursive=true"); !slices.Equal(got, []string{tr.series}) {
		t.Errorf("after removing the movie: %v", got)
	}
}

func TestPlayedTitlesLeaveUnplayedListings(t *testing.T) {
	tr := newTracking(t)
	before := tr.list(t, "/Items?ParentId="+tr.views["Top"])
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie)
	unplayed := tr.list(t, "/Items?Filters=IsUnplayed&ParentId="+tr.views["Top"])
	if want := slices.DeleteFunc(slices.Clone(before), func(id string) bool { return id == tr.movie }); !slices.Equal(unplayed, want) {
		t.Errorf("unplayed: %v, want %v", unplayed, want)
	}
	if played := tr.list(t, "/Items?Filters=IsPlayed&Recursive=true&IncludeItemTypes=Movie"); !slices.Equal(played, []string{tr.movie}) {
		t.Errorf("played: %v", played)
	}
	var latest []BaseItemDto
	tr.get(t, "/Items/Latest?ParentId="+tr.views["Top"], tr.token, &latest)
	if slices.Contains(itemIDs(latest), tr.movie) {
		t.Error("a played title is among the latest")
	}
}

func TestUserDataCanBeReadAndChanged(t *testing.T) {
	tr := newTracking(t)
	// Apps upload what was played offline as it is: a position near the end
	// stays a position.
	status, body := tr.call(http.MethodPost, "/UserItems/"+tr.movie+"/UserData", app("tv", tr.token),
		map[string]any{"PlaybackPositionTicks": 52_000_000_000, "IsFavorite": true, "PlayedPercentage": 12.5, "LastPlayedDate": nil})
	var data UserItemData
	if status != http.StatusOK {
		t.Fatalf("update: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	if data.PlaybackPositionTicks != 52_000_000_000 || !data.IsFavorite || data.Played || data.PlayCount != 0 || data.LastPlayedDate != nil {
		t.Errorf("updated: %+v", data)
	}
	// The movie runs 1h30.
	var read UserItemData
	if status := tr.send(t, http.MethodGet, "/UserItems/"+tr.movie+"/UserData", &read); status != http.StatusOK ||
		read.PlaybackPositionTicks != data.PlaybackPositionTicks || !read.IsFavorite || read.PlayedPercentage == nil ||
		int(*read.PlayedPercentage) != 96 {
		t.Errorf("read back: %d %+v", status, read)
	}
	if liked := tr.mark(t, http.MethodPost, "/UserItems/"+tr.movie+"/Rating?likes=true"); liked.Likes == nil || !*liked.Likes || *liked.Rating != 10 {
		t.Errorf("liked: %+v", liked)
	}
	if disliked := tr.mark(t, http.MethodPost, "/Users/"+tr.user+"/Items/"+tr.movie+"/Rating?likes=false"); *disliked.Likes || *disliked.Rating != 1 {
		t.Errorf("disliked: %+v", disliked)
	}
	if forgotten := tr.mark(t, http.MethodPost, "/UserItems/"+tr.movie+"/Rating"); forgotten.Likes != nil || forgotten.Rating != nil {
		t.Errorf("rating removed: %+v", forgotten)
	}
}

func TestUserDataRequestsAreChecked(t *testing.T) {
	tr := newTracking(t)
	for _, path := range []string{"/UserPlayedItems/00000000000000000000000000000001", "/UserFavoriteItems/00000000000000000000000000000001"} {
		if status := tr.send(t, http.MethodPost, path, nil); status != http.StatusNotFound {
			t.Errorf("%s: %d", path, status)
		}
	}
	tr.testServer.user("other", nil)
	other, _ := tr.store.Authenticate(t.Context(), "other", "correct horse")
	for _, path := range []string{"/UserFavoriteItems/" + tr.movie + "?userId=" + other.ID.String(), "/Users/" + other.ID.String() + "/PlayedItems/" + tr.movie} {
		if status, body := tr.call(http.MethodPost, path, app("tv", tr.token), nil); status != http.StatusForbidden || string(body) != "Error processing request." {
			t.Errorf("%s for another user: %d %s", path, status, body)
		}
	}
	if status, body := tr.call(http.MethodPost, "/UserItems/"+tr.movie+"/Rating?likes=maybe", app("tv", tr.token), nil); status != http.StatusBadRequest {
		t.Errorf("likes=maybe: %d %s", status, body)
	}
}

func TestFavoritePeopleAreListed(t *testing.T) {
	tr := newTracking(t)
	var movie BaseItemDto
	tr.get(t, "/Users/"+tr.user+"/Items/"+tr.movie, tr.token, &movie)
	actor := (*movie.People)[0]
	tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+actor.Id)
	tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+tr.movie)
	if got := tr.list(t, "/Persons?isFavorite=true&userId="+tr.user); !slices.Equal(got, []string{actor.Id}) {
		t.Errorf("favorite people: %v", got)
	}
	if got := tr.list(t, "/Persons"); len(got) != 0 {
		t.Errorf("people: %v", got)
	}
}

// TestUserDataResponsesMatchJellyfin compares user data responses with those
// recorded from Jellyfin 12.1 by scripts/jellyfin-fixtures.sh, after the same
// marks and resume points.
func TestUserDataResponsesMatchJellyfin(t *testing.T) {
	tr := newTracking(t)
	user, signed := tr.user, app("tv", tr.token)
	var page QueryResult
	tr.get(t, "/Items?ParentId="+tr.views["Top"], tr.token, &page)
	favorite, unplayed := page.Items[1].Id, page.Items[2].Id
	ago := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }
	// A resume point at 40% of the item's runtime, last played some time ago.
	resumeAt := func(id string, d time.Duration) {
		t.Helper()
		var item BaseItemDto
		tr.get(t, "/Users/"+user+"/Items/"+id, tr.token, &item)
		if status, body := tr.call(http.MethodPost, "/UserItems/"+id+"/UserData?userId="+user, signed,
			map[string]any{"PlaybackPositionTicks": *item.RunTimeTicks * 2 / 5, "LastPlayedDate": ago(d)}); status != http.StatusOK {
			t.Fatalf("resume point: %d %s", status, body)
		}
	}
	_, played := tr.call(http.MethodPost, "/UserPlayedItems/"+tr.movie+"?userId="+user, signed, nil)
	_, favorited := tr.call(http.MethodPost, "/UserFavoriteItems/"+favorite+"?userId="+user, signed, nil)
	tr.call(http.MethodPost, "/Users/"+user+"/PlayedItems/"+tr.episodes[0]+"?DatePlayed="+ago(2*time.Hour), signed, nil)
	resumeAt(unplayed, time.Hour)
	resumeAt(tr.episodes[0], 2*time.Minute)
	resumeAt(tr.episodes[2], time.Minute)
	_, resume := tr.call(http.MethodGet, "/UserItems/Resume?userId="+user+"&limit=12&fields=PrimaryImageAspectRatio&mediaTypes=Video"+
		"&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb&enableTotalRecordCount=false", signed, nil)
	_, nextUp := tr.call(http.MethodGet, "/Shows/NextUp?userId="+user+"&limit=24&fields=PrimaryImageAspectRatio&fields=DateCreated&fields=Path"+
		"&fields=MediaSourceCount&imageTypeLimit=1&enableImageTypes=Primary&enableImageTypes=Backdrop&enableImageTypes=Thumb"+
		"&nextUpDateCutoff="+time.Now().AddDate(-1, 0, 0).Format("2006-01-02")+"&enableTotalRecordCount=false&enableResumable=false&enableRewatching=false", signed, nil)
	_, favorites := tr.call(http.MethodGet, "/Users/"+user+"/Items?SortBy=SeriesSortName%2CSortName&SortOrder=Ascending&Filters=IsFavorite"+
		"&Recursive=true&Fields=PrimaryImageAspectRatio&CollapseBoxSetItems=false&ExcludeLocationTypes=Virtual&EnableTotalRecordCount=false"+
		"&Limit=20&IncludeItemTypes=Movie", signed, nil)
	_, series := tr.call(http.MethodGet, "/Users/"+user+"/Items/"+tr.series, signed, nil)
	_, episodes := tr.call(http.MethodGet, "/Shows/"+tr.series+"/Episodes?userId="+user+
		"&Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount,Overview", signed, nil)
	for fixture, body := range map[string][]byte{
		"user-item-data":       played,
		"favorite-user-data":   favorited,
		"resume-items":         resume,
		"next-up":              nextUp,
		"favorites":            favorites,
		"series-in-progress":   series,
		"episodes-in-progress": episodes,
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: %v in %s", fixture, err, body)
		}
		for _, difference := range compareShapes(fixture, want, got, userDataShapes) {
			t.Error(difference)
		}
	}
}

// userDataShapes lists what Polyfin cannot match in the user data fixtures.
var userDataShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files, and the test addon has no streams.
		"Path": {"*"}, "Container": {"*"},
		// Metadata addons do not provide these.
		"OriginalLanguage": {"*"}, "CommunityRating": {"episodes-in-progress", "next-up"},
		// The test addon gives these episodes no still.
		"PrimaryImageAspectRatio": {"next-up", "resume-items"},
	},
	returned: map[string][]string{
		// The recorded episode had no air date.
		"PremiereDate": {"resume-items"}, "ProductionYear": {"resume-items"},
	},
}
