package jellyfin

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	xdraw "golang.org/x/image/draw"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/mosaic"
	"github.com/moodiness/polyfin/internal/stremio"
)

// boundSide is the width of the test's pictures at the bound and over
// it: with boundRows rows they hold maxDecodedPixels pixels exactly.
const (
	boundSide = 4096
	boundRows = maxDecodedPixels / boundSide
)

// flatPNG is a black PNG of width by height pixels, of one bit each, a
// few kilobytes whatever its size. When cut, its pixels' checksum is
// wrong: its header reads, and its decoding fails at the end.
func flatPNG(t *testing.T, width, height int, cut bool) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		var length, sum [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(data)))
		out.Write(length[:])
		body := append([]byte(kind), data...)
		out.Write(body)
		binary.BigEndian.PutUint32(sum[:], crc32.ChecksumIEEE(body))
		out.Write(sum[:])
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:], uint32(width))
	binary.BigEndian.PutUint32(header[4:], uint32(height))
	header[8], header[9] = 1, 0 // one bit a pixel, grey
	chunk("IHDR", header)
	var pixels bytes.Buffer
	deflate, err := zlib.NewWriterLevel(&pixels, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	row := make([]byte, 1+(width+7)/8)
	for range height {
		_, _ = deflate.Write(row)
	}
	if err := deflate.Close(); err != nil {
		t.Fatal(err)
	}
	data := pixels.Bytes()
	if cut {
		data[len(data)-1] ^= 0xff
	}
	chunk("IDAT", data)
	chunk("IEND", nil)
	return out.Bytes()
}

// pictures is a server whose library lists one movie for each picture, by
// its name, with the picture as its poster.
type pictures struct {
	testServer
	token string
	view  string
	ids   map[string]string
	mu    sync.Mutex
	// served counts the downloads of each picture.
	served map[string]int
}

func newPictures(t *testing.T, posters map[string][]byte) *pictures {
	t.Helper()
	p := &pictures{testServer: newTestServer(t, 10), ids: map[string]string{}, served: map[string]int{}}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "pictures", Name: "Pictures", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			var metas []stremio.Meta
			for name := range posters {
				metas = append(metas, stremio.Meta{ID: "tt" + name, Type: "movie", Name: name, Poster: server.URL + "/posters/" + name,
					ReleaseInfo: "2020", ImdbRating: "7.5"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/posters/"):
			name := strings.TrimPrefix(path, "/posters/")
			p.mu.Lock()
			p.served[name]++
			p.mu.Unlock()
			w.Header().Set("Content-Type", http.DetectContentType(posters[name]))
			_, _ = w.Write(posters[name])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	p.user("member", nil)
	addon, err := p.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	p.token = p.signIn("member", "tv")
	p.view = p.viewIDs(t, p.token)["Top"]
	for _, item := range p.list(t) {
		p.ids[item.Name] = item.Id
	}
	for name := range posters {
		if p.ids[name] == "" {
			t.Fatalf("the library lists %v, not %s", p.ids, name)
		}
	}
	return p
}

// list lists the library's movies as the member.
func (p *pictures) list(t *testing.T) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := p.get(t, "/Items?ParentId="+p.view, p.token, &page); status != http.StatusOK {
		t.Fatalf("the library: %d", status)
	}
	return page.Items
}

// decoded is the count of the pictures the server decoded.
func (p *pictures) decoded() int64 { return p.handler.images.decoded.Load() }

// poster asks for a movie's poster with query, without credentials as
// apps do.
func (p *pictures) poster(t *testing.T, name, query string) (int, []byte) {
	t.Helper()
	status, _, body := p.send(http.MethodGet, "/Items/"+p.ids[name]+"/Images/Primary?"+query, "", "", "")
	return status, body
}

// A picture is decoded from its header's word: a JPEG, PNG or WebP one of
// at most maxDecodedPixels pixels.
func TestOnlyPicturesWithinThePixelBoundAreDecoded(t *testing.T) {
	t.Parallel()
	var animation bytes.Buffer
	if err := gif.Encode(&animation, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		picture []byte
		allowed bool
	}{
		{"a poster", greyPoster(t), true},
		{"a small PNG", flatPNG(t, 200, 300, false), true},
		{"a PNG at the bound", flatPNG(t, boundSide, boundRows, false), true},
		{"a PNG one row over the bound", flatPNG(t, boundSide, boundRows+1, false), false},
		{"a PNG one column over the bound", flatPNG(t, boundSide+1, boundRows, false), false},
		{"a GIF", animation.Bytes(), false},
		{"no picture", []byte("<svg xmlns='http://www.w3.org/2000/svg'/>"), false},
		{"nothing", nil, false},
	} {
		if err := decodable(c.picture, nil); (err == nil) != c.allowed {
			t.Errorf("%s: %v, allowed %v", c.name, err, c.allowed)
		}
	}
	// The sides a header may tell do not wrap the count of pixels around.
	wide := flatPNG(t, 1, 1, false)
	binary.BigEndian.PutUint32(wide[16:], 1<<28)
	binary.BigEndian.PutUint32(wide[20:], 1<<28)
	binary.BigEndian.PutUint32(wide[29:], crc32.ChecksumIEEE(wide[12:29]))
	if err := decodable(wide, nil); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("a PNG of the largest sides: %v", err)
	}
}

// A poster of more pixels than the bound is answered as it is to an app
// asking for a smaller one, without credentials, and never decoded,
// however often it is asked for; one whose decoding fails is decoded
// once. A poster at the bound is resized, and an ordinary one as before.
func TestAPosterOverThePixelBoundIsAnsweredAsItIs(t *testing.T) {
	t.Parallel()
	large := flatPNG(t, boundSide, boundRows+1, false)
	cut := flatPNG(t, 600, 900, true)
	poster := greyPoster(t)
	p := newPictures(t, map[string][]byte{"Large": large, "Cut": cut, "Bound": flatPNG(t, boundSide, boundRows, false), "Poster": poster})

	for round := range 3 {
		if status, body := p.poster(t, "Large", "maxWidth=120"); status != http.StatusOK || !bytes.Equal(body, large) {
			t.Fatalf("round %d of the large poster: %d, %d bytes, want its own %d", round, status, len(body), len(large))
		}
	}
	if got := p.decoded(); got != 0 {
		t.Fatalf("%d pictures decoded for the large poster, want none", got)
	}
	for round := range 3 {
		if status, body := p.poster(t, "Cut", "maxWidth=120"); status != http.StatusOK || !bytes.Equal(body, cut) {
			t.Fatalf("round %d of the cut poster: %d, %d bytes, want its own %d", round, status, len(body), len(cut))
		}
	}
	if got := p.decoded(); got != 1 {
		t.Fatalf("%d pictures decoded for the cut poster, want it once", got)
	}

	status, body := p.poster(t, "Bound", "maxWidth=120")
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if status != http.StatusOK || err != nil || format != "png" || config.Width != 120 {
		t.Errorf("the poster at the bound: %d %v %s %+v", status, err, format, config)
	}
	if got := p.decoded(); got != 2 {
		t.Errorf("%d pictures decoded once the poster at the bound was, want 2", got)
	}

	source, err := jpeg.Decode(bytes.NewReader(poster))
	if err != nil {
		t.Fatal(err)
	}
	scaled := image.NewRGBA(image.Rect(0, 0, 120, 180))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	var want bytes.Buffer
	if err := jpeg.Encode(&want, scaled, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if status, body := p.poster(t, "Poster", "maxWidth=120"); status != http.StatusOK || !bytes.Equal(body, want.Bytes()) {
		t.Errorf("the ordinary poster: %d, %d bytes, want the %d of its resize", status, len(body), want.Len())
	}
	if status, body := p.poster(t, "Poster", ""); status != http.StatusOK || !bytes.Equal(body, poster) {
		t.Errorf("the ordinary poster as it is: %d, %d bytes", status, len(body))
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for name, count := range p.served {
		if count != 1 {
			t.Errorf("%s was downloaded %d times, want once", name, count)
		}
	}
}

// A poster of more pixels than the bound gets no badges: it is answered
// as it is, and never decoded.
func TestAPosterOverThePixelBoundGetsNoBadges(t *testing.T) {
	t.Parallel()
	large := flatPNG(t, boundSide, boundRows+1, false)
	p := newPictures(t, map[string][]byte{"Large": large})
	p.setting(t, func(settings *accounts.Settings) {
		settings.PosterBadges = true
		settings.PosterRatings = true
	})
	tag := p.list(t)[0].ImageTags["Primary"]
	if badgesIn(tag) == "" {
		t.Fatalf("the poster's tag carries no badges: %q", tag)
	}
	for _, query := range []string{"tag=" + tag, "tag=" + tag + "&maxWidth=300", "tag=" + tag} {
		if status, body := p.poster(t, "Large", query); status != http.StatusOK || !bytes.Equal(body, large) {
			t.Errorf("%s: %d, %d bytes, want the poster's own %d", query, status, len(body), len(large))
		}
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded, want none", got)
	}
}

// A DLNA device gets no thumbnail of a poster of more pixels than the
// bound that is no JPEG, as of one that is no picture: it is not decoded.
func TestADLNAThumbnailLeavesOutAPosterOverThePixelBound(t *testing.T) {
	t.Parallel()
	p := newPictures(t, map[string][]byte{"Large": flatPNG(t, boundSide, boundRows+1, false), "Small": flatPNG(t, 600, 900, false)})
	art := func(name string) *httptest.ResponseRecorder {
		t.Helper()
		id, err := accounts.ParseID(p.ids[name])
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		p.handler.Art(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dlna/art/"+p.ids[name]+".jpg", nil), id, 320)
		return recorder
	}
	if answer := art("Large"); answer.Code != http.StatusNotFound || answer.Body.Len() != 0 {
		t.Errorf("the large poster's thumbnail: %d, %d bytes", answer.Code, answer.Body.Len())
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded for the large poster, want none", got)
	}
	answer := art("Small")
	if config, err := jpeg.DecodeConfig(answer.Body); answer.Code != http.StatusOK || err != nil || config.Width != 320 {
		t.Errorf("the small poster's thumbnail: %d %v %+v", answer.Code, err, config)
	}
}

// The images Polyfin draws leave out a picture of more pixels than the
// bound, as one that is no picture, without decoding it: a collection's
// image is drawn without it, and a studio whose logo it is has no plate.
func TestDrawnImagesLeaveOutAPictureOverThePixelBound(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	large := flatPNG(t, boundSide, boundRows+1, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(large)
	}))
	t.Cleanup(server.Close)
	h := s.handler
	h.images.open(h.imageDir(), h.Logger)

	collection := library.GeneratedImage{Name: "Harbor", Posters: []string{server.URL + "/1.png", server.URL + "/2.png"}}
	drawn, err := h.generatedArtwork(t.Context(), collection.URL(), collection, false)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := jpeg.Encode(&want, mosaic.Draw(collection.Name, make([]image.Image, 2), false, h.fonts.of(h.FontsDir)), &jpeg.Options{Quality: 88}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(drawn.body, want.Bytes()) {
		t.Error("the collection's image is not the one drawn without posters")
	}
	if kept, ok := h.images.get(collection.URL()); !ok || !bytes.Equal(kept.body, drawn.body) {
		t.Error("the collection's image is not kept")
	}

	studio := library.GeneratedImage{Logo: server.URL + "/logo.png", Wide: true}
	_, err = h.studioLogoArtwork(t.Context(), studio.URL(), studio)
	if failed, ok := errors.AsType[downloadError](err); !ok || failed.status != http.StatusNotFound {
		t.Errorf("the studio's plate: %v, want none", err)
	}
	if got := h.images.decoded.Load(); got != 0 {
		t.Errorf("%d pictures decoded, want none", got)
	}
}

// The pictures remembered as failing to decode are bounded: past the
// bound the least recently used is forgotten, and decoded once more when
// it is asked for again. Clearing the image cache forgets them all.
func TestThePicturesThatFailToDecodeAreRememberedWithinABound(t *testing.T) {
	t.Parallel()
	h := &Handler{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.images.open("", h.Logger)
	cut := flatPNG(t, 20, 30, true)
	// Bytes after a PNG's end are not read: each picture is another.
	picture := func(n int) []byte { return binary.BigEndian.AppendUint32(bytes.Clone(cut), uint32(n)) }
	const more = 50
	for n := range maxUndecodable + more {
		if _, _, err := h.decodePicture(picture(n), nil); err == nil || errors.Is(err, errUndecodable) {
			t.Fatalf("picture %d: %v", n, err)
		}
		if kept := h.images.undecodable.Len(); kept > maxUndecodable || kept != min(n+1, maxUndecodable) {
			t.Fatalf("%d pictures remembered after %d failed, at most %d are", kept, n+1, maxUndecodable)
		}
	}
	decoded := h.images.decoded.Load()
	if decoded != maxUndecodable+more {
		t.Fatalf("%d pictures decoded, want %d", decoded, maxUndecodable+more)
	}
	for round := range 3 {
		if _, _, err := h.decodePicture(picture(maxUndecodable+more-1), nil); !errors.Is(err, errUndecodable) {
			t.Errorf("round %d of the last picture: %v", round, err)
		}
	}
	if got := h.images.decoded.Load(); got != decoded {
		t.Errorf("%d pictures decoded again", got-decoded)
	}
	// The first was forgotten for the last.
	if _, _, err := h.decodePicture(picture(0), nil); err == nil || errors.Is(err, errUndecodable) {
		t.Errorf("the first picture: %v", err)
	}
	if got := h.images.decoded.Load(); got != decoded+1 {
		t.Errorf("%d pictures decoded for the first, want 1", got-decoded)
	}
	// A picture that decodes is not remembered.
	if _, format, err := h.decodePicture(flatPNG(t, 20, 30, false), nil); err != nil || format != "png" {
		t.Errorf("a whole picture: %s %v", format, err)
	}
	if kept := h.images.undecodable.Len(); kept != maxUndecodable {
		t.Errorf("%d pictures remembered, want %d", kept, maxUndecodable)
	}
	h.images.clear()
	if kept := h.images.undecodable.Len(); kept != 0 {
		t.Errorf("%d pictures remembered once the cache was cleared", kept)
	}
}

// Every picture is decoded while its caller holds a place among the
// resizes: an item's image resized, one with badges, a DLNA thumbnail, a
// collection's image and a studio's plate.
func TestEveryPictureIsDecodedInAPlaceAmongTheResizes(t *testing.T) {
	t.Parallel()
	small := flatPNG(t, 600, 900, false)
	p := newPictures(t, map[string][]byte{"Poster": greyPoster(t), "Small": small})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(small)
	}))
	t.Cleanup(server.Close)
	h := p.handler
	h.images.open(h.imageDir(), h.Logger)
	// The test asks for one image at a time: a decode that finds every
	// place free is one whose caller holds none.
	var unheld atomic.Int64
	hook := func([]byte) {
		if len(h.images.resizes) == 0 {
			unheld.Add(1)
		}
	}
	h.images.decoding.Store(&hook)
	step := func(name string, decodes int64, ask func()) {
		t.Helper()
		before := p.decoded()
		ask()
		if got := p.decoded() - before; got != decodes {
			t.Errorf("%s: %d pictures decoded, want %d", name, got, decodes)
		}
		if got := unheld.Swap(0); got != 0 {
			t.Errorf("%s: %d pictures decoded outside a place among the resizes", name, got)
		}
	}

	step("a resized poster", 1, func() {
		if status, _ := p.poster(t, "Poster", "maxWidth=120"); status != http.StatusOK {
			t.Errorf("the resized poster: %d", status)
		}
	})
	// The thumbnail is the poster resized, a PNG, then made a JPEG.
	step("a DLNA thumbnail", 2, func() {
		id, err := accounts.ParseID(p.ids["Small"])
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		h.Art(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dlna/art/"+p.ids["Small"]+".jpg", nil), id, 320)
		if recorder.Code != http.StatusOK {
			t.Errorf("the thumbnail: %d", recorder.Code)
		}
	})
	step("a collection's image", 2, func() {
		collection := library.GeneratedImage{Name: "Harbor", Posters: []string{server.URL + "/1.png", server.URL + "/2.png"}}
		if _, err := h.generatedArtwork(t.Context(), collection.URL(), collection, false); err != nil {
			t.Error(err)
		}
	})
	step("a studio's plate", 1, func() {
		studio := library.GeneratedImage{Logo: server.URL + "/logo.png", Wide: true}
		if _, err := h.studioLogoArtwork(t.Context(), studio.URL(), studio); err != nil {
			t.Error(err)
		}
	})
	p.setting(t, func(settings *accounts.Settings) {
		settings.PosterBadges = true
		settings.PosterRatings = true
	})
	var tag string
	for _, item := range p.list(t) {
		if item.Name == "Poster" {
			tag = item.ImageTags["Primary"]
		}
	}
	if badgesIn(tag) == "" {
		t.Fatalf("the poster's tag carries no badges: %q", tag)
	}
	step("a poster with badges", 1, func() {
		if status, _ := p.poster(t, "Poster", "tag="+tag); status != http.StatusOK {
			t.Errorf("the poster with badges: %d", status)
		}
	})
}

// Collections' images asked for at once decode their posters in their
// places among the resizes: no more decoded posters are held at once than
// those places times the posters of an image, however many images wait.
func TestImagesDrawnAtOnceHoldTheirDecodedPostersWithinTheResizes(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	h := s.handler
	h.images.open(h.imageDir(), h.Logger)
	tiles, images := mosaic.Tiles(false), imageResizes+2
	// Each image has posters of its own: bytes after a PNG's end are not
	// read, and tell whose poster is decoded.
	small := flatPNG(t, 20, 30, false)
	owner, posters := map[string]int{}, map[string][]byte{}
	collections := make([]library.GeneratedImage, images)
	for n := range images {
		collections[n].Name = "Harbor " + string(rune('A'+n))
		for tile := range tiles {
			poster := binary.BigEndian.AppendUint32(bytes.Clone(small), uint32(n*tiles+tile))
			path := "/" + string(rune('a'+n)) + string(rune('0'+tile)) + ".png"
			owner[string(poster)], posters[path] = n, poster
			collections[n].Posters = append(collections[n].Posters, path)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(posters[r.URL.Path])
	}))
	t.Cleanup(server.Close)
	for n := range collections {
		for tile, path := range collections[n].Posters {
			collections[n].Posters[tile] = server.URL + path
		}
	}

	// Each decode waits for its image's gate, or for free. An image holds
	// the posters it decoded from its first until its last is let through.
	var mu sync.Mutex
	var inside, held, high, unheld int
	entered := make([]int, images)
	events := make(chan int, images*tiles)
	gates := make([]chan struct{}, images)
	for n := range gates {
		gates[n] = make(chan struct{})
	}
	free := make(chan struct{})
	release := sync.OnceFunc(func() { close(free) })
	hook := func(data []byte) {
		n, ok := owner[string(data)]
		if !ok {
			return
		}
		mu.Lock()
		entered[n]++
		last := entered[n] == tiles
		inside++
		held++
		high = max(high, held)
		if inside > len(h.images.resizes) {
			unheld++
			release()
		}
		mu.Unlock()
		events <- n
		select {
		case <-gates[n]:
		case <-free:
		}
		mu.Lock()
		inside--
		if last {
			held -= tiles
		}
		mu.Unlock()
	}
	h.images.decoding.Store(&hook)

	results := make(chan error, images)
	for _, collection := range collections {
		go func() {
			_, err := h.generatedArtwork(t.Context(), collection.URL(), collection, false)
			results <- err
		}()
	}
	// As many images as there are places reach their first poster; each is
	// then let through to its last, which it holds with the others. A
	// decode outside a place sets everything free, and is told below.
	freed := func() bool {
		select {
		case <-free:
			return true
		default:
			return false
		}
	}
	var drawing []int
	for len(drawing) < imageResizes && !freed() {
		select {
		case n := <-events:
			drawing = append(drawing, n)
		case <-free:
		}
	}
	for _, n := range drawing {
		for range tiles - 1 {
			select {
			case gates[n] <- struct{}{}:
				select {
				case <-events:
				case <-free:
				}
			case <-free:
			}
		}
	}
	if !freed() {
		mu.Lock()
		if held != imageResizes*tiles || len(h.images.resizes) != imageResizes {
			t.Errorf("%d decoded posters held in %d places, want %d in %d", held, len(h.images.resizes), imageResizes*tiles, imageResizes)
		}
		mu.Unlock()
	}
	release()
	for range images {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if unheld != 0 {
		t.Errorf("%d posters decoded outside a place among the resizes", unheld)
	}
	if high != imageResizes*tiles {
		t.Errorf("%d decoded posters held at once at most, want the %d of %d images", high, imageResizes*tiles, imageResizes)
	}
	for n, count := range entered {
		if count != tiles {
			t.Errorf("image %d decoded %d posters, want %d", n, count, tiles)
		}
	}
}

// headedJPEG is a JPEG picture of width by height pixels up to its frame
// header and no further, sequential or progressive, with a component
// for each of sampling: its samples across and down a unit, 0x22 for a
// first component whose colours are sampled at half the width and
// height. Its header reads, and nothing of it can be decoded.
func headedJPEG(width, height int, progressive bool, sampling ...byte) []byte {
	out := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}
	marker := byte(0xc0)
	if progressive {
		marker = 0xc2
	}
	out = append(out, 0xff, marker, 0, byte(8+3*len(sampling)), 8, byte(height>>8), byte(height), byte(width>>8), byte(width), byte(len(sampling)))
	for i, samples := range sampling {
		out = append(out, byte(i+1), samples, 0)
	}
	return out
}

// headedPNG is a PNG picture's header and no further: width by height
// pixels of depth bits a channel, of a colour type (2 is RGB, 6 RGB with
// transparency).
func headedPNG(t *testing.T, width, height int, depth, colour byte) []byte {
	t.Helper()
	header := flatPNG(t, 1, 1, false)[:33]
	binary.BigEndian.PutUint32(header[16:], uint32(width))
	binary.BigEndian.PutUint32(header[20:], uint32(height))
	header[24], header[25] = depth, colour
	binary.BigEndian.PutUint32(header[29:], crc32.ChecksumIEEE(header[12:29]))
	return header
}

// Ordinary artwork is within the memory of a picture, told from its
// header alone: a poster of 2,000 by 3,000 and a 4K backdrop, as a
// sequential JPEG, a progressive one, a PNG and a PNG of 16 bits a
// channel, resized to every width the route resizes them to, the largest
// included, and with badges drawn at their size. The counts are the ones
// the decoders and the scaler tell.
func TestOrdinaryArtworkIsWithinTheMemoryOfAPicture(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name                       string
		header                     []byte
		width, height, largest     int
		decoding, resized, redrawn int64
	}{
		{"a poster, sequential JPEG", headedJPEG(2000, 3000, false, 0x22, 0x11, 0x11), 2000, 3000, 1280, 43_091_727, 157_673_444, 144_623_844},
		{"a poster, progressive JPEG", headedJPEG(2000, 3000, true, 0x22, 0x11, 0x11), 2000, 3000, 1280, 80_027_493, 157_673_444, 144_623_844},
		{"a poster, progressive JPEG of whole colours", headedJPEG(2000, 3000, true, 0x11, 0x11, 0x11), 2000, 3000, 1280, 116_963_259, 157_673_444, 144_623_844},
		{"a poster, PNG", headedPNG(t, 2000, 3000, 8, 2), 2000, 3000, 1280, 24_016_002, 157_049_600, 144_000_000},
		{"a poster, PNG of 16 bits", headedPNG(t, 2000, 3000, 16, 6), 2000, 3000, 1280, 48_032_002, 181_049_600, 168_000_000},
		{"a backdrop, sequential JPEG", headedJPEG(3840, 2160, false, 0x22, 0x11, 0x11), 3840, 2160, 2560, 59_369_527, 226_034_244, 199_813_444},
		{"a backdrop, progressive JPEG", headedJPEG(3840, 2160, true, 0x22, 0x11, 0x11), 3840, 2160, 2560, 110_257_693, 226_034_244, 199_813_444},
		{"a backdrop, progressive JPEG of whole colours", headedJPEG(3840, 2160, true, 0x11, 0x11, 0x11), 3840, 2160, 2560, 161_145_859, 226_034_244, 199_813_444},
		{"a backdrop, PNG", headedPNG(t, 3840, 2160, 8, 2), 3840, 2160, 2560, 33_208_322, 225_286_400, 199_065_600},
		{"a backdrop, PNG of 16 bits", headedPNG(t, 3840, 2160, 16, 6), 3840, 2160, 2560, 66_416_642, 258_464_000, 232_243_200},
	} {
		largest := 0
		var cost pictureCost
		for _, step := range widthSteps {
			if resizedWidth(url.Values{"maxWidth": {strconv.Itoa(step)}}, c.width, c.height) != step {
				continue
			}
			largest = step
			if err := decodable(c.header, func(p pictureCost) int64 { cost = p; return p.resized(step) }); err != nil {
				t.Errorf("%s resized to %d wide: %v", c.name, step, err)
			}
		}
		if largest != c.largest {
			t.Errorf("%s is resized to %d wide at most, want %d", c.name, largest, c.largest)
			continue
		}
		if got := cost.resized(largest); cost.decoding != c.decoding || got != c.resized || got > maxPictureMemory {
			t.Errorf("%s: %d bytes while decoded and %d resized to %d wide, want %d and %d", c.name, cost.decoding, got, largest, c.decoding, c.resized)
		}
		if err := decodable(c.header, pictureCost.redrawn); err != nil || cost.redrawn() != c.redrawn {
			t.Errorf("%s with badges at its size: %v, %d bytes, want %d", c.name, err, cost.redrawn(), c.redrawn)
		}
	}
}

// A picture whose decoding or drawing would hold more than the memory of
// a picture is refused from its header, whatever its pixels: the heaviest
// the pixel bound lets through, a strip whose scaler's weights alone are
// past it, and a large picture at the larger widths only.
func TestAPictureTooHeavyToDrawIsRefusedFromItsHeader(t *testing.T) {
	t.Parallel()
	resized := func(width int) func(pictureCost) int64 {
		return func(p pictureCost) int64 { return p.resized(width) }
	}
	plate := func(p pictureCost) int64 { return p.drawnInto(studioThumbWidth, studioThumbHeight) }
	heaviest := headedJPEG(boundSide, boundRows, true, 0x11, 0x11, 0x11, 0x11)
	strip := flatPNG(t, maxDecodedPixels, 1, false)
	cover := headedJPEG(4000, 4000, false, 0x22, 0x11, 0x11)
	for _, c := range []struct {
		name    string
		picture []byte
		held    func(pictureCost) int64
		allowed bool
	}{
		{"a progressive JPEG of four components at the pixel bound", heaviest, nil, false},
		{"the same resized", heaviest, resized(120), false},
		{"a sequential JPEG at the pixel bound", headedJPEG(boundSide, boundRows, false, 0x22, 0x11, 0x11), resized(1280), true},
		{"a progressive JPEG at the pixel bound", headedJPEG(boundSide, boundRows, true, 0x22, 0x11, 0x11), resized(1280), true},
		{"a strip by itself", strip, nil, true},
		{"a strip resized", strip, resized(120), false},
		{"a strip resized to the largest width", strip, resized(3200), false},
		{"a strip drawn at its size", strip, pictureCost.redrawn, false},
		{"a strip encoded", strip, pictureCost.encoded, false},
		{"a strip on a studio's plate", strip, plate, false},
		{"a logo on a studio's plate", headedPNG(t, 2000, 1000, 8, 6), plate, true},
		{"a cover of 4,000 by 4,000 resized to 1,280", cover, resized(1280), true},
		{"a cover of 4,000 by 4,000 resized to 1,600", cover, resized(1600), false},
		{"a cover of 4,000 by 4,000 with badges at its size", cover, pictureCost.redrawn, false},
		{"a backdrop of 5,120 by 2,880 resized to 1,920", headedJPEG(5120, 2880, true, 0x22, 0x11, 0x11), resized(1920), true},
		{"a backdrop of 5,120 by 2,880 resized to 2,560", headedJPEG(5120, 2880, true, 0x22, 0x11, 0x11), resized(2560), false},
	} {
		if err := decodable(c.picture, c.held); (err == nil) != c.allowed {
			t.Errorf("%s: %v, allowed %v", c.name, err, c.allowed)
		}
	}

	// The coefficients a progressive picture keeps are told by its frame
	// header, and counted in full wherever it is not reached as the
	// decoder reaches it.
	comments := []byte{0xff, 0xd8}
	for range maxJPEGSegments {
		comments = append(comments, 0xff, 0xfe, 0, 2)
	}
	sequential := headedJPEG(600, 900, false, 0x22, 0x11, 0x11)
	for _, c := range []struct {
		name        string
		picture     []byte
		samples, of int64
	}{
		{"a sequential frame", sequential, 0, 1},
		{"a progressive frame of colours at half", headedJPEG(600, 900, true, 0x22, 0x11, 0x11), 6, 4},
		{"a progressive frame of whole colours", headedJPEG(600, 900, true, 0x11, 0x11, 0x11), 3, 1},
		{"a frame after more segments than are read", append(comments, sequential[20:]...), 3, 1},
		{"a frame after a byte that heads no segment", append([]byte{0xff, 0xd8, 0}, sequential[2:]...), 3, 1},
		{"a frame after a fill byte", append([]byte{0xff, 0xd8, 0xff}, sequential[2:]...), 3, 1},
		{"a frame cut short", headedJPEG(600, 900, true, 0x22, 0x11, 0x11)[:32], 3, 1},
		{"no frame", []byte{0xff, 0xd8}, 3, 1},
	} {
		if samples, of := progressiveJPEG(c.picture, 3); samples != c.samples || of != c.of {
			t.Errorf("%s keeps %d coefficients for each %d pixels, want %d for each %d", c.name, samples, of, c.samples, c.of)
		}
	}
}

// A poster too heavy to decode and resize is answered as it is, as one of
// too many pixels is, and never decoded, however often it is asked for:
// the heaviest picture the pixel bound lets through, and a strip.
func TestAPosterTooHeavyToResizeIsAnsweredAsItIs(t *testing.T) {
	t.Parallel()
	heaviest := headedJPEG(boundSide, boundRows, true, 0x11, 0x11, 0x11, 0x11)
	strip := flatPNG(t, maxDecodedPixels, 1, false)
	p := newPictures(t, map[string][]byte{"Heaviest": heaviest, "Strip": strip})
	for name, picture := range map[string][]byte{"Heaviest": heaviest, "Strip": strip} {
		for round := range 3 {
			if status, body := p.poster(t, name, "maxWidth=120"); status != http.StatusOK || !bytes.Equal(body, picture) {
				t.Fatalf("round %d of %s: %d, %d bytes, want its own %d", round, name, status, len(body), len(picture))
			}
		}
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded, want none", got)
	}
}

// A poster too heavy to draw gets no badges, at its size nor at a smaller
// one, no DLNA thumbnail and, as a studio's logo, no plate, as one of too
// many pixels: it is never decoded.
func TestAPictureTooHeavyToDrawGetsNoBadgesNoThumbnailAndNoPlate(t *testing.T) {
	t.Parallel()
	strip := flatPNG(t, maxDecodedPixels, 1, false)
	p := newPictures(t, map[string][]byte{"Strip": strip})
	p.setting(t, func(settings *accounts.Settings) {
		settings.PosterBadges = true
		settings.PosterRatings = true
	})
	tag := p.list(t)[0].ImageTags["Primary"]
	if badgesIn(tag) == "" {
		t.Fatalf("the poster's tag carries no badges: %q", tag)
	}
	for _, query := range []string{"tag=" + tag, "tag=" + tag + "&maxWidth=300"} {
		if status, body := p.poster(t, "Strip", query); status != http.StatusOK || !bytes.Equal(body, strip) {
			t.Errorf("%s: %d, %d bytes, want the poster's own %d", query, status, len(body), len(strip))
		}
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded for the badges, want none", got)
	}

	id, err := accounts.ParseID(p.ids["Strip"])
	if err != nil {
		t.Fatal(err)
	}
	thumbnail := httptest.NewRecorder()
	p.handler.Art(thumbnail, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/dlna/art/"+p.ids["Strip"]+".jpg", nil), id, 320)
	if thumbnail.Code != http.StatusNotFound || thumbnail.Body.Len() != 0 {
		t.Errorf("the thumbnail: %d, %d bytes", thumbnail.Code, thumbnail.Body.Len())
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded once the thumbnail was asked for, want none", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(strip)
	}))
	t.Cleanup(server.Close)
	studio := library.GeneratedImage{Logo: server.URL + "/logo.png", Wide: true}
	_, err = p.handler.studioLogoArtwork(t.Context(), studio.URL(), studio)
	if failed, ok := errors.AsType[downloadError](err); !ok || failed.status != http.StatusNotFound {
		t.Errorf("the studio's plate: %v, want none", err)
	}
	if got := p.decoded(); got != 0 {
		t.Errorf("%d pictures decoded once the plate was asked for, want none", got)
	}
}

// A picture is decoded when its caller holds the memory of a picture to
// the byte, and not when it holds one byte more.
func TestAPictureIsDecodedUpToTheMemoryOfAPictureAndNotAByteOver(t *testing.T) {
	t.Parallel()
	h := newTestServer(t, 10).handler
	picture := flatPNG(t, 20, 30, false)
	for _, c := range []struct {
		beside  int64
		decoded int64
	}{{-1, 1}, {0, 2}, {1, 2}} {
		// The caller holds the memory of a picture, less or more a byte.
		_, _, err := h.decodePicture(picture, func(pictureCost) int64 { return maxPictureMemory + c.beside })
		if (err == nil) != (c.beside <= 0) {
			t.Errorf("%d bytes past the memory of a picture: %v", c.beside, err)
		}
		if got := h.images.decoded.Load(); got != c.decoded {
			t.Errorf("%d bytes past the memory of a picture: %d pictures decoded, want %d", c.beside, got, c.decoded)
		}
	}
}

// The posters of a collection's image share the memory of a picture:
// those that would take the image past it, with the ones decoded before
// them, are left out and not decoded.
func TestACollectionsImageHoldsItsPostersWithinTheMemoryOfAPicture(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	// Each poster counts 64 MB decoded, a quarter of the memory of a
	// picture, and the scaler and the drawing take half of it.
	poster := flatPNG(t, 4000, 4000, false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(poster)
	}))
	t.Cleanup(server.Close)
	h := s.handler
	h.images.open(h.imageDir(), h.Logger)
	collection := library.GeneratedImage{Name: "Harbor", Posters: []string{server.URL + "/1.png", server.URL + "/2.png", server.URL + "/3.png", server.URL + "/4.png"}}
	if _, err := h.generatedArtwork(t.Context(), collection.URL(), collection, false); err != nil {
		t.Fatal(err)
	}
	if got := h.images.decoded.Load(); got != 2 {
		t.Errorf("%d posters decoded, want the 2 that the memory of a picture holds", got)
	}
}
