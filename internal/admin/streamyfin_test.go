package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/streamyfin"
	"github.com/moodiness/polyfin/internal/stremio"
)

// streamyfinState is what the admin app reads of Streamyfin's rows.
type streamyfinState struct {
	Rows []struct {
		Kind        string  `json:"kind"`
		ItemID      *string `json:"itemId"`
		Title       *string `json:"title"`
		Name        string  `json:"name"`
		DefaultName string  `json:"defaultName"`
		LibraryName *string `json:"libraryName"`
		Available   bool    `json:"available"`
	} `json:"rows"`
	Libraries []struct {
		ItemID      string `json:"itemId"`
		Name        string `json:"name"`
		Collections bool   `json:"collections"`
	} `json:"libraries"`
}

// streamyfinCall sends a request and decodes its answer into into, or
// answers the error code.
func (b browser) streamyfinCall(method, path string, body, into any) (int, string) {
	b.api.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequestWithContext(b.api.t.Context(), method, b.api.url+"/admin/api"+path, reader)
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		return response.StatusCode, failure.Error
	}
	if err := json.Unmarshal(raw, into); err != nil {
		b.api.t.Fatalf("%s %s: %v %s", method, path, err, raw)
	}
	return response.StatusCode, ""
}

// An administrator chooses Streamyfin's rows among the server's libraries
// of movies, series and collections and the collections of the latter;
// the rows keep their order and titles, and name what they show. Rows of
// unknown items or kinds, and too many rows, are refused; members may not
// see or change them.
func TestStreamyfinRowsAreChosenByAdministrators(t *testing.T) {
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		o.Streamyfin = streamyfin.New(d.pool)
		o.StreamyfinItems = o.LibraryImages.(*library.Service)
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "local", Name: "Local", Version: "1.0.0",
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "collection", ID: "groups", Name: "Groups"},
					{Type: "tv", ID: "channels", Name: "Channels"}}})
		case strings.HasPrefix(path, "/catalog/collection/groups"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "group:1", Type: "collection", Name: "First group"},
				{ID: "group:2", Type: "collection", Name: "Second group"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		}
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	addonID, _ := body["id"].(string)
	if status, _, code := administrator.saveLibraries(
		map[string]any{"addonId": addonID, "catalogType": "movie", "catalogId": "top"},
		map[string]any{"addonId": addonID, "catalogType": "collection", "catalogId": "groups"},
		map[string]any{"addonId": addonID, "catalogType": "tv", "catalogId": "channels"},
	); status != http.StatusOK {
		t.Fatalf("libraries: %d %s", status, code)
	}

	var state streamyfinState
	if status, code := administrator.streamyfinCall(http.MethodGet, "/streamyfin", nil, &state); status != http.StatusOK || len(state.Rows) != 0 {
		t.Fatalf("no rows: %d %s %+v", status, code, state.Rows)
	}
	var libraries []string
	ids := map[string]string{}
	for _, l := range state.Libraries {
		libraries = append(libraries, fmt.Sprintf("%s %v", l.Name, l.Collections))
		ids[l.Name] = l.ItemID
	}
	// Live TV catalogs make no library a row can show.
	if !slices.Equal(libraries, []string{"Top false", "Groups true"}) {
		t.Fatalf("libraries: %v", libraries)
	}
	var collections []struct {
		ItemID string `json:"itemId"`
		Name   string `json:"name"`
	}
	if status, code := administrator.streamyfinCall(http.MethodGet, "/streamyfin/libraries/"+ids["Groups"]+"/collections", nil, &collections); status != http.StatusOK ||
		len(collections) != 2 || collections[1].Name != "Second group" {
		t.Fatalf("collections: %d %s %+v", status, code, collections)
	}
	if status, _ := administrator.streamyfinCall(http.MethodGet, "/streamyfin/libraries/"+ids["Top"]+"/collections", nil, &collections); status != http.StatusNotFound {
		t.Errorf("the collections of a movie library: %d", status)
	}

	rows := []map[string]any{
		{"kind": "resume"}, {"kind": "nextUp", "title": "Keep watching"},
		{"kind": "library", "itemId": ids["Groups"]}, {"kind": "collection", "itemId": collections[1].ItemID, "title": "  "},
	}
	if status, code := administrator.streamyfinCall(http.MethodPut, "/streamyfin", map[string]any{"rows": rows}, &state); status != http.StatusOK {
		t.Fatalf("saving: %d %s", status, code)
	}
	var got []string
	for _, row := range state.Rows {
		library := ""
		if row.LibraryName != nil {
			library = " in " + *row.LibraryName
		}
		got = append(got, fmt.Sprintf("%s %s (%s)%s %v", row.Kind, row.Name, row.DefaultName, library, row.Available))
	}
	want := []string{"resume Continue Watching (Continue Watching) true", "nextUp Keep watching (Next Up) true",
		"library Groups (Groups) true", "collection Second group (Second group) in Groups true"}
	if !slices.Equal(got, want) || state.Rows[3].Title != nil {
		t.Errorf("saved rows:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	for name, refused := range map[string]struct {
		rows []map[string]any
		code string
	}{
		"an unknown kind":                 {[]map[string]any{{"kind": "latest"}}, "invalid_streamyfin_row"},
		"a library row without a library": {[]map[string]any{{"kind": "library"}}, "invalid_streamyfin_row"},
		"an item for the user's own rows": {[]map[string]any{{"kind": "resume", "itemId": ids["Top"]}}, "invalid_streamyfin_row"},
		"a collection as a library":       {[]map[string]any{{"kind": "library", "itemId": collections[0].ItemID}}, "invalid_streamyfin_row"},
		"a library as a collection":       {[]map[string]any{{"kind": "collection", "itemId": ids["Top"]}}, "invalid_streamyfin_row"},
		"a title too long":                {[]map[string]any{{"kind": "resume", "title": strings.Repeat("a", 65)}}, "invalid_streamyfin_row"},
		"too many rows":                   {slices.Repeat([]map[string]any{{"kind": "library", "itemId": ids["Top"]}}, 31), "too_many_streamyfin_rows"},
	} {
		if status, code := administrator.streamyfinCall(http.MethodPut, "/streamyfin", map[string]any{"rows": refused.rows}, &state); status != http.StatusBadRequest || code != refused.code {
			t.Errorf("%s: %d %s, want %s", name, status, code, refused.code)
		}
	}
	// The refused saves changed nothing.
	if _, _ = administrator.streamyfinCall(http.MethodGet, "/streamyfin", nil, &state); len(state.Rows) != 4 {
		t.Errorf("after refused saves: %+v", state.Rows)
	}

	member := api.signedIn("member", false)
	if status, _ := member.streamyfinCall(http.MethodGet, "/streamyfin", nil, &state); status != http.StatusForbidden {
		t.Errorf("a member reading the rows: %d", status)
	}
	if status, _ := member.streamyfinCall(http.MethodPut, "/streamyfin", map[string]any{"rows": []any{}}, &state); status != http.StatusForbidden {
		t.Errorf("a member saving the rows: %d", status)
	}
}
