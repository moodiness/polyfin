package jellyfin

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"slices"
	"testing"
	"time"
)

// studiosEnv is a posterlessEnv whose TMDB names North Studio, of a logo,
// the company of Bare Harbor, and Harbor Network, of none, that of Gone
// Pier, both read with the movies' details.
func studiosEnv(t *testing.T) (posterlessEnv, map[string]BaseItemDto) {
	t.Helper()
	e := newPosterlessEnv(t, "k3y")
	logo := pngOf(t, 200, 50)
	e.tmdb.mu.Lock()
	e.tmdb.details["/3/movie/9301"]["production_companies"] = []any{map[string]any{"id": 71, "name": "North Studio", "logo_path": "/north.png"}}
	e.tmdb.details["/3/movie/9302"]["production_companies"] = []any{map[string]any{"id": 72, "name": "Harbor Network", "logo_path": nil}}
	for _, path := range []string{"/3/movie/9301", "/3/movie/9302"} {
		e.tmdb.details[path]["credits"] = map[string]any{"cast": []any{}, "crew": []any{}}
	}
	e.tmdb.logos["/t/p/w500/north.png"] = logo
	e.tmdb.logos["/t/p/w500/north-new.png"] = logo
	e.tmdb.mu.Unlock()
	movies := e.list(t)
	// A page waits a moment for the details: on a loaded machine, they
	// show at a next open.
	for _, name := range []string{"Bare Harbor", "Gone Pier"} {
		var dto BaseItemDto
		for range 100 {
			if status := e.get(t, "/Items/"+movies[name].Id, e.token, &dto); status != http.StatusOK {
				t.Fatalf("%s: %d", name, status)
			}
			if dto.Studios != nil && len(*dto.Studios) > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if dto.Studios == nil || len(*dto.Studios) == 0 {
			t.Fatalf("%s shows no studio", name)
		}
		movies[name] = dto
	}
	return e, movies
}

// pngSize is the size of a PNG.
func pngSize(t *testing.T, body []byte) image.Point {
	t.Helper()
	config, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("no PNG: %v", err)
	}
	return image.Pt(config.Width, config.Height)
}

// A title's studio links to its page: its identifier opens as a Studio
// item, with the tags of its logo's images, also once the names given
// identifiers are forgotten, as after a restart. Its Thumb image is its
// logo on a wide plate, its Primary one on a square one, resized for
// smaller asks; asked with margin=narrow, the logo takes the plate less a
// sixteenth of each side rather than an eighth. A studio of no logo has
// none. The tags change with the logo.
func TestAStudioOpensWithItsLogo(t *testing.T) {
	t.Parallel()
	e, movies := studiosEnv(t)
	studios := *movies["Bare Harbor"].Studios
	if len(studios) != 1 || studios[0].Name != "North Studio" || studios[0].Id != nameID("studio", "North Studio") {
		t.Fatalf("Bare Harbor's studios: %+v", studios)
	}
	id := studios[0].Id
	open := func() BaseItemDto {
		t.Helper()
		var studio BaseItemDto
		if status := e.get(t, "/Items/"+id, e.token, &studio); status != http.StatusOK {
			t.Fatalf("opening North Studio: %d", status)
		}
		return studio
	}
	studio := open()
	if studio.Type != "Studio" || studio.Name != "North Studio" || studio.ImageTags["Thumb"] == "" || studio.ImageTags["Primary"] == "" ||
		studio.MovieCount == nil || *studio.MovieCount != 1 {
		t.Fatalf("North Studio: %+v", studio)
	}
	e.handler.names = namedItems{}
	if again := open(); again.Name != "North Studio" || again.ImageTags["Thumb"] != studio.ImageTags["Thumb"] {
		t.Errorf("after a restart: %+v", again)
	}

	for _, ask := range []struct {
		imageType, query string
		size             image.Point
	}{{"Thumb", "", image.Pt(960, 540)}, {"Primary", "", image.Pt(600, 600)}, {"Thumb", "&maxWidth=480", image.Pt(480, 270)}} {
		status, body := e.image(t, studio, ask.imageType, studio.ImageTags[ask.imageType]+ask.query)
		if status != http.StatusOK || pngSize(t, body) != ask.size {
			t.Errorf("%s%s: %d", ask.imageType, ask.query, status)
		}
	}
	// The logo, black, across the middle row of the Thumb: 4:1, it fills
	// the width of the plate less its margins.
	logoWidth := func(body []byte) int {
		t.Helper()
		plate, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		black := 0
		for x := range plate.Bounds().Dx() {
			if r, _, _, _ := plate.At(x, plate.Bounds().Dy()/2).RGBA(); r < 0x8000 {
				black++
			}
		}
		return black
	}
	for _, ask := range []struct {
		query string
		width int
	}{{"", 960 - 2*120}, {"&margin=narrow", 960 - 2*60}, {"&MARGIN=Narrow", 960 - 2*60}} {
		status, body := e.image(t, studio, "Thumb", studio.ImageTags["Thumb"]+ask.query)
		if status != http.StatusOK || pngSize(t, body) != image.Pt(960, 540) {
			t.Fatalf("Thumb%s: %d", ask.query, status)
		}
		if got := logoWidth(body); got < ask.width-2 || got > ask.width+2 {
			t.Errorf("Thumb%s: the logo is %d wide, want %d", ask.query, got, ask.width)
		}
	}
	harbor := (*movies["Gone Pier"].Studios)[0]
	var plain BaseItemDto
	if status := e.get(t, "/Items/"+harbor.Id, e.token, &plain); status != http.StatusOK || plain.Name != "Harbor Network" || len(plain.ImageTags) != 0 {
		t.Errorf("Harbor Network: %d %+v", status, plain)
	}
	if status, _ := e.image(t, plain, "Thumb", ""); status != http.StatusNotFound {
		t.Errorf("Harbor Network's Thumb: %d", status)
	}

	if _, err := e.pool.Exec(t.Context(), `UPDATE tmdb_studios SET logo_path = '/north-new.png' WHERE tmdb_id = 71`); err != nil {
		t.Fatal(err)
	}
	changed := open()
	if changed.ImageTags["Thumb"] == studio.ImageTags["Thumb"] || changed.ImageTags["Primary"] == studio.ImageTags["Primary"] {
		t.Errorf("the tags did not change with the logo: %v", changed.ImageTags)
	}
	if status, body := e.image(t, changed, "Primary", changed.ImageTags["Primary"]); status != http.StatusOK || pngSize(t, body) != image.Pt(600, 600) {
		t.Errorf("the new Primary: %d", status)
	}

	// Its titles list by its identifier and by its name.
	for _, path := range []string{"/Items?recursive=true&studioIds=" + id, "/Items?recursive=true&studios=north%20studio"} {
		var page QueryResult
		if status := e.get(t, path, e.token, &page); status != http.StatusOK || len(page.Items) != 1 || page.Items[0].Name != "Bare Harbor" ||
			page.TotalRecordCount != 1 {
			t.Errorf("%s: %d %+v", path, status, page)
		}
	}
}

// /Studios lists the studios of the libraries' titles by name, with their
// counts and the tags of their logos, narrowed by search, letters,
// library and favorites, in pages.
func TestStudiosListsTheLibrariesStudios(t *testing.T) {
	t.Parallel()
	e, movies := studiosEnv(t)
	list := func(query string) QueryResult {
		t.Helper()
		var page QueryResult
		if status := e.get(t, "/Studios?"+query, e.token, &page); status != http.StatusOK {
			t.Fatalf("/Studios?%s: %d", query, status)
		}
		return page
	}
	named := func(page QueryResult) []string {
		var names []string
		for _, item := range page.Items {
			names = append(names, item.Name)
		}
		return names
	}
	all := list("")
	if !slices.Equal(named(all), []string{"Harbor Network", "North Studio"}) || all.TotalRecordCount != 2 {
		t.Fatalf("the studios: %v %d", named(all), all.TotalRecordCount)
	}
	harbor, north := all.Items[0], all.Items[1]
	if north.Type != "Studio" || north.Id != (*movies["Bare Harbor"].Studios)[0].Id || north.ImageTags["Thumb"] == "" || north.ImageTags["Primary"] == "" ||
		north.MovieCount == nil || *north.MovieCount != 1 || north.SeriesCount == nil || *north.SeriesCount != 0 || len(harbor.ImageTags) != 0 {
		t.Errorf("their items: %+v %+v", harbor, north)
	}
	for query, want := range map[string][]string{
		"searchTerm=NORTH":              {"North Studio"},
		"nameStartsWith=h":              {"Harbor Network"},
		"nameLessThan=I":                {"Harbor Network"},
		"nameStartsWithOrGreater=I":     {"North Studio"},
		"parentId=" + e.library:         {"Harbor Network", "North Studio"},
		"parentId=" + nameID("x", "y"):  nil,
		"startIndex=1&limit=1":          {"North Studio"},
		"isFavorite=true":               nil,
		"isFavorite=false&searchTerm=r": {"Harbor Network", "North Studio"},
	} {
		if got := named(list(query)); !slices.Equal(got, want) {
			t.Errorf("%s: %v", query, got)
		}
	}
	if paged := list("startIndex=1&limit=1"); paged.TotalRecordCount != 2 {
		t.Errorf("a page counts %d", paged.TotalRecordCount)
	}
	if status, body := e.call(http.MethodPost, "/UserFavoriteItems/"+north.Id, app("web", e.token), nil); status != http.StatusOK {
		t.Fatalf("marking North Studio a favorite: %d %s", status, body)
	}
	if got := named(list("isFavorite=true")); !slices.Equal(got, []string{"North Studio"}) {
		t.Errorf("the favorites: %v", got)
	}
	if got := named(list("isFavorite=false")); !slices.Equal(got, []string{"Harbor Network"}) {
		t.Errorf("the others: %v", got)
	}
	var studio BaseItemDto
	if status := e.get(t, "/Items/"+north.Id, e.token, &studio); status != http.StatusOK || !studio.UserData.IsFavorite {
		t.Errorf("North Studio's page: %d %+v", status, studio.UserData)
	}
}
