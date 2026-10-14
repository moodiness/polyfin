package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"

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
// out of this request's image only.
func (h *Handler) generatedArtwork(ctx context.Context, target string, g library.GeneratedImage, confined bool) (artwork, error) {
	confined = confined || g.Confined
	drawn, err := h.images.fetch(ctx, target, func(ctx context.Context) (artwork, error) {
		posters := make([]image.Image, min(len(g.Posters), mosaic.Tiles(g.Wide)))
		var missing bool
		var mu sync.Mutex
		var group errgroup.Group
		group.SetLimit(generatedFetches)
		for i := range posters {
			poster := g.Posters[i]
			group.Go(func() error {
				relayed, err := h.fetchArtwork(ctx, poster, confined, h.iptvHost(ctx, poster))
				if err != nil {
					if failed, ok := errors.AsType[downloadError](err); !ok || failed.status != http.StatusNotFound && failed.status != http.StatusForbidden {
						mu.Lock()
						missing = true
						mu.Unlock()
					}
					return nil
				}
				if decoded, _, err := image.Decode(bytes.NewReader(relayed.body)); err == nil {
					posters[i] = decoded
				}
				return nil
			})
		}
		_ = group.Wait()
		h.images.resizes <- struct{}{}
		defer func() { <-h.images.resizes }()
		var out bytes.Buffer
		if err := jpeg.Encode(&out, mosaic.Draw(g.Name, posters, g.Wide, h.fonts.of(h.FontsDir)), &jpeg.Options{Quality: 88}); err != nil {
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
