package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestUsersWhoManageSubtitlesAddFilesAdministratorsDeleteThem(t *testing.T) {
	p := playing(t)
	p.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	adminToken := p.signIn("admin", "phone")
	member := p.user
	upload := func(token string, body any) (int, []byte) {
		t.Helper()
		return p.call(http.MethodPost, "/Videos/"+p.movie+"/Subtitles", app("tv", token), body)
	}
	file := map[string]any{"Language": "eng", "Format": "SRT", "IsForced": true, "IsHearingImpaired": false,
		"Data": base64.StdEncoding.EncodeToString([]byte("1\r\n00:00:01,000 --> 00:00:03,000\r\nHello there\r\n"))}

	// Like Jellyfin, only users who may manage subtitles add files; a new
	// user may not.
	if status, _ := upload(p.token, file); status != http.StatusForbidden {
		t.Fatalf("member without the permission: %d", status)
	}
	var dto struct{ Policy map[string]any }
	p.get(t, "/Users/"+member.ID.String(), adminToken, &dto)
	if dto.Policy["EnableSubtitleManagement"] != false {
		t.Fatalf("a new user's policy: %v", dto.Policy["EnableSubtitleManagement"])
	}
	dto.Policy["EnableSubtitleManagement"] = true
	encoded, _ := json.Marshal(dto.Policy)
	if status, body := p.postRaw("/Users/"+member.ID.String()+"/Policy", app("phone", adminToken), string(encoded)); status != http.StatusNoContent {
		t.Fatalf("policy: %d %s", status, body)
	}
	if stored, _ := p.store.User(t.Context(), member.ID); !stored.SubtitleManagement {
		t.Fatal("the policy did not grant the permission")
	}

	// What Polyfin cannot keep is refused.
	for name, change := range map[string]func(map[string]any){
		"an image format":  func(f map[string]any) { f["Format"] = "sup" },
		"not base64":       func(f map[string]any) { f["Data"] = "%%%" },
		"not subtitles":    func(f map[string]any) { f["Data"] = base64.StdEncoding.EncodeToString([]byte("hello")) },
		"without language": func(f map[string]any) { f["Language"] = "" },
	} {
		bad := map[string]any{}
		for key, value := range file {
			bad[key] = value
		}
		change(bad)
		if status, body := upload(p.token, bad); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	if status, _ := p.call(http.MethodPost, "/Videos/"+strings.Repeat("ab", 16)+"/Subtitles", app("tv", p.token), file); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}

	if status, body := upload(p.token, file); status != http.StatusNoContent {
		t.Fatalf("upload: %d %s", status, body)
	}
	// The file comes first among the subtitles of every version, before
	// the addon's.
	streams := func() []string {
		t.Helper()
		status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{"UserId": member.ID.String()})
		var info playbackInfoResponse
		if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo: %d %s", status, body)
		}
		var described []string
		for _, source := range info.MediaSources {
			for _, stream := range source.MediaStreams {
				if stream.IsExternal {
					described = append(described, stream.Codec+" "+stream.Language+" "+map[bool]string{true: "forced", false: "-"}[stream.IsForced])
				}
			}
		}
		return described
	}
	// The version PlaybackInfo picks, the analyzed one, as any other.
	if got, want := strings.Join(streams(), ", "), "subrip eng forced, subrip fra -"; got != want {
		t.Errorf("streams: %s\nwant %s", got, want)
	}
	status, _, text := p.send(http.MethodGet, "/Videos/"+p.movie+"/"+p.versions[1].ID.String()+"/Subtitles/0/0/Stream.srt?ApiKey="+p.token, "", "", "")
	if status != http.StatusOK || !strings.Contains(string(text), "Hello there") {
		t.Errorf("uploaded file: %d %q", status, text)
	}

	// Only administrators delete, and only files users added.
	remove := func(token, index string) int {
		status, _ := p.call(http.MethodDelete, "/Videos/"+p.movie+"/Subtitles/"+index, app("tv", token), nil)
		return status
	}
	if status := remove(p.token, "0"); status != http.StatusForbidden {
		t.Errorf("member deleting: %d", status)
	}
	if status := remove(adminToken, "1"); status != http.StatusBadRequest {
		t.Errorf("deleting the addon's file: %d", status)
	}
	if status := remove(adminToken, "zero"); status != http.StatusBadRequest {
		t.Errorf("deleting no index: %d", status)
	}
	if status := remove(adminToken, "0"); status != http.StatusNoContent {
		t.Fatalf("deleting: %d", status)
	}
	if got := strings.Join(streams(), ", "); got != "subrip fra -" {
		t.Errorf("after deleting: %s", got)
	}
	if status := remove(adminToken, "0"); status != http.StatusBadRequest {
		t.Errorf("deleting again: %d", status)
	}
}

func TestRefreshDropsWhatPolyfinKeepsOfTheTitle(t *testing.T) {
	s := newTestServer(t, 10)
	var metas, streams atomic.Int32
	var certification atomic.Value
	certification.Store("PG")
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		movie := stremio.Meta{ID: "tt5000", Type: "movie", Name: "Heist", Genres: []string{"Drama"},
			Extras: &stremio.Extras{Certification: certification.Load().(string)}}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "counted", Name: "Counted", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt5000.json":
			metas.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt5000.json":
			streams.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "1080p", URL: "https://example.com/heist.mkv"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	installed, err := s.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	s.user("member", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token, adminToken := s.signIn("member", "tv"), s.signIn("admin", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	open := func() BaseItemDto {
		t.Helper()
		var item BaseItemDto
		if status := s.get(t, "/Items/"+movie+"?fields=MediaSources", token, &item); status != http.StatusOK {
			t.Fatalf("item: %d", status)
		}
		return item
	}
	open()
	open()
	if metas.Load() != 1 || streams.Load() != 1 {
		t.Fatalf("before refreshing, the addon was asked %d descriptions and %d version lists", metas.Load(), streams.Load())
	}
	certification.Store("R")
	if status, _ := s.call(http.MethodPost, "/Items/"+movie+"/Refresh", app("tv", token), nil); status != http.StatusForbidden {
		t.Errorf("member refreshing: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Items/"+movie+"/Refresh?metadataRefreshMode=FullRefresh&replaceAllMetadata=true", app("tv", adminToken), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d", status)
	}
	// The description is asked for again at once, with the rating.
	if metas.Load() != 2 {
		t.Errorf("descriptions after refreshing: %d", metas.Load())
	}
	var rating string
	if err := s.pool.QueryRow(t.Context(), "SELECT data->>'rating' FROM items WHERE id = $1", movie).Scan(&rating); err != nil || rating != "R" {
		t.Errorf("kept rating: %q %v", rating, err)
	}
	if item := open(); item.OfficialRating != "R" || streams.Load() != 2 {
		t.Errorf("after refreshing: rating %q, %d version lists", item.OfficialRating, streams.Load())
	}
	for path, want := range map[string]int{
		"/Items/" + strings.Repeat("ab", 16) + "/Refresh":             http.StatusNotFound,
		"/Items/" + movie + "/Refresh?metadataRefreshMode=Everything": http.StatusBadRequest,
		"/Items/" + views.Items[0].Id + "/Refresh":                    http.StatusNoContent,
	} {
		if status, body := s.call(http.MethodPost, path, app("tv", adminToken), nil); status != want {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
}
