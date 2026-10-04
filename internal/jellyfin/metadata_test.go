package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestMetadataEditorAnswersAsJellyfinForAdministrators(t *testing.T) {
	e := newEdited(t)
	allowed := e.ids["Allowed"]
	status, body := e.call(http.MethodGet, "/Items/"+allowed+"/MetadataEditor", app("dashboard", e.admin), nil)
	if status != http.StatusOK {
		t.Fatalf("metadata editor: %d %s", status, body)
	}
	matchesFixture(t, "metadata-editor", body, shapeRules{})
	var info MetadataEditorInfo
	_ = json.Unmarshal(body, &info)
	if len(info.ParentalRatingOptions) == 0 || len(info.Countries) == 0 || len(info.Cultures) == 0 ||
		!slices.IsSortedFunc(info.Cultures, func(a, b CultureDto) int { return strings.Compare(a.DisplayName, b.DisplayName) }) {
		t.Errorf("lists: %d ratings, %d countries, %d cultures", len(info.ParentalRatingOptions), len(info.Countries), len(info.Cultures))
	}
	status, body = e.call(http.MethodGet, "/Items/"+allowed+"/ExternalIdInfos", app("dashboard", e.admin), nil)
	if status != http.StatusOK {
		t.Fatalf("external ids: %d %s", status, body)
	}
	matchesFixture(t, "external-id-infos", body, shapeRules{})
	for path, want := range map[string]string{
		"/Items/" + allowed + "/RemoteImages/Providers":                    `[]`,
		"/Items/" + allowed + "/RemoteImages?type=Primary&limit=10":        `{"Images":[],"TotalRecordCount":0,"Providers":[]}`,
		"/Items/" + e.ids["Show"] + "/RemoteImages/Providers":              `[]`,
		"/Items/" + e.ids["Top"] + "/MetadataEditor?unused=1":              "",
		"/Items/" + collectionsViewID.String() + "/RemoteImages/Providers": "",
	} {
		status, body := e.call(http.MethodGet, path, app("dashboard", e.admin), nil)
		if status != http.StatusOK || want != "" && strings.TrimSpace(string(body)) != want {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	// Members may not edit, as in Jellyfin; unknown items are not found.
	for _, path := range []string{"/Items/" + allowed + "/MetadataEditor", "/Items/" + allowed + "/ExternalIdInfos"} {
		if status, body := e.call(http.MethodGet, path, app("tv", e.member), nil); status != http.StatusForbidden || len(body) > 0 {
			t.Errorf("member %s: %d %s", path, status, body)
		}
	}
	if status, _ := e.call(http.MethodPost, "/Items/"+allowed, app("tv", e.member), map[string]any{"Name": "Mine"}); status != http.StatusForbidden {
		t.Errorf("member edit: %d", status)
	}
	if status, _ := e.call(http.MethodGet, "/Items/0123456789abcdef0123456789abcdef/MetadataEditor", app("dashboard", e.admin), nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
}

// editorBody is what jellyfin-web's metadata editor posts: every field of
// its form, as text.
func editorBody(id string, fields map[string]any) map[string]any {
	body := map[string]any{"Id": id, "Name": "", "OriginalTitle": "", "ForcedSortName": "", "CommunityRating": "", "CriticRating": "",
		"Overview": "", "Genres": []string{}, "Tags": []string{}, "Studios": []map[string]string{}, "PremiereDate": nil, "EndDate": nil,
		"ProductionYear": "", "OfficialRating": "", "CustomRating": "", "Taglines": []string{}, "People": []any{},
		"LockData": false, "LockedFields": []string{}, "ProviderIds": map[string]string{"Imdb": "tt1"}, "DateCreated": "1970-01-01T00:00:00.000Z"}
	for key, value := range fields {
		body[key] = value
	}
	return body
}

func TestMetadataEditsWinForEveryUserUntilCleared(t *testing.T) {
	e := newEdited(t)
	allowed, restricted := e.ids["Allowed"], e.ids["Restricted"]
	if status, _ := e.item(t, e.child, e.childID, allowed); status != http.StatusOK {
		t.Fatalf("the child sees the PG title: %d", status)
	}
	if status, _ := e.item(t, e.child, e.childID, restricted); status != http.StatusNotFound {
		t.Fatalf("the child sees the R title: %d", status)
	}
	edit := editorBody(allowed, map[string]any{"Name": "Renamed", "ForcedSortName": "aaa", "Overview": "Edited overview",
		"CommunityRating": "7.5", "CriticRating": "88", "OfficialRating": "R", "Genres": []string{"Comedy", "comedy", " Drama "},
		"Tags": []string{"night"}, "Studios": []map[string]string{{"Name": "North"}}, "Taglines": []string{"A line"},
		"PremiereDate": "2001-02-03", "ProductionYear": "2001"})
	if status, body := e.call(http.MethodPost, "/Items/"+allowed, app("dashboard", e.admin), edit); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	// Restricted is rated PG by the administrator: the child may see it.
	if status, body := e.call(http.MethodPost, "/Items/"+restricted, app("dashboard", e.admin),
		editorBody(restricted, map[string]any{"Name": "Restricted", "OfficialRating": "PG"})); status != http.StatusNoContent {
		t.Fatalf("rating edit: %d %s", status, body)
	}

	status, dto := e.item(t, e.member, e.memberID, allowed)
	if status != http.StatusOK || dto.Name != "Renamed" || dto.SortName != "aaa" || dto.ForcedSortName != "aaa" ||
		dto.Overview == nil || *dto.Overview != "Edited overview" || dto.CommunityRating == nil || *dto.CommunityRating != 7.5 ||
		dto.CriticRating == nil || *dto.CriticRating != 88 || dto.OfficialRating != "R" || dto.ProductionYear == nil || *dto.ProductionYear != 2001 ||
		dto.PremiereDate == nil || !strings.HasPrefix(string(mustJSON(t, dto.PremiereDate)), `"2001-02-03`) ||
		!slices.Equal(*dto.Genres, []string{"Comedy", "Drama"}) || !slices.Equal(*dto.Tags, []string{"night"}) ||
		!slices.Equal(*dto.Taglines, []string{"A line"}) || len(*dto.Studios) != 1 || (*dto.Studios)[0].Name != "North" ||
		dto.OriginalTitle != "Allowed" || (*dto.ProviderIds)["Imdb"] != "tt1" {
		t.Errorf("member's view of the edit: %d %s", status, mustJSON(t, dto))
	}
	if names := e.listed(t, e.member, "Top"); !slices.Contains(names, "Renamed") || slices.Contains(names, "Allowed") {
		t.Errorf("member's listing: %v", names)
	}
	// Searches find the title by its new name, which the addon does not know.
	var found QueryResult
	e.get(t, "/Items?userId="+e.memberID+"&recursive=true&searchTerm=renamed&includeItemTypes=Movie", e.member, &found)
	if !slices.Contains(itemNames(found.Items), "Renamed") {
		t.Errorf("search: %v", itemNames(found.Items))
	}
	// Parental control judges by the ratings set.
	if status, _ := e.item(t, e.child, e.childID, allowed); status != http.StatusNotFound {
		t.Errorf("the child sees the title rated R: %d", status)
	}
	if status, _ := e.item(t, e.child, e.childID, restricted); status != http.StatusOK {
		t.Errorf("the child does not see the title rated PG: %d", status)
	}
	if names := e.listed(t, e.child, "Top"); slices.Contains(names, "Renamed") || !slices.Contains(names, "Restricted") {
		t.Errorf("child's listing: %v", names)
	}

	// The editor sends the values it shows: those equal to the addon's are
	// not kept, and cleared fields follow the addon again.
	if status, body := e.call(http.MethodPost, "/Items/"+allowed, app("dashboard", e.admin),
		editorBody(allowed, map[string]any{"Name": "Allowed", "OfficialRating": "PG", "Genres": []string{"Drama"}})); status != http.StatusNoContent {
		t.Fatalf("clear: %d %s", status, body)
	}
	if o := e.library.Overrides(mustID(t, allowed)); o.Name != nil || o.OfficialRating != nil || o.Genres != nil || o.Overview != nil {
		t.Errorf("kept after clearing: %+v", o)
	}
	status, dto = e.item(t, e.member, e.memberID, allowed)
	if status != http.StatusOK || dto.Name != "Allowed" || dto.OfficialRating != "PG" || dto.Overview != nil || dto.CommunityRating != nil ||
		dto.ForcedSortName != "" || len(*dto.Tags) != 0 || !slices.Equal(*dto.Genres, []string{"Drama"}) {
		t.Errorf("after clearing: %d %s", status, mustJSON(t, dto))
	}
	if status, _ := e.item(t, e.child, e.childID, allowed); status != http.StatusOK {
		t.Errorf("the child no longer sees the PG title: %d", status)
	}
}

func TestSeriesEditsShowOnTheirEpisodes(t *testing.T) {
	e := newEdited(t)
	show := e.ids["Show"]
	if status, body := e.call(http.MethodPost, "/Items/"+show, app("dashboard", e.admin),
		editorBody(show, map[string]any{"Name": "Renamed Show", "OfficialRating": "TV-PG"})); status != http.StatusNoContent {
		t.Fatalf("edit: %d %s", status, body)
	}
	var episodes QueryResult
	if status := e.get(t, "/Shows/"+show+"/Episodes?userId="+e.memberID, e.member, &episodes); status != http.StatusOK || len(episodes.Items) == 0 {
		t.Fatalf("episodes: %d", status)
	}
	if episode := episodes.Items[0]; episode.SeriesName != "Renamed Show" || episode.OfficialRating != "TV-PG" {
		t.Errorf("episode: %s", mustJSON(t, episode))
	}
	// The child, kept from TV-MA, may watch the series rated TV-PG.
	if status := e.get(t, "/Shows/"+show+"/Episodes?userId="+e.childID, e.child, &episodes); status != http.StatusOK || len(episodes.Items) == 0 {
		t.Errorf("child's episodes: %d %d", status, len(episodes.Items))
	}
}

func TestUploadedArtworkReplacesTheAddonsUntilDeleted(t *testing.T) {
	e := newEdited(t)
	allowed := e.ids["Allowed"]
	upload := func(token, imageType, contentType string, data []byte) int {
		status, _, _ := e.send(http.MethodPost, "/Items/"+allowed+"/Images/"+imageType, app("dashboard", token), contentType,
			base64.StdEncoding.EncodeToString(data))
		return status
	}
	if status := upload(e.member, "Primary", "image/png", picture(t, 20, 30, 255)); status != http.StatusForbidden {
		t.Errorf("member upload: %d", status)
	}
	if status := upload(e.admin, "Primary", "text/plain", picture(t, 20, 30, 255)); status != http.StatusBadRequest {
		t.Errorf("text upload: %d", status)
	}
	if status := upload(e.admin, "Primary", "image/png", []byte("not a picture")); status != http.StatusBadRequest {
		t.Errorf("not a picture: %d", status)
	}
	if status := upload(e.admin, "Primary", "image/png", picture(t, 4000, 100, 255)); status != http.StatusNoContent {
		t.Fatalf("upload: %d", status)
	}
	if status := upload(e.admin, "Banner", "image/png", picture(t, 10, 10, 128)); status != http.StatusNoContent {
		t.Fatalf("banner upload: %d", status)
	}
	_, dto := e.item(t, e.member, e.memberID, allowed)
	tag := dto.ImageTags["Primary"]
	if tag == "" || dto.ImageTags["Banner"] == "" {
		t.Fatalf("tags: %v", dto.ImageTags)
	}
	response, body := e.fetch(t, e.url+"/Items/"+allowed+"/Images/Primary?tag="+tag)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" || response.Header.Get("ETag") != `"`+tag+`"` {
		t.Fatalf("image: %d %v", response.StatusCode, response.Header)
	}
	config, _, err := image.DecodeConfig(strings.NewReader(string(body)))
	if err != nil || config.Width != 3840 || config.Height != 96 {
		t.Errorf("normalized image: %+v %v", config, err)
	}
	if response, _ := e.fetch(t, e.url+"/Items/"+allowed+"/Images/Banner"); response.Header.Get("Content-Type") != "image/png" {
		t.Errorf("banner: %d %v", response.StatusCode, response.Header)
	}
	var images []ImageInfo
	e.get(t, "/Items/"+allowed+"/Images", e.admin, &images)
	if len(images) != 2 || images[0].ImageType != "Primary" || images[0].ImageTag != tag || images[1].ImageType != "Banner" {
		t.Errorf("image infos: %+v", images)
	}
	// Listings tag the upload too.
	var page QueryResult
	e.get(t, "/Items?ParentId="+e.ids["Top"], e.member, &page)
	if i := slices.IndexFunc(page.Items, func(d BaseItemDto) bool { return d.Id == allowed }); i < 0 || page.Items[i].ImageTags["Primary"] != tag {
		t.Errorf("listing tags: %v", page.Items)
	}

	if status, _ := e.call(http.MethodDelete, "/Items/"+allowed+"/Images/Primary", app("tv", e.member), nil); status != http.StatusForbidden {
		t.Errorf("member delete: %d", status)
	}
	if status, _ := e.call(http.MethodDelete, "/Items/"+allowed+"/Images/Primary", app("dashboard", e.admin), nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	_, dto = e.item(t, e.member, e.memberID, allowed)
	if dto.ImageTags["Primary"] != "" || dto.ImageTags["Banner"] == "" {
		t.Errorf("tags after delete: %v", dto.ImageTags)
	}
	if response, _ := e.fetch(t, e.url+"/Items/"+allowed+"/Images/Primary"); response.StatusCode != http.StatusNotFound {
		t.Errorf("deleted image: %d", response.StatusCode)
	}
}

func TestIdentifySearchesTheServersAddonsButCannotRebind(t *testing.T) {
	e := newEdited(t)
	var results []RemoteSearchResult
	status, body := e.call(http.MethodPost, "/Items/RemoteSearch/Movie", app("dashboard", e.admin),
		map[string]any{"SearchInfo": map[string]any{"Name": "Restricted", "Year": "", "ProviderIds": map[string]string{}}, "ItemId": e.ids["Allowed"]})
	if status != http.StatusOK || json.Unmarshal(body, &results) != nil || len(results) != 1 ||
		results[0].Name != "Restricted" || results[0].ProviderIds["Imdb"] != "tt2" || results[0].SearchProviderName != "Rated" {
		t.Errorf("movie search: %d %s", status, body)
	}
	for path, token := range map[string]string{"/Items/RemoteSearch/Book": e.admin, "/Items/RemoteSearch/Series": e.member} {
		status, body := e.call(http.MethodPost, path, app("tv", token), map[string]any{"SearchInfo": map[string]any{"Name": "Show"}})
		if status != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	if status, _ := e.call(http.MethodPost, "/Items/RemoteSearch/Person", app("tv", e.member), map[string]any{"SearchInfo": map[string]any{}}); status != http.StatusForbidden {
		t.Errorf("member person search: %d", status)
	}
	status, body = e.call(http.MethodPost, "/Items/RemoteSearch/Apply/"+e.ids["Allowed"], app("dashboard", e.admin), results[0])
	if status != http.StatusInternalServerError || string(body) != "Error processing request." {
		t.Errorf("apply: %d %s", status, body)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
