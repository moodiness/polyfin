package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/stremio"
)

// greyPoster is a 600 by 900 grey JPEG.
func greyPoster(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0x80
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// badgedAddon serves two movies, tt0000 and tt0001, rated 7.5 by the
// addon, and a series, tt0100, rated 8, whose second season started 5
// days ago, all with the same poster.
func badgedAddon(t *testing.T, poster []byte) string {
	t.Helper()
	var server *httptest.Server
	day := func(days int) string { return time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339) }
	movie := func(i int) stremio.Meta {
		return stremio.Meta{ID: fmt.Sprintf("tt000%d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i), Poster: server.URL + "/poster.jpg",
			ReleaseInfo: "2020", ImdbRating: "7.5"}
	}
	show := func() stremio.Meta {
		return stremio.Meta{ID: "tt0100", Type: "series", Name: "Show", Poster: server.URL + "/poster.jpg", ReleaseInfo: "2020-",
			ImdbRating: "8", Videos: []stremio.Video{
				{ID: "tt0100:1:1", Season: 1, Episode: 1, Released: day(400)},
				{ID: "tt0100:1:2", Season: 1, Episode: 2, Released: day(393)},
				{ID: "tt0100:2:1", Season: 2, Episode: 1, Released: day(5)},
				{ID: "tt0100:2:2", Season: 2, Episode: 2, Released: day(-2)},
			}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "badged", Name: "Badged", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie(0), movie(1)}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			listed := show()
			listed.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{listed}})
		case path == "/meta/series/tt0100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show()})
		case path == "/meta/movie/tt0000.json", path == "/meta/movie/tt0001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie(int(path[len(path)-6] - '0'))})
		case path == "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(poster)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// badged is a server listing badgedAddon's titles to a member, with their
// ratings looked up on a fake MDBList once a key is set.
type badged struct {
	testServer
	token  string
	member accounts.User
	views  map[string]string
	poster []byte
}

func newBadged(t *testing.T) badged {
	t.Helper()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	f := newFakeMDBList(t)
	lookups := ratings.New(s.pool, f.url, "test", slog.New(slog.NewTextHandler(io.Discard, nil)), s.store.Settings)
	t.Cleanup(lookups.Close)
	s.library.UseRatings(lookups)
	poster := greyPoster(t)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), badgedAddon(t, poster), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	return badged{testServer: s, token: token, member: member, views: ids, poster: poster}
}

// tags lists the Primary image tags of the titles of the libraries, by
// name, with their identifiers.
func (b badged) tags(t *testing.T) (tags, ids map[string]string) {
	t.Helper()
	tags, ids = map[string]string{}, map[string]string{}
	for _, view := range []string{"Top", "Shows"} {
		var page QueryResult
		if status := b.get(t, "/Items?ParentId="+b.views[view], b.token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", view, status)
		}
		for _, item := range page.Items {
			tags[item.Name], ids[item.Name] = item.ImageTags["Primary"], item.Id
		}
	}
	return tags, ids
}

// badgesIn returns what a badged tag shows, empty for a tag of the image
// itself. The signature may hold dashes.
func badgesIn(tag string) string {
	parts := strings.SplitN(tag, "-", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

// fetch asks for an item's Primary image by tag, without credentials as
// apps do.
func (b badged) fetch(t *testing.T, id, tag, query string) (int, http.Header, []byte) {
	t.Helper()
	return b.send(http.MethodGet, "/Items/"+id+"/Images/Primary?tag="+tag+query, "", "", "")
}

// While turned on, the Primary images of movies and series carry their
// ratings and a series its new season, every app seeing them through a
// tag of their own: the tag changes as MDBList's ratings come, and turning
// the badges off gives the image and its tag back. Hidden titles stay out
// of the listings, and a tag whose badges Polyfin did not sign draws
// none.
func TestPostersCarryBadgesWhileTurnedOn(t *testing.T) {
	t.Parallel()
	b := newBadged(t)
	plain, ids := b.tags(t)
	for name, tag := range plain {
		if tag == "" || strings.Contains(tag, "-") {
			t.Fatalf("%s before badges: %q", name, tag)
		}
	}
	// The series' episodes are known once its page was opened.
	if status := b.get(t, "/Users/"+b.member.ID.String()+"/Items/"+ids["Show"], b.token, nil); status != http.StatusOK {
		t.Fatalf("the show: %d", status)
	}
	eventually(t, "the show's description kept", func() bool {
		var n int
		_ = b.pool.QueryRow(t.Context(), "SELECT count(*) FROM metas WHERE meta_id = 'tt0100'").Scan(&n)
		return n > 0
	})
	b.setting(t, func(settings *accounts.Settings) {
		settings.PosterBadges = true
		settings.PosterRatings = true
	})
	addons, _ := b.tags(t)
	want := map[string]string{"Movie 0": "m75_x_x_0", "Movie 1": "m75_x_x_0", "Show": "m80_x_x_2"}
	for name, shown := range want {
		if got := badgesIn(addons[name]); got != shown || !strings.HasPrefix(addons[name], plain[name]+"-") {
			t.Errorf("%s with the addon's rating: %q, want badges %s", name, addons[name], shown)
		}
	}
	var hover PosterRatings
	b.get(t, "/Polyfin/Items/Ratings?Ids="+ids["Movie 0"], b.token, &hover)
	if hover.Enabled {
		t.Error("the web player shows ratings on hover over the badges")
	}

	status, header, body := b.fetch(t, ids["Show"], addons["Show"], "&maxWidth=300")
	if status != http.StatusOK || header.Get("Content-Type") != "image/jpeg" || header.Get("Cache-Control") != immutableImage {
		t.Fatalf("the show's badged poster: %d %v", status, header)
	}
	drawn, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if width := drawn.Bounds().Dx(); width != 320 {
		t.Errorf("badged poster %d wide, want 320", width)
	}
	// The new season's green badge sits in the top right corner, where
	// the poster is grey.
	if r, g, _, _ := color.RGBAModel.Convert(drawn.At(268, 26)).RGBA(); g>>8 < r>>8+40 {
		t.Errorf("no new season's badge: %v", drawn.At(268, 26))
	}

	b.setting(t, func(settings *accounts.Settings) { settings.MDBListRatingsKey = "server-key" })
	want = map[string]string{"Movie 0": "m81i_97_60_0", "Movie 1": "m62i_40_90_0", "Show": "m89i_88_91_2"}
	var rated map[string]string
	eventually(t, "MDBList's ratings in the tags", func() bool {
		rated, _ = b.tags(t)
		for name, shown := range want {
			if badgesIn(rated[name]) != shown {
				return false
			}
		}
		return true
	})
	b.setting(t, func(settings *accounts.Settings) { settings.PosterBadgeSize = "large" })
	if large, _ := b.tags(t); badgesIn(large["Movie 0"]) != "l81i_97_60_0" {
		t.Errorf("large badges: %q", large["Movie 0"])
	}

	if status, _, _ := b.send(http.MethodPost, "/Polyfin/UserItems/"+ids["Movie 1"]+"/Hidden", app("tv", b.token), "", ""); status != http.StatusNoContent {
		t.Fatalf("hiding Movie 1: %d", status)
	}
	if listed, _ := b.tags(t); listed["Movie 1"] != "" || listed["Movie 0"] == "" {
		t.Errorf("listed with Movie 1 hidden: %v", listed)
	}

	forged := strings.Replace(rated["Movie 0"], "-m81i_97_60_0-", "-m10i_97_60_0-", 1)
	if status, _, body := b.fetch(t, ids["Movie 0"], forged, ""); status != http.StatusOK || !bytes.Equal(body, b.poster) {
		t.Errorf("a forged tag: %d, the poster as it is %v", status, bytes.Equal(body, b.poster))
	}

	b.setting(t, func(settings *accounts.Settings) { settings.PosterBadges = false })
	if off, _ := b.tags(t); off["Movie 0"] != plain["Movie 0"] || off["Show"] != plain["Show"] {
		t.Errorf("tags once off: %v, want %v", off, plain)
	}
	status, header, body = b.fetch(t, ids["Movie 0"], rated["Movie 0"], "")
	if status != http.StatusOK || !bytes.Equal(body, b.poster) || header.Get("Cache-Control") != taggedImage {
		t.Errorf("a badged tag once off: %d %v, the poster as it is %v", status, header, bytes.Equal(body, b.poster))
	}
}

// The admin app previews the badges on real titles at any size, whether
// they are on or not, through tags apps may not keep.
func TestPosterPreviewsDrawBadgesWhileOff(t *testing.T) {
	t.Parallel()
	b := newBadged(t)
	b.tags(t)
	previews, err := b.handler.PosterPreviews(t.Context(), b.member, "small", 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 3 {
		t.Fatalf("previews: %+v", previews)
	}
	for _, preview := range previews {
		if !strings.HasPrefix(preview.BadgedTag, preview.Tag+"-s") {
			t.Errorf("%s: %+v", preview.Name, preview)
		}
	}
	status, header, body := b.fetch(t, previews[0].ID.String(), previews[0].BadgedTag, "")
	if status != http.StatusOK || header.Get("Cache-Control") != "no-store" || bytes.Equal(body, b.poster) {
		t.Errorf("a preview: %d %v, the poster as it is %v", status, header, bytes.Equal(body, b.poster))
	}
}

// The poster address template replaces the addons' posters of movies and
// series with its own, filled in with their identifiers and type, and the
// addon's comes back for a title whose templated poster cannot be
// downloaded, and for every title once the template is emptied.
func TestPosterTemplateReplacesTheAddonsPosters(t *testing.T) {
	t.Parallel()
	b := newBadged(t)
	templated := bytes.Repeat([]byte("templated "), 10)
	posters := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/tt0000.jpg" && r.URL.Path != "/series/tt0100.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(templated)
	}))
	t.Cleanup(posters.Close)
	plain, ids := b.tags(t)
	b.setting(t, func(settings *accounts.Settings) { settings.PosterTemplate = posters.URL + "/{type}/{imdb_id}.jpg" })
	tags, _ := b.tags(t)
	for name, path := range map[string]string{"Movie 0": "/movie/tt0000.jpg", "Movie 1": "/movie/tt0001.jpg", "Show": "/series/tt0100.jpg"} {
		if want := library.ImageTag(posters.URL + path); tags[name] != want {
			t.Errorf("%s: tag %q, want the template's %q", name, tags[name], want)
		}
	}
	for name, want := range map[string][]byte{"Movie 0": templated, "Show": templated, "Movie 1": b.poster} {
		status, header, body := b.fetch(t, ids[name], tags[name], "")
		if status != http.StatusOK || !bytes.Equal(body, want) {
			t.Errorf("%s: %d, %d bytes", name, status, len(body))
		}
		// Apps keep the addon's poster standing in for the template's for
		// a while only.
		if name == "Movie 1" && header.Get("Cache-Control") != taggedImage {
			t.Errorf("the addon's poster in place of the template's: %v", header)
		}
	}
	b.setting(t, func(settings *accounts.Settings) { settings.PosterTemplate = "" })
	if back, _ := b.tags(t); back["Movie 0"] != plain["Movie 0"] || back["Show"] != plain["Show"] {
		t.Errorf("tags without the template: %v, want %v", back, plain)
	}
}
