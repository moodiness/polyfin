package jellyfin

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Studios.
//
// A title's studios link to their page by the identifier of their name
// (see names.go), as genres do. /Studios lists the studios of the titles
// the user reaches (see library.Service.Studios), and a studio's page its
// titles (see library.Service.StudioTitles). A studio of a logo on TMDB
// shows it on a light plate Polyfin draws, Thumb wide and Primary square:
// TMDB's logos are mostly dark on transparency, which a dark app would
// not show. The plates are drawn the first time an app asks for them,
// then kept as generated images are (see generated.go), and resized for
// smaller asks as artwork is. An app may ask for a plate of a narrow
// margin, a sixteenth of each side rather than an eighth, with
// margin=narrow: the LumaaGlaass theme does for a title's page and a
// studio's, where the logo is small. Jellyfin shows the Thumb images of
// its own artwork repository, by name (StudiosImageProvider.cs lines
// 54-56).

// The plates of studios' logos: their sizes, and their colour, a light
// neutral grey.
const (
	studioThumbWidth, studioThumbHeight = 960, 540
	studioPrimarySide                   = 600
)

var studioPlate = color.RGBA{R: 0xEE, G: 0xEE, B: 0xEC, A: 0xFF}

// drawStudioLogo draws logo on a plate, wide or square, fitted within the
// plate less an eighth of each side on each side, a sixteenth when narrow,
// and centred.
func drawStudioLogo(logo image.Image, wide, narrow bool) *image.RGBA {
	width, height := studioPrimarySide, studioPrimarySide
	if wide {
		width, height = studioThumbWidth, studioThumbHeight
	}
	margin := 8
	if narrow {
		margin = 16
	}
	plate := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.Draw(plate, plate.Bounds(), image.NewUniform(studioPlate), image.Point{}, xdraw.Src)
	bounds := logo.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return plate
	}
	boxWidth, boxHeight := float64(width-2*(width/margin)), float64(height-2*(height/margin))
	scale := min(boxWidth/float64(bounds.Dx()), boxHeight/float64(bounds.Dy()))
	fitted := image.Pt(max(1, int(math.Round(float64(bounds.Dx())*scale))), max(1, int(math.Round(float64(bounds.Dy())*scale))))
	at := image.Pt((width-fitted.X)/2, (height-fitted.Y)/2)
	xdraw.CatmullRom.Scale(plate, image.Rectangle{Min: at, Max: at.Add(fitted)}, logo, bounds, xdraw.Over, nil)
	return plate
}

// studioLogoArtwork returns the plate of a studio's logo target stands
// for (see library.GeneratedImage), drawn once for every request that
// asks for it at once and kept, from TMDB's logo, which TMDB serves as a
// PNG. A logo TMDB does not have is not found.
func (h *Handler) studioLogoArtwork(ctx context.Context, target string, g library.GeneratedImage) (artwork, error) {
	return h.images.fetch(ctx, target, func(ctx context.Context) (artwork, error) {
		relayed, err := h.fetchArtwork(ctx, g.Logo, false, false)
		if err != nil {
			return artwork{}, err
		}
		logo, _, err := image.Decode(bytes.NewReader(relayed.body))
		if err != nil {
			return artwork{}, downloadError{http.StatusNotFound, err}
		}
		h.images.resizes <- struct{}{}
		defer func() { <-h.images.resizes }()
		var out bytes.Buffer
		if err := png.Encode(&out, drawStudioLogo(logo, g.Wide, g.NarrowMargin)); err != nil {
			return artwork{}, downloadError{http.StatusInternalServerError, err}
		}
		return artwork{body: out.Bytes(), contentType: "image/png"}, nil
	})
}

// studioArtwork returns the image of type imageType of the studio id
// stands for, if any (see library.Service.StudioArtwork), of the narrow
// margin when query asks for it (margin=narrow, named in any case).
func (h *Handler) studioArtwork(ctx context.Context, id accounts.ID, imageType string, query url.Values) (library.Artwork, bool) {
	name, ok := h.studioByID(ctx, id.String())
	if !ok {
		return library.Artwork{}, false
	}
	narrow := false
	for key, values := range query {
		if strings.EqualFold(key, "margin") && strings.EqualFold(values[0], "narrow") {
			narrow = true
		}
	}
	found, err := h.Library.StudioArtwork(ctx, name, imageType, narrow)
	return found, err == nil
}

// studioImageTags are the tags of the images of the studio named name.
func (h *Handler) studioImageTags(ctx context.Context, name string) map[string]string {
	logo, err := h.Library.StudioLogo(ctx, name)
	if err != nil {
		return map[string]string{}
	}
	return h.Library.StudioImageTags(logo)
}

// studios lists the studios of the titles the user reaches, those of the
// library parentId only when given, sorted by name: those whose name
// contains searchTerm, within the letters asked (see letterRange), the
// user's favorites only, or none of them, with isFavorite, in pages of
// startIndex and limit, all of them without a limit. Each is described as
// Jellyfin describes a studio, with the counts of its movies and series
// among those titles, and the tags of its logo's images.
func (h *Handler) studios(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	parent, _ := b.guid(r, "parentId")
	favorite, favoriteSet := b.bool(r, "isFavorite")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	studios, err := h.Library.Studios(r.Context(), user, parent)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	term := strings.ToLower(strings.TrimSpace(query(r, "searchTerm")))
	inRange := letterRange(r)
	studios = slices.DeleteFunc(studios, func(s library.Studio) bool {
		name := strings.ToLower(s.Name)
		return !strings.Contains(name, term) || !inRange(name)
	})
	ids := make([]accounts.ID, len(studios))
	for i, studio := range studios {
		ids[i], _ = accounts.ParseID(h.names.id(studioNamed, studio.Name))
	}
	data, err := h.UserData.Get(r.Context(), user.ID, ids)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items := make([]BaseItemDto, 0, len(studios))
	for i, studio := range studios {
		if favoriteSet && data[ids[i]].Favorite != favorite {
			continue
		}
		id := ids[i].String()
		items = append(items, BaseItemDto{
			Name:              studio.Name,
			ServerId:          h.ServerID,
			Id:                id,
			Type:              studioNamed.itemType,
			ImageTags:         h.Library.StudioImageTags(studio.Logo),
			BackdropImageTags: []string{},
			ImageBlurHashes:   map[string]map[string]string{},
			LocationType:      "FileSystem",
			MediaType:         "Unknown",
			MovieCount:        new(studio.Movies),
			SeriesCount:       new(studio.Series),
			UserData:          UserItemData{Key: studio.Name, ItemId: id, IsFavorite: data[ids[i]].Favorite},
		})
	}
	total := len(items)
	if limit < 0 {
		limit = total
	}
	writeJSON(w, http.StatusOK, pageOf(items, start, limit, total))
}

// studioItem is the item a studio's identifier stands for, as the user
// data endpoints act on it: its favorite mark (see userDataItem).
func (h *Handler) studioItem(ctx context.Context, id accounts.ID) (library.Item, error) {
	name, ok := h.studioByID(ctx, id.String())
	if !ok {
		return library.Item{}, library.ErrNotFound
	}
	return library.Item{ID: id, Name: name}, nil
}
