package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// guidedTV is a TV addon whose second channel takes an XMLTV guide, with
// an icon: Native EPG programmes on One, the guide's on Two.
func guidedTV(t *testing.T) (testServer, string, string) {
	t.Helper()
	addon := newTVAddon(t, true, "")
	s, token, user := tuned(t, addon)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><tv>
  <channel id="two-x"><display-name>Two</display-name></channel>
  <programme channel="two-x" start="%s" stop="%s"><title>Talk Show</title><icon src="ICON"/><category>Sports</category></programme>
  <programme channel="two-x" start="%s" stop="%s"><title>Late Movie</title><category>Movie</category></programme>
</tv>`, at(-time.Hour), at(time.Hour), at(time.Hour), at(3*time.Hour))
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.ReplaceAll(body, "ICON", "http://"+r.Host+"/talk.png"))
	}))
	t.Cleanup(guide.Close)
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil || len(libraries) != 1 {
		t.Fatal(libraries, err)
	}
	key := addons.LibraryKey{AddonID: libraries[0].AddonID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := s.addons.SetGuide(t.Context(), addons.Shared(), key, guide.URL+"/epg"); err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	return s, token, user.ID.String()
}

// The theme asks each view for its collections: Polyfin's own views, the
// Live TV view among them, list none, as they did, without looking them
// up as items.
func TestOwnViewsListNoCollections(t *testing.T) {
	t.Parallel()
	s, token, user := guidedTV(t)
	for name, view := range map[string]string{"Live TV": liveTvViewID.String(), "Playlists": playlistsViewID.String()} {
		status, body := s.call(http.MethodGet, "/Users/"+user+"/Items?ParentId="+view+
			"&IncludeItemTypes=BoxSet&EnableImages=false&EnableUserData=false&SortBy=Random&Limit=100", app("web", token), nil)
		var listed QueryResult
		if status != http.StatusOK || json.Unmarshal(body, &listed) != nil || len(listed.Items) != 0 || listed.TotalRecordCount != 0 {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	// The recordings folder of a server that records nothing is refused,
	// as it was.
	status, body := s.call(http.MethodGet, "/Users/"+user+"/Items?ParentId="+recordingsFolderID.String()+
		"&IncludeItemTypes=BoxSet&SortBy=Random&Limit=100", app("web", token), nil)
	if status != http.StatusBadRequest {
		t.Errorf("Recordings: %d %s", status, body)
	}
}

// A page of channels lists what the whole listing lists there, and only
// its channels are recorded; the favorites are those of every channel.
func TestChannelPagesRecordTheirChannels(t *testing.T) {
	t.Parallel()
	s, token, user := guidedTV(t)
	channelRecords := func() int {
		var n int
		if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM items WHERE kind = 'channel'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var page QueryResult
	s.get(t, "/LiveTv/Channels?StartIndex=1&Limit=1&AddCurrentProgram=false&EnableUserData=false", token, &page)
	if len(page.Items) != 1 || page.TotalRecordCount != 2 || page.StartIndex != 1 || page.Items[0].Name != "Two" {
		t.Fatalf("second page: %+v", page)
	}
	if n := channelRecords(); n != 1 {
		t.Errorf("%d channels recorded for a page of one", n)
	}
	var all QueryResult
	s.get(t, "/LiveTv/Channels", token, &all)
	if len(all.Items) != 2 || all.Items[1].Id != page.Items[0].Id || all.Items[1].CurrentProgram == nil ||
		all.Items[1].CurrentProgram.Name != "Talk Show" {
		t.Fatalf("every channel: %+v", all.Items)
	}
	if n := channelRecords(); n != 2 {
		t.Errorf("%d channels recorded for both", n)
	}
	if status, _ := s.call(http.MethodPost, "/Users/"+user+"/FavoriteItems/"+all.Items[1].Id, app("web", token), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	var favorites QueryResult
	s.get(t, "/LiveTv/Channels?isFavorite=true", token, &favorites)
	if len(favorites.Items) != 1 || favorites.TotalRecordCount != 1 || favorites.Items[0].Id != all.Items[1].Id || !favorites.Items[0].UserData.IsFavorite {
		t.Errorf("favorites: %+v", favorites.Items)
	}
	s.get(t, "/LiveTv/Channels?StartIndex=1", token, &page)
	if len(page.Items) != 1 || !page.Items[0].UserData.IsFavorite || page.Items[0].CurrentProgram == nil {
		t.Errorf("a page's user data and programme: %+v", page.Items)
	}
}

// On Now with a limit lists the first channels' programmes airing now,
// as the whole listing does, and counts, like Jellyfin, at most the
// programmes it reads.
func TestOnNowListsTheFirstChannels(t *testing.T) {
	t.Parallel()
	s, token, _ := guidedTV(t)
	var all QueryResult
	airingNow(t, s, token, "Now", "Talk Show")
	s.get(t, "/LiveTv/Programs/Recommended?isAiring=true&fields=ChannelInfo", token, &all)
	if names := programNames(all); !slices.Equal(names, []string{"Now", "Talk Show"}) || all.TotalRecordCount != 2 {
		t.Fatalf("airing now: %q, %d", names, all.TotalRecordCount)
	}
	for _, query := range []string{"limit=1", "limit=1&enableTotalRecordCount=false", "limit=1&startIndex=1", "limit=24"} {
		var page QueryResult
		s.get(t, "/LiveTv/Programs/Recommended?isAiring=true&fields=ChannelInfo&"+query, token, &page)
		start := 0
		if strings.Contains(query, "startIndex=1") {
			start = 1
		}
		want := programNames(QueryResult{Items: all.Items[start:min(start+1, len(all.Items))]})
		if query == "limit=24" {
			want = programNames(all)
		}
		if got := programNames(page); !slices.Equal(got, want) || page.TotalRecordCount != 2 || page.Items[0].ChannelName != all.Items[start].ChannelName {
			t.Errorf("%s: %q (%d), want %q", query, got, page.TotalRecordCount, want)
		}
	}
}

// Like Jellyfin's, a programme listing without EnableImages has no image
// tags, and one without EnableUserData no user data; both are given by
// default. jellyfin-web's guide asks for neither.
func TestProgrammesFollowEnableImagesAndEnableUserData(t *testing.T) {
	t.Parallel()
	s, token, _ := guidedTV(t)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	two := channels.Items[1].Id
	now := time.Now().UTC()
	listing := func(options map[string]any) []map[string]json.RawMessage {
		t.Helper()
		query := map[string]any{"channelIds": two, "MinEndDate": now.Format(time.RFC3339), "MaxStartDate": now.Add(24 * time.Hour).Format(time.RFC3339),
			"SortBy": "StartDate"}
		for key, value := range options {
			query[key] = value
		}
		status, body := s.call(http.MethodPost, "/LiveTv/Programs", app("web", token), query)
		var result struct {
			Items            []map[string]json.RawMessage
			TotalRecordCount int
		}
		if status != http.StatusOK || json.Unmarshal(body, &result) != nil || len(result.Items) != 2 || result.TotalRecordCount != 2 {
			t.Fatalf("%v: %d %s", options, status, body)
		}
		return result.Items
	}
	lean := listing(map[string]any{"EnableImages": false, "EnableUserData": false, "ImageTypeLimit": 1, "EnableTotalRecordCount": false})
	full := listing(nil)
	for i := range lean {
		for _, key := range []string{"ImageTags", "BackdropImageTags", "UserData"} {
			if _, ok := lean[i][key]; ok {
				t.Errorf("without images nor user data, %s: %s", key, lean[i][key])
			}
		}
		if string(lean[i]["ImageBlurHashes"]) != "{}" {
			t.Errorf("ImageBlurHashes, which Jellyfin always gives: %s", lean[i]["ImageBlurHashes"])
		}
		// The rest is the same.
		for key, value := range full[i] {
			if key != "ImageTags" && key != "BackdropImageTags" && key != "UserData" && string(lean[i][key]) != string(value) {
				t.Errorf("%s: %s, %s by default", key, lean[i][key], value)
			}
		}
		if len(lean[i]) != len(full[i])-3 {
			t.Errorf("fields without images nor user data %d, by default %d", len(lean[i]), len(full[i]))
		}
	}
	var tags map[string]string
	if err := json.Unmarshal(full[0]["ImageTags"], &tags); err != nil || tags["Primary"] == "" || full[0]["UserData"] == nil {
		t.Errorf("by default, images and user data: %s %s", full[0]["ImageTags"], full[0]["UserData"])
	}
	// Each alone.
	if items := listing(map[string]any{"EnableImages": false}); items[0]["ImageTags"] != nil || items[0]["UserData"] == nil {
		t.Errorf("without images: %v", items[0])
	}
	if items := listing(map[string]any{"EnableUserData": false}); items[0]["ImageTags"] == nil || items[0]["UserData"] != nil {
		t.Errorf("without user data: %v", items[0])
	}
	// The same through the query string.
	var result struct{ Items []map[string]json.RawMessage }
	s.get(t, "/LiveTv/Programs?channelIds="+two+"&enableImages=false", token, &result)
	if len(result.Items) == 0 || result.Items[0]["ImageTags"] != nil {
		t.Errorf("GET without images: %v", result.Items)
	}
}
