package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A title listed by an addon that does not describe it, a local folder or
// a catalog addon, shows the artwork of the addon that describes it: each
// image its details list is served, whichever library listed it. An extra
// of the local title shows the title's backdrop, served by the extra's
// item, and its poster as its Primary image.
func TestListedTitlesServeTheArtworkTheirDetailsList(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	member := s.user("member", nil)

	// Each image the metadata addon gives is a picture of its own size.
	sizes := map[string]int{"poster": 20, "background": 30, "logo": 40, "landscape": 50}
	pictures := map[string]string{}
	for name, width := range sizes {
		pictures[name] = string(picture(t, width, 10, 255))
	}
	var server *httptest.Server
	described := func(id, name string) stremio.Meta {
		art := server.URL + "/art/" + id + "-"
		return stremio.Meta{ID: id, Type: "movie", Name: name, Year: "2008", Poster: art + "poster.png", Background: art + "background.png",
			Logo: art + "logo.png", LandscapePoster: art + "landscape.png"}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/metadata/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "metadata", Name: "Metadata", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "meta"}}})
		case path == "/metadata/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": described("tt1000", "Shelved")})
		case path == "/metadata/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": described("tt3000", "Listed")})
		case path == "/listings/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "listings", Name: "Listings", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: "Picks"}}})
		case strings.HasPrefix(path, "/listings/catalog/movie/picks"):
			// The listing gives a poster only, as catalogs often do.
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "tt3000", Type: "movie", Name: "Listed", Poster: server.URL + "/art/tt3000-poster.png"}}})
		case strings.HasPrefix(path, "/art/"):
			name := strings.TrimSuffix(path[strings.LastIndex(path, "-")+1:], ".png")
			if body, ok := pictures[name]; ok {
				w.Header().Set("Content-Type", "image/png")
				_, _ = io.WriteString(w, body)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	for _, manifest := range []string{"/metadata/manifest.json", "/listings/manifest.json"} {
		if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+manifest, false); err != nil {
			t.Fatal(err)
		}
	}

	// A local folder whose movie is linked to a title the metadata addon
	// describes, without artwork beside it, and its local trailer.
	dir := t.TempDir()
	for _, name := range []string{"Shelved (2008) 1080p.mkv", "Shelved (2008) 1080p-trailer.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("\x1a\x45\xdf\xa3 media bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	folder, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), folder.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), folder.ID, "Shelved (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}

	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	listed := map[string]string{}
	for _, view := range views.Items {
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, token, &page)
		for _, item := range page.Items {
			listed[view.Name+"/"+item.Name] = item.Id
		}
	}
	for _, title := range []string{"Shelf/Shelved", "Picks/Listed"} {
		id := listed[title]
		if id == "" {
			t.Fatalf("%s is not listed: %v", title, listed)
		}
		var details BaseItemDto
		if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+id, token, &details); status != http.StatusOK {
			t.Fatalf("%s: details %d", title, status)
		}
		tags := map[string]string{"Primary": details.ImageTags["Primary"], "Logo": details.ImageTags["Logo"], "Thumb": details.ImageTags["Thumb"]}
		if len(details.BackdropImageTags) == 1 {
			tags["Backdrop"] = details.BackdropImageTags[0]
		}
		for imageType, want := range map[string]string{"Primary": "poster", "Backdrop": "background", "Logo": "logo", "Thumb": "landscape"} {
			if tags[imageType] == "" {
				t.Errorf("%s: the details list no %s: %v %v", title, imageType, details.ImageTags, details.BackdropImageTags)
				continue
			}
			response, body := fetchURL(t, s.url+"/Items/"+id+"/Images/"+imageType+"?tag="+tags[imageType], nil)
			if response.StatusCode != http.StatusOK || body != pictures[want] || response.Header.Get("ETag") != `"`+tags[imageType]+`"` {
				t.Errorf("%s: %s image %d, ETag %s for tag %s", title, imageType, response.StatusCode, response.Header.Get("ETag"), tags[imageType])
			}
		}
	}

	var trailers []BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+listed["Shelf/Shelved"]+"/LocalTrailers", token, &trailers)
	if len(trailers) != 1 {
		t.Fatalf("local trailers: %+v", trailers)
	}
	var trailer BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+trailers[0].Id, token, &trailer)
	for _, listing := range []BaseItemDto{trailers[0], trailer} {
		if len(listing.BackdropImageTags) != 1 {
			t.Errorf("the trailer lists no backdrop: %v", listing.BackdropImageTags)
			continue
		}
		tag := listing.BackdropImageTags[0]
		response, body := fetchURL(t, s.url+"/Items/"+trailer.Id+"/Images/Backdrop?tag="+tag, nil)
		if response.StatusCode != http.StatusOK || body != pictures["background"] || response.Header.Get("ETag") != `"`+tag+`"` {
			t.Errorf("the trailer's backdrop: %d, ETag %s for tag %s", response.StatusCode, response.Header.Get("ETag"), tag)
		}
	}
	if response, body := fetchURL(t, s.url+"/Items/"+trailer.Id+"/Images/Primary", nil); response.StatusCode != http.StatusOK || body != pictures["poster"] {
		t.Errorf("the trailer's Primary image: %d", response.StatusCode)
	}
}
