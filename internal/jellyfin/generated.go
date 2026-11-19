package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/mosaic"
)

// Generated images.
//
// The collections and libraries that have no image of their own may show
// one Polyfin draws (see library.GeneratedImage): it is drawn the first
// time an app asks for it, from the posters it shows, downloaded as any
// artwork is relayed, then kept as relayed artwork is, in memory and in
// the cache folder's images (see imageCache), at 1,000 by 1,500 pixels for
// a poster and 1,920 by 1,080 for a wide image. Apps asking for smaller
// ones get them resized, as artwork is. Its URL changes with what it
// shows, which gives it a new tag.
//
// The Live TV, Replay, Collections and Playlists views show an emblem of
// their own (see library.EmblemImage), drawn without posters.

// viewEmblems are the emblems of Polyfin's views that list no titles to
// draw an image from.
var viewEmblems = map[accounts.ID]string{
	liveTvViewID:         mosaic.EmblemLiveTV,
	library.ReplayViewID: mosaic.EmblemReplay,
	collectionsViewID:    mosaic.EmblemCollections,
	playlistsViewID:      mosaic.EmblemPlaylists,
}

// withEmblem gives item, one of the views of viewEmblems, its emblem as
// its Primary image, wide.
func withEmblem(item library.Item) library.Item {
	item.Images.Primary, item.WidePrimary = library.EmblemImage(viewEmblems[item.ID]).URL(), true
	return item
}

// viewArtwork returns the image of type imageType of a view of Polyfin's
// that has one of its own: an emblem, the same for everyone, or, by its
// tag, a user's image of a watchlist or of a smart list shown as a
// library (see library.Service.PersonalArtwork). A playlist with nothing
// to draw shows the Playlists emblem (see playlistDtos), which its tag
// names whatever the playlist.
func (h *Handler) viewArtwork(ctx context.Context, id accounts.ID, imageType, tag string) (library.Artwork, bool) {
	if !strings.EqualFold(imageType, "Primary") {
		return library.Artwork{}, false
	}
	if emblem, ok := viewEmblems[id]; ok {
		return library.EmblemArtwork(emblem), true
	}
	if tag != "" && tag == library.ImageTag(library.EmblemImage(mosaic.EmblemPlaylists).URL()) {
		return library.EmblemArtwork(mosaic.EmblemPlaylists), true
	}
	return h.Library.PersonalArtwork(ctx, id, tag)
}

// fontsFor holds the fonts names are written with, read again when the
// fonts folder changes (tests change it).
type fontsFor struct {
	mu    sync.Mutex
	dir   string
	fonts *mosaic.Fonts
}

func (f *fontsFor) of(dir string) *mosaic.Fonts {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fonts == nil || f.dir != dir {
		f.dir, f.fonts = dir, mosaic.NewFonts(dir)
	}
	return f.fonts
}

// partialImage is an image drawn without some of its posters, which could
// not be downloaded for now: it is served, but not kept, so that the next
// request draws it again.
type partialImage struct{ image artwork }

func (partialImage) Error() string { return "some posters could not be downloaded" }

// generatedFetches bounds the posters of a generated image downloaded at
// once.
const generatedFetches = 3

// generatedArtwork returns the generated image target stands for, drawn
// once for every request that asks for it at once, from the posters the
// image's shape takes, downloaded from public addresses only when confined
// or g.Confined. A poster its server does not have, or that is no
// picture, is left out for good; one that cannot be downloaded now is left
// out of this request's image only. The downloads keep the posters as
// they came, six at most, each within the bound of a download; they are
// decoded once the drawing has its place among the resizes, so that the
// images drawn at once hold no more decoded posters than those places
// allow (see drawPosters). No place is held while downloading. A studio's
// logo is drawn on its plate (see studioLogoArtwork).
func (h *Handler) generatedArtwork(ctx context.Context, target string, g library.GeneratedImage, confined bool) (artwork, error) {
	if g.Logo != "" {
		return h.studioLogoArtwork(ctx, target, g)
	}
	confined = confined || g.Confined
	drawn, err := h.images.fetch(ctx, target, func(ctx context.Context) (artwork, error) {
		bodies := make([][]byte, min(len(g.Posters), mosaic.Tiles(g.Wide)))
		var missing bool
		var mu sync.Mutex
		var group errgroup.Group
		group.SetLimit(generatedFetches)
		for i := range bodies {
			poster := g.Posters[i]
			group.Go(func() (err error) {
				defer recoverInto(h.Logger, &err, "A poster of a generated image ended on a panic")
				relayed, err := h.fetchArtwork(ctx, poster, confined, h.iptvHost(ctx, poster))
				if err != nil {
					if failed, ok := errors.AsType[downloadError](err); !ok || failed.status != http.StatusNotFound && failed.status != http.StatusForbidden {
						mu.Lock()
						missing = true
						mu.Unlock()
					}
					return nil
				}
				bodies[i] = relayed.body
				return nil
			})
		}
		// A poster whose fetch ended on a panic is missing, as one that
		// failed is.
		if group.Wait() != nil {
			missing = true
		}
		h.images.resizes <- struct{}{}
		defer func() { <-h.images.resizes }()
		var out bytes.Buffer
		var drawing *image.RGBA
		if g.Emblem != "" {
			drawing = mosaic.DrawEmblem(g.Emblem)
		} else {
			drawing = h.drawPosters(g, bodies)
		}
		if err := jpeg.Encode(&out, drawing, &jpeg.Options{Quality: 88}); err != nil {
			return artwork{}, downloadError{http.StatusInternalServerError, err}
		}
		result := artwork{body: out.Bytes(), contentType: "image/jpeg"}
		if missing {
			return artwork{}, partialImage{result}
		}
		return result, nil
	})
	if partial, ok := errors.AsType[partialImage](err); ok {
		return partial.image, nil
	}
	return drawn, err
}

// drawPosters draws the image of g from the posters downloaded, bodies,
// nil for one missing: each is decoded, then scaled into its cell and let
// go before the next is decoded, so that the drawing holds one decoded
// poster at a time, within the memory of a picture (see
// maxPictureMemory) beside the drawing. A poster that would hold more, or
// that fails to decode, is left out as one that is no picture: the
// posters within the bound by their header make the cells, and the image
// shows the first of them it has cells for (see mosaic.Drawing.Shown).
// It is drawn again without one that then fails to decode. The drawing is
// encoded once its posters are let go: it and its encoding hold 20 bytes
// for each of its pixels, 41 MB at most.
func (h *Handler) drawPosters(g library.GeneratedImage, bodies [][]byte) *image.RGBA {
	width, height := int64(mosaic.PosterWidth), int64(mosaic.PosterHeight)
	if g.Wide {
		width, height = mosaic.WideWidth, mosaic.WideHeight
	}
	held := func(p pictureCost) int64 {
		return h.images.drawingBeside + 4*width*height + max(p.decoding, p.decoded+p.fitting(width, height))
	}
	var candidates [][]byte
	for _, body := range bodies {
		if body != nil && decodable(body, held) == nil {
			candidates = append(candidates, body)
		}
	}
again:
	for {
		drawing := mosaic.NewDrawing(len(candidates), g.Wide)
		for i, body := range candidates[:drawing.Shown()] {
			poster, _, err := h.decodePicture(body, held)
			if err != nil {
				candidates = slices.Delete(candidates, i, i+1)
				continue again
			}
			drawing.Cover(i, poster)
		}
		return drawing.Finish(g.Name, h.fonts.of(h.FontsDir))
	}
}
