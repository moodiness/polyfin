package jellyfin

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// imageSearch is a server whose addons describe a movie, Searched, and a
// series, Show: Alpha, which lists both, and Beta, which only describes
// the movie. A fake TMDB knows them, and one image server serves the
// addons' artwork and TMDB's images, counting the requests by path.
type imageSearch struct {
	testServer
	admin, member, child string
	movie, show, episode string
	mu                   sync.Mutex
	imageHits, tmdbHits  map[string]int
	imagesURL            string
}

func (s *imageSearch) hits(counts map[string]int, path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return counts[path]
}

// pngOf is an opaque PNG picture of width by height.
func pngOf(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func newImageSearch(t *testing.T) *imageSearch {
	t.Helper()
	s := &imageSearch{imageHits: map[string]int{}, tmdbHits: map[string]int{}}
	sizes := map[string][2]int{
		"/alpha/poster.png": {600, 900}, "/alpha/background.png": {1280, 720}, "/alpha/wide.png": {1000, 562},
		"/beta/poster.png": {300, 450}, "/beta/logo.png": {400, 150}, "/secret.png": {10, 10},
		"/t/p/original/fr.png": {1000, 1500}, "/t/p/w342/fr.png": {342, 513}, "/t/p/w300/still.png": {300, 169},
		"/t/p/original/still.png": {1920, 1080},
	}
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.imageHits[r.URL.Path]++
		s.mu.Unlock()
		size, ok := sizes[r.URL.Path]
		if !ok {
			size = [2]int{20, 30}
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngOf(t, size[0], size[1]))
	}))
	t.Cleanup(images.Close)
	s.imagesURL = images.URL
	// TMDB gives null for an image without a language.
	poster := func(path string, language any, rating float64, width, height int) map[string]any {
		return map[string]any{"file_path": path, "iso_639_1": language, "vote_average": rating, "vote_count": 10, "width": width, "height": height}
	}
	tmdbAnswers := map[string]any{
		"/3/find/tt10": map[string]any{"movie_results": []map[string]any{{"id": 550}}, "tv_results": []any{}},
		"/3/movie/550/images": map[string]any{
			"posters": []any{poster("/en.png", "en", 5, 1000, 1500), poster("/fr.png", "fr", 8, 1000, 1500),
				poster("/none.png", nil, 6, 1000, 1500), poster("/de.png", "de", 9, 1000, 1500)},
			"backdrops": []any{poster("/back.png", nil, 4, 1920, 1080)},
			"logos":     []any{poster("/logo.svg", "en", 7, 500, 200), poster("/logo.png", "en", 3, 500, 200)},
		},
		"/3/tv/77/season/1/episode/1/images": map[string]any{"stills": []any{poster("/still.png", nil, 6, 1920, 1080)}},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.tmdbHits[r.URL.Path]++
		s.mu.Unlock()
		if r.URL.Query().Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		answer, ok := tmdbAnswers[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(api.Close)

	generated := "polyfin-generated:poster?" + url.Values{"v": {"1"}, "name": {"x"}, "poster": {images.URL + "/secret.png"}}.Encode()
	alpha := map[string]stremio.Meta{
		"tt10": {ID: "tt10", Type: "movie", Name: "Searched", Poster: images.URL + "/alpha/poster.png", Background: images.URL + "/alpha/background.png",
			LandscapePoster: images.URL + "/alpha/wide.png", Logo: generated, Extras: &stremio.Extras{Certification: "R"}},
		"tt20": {ID: "tt20", Type: "series", Name: "Show", TmdbID: "77", Poster: images.URL + "/alpha/show.png",
			Videos: []stremio.Video{{ID: "tt20:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"}}},
	}
	beta := map[string]stremio.Meta{
		"tt10": {ID: "tt10", Type: "movie", Name: "Searched", Poster: images.URL + "/beta/poster.png", Logo: images.URL + "/beta/logo.png"},
	}
	addon := func(name string, metas map[string]stremio.Meta, catalogs []stremio.Catalog) string {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
			parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
			switch {
			case path == "/manifest":
				_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: strings.ToLower(name), Name: name, Version: "1",
					Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
					Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}}, Catalogs: catalogs})
			case len(parts) >= 3 && parts[0] == "catalog":
				var listed []stremio.Meta
				for _, m := range metas {
					if m.Type == parts[1] {
						m.Videos = nil
						listed = append(listed, m)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": listed})
			case len(parts) == 3 && parts[0] == "meta":
				if m, ok := metas[parts[2]]; ok {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": m})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"meta": nil})
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)
		return server.URL + "/manifest.json"
	}
	s.testServer = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.TMDB = tmdb.New(api.URL+"/3", images.URL+"/t/p/", "test", o.Accounts.Settings)
	})
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	for _, manifest := range []string{
		addon("Alpha", alpha, []stremio.Catalog{{Type: "movie", ID: "movies", Name: "Movies"}, {Type: "series", ID: "shows", Name: "Shows"}}),
		addon("Beta", beta, nil),
	} {
		if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
			t.Fatal(err)
		}
	}
	s.admin, s.member, s.child = s.signIn("admin", "dashboard"), s.signIn("member", "tv"), s.signIn("child", "tablet")
	for name, id := range s.viewIDs(t, s.admin) {
		var page QueryResult
		s.get(t, "/Items?ParentId="+id, s.admin, &page)
		for _, item := range page.Items {
			switch {
			case name == "Movies" && item.Name == "Searched":
				s.movie = item.Id
			case name == "Shows" && item.Name == "Show":
				s.show = item.Id
			}
		}
	}
	if s.movie == "" || s.show == "" {
		t.Fatalf("titles: %q %q", s.movie, s.show)
	}
	var episodes QueryResult
	if status := s.get(t, "/Shows/"+s.show+"/Episodes", s.admin, &episodes); status != http.StatusOK || len(episodes.Items) != 1 {
		t.Fatalf("episodes: %d %+v", status, episodes.Items)
	}
	s.episode = episodes.Items[0].Id
	return s
}

// remote lists an item's remote images as token, with query.
func (s *imageSearch) remote(t *testing.T, token, item, query string) RemoteImageResult {
	t.Helper()
	var result RemoteImageResult
	if status := s.get(t, "/Items/"+item+"/RemoteImages?"+query, token, &result); status != http.StatusOK {
		t.Fatalf("remote images of %s?%s: %d", item, query, status)
	}
	return result
}

// providers lists an item's image providers, as JSON.
func (s *imageSearch) providers(t *testing.T, item string) string {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Items/"+item+"/RemoteImages/Providers", app("dashboard", s.admin), nil)
	if status != http.StatusOK {
		t.Fatalf("providers of %s: %d %s", item, status, body)
	}
	return strings.TrimSpace(string(body))
}

// download asks to keep imageUrl as an item's image of imageType, as token.
func (s *imageSearch) download(token, item, imageType, imageURL string) int {
	status, _ := s.call(http.MethodPost, "/Items/"+item+"/RemoteImages/Download?"+url.Values{
		"Type": {imageType}, "ImageUrl": {imageURL}, "ProviderName": {"Whatever"}}.Encode(), app("dashboard", token), nil)
	return status
}

// width fetches an image without credentials and returns its status,
// content type and width.
func (s *imageSearch) width(t *testing.T, address string) (int, string, int) {
	t.Helper()
	response, body := s.fetch(t, address)
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return response.StatusCode, response.Header.Get("Content-Type"), 0
	}
	return response.StatusCode, response.Header.Get("Content-Type"), config.Width
}

func names(images []RemoteImageInfo) []string {
	var names []string
	for _, i := range images {
		names = append(names, i.ProviderName+" "+i.Type+" "+i.Language)
	}
	return names
}

func TestRemoteImagesWithoutTMDBKeyListTheAddonsOnly(t *testing.T) {
	t.Parallel()
	s := newImageSearch(t)
	if got := s.providers(t, s.movie); got != `[{"Name":"Addons","SupportedImages":["Primary","Backdrop","Logo","Thumb"]}]` {
		t.Errorf("movie's providers: %s", got)
	}
	if got := s.providers(t, s.episode); got != `[]` {
		t.Errorf("episode's providers: %s", got)
	}
	primary := s.remote(t, s.admin, s.movie, "type=Primary&limit=10")
	if strings.Join(names(primary.Images), ",") != "Alpha Primary ,Beta Primary " || primary.TotalRecordCount != 2 ||
		strings.Join(primary.Providers, ",") != "Addons" {
		t.Fatalf("posters: %+v", primary)
	}
	base := s.url + "/Items/" + s.movie + "/RemoteImages/Image?image="
	for _, i := range primary.Images {
		if !strings.HasPrefix(i.Url, base) || !strings.HasPrefix(i.ThumbnailUrl, base) || i.CommunityRating != nil || i.Width != nil {
			t.Errorf("addresses: %+v", i)
		}
	}
	// Images are relayed without credentials; a thumbnail is resized.
	if status, contentType, width := s.width(t, primary.Images[0].Url); status != http.StatusOK || contentType != "image/png" || width != 600 {
		t.Errorf("Alpha's poster: %d %s %d", status, contentType, width)
	}
	if status, _, width := s.width(t, primary.Images[0].ThumbnailUrl); status != http.StatusOK || width != remoteThumbnailWidth {
		t.Errorf("Alpha's thumbnail: %d %d", status, width)
	}
	if status, _, _ := s.width(t, primary.Images[0].Url[:len(primary.Images[0].Url)-2]+"AA"); status != http.StatusNotFound {
		t.Errorf("a forged address: %d", status)
	}
	if status, _, _ := s.width(t, strings.Replace(primary.Images[0].Url, s.movie, s.show, 1)); status != http.StatusNotFound {
		t.Errorf("another item's address: %d", status)
	}
	// Alpha's logo, in the scheme of generated images, is no image.
	if logos := s.remote(t, s.admin, s.movie, "type=Logo"); strings.Join(names(logos.Images), ",") != "Beta Logo " {
		t.Errorf("logos: %v", names(logos.Images))
	}
	if thumbs := s.remote(t, s.admin, s.movie, "type=Thumb"); len(thumbs.Images) != 2 || thumbs.Images[0].ProviderName != "Alpha" {
		t.Errorf("wide images: %v", names(thumbs.Images))
	}
	for _, query := range []string{"type=Banner", "type=Primary&providerName=TMDB"} {
		if result := s.remote(t, s.admin, s.movie, query); len(result.Images) != 0 || result.TotalRecordCount != 0 {
			t.Errorf("%s: %+v", query, result)
		}
	}
	if all := s.remote(t, s.admin, s.movie, ""); all.TotalRecordCount != 6 {
		t.Errorf("every type: %v", names(all.Images))
	}
	// Members list what they can open; parental control applies.
	if result := s.remote(t, s.member, s.movie, "type=Primary"); result.TotalRecordCount != 2 {
		t.Errorf("member: %+v", result)
	}
	if status, _ := s.call(http.MethodGet, "/Items/"+s.movie+"/RemoteImages?type=Primary", app("tablet", s.child), nil); status != http.StatusNotFound {
		t.Errorf("a child kept from the movie: %d", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tmdbHits) != 0 || s.imageHits["/secret.png"] != 0 {
		t.Errorf("TMDB asked %v, generated poster fetched %d times", s.tmdbHits, s.imageHits["/secret.png"])
	}
}

func TestRemoteImagesSearchTMDBWithAKey(t *testing.T) {
	t.Parallel()
	s := newImageSearch(t)
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey, settings.Language = "k3y", "fr" })
	if got := s.providers(t, s.movie); got != `[{"Name":"TMDB","SupportedImages":["Primary","Backdrop","Logo","Thumb"]},`+
		`{"Name":"Addons","SupportedImages":["Primary","Backdrop","Logo","Thumb"]}]` {
		t.Errorf("movie's providers: %s", got)
	}
	if got := s.providers(t, s.episode); got != `[{"Name":"TMDB","SupportedImages":["Primary"]}]` {
		t.Errorf("episode's providers: %s", got)
	}
	// The server's language first, then images without one, by votes.
	posters := s.remote(t, s.admin, s.movie, "type=Primary&providerName=TMDB&includeAllLanguages=false")
	if strings.Join(names(posters.Images), ",") != "TMDB Primary fr,TMDB Primary " || strings.Join(posters.Providers, ",") != "TMDB,Addons" {
		t.Fatalf("posters: %v %v", names(posters.Images), posters.Providers)
	}
	fr := posters.Images[0]
	if fr.Width == nil || *fr.Width != 1000 || *fr.Height != 1500 || fr.CommunityRating == nil || *fr.CommunityRating != 8 || *fr.VoteCount != 10 ||
		!strings.HasPrefix(fr.ThumbnailUrl, s.url+"/Items/"+s.movie+"/RemoteImages/Image?image=") || strings.Contains(fr.Url, s.imagesURL) {
		t.Errorf("French poster: %+v", fr)
	}
	all := s.remote(t, s.admin, s.movie, "type=Primary&providerName=TMDB&includeAllLanguages=true")
	if strings.Join(names(all.Images), ",") != "TMDB Primary fr,TMDB Primary ,TMDB Primary de,TMDB Primary en" {
		t.Errorf("all languages: %v", names(all.Images))
	}
	if page := s.remote(t, s.admin, s.movie, "type=Primary&startIndex=1&limit=2"); page.TotalRecordCount != 4 ||
		strings.Join(names(page.Images), ",") != "TMDB Primary ,Alpha Primary " {
		t.Errorf("page: %d %v", page.TotalRecordCount, names(page.Images))
	}
	if logos := s.remote(t, s.admin, s.movie, "type=Logo&providerName=TMDB&includeAllLanguages=true"); len(logos.Images) != 1 {
		t.Errorf("logos, SVG left out: %v", names(logos.Images))
	}
	if backdrops := s.remote(t, s.admin, s.movie, "type=Thumb&providerName=TMDB"); strings.Join(names(backdrops.Images), ",") != "TMDB Thumb " {
		t.Errorf("wide images: %v", names(backdrops.Images))
	}
	if status, _, width := s.width(t, fr.ThumbnailUrl); status != http.StatusOK || width != 342 || s.hits(s.imageHits, "/t/p/w342/fr.png") != 1 {
		t.Errorf("thumbnail: %d %d", status, width)
	}
	// The movie's TMDB identifier was looked up by its IMDb one, once:
	// TMDB's answers are kept.
	if s.hits(s.tmdbHits, "/3/find/tt10") != 1 || s.hits(s.tmdbHits, "/3/movie/550/images") != 1 {
		t.Errorf("TMDB asked %v", s.tmdbHits)
	}
	// The series gives its TMDB identifier: its episode lists its stills.
	stills := s.remote(t, s.admin, s.episode, "type=Primary")
	if strings.Join(names(stills.Images), ",") != "TMDB Primary " || *stills.Images[0].Width != 1920 {
		t.Fatalf("stills: %+v", stills)
	}
	if status, _, width := s.width(t, stills.Images[0].ThumbnailUrl); status != http.StatusOK || width != 300 {
		t.Errorf("still's thumbnail: %d %d", status, width)
	}
	if s.hits(s.tmdbHits, "/3/find/tt20") != 0 {
		t.Errorf("the series was looked up: %v", s.tmdbHits)
	}
}

func TestDownloadedRemoteImageReplacesTheAddonsUntilDeleted(t *testing.T) {
	t.Parallel()
	s := newImageSearch(t)
	s.setting(t, func(settings *accounts.Settings) { settings.TMDBKey, settings.Language = "k3y", "fr" })
	tags := func() map[string]string {
		var dto BaseItemDto
		if status := s.get(t, "/Items/"+s.movie, s.admin, &dto); status != http.StatusOK {
			t.Fatalf("movie: %d", status)
		}
		return dto.ImageTags
	}
	before := tags()["Primary"]
	if before == "" {
		t.Fatal("the movie has no poster")
	}
	posters := s.remote(t, s.admin, s.movie, "type=Primary&providerName=TMDB")
	episode := s.remote(t, s.admin, s.episode, "type=Primary")
	chosen := posters.Images[0].Url
	for name, imageURL := range map[string]string{
		"an address Polyfin did not list": s.imagesURL + "/secret.png",
		"a thumbnail":                     posters.Images[0].ThumbnailUrl,
		"another item's image":            strings.Replace(episode.Images[0].Url, s.episode, s.movie, 1),
		"a forged address":                strings.Replace(chosen, "image=", "image=e", 1),
		"no address":                      "",
	} {
		if status := s.download(s.admin, s.movie, "Primary", imageURL); status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, status)
		}
	}
	if status := s.download(s.admin, s.movie, "Backdrop", chosen); status != http.StatusBadRequest {
		t.Errorf("a poster as a backdrop: %d", status)
	}
	if status := s.download(s.member, s.movie, "Primary", chosen); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if s.hits(s.imageHits, "/secret.png") != 0 || s.hits(s.imageHits, "/t/p/original/fr.png") != 0 || tags()["Primary"] != before {
		t.Fatalf("refused downloads fetched or kept an image: %v", tags())
	}

	if status := s.download(s.admin, s.movie, "Primary", chosen); status != http.StatusNoContent {
		t.Fatalf("download: %d", status)
	}
	after := tags()["Primary"]
	if after == "" || after == before || s.hits(s.imageHits, "/t/p/original/fr.png") != 1 {
		t.Errorf("tags after the download: %s, was %s", after, before)
	}
	// Kept as an upload is: a JPEG in the database, served by tag.
	if status, contentType, width := s.width(t, s.url+"/Items/"+s.movie+"/Images/Primary?tag="+after); status != http.StatusOK ||
		contentType != "image/jpeg" || width != 1000 {
		t.Errorf("downloaded poster: %d %s %d", status, contentType, width)
	}
	if status, _ := s.call(http.MethodDelete, "/Items/"+s.movie+"/Images/Primary", app("dashboard", s.admin), nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if got := tags()["Primary"]; got != before {
		t.Errorf("after the delete: %s, want the addon's %s", got, before)
	}
	if status, _, width := s.width(t, s.url+"/Items/"+s.movie+"/Images/Primary?tag="+before); status != http.StatusOK || width != 600 {
		t.Errorf("the addon's poster again: %d %d", status, width)
	}
}

// An addon's image is relayed under its addon's confinement: a member's
// own addon, confined to public addresses, cannot have Polyfin reach a
// local one through Edit images.
func TestConfinedAddonsImagesStayConfined(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var mu sync.Mutex
	hits := 0
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngOf(t, 60, 90))
	}))
	t.Cleanup(local.Close)
	meta := stremio.Meta{ID: "own:1", Type: "movie", Name: "Mine", Poster: local.URL + "/poster.png"}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "own", Name: "Own", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"own:"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}})
		case strings.HasPrefix(path, "/catalog/movie/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta}})
		case r.URL.Path == "/meta/movie/own:1.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	// No test server is on a public address: the owner's catalog and the
	// title's description are read while they are an administrator, whose
	// own addons are not confined. They then become a member, and their
	// addon is confined.
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	installed, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), addon.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	title := func() string {
		t.Helper()
		token := s.signIn("owner", "tv")
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)["Mine"], token, &page); status != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("the owner's library: %d %+v", status, page.Items)
		}
		return page.Items[0].Id
	}
	id := title()
	var result RemoteImageResult
	if status := s.get(t, "/Items/"+id+"/RemoteImages?type=Primary", s.signIn("owner", "tv"), &result); status != http.StatusOK || len(result.Images) != 1 {
		t.Fatalf("remote images while unconfined: %d %+v", status, result)
	}
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	title()
	admin := s.signIn("admin", "dashboard")
	result = RemoteImageResult{}
	if status := s.get(t, "/Items/"+id+"/RemoteImages?type=Primary", admin, &result); status != http.StatusOK || len(result.Images) != 1 ||
		result.Images[0].ProviderName != "Own" {
		t.Fatalf("remote images: %d %+v", status, result)
	}
	response, _ := s.fetch(t, result.Images[0].Url)
	status, _ := s.call(http.MethodPost, "/Items/"+id+"/RemoteImages/Download?"+url.Values{"type": {"Primary"},
		"imageUrl": {result.Images[0].Url}}.Encode(), app("dashboard", admin), nil)
	mu.Lock()
	defer mu.Unlock()
	if response.StatusCode != http.StatusForbidden || status != http.StatusForbidden || hits != 0 {
		t.Errorf("the confined addon's local poster: relayed %d, downloaded %d, reached %d times", response.StatusCode, status, hits)
	}
	if parsed, err := accounts.ParseID(id); err != nil {
		t.Fatal(err)
	} else if _, ok := s.library.UploadedArtwork(parsed, "Primary"); ok {
		t.Error("the refused image was kept")
	}
}
