package jellyfin

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// posterlessTMDB is a fake TMDB: its API finds the movies of the IMDb
// identifiers it is given, gives their details, poster and backdrop, with
// their images in the languages asked for when they are appended, and its
// image server serves them. It records the API's requests.
type posterlessTMDB struct {
	*httptest.Server
	mu sync.Mutex
	// asked are the paths asked, and queries those with their query, the
	// key left out.
	asked, queries []string
	details        map[string]map[string]any
	images         map[string]map[string][]map[string]any
}

func newPosterlessTMDB(t *testing.T, picture []byte) *posterlessTMDB {
	t.Helper()
	// tt0893001 and tt0893002 have a poster, the first a backdrop too;
	// TMDB does not know tt0893003. The first has posters in French, in
	// English and without text, and backdrops in French and without
	// text; the second, posters in English and without text.
	movies := map[string]int{"tt0893001": 9301, "tt0893002": 9302}
	image := func(path, language string, rating float64) map[string]any {
		var lang any
		if language != "" {
			lang = language
		}
		return map[string]any{"file_path": path, "iso_639_1": lang, "vote_average": rating, "vote_count": 3}
	}
	f := &posterlessTMDB{details: map[string]map[string]any{
		"/3/movie/9301": {"id": 9301, "poster_path": "/p9301.jpg", "backdrop_path": "/b9301.jpg"},
		"/3/movie/9302": {"id": 9302, "poster_path": "/p9302.jpg", "backdrop_path": nil},
	}, images: map[string]map[string][]map[string]any{
		"/3/movie/9301": {
			"posters": {image("/p9301-en.jpg", "en", 9), image("/p9301-fr.jpg", "fr", 5), image("/p9301-fr-low.jpg", "fr", 2),
				image("/p9301-de.jpg", "de", 10), image("/p9301-none.jpg", "", 8)},
			"backdrops": {image("/b9301-fr.jpg", "fr", 9), image("/b9301-none.jpg", "", 1)},
		},
		"/3/movie/9302": {
			"posters":   {image("/p9302-none.jpg", "", 9), image("/p9302-en.jpg", "en", 1)},
			"backdrops": {},
		},
	}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/t/p/original/") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(picture)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		query := r.URL.Query()
		query.Del("api_key")
		f.asked = append(f.asked, r.URL.Path)
		f.queries = append(f.queries, r.URL.Path+"?"+query.Encode())
		if imdb, ok := strings.CutPrefix(r.URL.Path, "/3/find/"); ok {
			found := []any{}
			if id, ok := movies[imdb]; ok {
				found = append(found, map[string]any{"id": id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": found, "tv_results": []any{}})
			return
		}
		details, ok := f.details[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		answer := maps.Clone(details)
		if query.Get("append_to_response") == "images" {
			// TMDB keeps the images of the languages asked for, "null"
			// standing for those without text.
			languages := strings.Split(query.Get("include_image_language"), ",")
			kept := map[string]any{}
			for kind, images := range f.images[r.URL.Path] {
				in := []map[string]any{}
				for _, image := range images {
					language, _ := image["iso_639_1"].(string)
					if slices.Contains(languages, cmp.Or(language, "null")) {
						in = append(in, image)
					}
				}
				kept[kind] = in
			}
			answer["images"] = kept
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(f.Close)
	return f
}

// requests returns the paths the API was asked, those starting with
// prefix.
func (f *posterlessTMDB) requests(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.asked), func(path string) bool { return !strings.HasPrefix(path, prefix) })
}

// requestsWithQueries returns the requests of the API, with their query,
// whose path is path.
func (f *posterlessTMDB) requestsWithQueries(path string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.queries), func(asked string) bool { return !strings.HasPrefix(asked, path+"?") })
}

// poster changes the poster of a movie's details.
func (f *posterlessTMDB) poster(movie, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.details[movie]["poster_path"] = path
}

// posterlessAddon lists movies the way some addons do: "Bare Harbor"
// (tt0893001) without a poster, "Gone Pier" (tt0893002) with a poster its
// server does not have, "Unknown Cove" (tt0893003), which TMDB does not
// know, without a poster, "Plain Wharf", of no IMDb or TMDB identifier,
// without one either, and "Kept Bay" (tt0893004) with its own.
func posterlessAddon(t *testing.T, picture []byte) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "bare", Name: "Bare", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "tt0893001", Type: "movie", Name: "Bare Harbor", ReleaseInfo: "2020"},
				{ID: "tt0893002", Type: "movie", Name: "Gone Pier", ReleaseInfo: "2020", Poster: server.URL + "/gone.jpg"},
				{ID: "tt0893003", Type: "movie", Name: "Unknown Cove", ReleaseInfo: "2020"},
				{ID: "bare:wharf", Type: "movie", Name: "Plain Wharf", ReleaseInfo: "2020"},
				{ID: "tt0893004", Type: "movie", Name: "Kept Bay", ReleaseInfo: "2020", Poster: server.URL + "/kept.jpg"},
			}})
		case path == "/kept.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(picture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// posterlessEnv is a server whose library lists posterlessAddon's movies,
// with TMDB's posters while key is saved.
type posterlessEnv struct {
	testServer
	tmdb    *posterlessTMDB
	token   string
	library string
	// addon is the address of the addon's server, picture TMDB's images.
	addon   string
	picture []byte
}

func newPosterlessEnv(t *testing.T, key string) posterlessEnv {
	t.Helper()
	tmdbPicture := jpegOf(t, 20, 30)
	fake := newPosterlessTMDB(t, tmdbPicture)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		client := tmdb.New(fake.URL+"/3", fake.URL+"/t/p/", "test", o.Accounts.Settings)
		o.Library.UseTMDB(client)
		// The passes of titles' credits wait an hour: no test runs one.
		queue := tmdb.NewQueue(client, tmdb.QueueOptions{Interval: time.Millisecond, PassCheck: time.Hour})
		t.Cleanup(queue.Close)
		o.Library.UseTitleCredits(queue)
	})
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey = key })
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	manifest := posterlessAddon(t, jpegOf(t, 10, 15))
	addon, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	e := posterlessEnv{testServer: s, tmdb: fake, token: s.signIn("admin", "tv"), picture: tmdbPicture,
		addon: strings.TrimSuffix(manifest, "/manifest.json")}
	var views QueryResult
	s.get(t, "/UserViews", e.token, &views)
	e.library = views.Items[0].Id
	return e
}

// list lists the library's movies, by name.
func (e posterlessEnv) list(t *testing.T) map[string]BaseItemDto {
	t.Helper()
	var page QueryResult
	e.get(t, "/Items?ParentId="+e.library, e.token, &page)
	movies := map[string]BaseItemDto{}
	for _, item := range page.Items {
		movies[item.Name] = item
	}
	if len(movies) != 5 {
		t.Fatalf("listed %d movies", len(movies))
	}
	return movies
}

// image asks for an item's image by its tag, and returns the status and
// the body.
func (e posterlessEnv) image(t *testing.T, item BaseItemDto, imageType, tag string) (int, []byte) {
	t.Helper()
	response, err := testClient.Get(e.url + "/Items/" + item.Id + "/Images/" + imageType + "?tag=" + tag)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

// While a TMDB key is saved, a movie whose addon gives no poster shows
// TMDB's, relayed by Polyfin, looked up only when an app asks for it, and
// its backdrop, which comes with the same answer and needs no request of
// its own. A poster of the addon's stays.
func TestAMissingPosterIsTMDBs(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	movies := e.list(t)
	if asked := e.tmdb.requests("/"); len(asked) != 0 {
		t.Errorf("listing asked TMDB %v", asked)
	}
	bare := movies["Bare Harbor"]
	if status, body := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK || !bytes.Equal(body, e.picture) {
		t.Fatalf("Bare Harbor's poster: %d, %d bytes", status, len(body))
	}
	if asked := e.tmdb.requests("/"); !slices.Equal(asked, []string{"/3/find/tt0893001", "/3/movie/9301"}) {
		t.Errorf("TMDB was asked %v", asked)
	}

	movies = e.list(t)
	bare = movies["Bare Harbor"]
	poster, backdrop := e.tmdb.URL+"/t/p/original/p9301.jpg", e.tmdb.URL+"/t/p/original/b9301.jpg"
	if bare.ImageTags["Primary"] != library.ImageTag(poster) || !slices.Equal(bare.BackdropImageTags, []string{library.ImageTag(backdrop)}) {
		t.Fatalf("once looked up: %v, backdrops %v", bare.ImageTags, bare.BackdropImageTags)
	}
	if status, body := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK || !bytes.Equal(body, e.picture) {
		t.Errorf("Bare Harbor's poster by its tag: %d, %d bytes", status, len(body))
	}
	if status, body := e.image(t, bare, "Backdrop", bare.BackdropImageTags[0]); status != http.StatusOK || !bytes.Equal(body, e.picture) {
		t.Errorf("Bare Harbor's backdrop: %d, %d bytes", status, len(body))
	}
	if tag := movies["Kept Bay"].ImageTags["Primary"]; tag != library.ImageTag(e.addon+"/kept.jpg") {
		t.Errorf("the addon's poster gave way: %q", tag)
	}
	if asked := e.tmdb.requests("/"); len(asked) != 2 {
		t.Errorf("TMDB was asked again: %v", asked)
	}
	// In English, the poster is that of the title's details, the answer
	// box sets read too.
	if asked := e.tmdb.requestsWithQueries("/3/movie/9301"); !slices.Equal(asked, []string{"/3/movie/9301?"}) {
		t.Errorf("the details were asked as %v", asked)
	}
}

// A title advertises a poster's tag only when one is served, but before
// TMDB was asked: without a tag, apps would never ask for the image, which
// looks it up. A title TMDB does not know, or has no poster of, answers
// 404 once, then advertises no tag, so that apps draw their placeholder;
// one TMDB has a poster of advertises it, which is served. A title of no
// TMDB or IMDb identifier advertises none, and no backdrop is advertised
// before the lookup.
func TestPosterTagsAreGivenOnlyForImagesServed(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	movies := e.list(t)
	bare, unknown := movies["Bare Harbor"], movies["Unknown Cove"]
	if bare.ImageTags["Primary"] == "" || unknown.ImageTags["Primary"] == "" || bare.PrimaryImageAspectRatio == nil {
		t.Fatalf("before the lookup: %v, %v", bare.ImageTags, unknown.ImageTags)
	}
	if len(bare.BackdropImageTags) != 0 {
		t.Errorf("a backdrop was advertised before the lookup: %v", bare.BackdropImageTags)
	}
	if tag := movies["Plain Wharf"].ImageTags["Primary"]; tag != "" {
		t.Errorf("a title of no identifier has the poster tag %q", tag)
	}
	if status, _ := e.image(t, unknown, "Primary", unknown.ImageTags["Primary"]); status != http.StatusNotFound {
		t.Errorf("Unknown Cove's poster: %d", status)
	}
	if status, _ := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK {
		t.Errorf("Bare Harbor's poster: %d", status)
	}
	movies = e.list(t)
	if tag := movies["Unknown Cove"].ImageTags["Primary"]; tag != "" {
		t.Errorf("a title TMDB does not know keeps the poster tag %q", tag)
	}
	bare = movies["Bare Harbor"]
	if status, _ := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); bare.ImageTags["Primary"] == "" || status != http.StatusOK {
		t.Errorf("Bare Harbor's poster tag %q answers %d", bare.ImageTags["Primary"], status)
	}
}

// A poster its server does not have gives way to TMDB's at once, in the
// very request that found it dead, and the movie then advertises TMDB's.
func TestADeadPosterIsTMDBs(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	gone := e.list(t)["Gone Pier"]
	if gone.ImageTags["Primary"] != library.ImageTag(e.addon+"/gone.jpg") {
		t.Fatalf("before any download: %v", gone.ImageTags)
	}
	if status, body := e.image(t, gone, "Primary", gone.ImageTags["Primary"]); status != http.StatusOK || !bytes.Equal(body, e.picture) {
		t.Fatalf("Gone Pier's poster: %d, %d bytes", status, len(body))
	}
	gone = e.list(t)["Gone Pier"]
	if gone.ImageTags["Primary"] != library.ImageTag(e.tmdb.URL+"/t/p/original/p9302.jpg") || len(gone.BackdropImageTags) != 0 {
		t.Errorf("once dead: %v, backdrops %v", gone.ImageTags, gone.BackdropImageTags)
	}
	if asked := e.tmdb.requests("/"); !slices.Equal(asked, []string{"/3/find/tt0893002", "/3/movie/9302"}) {
		t.Errorf("TMDB was asked %v", asked)
	}
}

// Without a key, nothing changes: titles without a poster advertise none,
// a dead poster stops being advertised, and TMDB is never asked.
func TestPostersWithoutAKeyNeverAskTMDB(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "")
	movies := e.list(t)
	for _, name := range []string{"Bare Harbor", "Unknown Cove", "Plain Wharf"} {
		if tag := movies[name].ImageTags["Primary"]; tag != "" {
			t.Errorf("%s has the poster tag %q", name, tag)
		}
	}
	if status, _ := e.image(t, movies["Bare Harbor"], "Primary", ""); status != http.StatusNotFound {
		t.Errorf("Bare Harbor's poster: %d", status)
	}
	gone := movies["Gone Pier"]
	if status, _ := e.image(t, gone, "Primary", gone.ImageTags["Primary"]); status != http.StatusNotFound {
		t.Errorf("Gone Pier's poster: %d", status)
	}
	if tag := e.list(t)["Gone Pier"].ImageTags["Primary"]; tag != "" {
		t.Errorf("the dead poster is advertised: %q", tag)
	}
	if asked := e.tmdb.requests("/"); len(asked) != 0 {
		t.Errorf("TMDB was asked %v without a key", asked)
	}
}

// Refresh metadata on a title asks TMDB for its poster again, at the next
// request of its image, as Jellyfin's refresh asks its providers again.
func TestRefreshAsksTMDBForThePosterAgain(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	bare := e.list(t)["Bare Harbor"]
	if status, _ := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK {
		t.Fatalf("Bare Harbor's poster: %d", status)
	}
	e.tmdb.poster("/3/movie/9301", "/p9301-new.jpg")
	if status, body := e.call(http.MethodPost, "/Items/"+bare.Id+"/Refresh?metadataRefreshMode=FullRefresh", app("tv", e.token), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	bare = e.list(t)["Bare Harbor"]
	if status, _ := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK {
		t.Fatalf("Bare Harbor's poster after the refresh: %d", status)
	}
	// The refresh also reads the title's credits again, on the same path
	// with append_to_response=credits: they are left out.
	details := slices.DeleteFunc(e.tmdb.requestsWithQueries("/3/movie/9301"), func(asked string) bool {
		return strings.Contains(asked, "append_to_response=credits")
	})
	if len(details) != 2 {
		t.Errorf("TMDB was asked %v", e.tmdb.requestsWithQueries("/3/movie/9301"))
	}
	if tag := e.list(t)["Bare Harbor"].ImageTags["Primary"]; tag != library.ImageTag(e.tmdb.URL+"/t/p/original/p9301-new.jpg") {
		t.Errorf("after the refresh, the poster tag is %q", tag)
	}
}

// In another metadata language than English, a title's poster is TMDB's
// in that language, else in English, else without text, the best rated
// of them, and its backdrop TMDB's without text first, as Jellyfin
// chooses them, from one request. What is kept is kept per language: a
// title looked up in English is looked up again in French.
func TestTMDBPostersFollowTheMetadataLanguage(t *testing.T) {
	t.Parallel()
	e := newPosterlessEnv(t, "k3y")
	e.setting(t, func(settings *accounts.Settings) { settings.Language = "en" })
	bare := e.list(t)["Bare Harbor"]
	if status, _ := e.image(t, bare, "Primary", bare.ImageTags["Primary"]); status != http.StatusOK {
		t.Fatalf("Bare Harbor's poster in English: %d", status)
	}
	image := func(path string) string { return library.ImageTag(e.tmdb.URL + "/t/p/original" + path) }
	if tag := e.list(t)["Bare Harbor"].ImageTags["Primary"]; tag != image("/p9301.jpg") {
		t.Fatalf("in English, the poster tag is %q", tag)
	}

	e.setting(t, func(settings *accounts.Settings) { settings.Language = "fr" })
	movies := e.list(t)
	bare, gone := movies["Bare Harbor"], movies["Gone Pier"]
	if tag := bare.ImageTags["Primary"]; tag == "" || tag == image("/p9301.jpg") {
		t.Errorf("in French, before the lookup, the poster tag is %q", tag)
	}
	for _, item := range []BaseItemDto{bare, gone} {
		if status, _ := e.image(t, item, "Primary", item.ImageTags["Primary"]); status != http.StatusOK {
			t.Errorf("%s's poster in French: %d", item.Name, status)
		}
	}
	movies = e.list(t)
	bare, gone = movies["Bare Harbor"], movies["Gone Pier"]
	if bare.ImageTags["Primary"] != image("/p9301-fr.jpg") || !slices.Equal(bare.BackdropImageTags, []string{image("/b9301-none.jpg")}) {
		t.Errorf("Bare Harbor in French: %v, backdrops %v", bare.ImageTags, bare.BackdropImageTags)
	}
	if gone.ImageTags["Primary"] != image("/p9302-en.jpg") {
		t.Errorf("Gone Pier, without a French poster: %v", gone.ImageTags)
	}
	const french = "append_to_response=images&include_image_language=fr%2Cen%2Cnull&language=fr"
	if asked := e.tmdb.requestsWithQueries("/3/movie/9301"); !slices.Equal(asked, []string{"/3/movie/9301?", "/3/movie/9301?" + french}) {
		t.Errorf("Bare Harbor's details were asked as %v", asked)
	}
	if asked := e.tmdb.requestsWithQueries("/3/movie/9302"); !slices.Equal(asked, []string{"/3/movie/9302?" + french}) {
		t.Errorf("Gone Pier's details were asked as %v", asked)
	}
}
