package admin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/letterboxd"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// newMovieSearchAddon serves a metadata addon whose movie search finds the
// invented movies whose names hold the term: two of them share a name and
// a year. A search waits for hold to be closed, when not nil.
func newMovieSearchAddon(t *testing.T, hold <-chan struct{}) *httptest.Server {
	t.Helper()
	movies := []stremio.Meta{
		{ID: "tt7200001", Type: "movie", Name: "Velvet Quarry", Year: "1993"},
		{ID: "tt7200002", Type: "movie", Name: "Salt Meridian", Year: "2008"},
		{ID: "tt7200003", Type: "movie", Name: "Echo Street", Year: "2016"},
		{ID: "tt7200004", Type: "movie", Name: "Echo Street", Year: "2016"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "movies", Name: "Movies", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search",
					Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
		case strings.HasPrefix(path, "/catalog/movie/search/search="):
			if hold != nil {
				select {
				case <-hold:
				case <-r.Context().Done():
					return
				}
			}
			term, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/movie/search/search="), ".json"))
			metas := []stremio.Meta{}
			for _, meta := range movies {
				if strings.Contains(strings.ToLower(meta.Name), strings.ToLower(term)) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// letterboxdExport is an export of invented movies: Velvet Quarry watched
// twice and rated 3.5 stars, Salt Meridian on the watchlist and liked,
// Echo Street, of which the addon knows two movies of 2016, and Paper
// Moon Pier, which it does not know.
func letterboxdExport(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"watched.csv": "Date,Name,Year,Letterboxd URI\n2020-02-03,Velvet Quarry,1993,https://boxd.it/bbb1\n" +
			"2020-02-04,Echo Street,2016,https://boxd.it/bbb3\n2020-02-05,Paper Moon Pier,1971,https://boxd.it/bbb4\n",
		"diary.csv": "Date,Name,Year,Letterboxd URI,Rating,Rewatch,Tags,Watched Date\n" +
			"2020-02-03,Velvet Quarry,1993,https://boxd.it/d1,,,,2020-02-01\n" +
			"2024-05-06,Velvet Quarry,1993,https://boxd.it/d2,,Yes,,2024-05-05\n",
		"ratings.csv":     "Date,Name,Year,Letterboxd URI,Rating\n2024-05-06,Velvet Quarry,1993,https://boxd.it/bbb1,3.5\n",
		"watchlist.csv":   "Date,Name,Year,Letterboxd URI\n2025-01-01,Salt Meridian,2008,https://boxd.it/bbb2\n",
		"likes/films.csv": "Date,Name,Year,Letterboxd URI\n2025-01-01,Salt Meridian,2008,https://boxd.it/bbb2\n",
	} {
		f, _ := archive.Create(name)
		_, _ = io.WriteString(f, content)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// upload posts a multipart form of fields, and of file under "file" when
// not nil, to path.
func (b browser) upload(path string, fields map[string]string, file []byte) (int, map[string]any) {
	b.api.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range fields {
		_ = form.WriteField(name, value)
	}
	if file != nil {
		part, _ := form.CreateFormFile("file", "letterboxd-export.zip")
		_, _ = part.Write(file)
	}
	_ = form.Close()
	return b.post(path, form.FormDataContentType(), &body)
}

// post posts body, of contentType, to path, and answers the status and
// the decoded answer.
func (b browser) post(path, contentType string, body io.Reader) (int, map[string]any) {
	b.api.t.Helper()
	request, _ := http.NewRequestWithContext(b.api.t.Context(), http.MethodPost, b.api.url+"/admin/api"+path, body)
	request.Header.Set("Content-Type", contentType)
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Error(err)
		return 0, nil
	}
	defer response.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)
	return response.StatusCode, decoded
}

// letterboxdDone waits for the Letterboxd import path answers to end, and
// fails unless it is done.
func letterboxdDone(t *testing.T, b browser, path string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, body, _ := b.call(http.MethodGet, path, nil)
		if current, _ := body["import"].(map[string]any); current != nil && current["state"] != "running" {
			if current["state"] != "done" || current["kind"] != "letterboxd" {
				t.Fatalf("import: %v", current)
			}
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A member uploads their Letterboxd export under My account, with its
// switches, and follows the import: the movies found are played with
// their dates and play counts, rated, put on the Watchlist, and made
// favorites with the switch on; those not found are listed with why and
// their link, and importing again adds nothing. The form tells whether
// the metadata addons search movies. Files that are not an export, or too
// large, are refused, and so is everything once the setting is off; an
// administrator imports for a user whatever the setting says, and a
// member may not.
func TestLetterboxdImport(t *testing.T) {
	t.Parallel()
	var lib *library.Service
	var data *userdata.Store
	var lists *watchlist.Store
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib = library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		data, lists, store = userdata.New(deps.pool), watchlist.New(deps.pool), deps.addons
		o.Letterboxd = letterboxd.New(letterboxd.Options{Titles: lib, UserData: data, Watchlist: lists, Logger: logger})
		t.Cleanup(o.Letterboxd.Close)
	})
	admin := api.signedIn("root", true)
	member := api.signedIn("ines", false)
	ines, err := api.store.Authenticate(t.Context(), "ines", "correct horse")
	if err != nil {
		t.Fatal(err)
	}

	if _, body, _ := member.call(http.MethodGet, "/account/server-import", nil); body["enabled"] != true ||
		body["letterboxd"] != true || body["searchable"] != false || body["import"] != nil {
		t.Errorf("without an addon searching movies: %v", body)
	}
	if _, err := store.Install(t.Context(), addons.Shared(), newMovieSearchAddon(t, nil).URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/server-import", nil); body["searchable"] != true {
		t.Errorf("with an addon searching movies: %v", body)
	}

	export := letterboxdExport(t)
	for name, refused := range map[string]struct {
		fields map[string]string
		file   []byte
		status int
		code   string
	}{
		"not a ZIP":    {map[string]string{"kind": "letterboxd"}, []byte("Date,Name\n"), http.StatusBadRequest, "letterboxd_export_invalid"},
		"no CSV files": {map[string]string{"kind": "letterboxd"}, emptyZip(t), http.StatusBadRequest, "letterboxd_export_invalid"},
		"too large":    {map[string]string{"kind": "letterboxd"}, make([]byte, letterboxd.MaxExportBytes+1), http.StatusRequestEntityTooLarge, "letterboxd_export_too_large"},
		"too many movies": {map[string]string{"kind": "letterboxd"}, manyMoviesExport(t, letterboxd.MaxMovies+1), http.StatusRequestEntityTooLarge,
			"letterboxd_export_too_large"},
		"without a file": {map[string]string{"kind": "letterboxd"}, nil, http.StatusBadRequest, "invalid_request"},
		"another kind":   {map[string]string{"kind": "jellyfin"}, export, http.StatusBadRequest, "invalid_request"},
		"a wrong switch": {map[string]string{"kind": "letterboxd", "ratings": "maybe"}, export, http.StatusBadRequest, "invalid_request"},
	} {
		if status, body := member.upload("/account/server-import", refused.fields, refused.file); status != refused.status ||
			body["error"] != refused.code {
			t.Errorf("%s: %d %v", name, status, body)
		}
	}

	status, body := member.upload("/account/server-import", map[string]string{"kind": "letterboxd", "likesAsFavorites": "true"}, export)
	if started, _ := body["import"].(map[string]any); status != http.StatusOK || started == nil || started["kind"] != "letterboxd" ||
		started["watchlist"] != true || started["ratings"] != true || started["likesAsFavorites"] != true {
		t.Fatalf("the member's import: %d %v", status, body)
	}
	done := letterboxdDone(t, member, "/account/server-import")
	added, _ := done["added"].(map[string]any)
	if done["movies"] != 4.0 || done["searched"] != 4.0 || added["played"] != 1.0 || added["watchlist"] != 1.0 ||
		added["ratings"] != 1.0 || added["favorites"] != 1.0 || done["unmatchedCount"] != 2.0 {
		t.Errorf("the member's import: %v", done)
	}
	if unmatched, _ := done["unmatched"].([]any); len(unmatched) != 2 ||
		unmatched[0].(map[string]any)["reason"] != "ambiguous" || unmatched[0].(map[string]any)["link"] != "https://boxd.it/bbb3" ||
		unmatched[1].(map[string]any)["reason"] != "not_found" || unmatched[1].(map[string]any)["year"] != 1971.0 {
		t.Errorf("unmatched: %v", done["unmatched"])
	}
	found, err := lib.Resolve(t.Context(), []library.TitleRef{{IMDb: "tt7200001"}, {IMDb: "tt7200002"}})
	if err != nil || len(found[0]) != 1 || len(found[1]) != 1 {
		t.Fatalf("titles: %v %v", found, err)
	}
	quarry, meridian := found[0][0].ID, found[1][0].ID
	got, err := data.Get(t.Context(), ines.ID, []accounts.ID{quarry, meridian})
	if err != nil {
		t.Fatal(err)
	}
	if q := got[quarry]; !q.Played || q.PlayCount != 2 || q.LastPlayed == nil ||
		!q.LastPlayed.Equal(time.Date(2024, 5, 5, 12, 0, 0, 0, time.UTC)) || q.Rating == nil || *q.Rating != 7 {
		t.Errorf("Velvet Quarry: %+v", q)
	}
	if m := got[meridian]; m.Played || !m.Favorite {
		t.Errorf("Salt Meridian: %+v", m)
	}
	if titles, err := lists.Titles(t.Context(), ines.ID); err != nil || len(titles) != 1 || titles[0] != meridian {
		t.Errorf("watchlist: %v %v", titles, err)
	}

	// The administrator imports the same export for her: nothing more.
	if status, body := member.upload("/users/"+ines.ID.String()+"/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusForbidden {
		t.Errorf("a member importing for a user: %d %v", status, body)
	}
	if status, body := admin.upload("/users/"+ines.ID.String()+"/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusOK {
		t.Fatalf("the administrator's import: %d %v", status, body)
	}
	again := letterboxdDone(t, admin, "/users/"+ines.ID.String()+"/server-import")
	if added, _ := again["added"].(map[string]any); added["played"] != 0.0 || added["watchlist"] != 0.0 || added["ratings"] != 0.0 ||
		added["favorites"] != 0.0 || again["startedBy"].(map[string]any)["name"] != "root" {
		t.Errorf("importing again: %v", again)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/server-import", nil); body["import"].(map[string]any)["id"] != again["id"] {
		t.Errorf("the member sees %v", body)
	}

	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "serverImports": false}
	if status, body, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning the setting off: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/server-import", nil); body["enabled"] != false || body["letterboxd"] != false ||
		body["import"] != nil {
		t.Errorf("with the setting off: %v", body)
	}
	if status, body := member.upload("/account/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusForbidden ||
		body["error"] != "server_imports_disabled" {
		t.Errorf("importing with the setting off: %d %v", status, body)
	}
	if status, body := admin.upload("/users/"+ines.ID.String()+"/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusOK {
		t.Errorf("the administrator's import with the setting off: %d %v", status, body)
	}
	letterboxdDone(t, admin, "/users/"+ines.ID.String()+"/server-import")
}

// One import into an account runs at a time: while a member's Letterboxd
// import runs, another, or their own import from a server, is refused,
// and an administrator's into their account too; an import into another
// account runs.
func TestLetterboxdImportOneAtATime(t *testing.T) {
	t.Parallel()
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		data := userdata.New(deps.pool)
		store = deps.addons
		o.Letterboxd = letterboxd.New(letterboxd.Options{Titles: lib, UserData: data, Watchlist: watchlist.New(deps.pool), Logger: logger})
		t.Cleanup(o.Letterboxd.Close)
		o.JellyfinImport = jellyfinimport.New(jellyfinimport.Options{Settings: o.Accounts.Settings, Titles: lib, UserData: data,
			Version: "1.2.3", Logger: logger})
		t.Cleanup(o.JellyfinImport.Close)
	})
	hold := make(chan struct{})
	released := false
	release := func() {
		if !released {
			released = true
			close(hold)
		}
	}
	if _, err := store.Install(t.Context(), addons.Shared(), newMovieSearchAddon(t, hold).URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	// The searches held end before the addon's server closes, which waits
	// for them.
	t.Cleanup(release)
	admin := api.signedIn("root", true)
	member := api.signedIn("ines", false)
	other := api.signedIn("theo", false)
	ines, err := api.store.Authenticate(t.Context(), "ines", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	export := letterboxdExport(t)
	if status, body := member.upload("/account/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusOK {
		t.Fatalf("the first import: %d %v", status, body)
	}
	if status, body := member.upload("/account/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusConflict ||
		body["error"] != "server_import_running" {
		t.Errorf("a second import: %d %v", status, body)
	}
	if status, body := admin.upload("/users/"+ines.ID.String()+"/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusConflict ||
		body["error"] != "server_import_running" {
		t.Errorf("the administrator's import meanwhile: %d %v", status, body)
	}
	fromServer := map[string]any{"kind": "jellyfin", "address": "http://127.0.0.1:1", "name": "Ines", "password": "secret"}
	if status, body, _ := member.call(http.MethodPost, "/account/server-import", fromServer); status != http.StatusConflict ||
		body["error"] != "server_import_running" {
		t.Errorf("an import from a server meanwhile: %d %v", status, body)
	}
	if status, body := other.upload("/account/server-import", map[string]string{"kind": "letterboxd"}, export); status != http.StatusOK {
		t.Errorf("an import into another account: %d %v", status, body)
	}
	release()
	letterboxdDone(t, member, "/account/server-import")
	letterboxdDone(t, other, "/account/server-import")
}

// While an export is being received for an account, a second one sent
// into it is refused at once, before its file is read; the first goes on
// and starts.
func TestLetterboxdUploadsAtOnce(t *testing.T) {
	t.Parallel()
	var service *letterboxd.Service
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		store = deps.addons
		service = letterboxd.New(letterboxd.Options{Titles: lib, UserData: userdata.New(deps.pool), Watchlist: watchlist.New(deps.pool),
			Logger: logger})
		o.Letterboxd = service
		t.Cleanup(o.Letterboxd.Close)
	})
	if _, err := store.Install(t.Context(), addons.Shared(), newMovieSearchAddon(t, nil).URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	admin := api.signedIn("root", true)
	member := api.signedIn("ines", false)
	ines, err := api.store.Authenticate(t.Context(), "ines", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	_ = writer.WriteField("kind", "letterboxd")
	part, _ := writer.CreateFormFile("file", "letterboxd-export.zip")
	_, _ = part.Write(letterboxdExport(t))
	_ = writer.Close()
	sent, held := form.Bytes()[:form.Len()/2], form.Bytes()[form.Len()/2:]

	// The member's upload sends half of its form, then waits.
	reader, rest := io.Pipe()
	type answer struct {
		status int
		body   map[string]any
	}
	first := make(chan answer, 1)
	go func() {
		status, body := member.post("/account/server-import", writer.FormDataContentType(), reader)
		first <- answer{status, body}
	}()
	if _, err := rest.Write(sent); err != nil {
		t.Fatal(err)
	}
	// The account is held once the handler reads the form.
	for deadline := time.Now().Add(30 * time.Second); !service.Running(ines.ID); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = rest.CloseWithError(errors.New("given up"))
			t.Fatal("the account is not held while the first export is received")
		}
	}
	if status, body := member.upload("/account/server-import", map[string]string{"kind": "letterboxd"}, letterboxdExport(t)); status != http.StatusConflict ||
		body["error"] != "server_import_running" {
		t.Errorf("a second upload meanwhile: %d %v", status, body)
	}
	if status, body := admin.upload("/users/"+ines.ID.String()+"/server-import", map[string]string{"kind": "letterboxd"}, letterboxdExport(t)); status != http.StatusConflict ||
		body["error"] != "server_import_running" {
		t.Errorf("the administrator's upload meanwhile: %d %v", status, body)
	}
	if _, err := rest.Write(held); err != nil {
		t.Fatal(err)
	}
	_ = rest.Close()
	if got := <-first; got.status != http.StatusOK {
		t.Errorf("the first upload: %d %v", got.status, got.body)
	}
	letterboxdDone(t, member, "/account/server-import")
}

// emptyZip is a ZIP holding a profile only.
func emptyZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	f, _ := archive.Create("profile.csv")
	_, _ = io.WriteString(f, "Username\nines\n")
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// manyMoviesExport is an export whose watched.csv names movies movies.
func manyMoviesExport(t *testing.T, movies int) []byte {
	t.Helper()
	var watched strings.Builder
	watched.WriteString("Date,Name,Year\n")
	for i := range movies {
		fmt.Fprintf(&watched, "2020-01-01,Movie %d,2000\n", i)
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	f, _ := archive.Create("watched.csv")
	_, _ = io.WriteString(f, watched.String())
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
