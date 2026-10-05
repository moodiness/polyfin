package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// libraryImageCall chooses a library's image and returns the status, the
// error code and the libraries answered.
func (b browser) libraryImageCall(scope string, body map[string]any) (int, string, []map[string]any) {
	b.api.t.Helper()
	encoded, _ := json.Marshal(body)
	request, _ := http.NewRequestWithContext(b.api.t.Context(), http.MethodPut, b.api.url+"/admin/api/scopes/"+scope+"/libraries/image",
		bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	var raw json.RawMessage
	_ = json.NewDecoder(response.Body).Decode(&raw)
	var failure struct {
		Error string `json:"error"`
	}
	var libraries []map[string]any
	if json.Unmarshal(raw, &libraries) != nil {
		_ = json.Unmarshal(raw, &failure)
	}
	return response.StatusCode, failure.Error, libraries
}

func pngPicture(t *testing.T) []byte {
	t.Helper()
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 64, 36))); err != nil {
		t.Fatal(err)
	}
	return picture.Bytes()
}

// The admin app chooses each library's image: none by default, automatic,
// or custom, uploaded or downloaded once from an address, which only
// administrators may take from a local network. The libraries answered
// tell the choice and the tag of the image apps show.
func TestLibraryImages(t *testing.T) {
	picture := pngPicture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "local", Name: "Local", Version: "1", Types: []string{"movie", "tv"},
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"},
					{Type: "movie", ID: "new", Name: "New"}, {Type: "tv", ID: "channels", Name: "Channels"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "tt0001", Type: "movie", Name: "First", Poster: server.URL + "/poster.jpg", Background: server.URL + "/backdrop.jpg"}}})
		case path == "/picture.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(picture)
		case path == "/page.html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<p>not an image</p>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	var store *addons.Store
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { store = deps.addons })
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	addon, err := store.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}, {AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}}); err != nil {
		t.Fatal(err)
	}
	top := map[string]any{"addonId": addon.ID.String(), "catalogType": "movie", "catalogId": "top"}
	with := func(fields map[string]any) map[string]any {
		body := map[string]any{}
		for k, v := range top {
			body[k] = v
		}
		for k, v := range fields {
			body[k] = v
		}
		return body
	}
	// find returns a catalog among the libraries answered.
	find := func(libraries []map[string]any, catalogType, id string) map[string]any {
		t.Helper()
		for _, l := range libraries {
			if l["catalogType"] == catalogType && l["catalogId"] == id {
				return l
			}
		}
		t.Fatalf("no %s/%s in %v", catalogType, id, libraries)
		return nil
	}

	libraries := administrator.libraries("shared")
	movie, tv, available := find(libraries, "movie", "top"), find(libraries, "tv", "channels"), find(libraries, "movie", "new")
	if movie["image"] != "none" || movie["imageTag"] != nil || movie["itemId"] == nil {
		t.Errorf("a library by default: %v", movie)
	}
	if tv["itemId"] != nil || available["itemId"] != nil || available["image"] != "none" {
		t.Errorf("catalogs that are not libraries: %v, %v", tv, available)
	}

	status, code, libraries := administrator.libraryImageCall("shared", with(map[string]any{"image": "automatic"}))
	if movie = find(libraries, "movie", "top"); status != http.StatusOK || movie["image"] != "automatic" ||
		movie["imageTag"] != library.ImageTag(server.URL+"/backdrop.jpg") {
		t.Errorf("automatic: %d %s %v", status, code, movie)
	}
	// Saving the libraries again, renamed, keeps their image.
	if status, body, _ := administrator.call(http.MethodPut, "/scopes/shared/libraries", map[string]any{"libraries": []map[string]any{
		with(map[string]any{"name": "Renamed"}), {"addonId": addon.ID.String(), "catalogType": "tv", "catalogId": "channels"}}}); status != http.StatusOK {
		t.Fatalf("saving the libraries: %d %v", status, body)
	}
	if movie = find(administrator.libraries("shared"), "movie", "top"); movie["image"] != "automatic" || movie["name"] != "Renamed" {
		t.Errorf("after saving the libraries: %v", movie)
	}

	status, code, libraries = administrator.libraryImageCall("shared", with(map[string]any{"image": "custom",
		"data": base64.StdEncoding.EncodeToString(picture)}))
	uploaded := find(libraries, "movie", "top")
	if status != http.StatusOK || uploaded["image"] != "custom" || uploaded["imageTag"] == nil || uploaded["imageTag"] == movie["imageTag"] {
		t.Errorf("uploading: %d %s %v", status, code, uploaded)
	}
	status, code, libraries = administrator.libraryImageCall("shared", with(map[string]any{"image": "custom", "url": server.URL + "/picture.png"}))
	if downloaded := find(libraries, "movie", "top"); status != http.StatusOK || downloaded["image"] != "custom" || downloaded["imageTag"] == nil {
		t.Errorf("downloading from a local address as an administrator: %d %s %v", status, code, downloaded)
	}

	for name, c := range map[string]struct {
		scope  string
		user   browser
		body   map[string]any
		status int
		code   string
	}{
		"unknown choice":      {"shared", administrator, with(map[string]any{"image": "poster"}), http.StatusBadRequest, "invalid_request"},
		"custom without data": {"shared", administrator, with(map[string]any{"image": "custom"}), http.StatusBadRequest, "invalid_request"},
		"automatic with data": {"shared", administrator, with(map[string]any{"image": "automatic", "url": server.URL + "/picture.png"}),
			http.StatusBadRequest, "invalid_request"},
		"not a picture": {"shared", administrator, with(map[string]any{"image": "custom", "data": base64.StdEncoding.EncodeToString([]byte("text"))}),
			http.StatusBadRequest, "invalid_image"},
		"not an address": {"shared", administrator, with(map[string]any{"image": "custom", "url": "ftp://example.com/a.png"}), http.StatusBadRequest,
			"invalid_image_url"},
		"not an image address": {"shared", administrator, with(map[string]any{"image": "custom", "url": server.URL + "/page.html"}),
			http.StatusBadGateway, "image_unreachable"},
		"not a library": {"shared", administrator, map[string]any{"addonId": addon.ID.String(), "catalogType": "movie", "catalogId": "new",
			"image": "automatic"}, http.StatusBadRequest, "invalid_library"},
		"a live TV catalog": {"shared", administrator, map[string]any{"addonId": addon.ID.String(), "catalogType": "tv", "catalogId": "channels",
			"image": "automatic"}, http.StatusBadRequest, "invalid_library"},
		"the server's, by a member": {"shared", member, with(map[string]any{"image": "automatic"}), http.StatusForbidden, "forbidden"},
		"another scope's library":   {"me", member, with(map[string]any{"image": "automatic"}), http.StatusBadRequest, "invalid_library"},
	} {
		if status, code, _ := c.user.libraryImageCall(c.scope, c.body); status != c.status || code != c.code {
			t.Errorf("%s: %d %s, want %d %s", name, status, code, c.status, c.code)
		}
	}

	// Choosing none drops the upload.
	status, code, libraries = administrator.libraryImageCall("shared", with(map[string]any{"image": "none"}))
	if movie = find(libraries, "movie", "top"); status != http.StatusOK || movie["image"] != "none" || movie["imageTag"] != nil {
		t.Errorf("removing: %d %s %v", status, code, movie)
	}

	// A member chooses the images of their own libraries, from public
	// addresses only.
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var own addons.Addon
	for _, user := range users {
		if user.Name == "member" {
			if own, err = store.Install(t.Context(), addons.Personal(user.ID), server.URL+"/manifest.json", false); err != nil {
				t.Fatal(err)
			}
		}
	}
	mine := map[string]any{"addonId": own.ID.String(), "catalogType": "movie", "catalogId": "top", "image": "custom"}
	mine["url"] = server.URL + "/picture.png"
	if status, code, _ := member.libraryImageCall("me", mine); status != http.StatusForbidden || code != "image_private_network" {
		t.Errorf("a member's local address: %d %s", status, code)
	}
	delete(mine, "url")
	mine["data"] = base64.StdEncoding.EncodeToString(picture)
	status, code, libraries = member.libraryImageCall("me", mine)
	if ownMovie := find(libraries, "movie", "top"); status != http.StatusOK || ownMovie["image"] != "custom" || ownMovie["imageTag"] == nil {
		t.Errorf("a member's upload: %d %s %v", status, code, ownMovie)
	}
	// The server's library is not the member's.
	if movie = find(administrator.libraries("shared"), "movie", "top"); movie["image"] != "none" {
		t.Errorf("the server's library after a member's upload: %v", movie)
	}
}
