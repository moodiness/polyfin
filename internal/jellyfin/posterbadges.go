package jellyfin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"slices"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/mosaic"
)

// Badges over posters.
//
// While Settings.PosterBadges is on, the Primary image of a movie or a
// series is drawn with badges over it (see mosaic.DrawBadges): its star
// rating, its critics' and audience scores, as a title's page shows them,
// and a series' new episode or season (see library.ReleaseStatuses).
// Every app shows them, as they are part of the image. The image keeps
// its address and gets a tag of its own, which changes with what the
// badges show, so that apps load it again:
//
//	<the image's tag>-<the badges>-<signature>
//
// Image requests come without a user, and the badges tell nothing a
// user may not see of a title they can list, so the tag itself carries
// what to draw; the signature, Polyfin's, keeps anyone from having other
// badges drawn. Without the setting, or for a title with nothing to show,
// the image keeps its own tag; a request with a badged tag after the
// setting was turned off gets the image without badges, which apps may
// not keep for good. The image as the addon gives it stays in the image
// cache, and each badged one is kept beside it, at each width asked for.
// Previews in the admin app have their own signature, drawn whether the
// setting is on or not, which apps may not keep at all.

const (
	posterBadgesPurpose  = "poster-badges"
	posterPreviewPurpose = "poster-badges-preview"
	// badgeMarkLength is how much of the signature a tag carries.
	badgeMarkLength = 12
)

// badgeSizes maps Settings.PosterBadgeSize to the letter tags carry and
// the size badges are drawn at.
var badgeSizes = map[string]struct {
	letter byte
	size   mosaic.BadgeSize
}{
	"small":  {'s', mosaic.BadgesSmall},
	"medium": {'m', mosaic.BadgesMedium},
	"large":  {'l', mosaic.BadgesLarge},
}

// encodeBadges writes what badges show, at a size of badgeSizes, as tags
// carry it: the size's letter, the star rating in tenths followed by "i"
// when it is IMDb's, the critics' score, the audience score and the
// status, "x" standing for a score there is none of.
func encodeBadges(size string, b mosaic.Badges) string {
	preset, ok := badgeSizes[size]
	if !ok {
		preset = badgeSizes["medium"]
	}
	var out strings.Builder
	out.WriteByte(preset.letter)
	if b.Star > 0 {
		out.WriteString(strconv.Itoa(int(math.Round(b.Star * 10))))
		if b.IMDb {
			out.WriteByte('i')
		}
	} else {
		out.WriteByte('x')
	}
	for _, score := range []int{b.Critics, b.Audience} {
		out.WriteByte('_')
		if score < 0 {
			out.WriteByte('x')
		} else {
			out.WriteString(strconv.Itoa(score))
		}
	}
	out.WriteByte('_')
	out.WriteString(strconv.Itoa(int(b.Status)))
	return out.String()
}

// decodeBadges reads what encodeBadges wrote.
func decodeBadges(text string) (mosaic.Badges, mosaic.BadgeSize, bool) {
	fields := strings.Split(text, "_")
	if len(fields) != 4 || fields[0] == "" {
		return mosaic.Badges{}, 0, false
	}
	var size mosaic.BadgeSize
	found := false
	for _, preset := range badgeSizes {
		if preset.letter == fields[0][0] {
			size, found = preset.size, true
		}
	}
	if !found {
		return mosaic.Badges{}, 0, false
	}
	b := mosaic.Badges{Critics: -1, Audience: -1}
	if star := fields[0][1:]; star != "x" {
		star, b.IMDb = strings.CutSuffix(star, "i")
		tenths, err := strconv.Atoi(star)
		if err != nil || tenths < 1 || tenths > 100 || strconv.Itoa(tenths) != star {
			return mosaic.Badges{}, 0, false
		}
		b.Star = float64(tenths) / 10
	}
	for i, score := range []*int{&b.Critics, &b.Audience} {
		if fields[i+1] == "x" {
			continue
		}
		value, err := strconv.Atoi(fields[i+1])
		if err != nil || value < 0 || value > 100 || strconv.Itoa(value) != fields[i+1] {
			return mosaic.Badges{}, 0, false
		}
		*score = value
	}
	status, err := strconv.Atoi(fields[3])
	if err != nil || status < int(mosaic.NoStatus) || status > int(mosaic.NewSeason) || len(fields[3]) != 1 {
		return mosaic.Badges{}, 0, false
	}
	b.Status = mosaic.Status(status)
	return b, size, !b.Empty()
}

// badgeMark signs what an item's image of tag shows, for purpose.
func (h *Handler) badgeMark(purpose string, id accounts.ID, tag, badges string) string {
	return h.Playback.Signer().Mark(purpose, id.String()+"\x00"+tag+"\x00"+badges)[:badgeMarkLength]
}

// badgedTag is the tag of an item's image of tag with badges over it.
func (h *Handler) badgedTag(purpose string, id accounts.ID, tag, badges string) string {
	return tag + "-" + badges + "-" + h.badgeMark(purpose, id, tag, badges)
}

// badgedRequest reads a tag of badgedTag: the image's own tag, what the
// badges show, and whether it is a preview's. ok is false for any other
// tag, which is then the image's own.
func (h *Handler) badgedRequest(id accounts.ID, tag string) (own string, badges string, preview, ok bool) {
	own, rest, found := strings.Cut(tag, "-")
	if !found || h.Playback == nil {
		return tag, "", false, false
	}
	badges, mark, found := strings.Cut(rest, "-")
	if !found || len(mark) != badgeMarkLength {
		return tag, "", false, false
	}
	switch {
	case hmac.Equal([]byte(mark), []byte(h.badgeMark(posterBadgesPurpose, id, own, badges))):
	case hmac.Equal([]byte(mark), []byte(h.badgeMark(posterPreviewPurpose, id, own, badges))):
		preview = true
	default:
		return tag, "", false, false
	}
	return own, badges, preview, true
}

// posterBadges returns what the badges of the movies and series among
// items show, as tags carry it (see encodeBadges), by item, while the
// settings draw them; nil otherwise.
func (h *Handler) posterBadges(ctx context.Context, items []library.Item) map[accounts.ID]string {
	settings := h.Accounts.Settings()
	if !settings.PosterBadges {
		return nil
	}
	return h.badgesOf(ctx, items, settings.PosterBadgeSize)
}

// badgesOf returns what the badges of the movies and series among items
// show at size, by item, those with nothing to show left out. Artwork an
// administrator uploaded shows none.
func (h *Handler) badgesOf(ctx context.Context, items []library.Item, size string) map[accounts.ID]string {
	if h.Playback == nil {
		return nil
	}
	var titles []library.Item
	for _, item := range items {
		if (item.Kind == library.KindMovie || item.Kind == library.KindSeries) && item.Images.Primary != "" {
			titles = append(titles, item)
		}
	}
	if len(titles) == 0 {
		return nil
	}
	titles = h.Library.Overridden(titles...)
	statuses, err := h.Library.ReleaseStatuses(ctx, titles)
	if err != nil && ctx.Err() == nil {
		h.Logger.Debug("New episodes could not be read for posters", "error", err)
	}
	audience := h.Accounts.Settings().CriticRatingShows == "audience"
	result := map[accounts.ID]string{}
	for _, item := range titles {
		if _, _, uploaded := library.Uploaded(item.Images.Primary); uploaded {
			continue
		}
		rating := h.posterRating(item, audience)
		b := mosaic.Badges{Critics: -1, Audience: -1}
		if rating.CommunityRating != nil {
			b.Star = math.Round(*rating.CommunityRating*10) / 10
			b.IMDb = rating.imdb
		}
		if rating.CriticsScore != nil {
			b.Critics = int(math.Round(min(100, max(0, *rating.CriticsScore))))
		}
		if rating.AudienceScore != nil {
			b.Audience = int(math.Round(min(100, max(0, *rating.AudienceScore))))
		}
		switch statuses[item.ID] {
		case library.NewEpisode:
			b.Status = mosaic.NewEpisode
		case library.NewSeason:
			b.Status = mosaic.NewSeason
		}
		if b.Star > 10 {
			b.Star = 10
		}
		if !b.Empty() {
			result[item.ID] = encodeBadges(size, b)
		}
	}
	return result
}

// badgedKey is the key the image at url with badges over it, as tags carry
// them, is kept by.
func badgedKey(url, badges string) string {
	return url + "\x00badges=" + badges
}

// badged returns the image at url, original, with badges over it, at
// width when it is not 0, made once and kept (see imageCache); the
// original when it cannot be drawn.
func (h *Handler) badged(ctx context.Context, url string, original artwork, badges string, width int) artwork {
	b, size, ok := decodeBadges(badges)
	if !ok || !resizable(original.contentType) {
		return original
	}
	key := badgedKey(url, badges)
	if width > 0 {
		key = resizedKey(key, width)
	}
	image, err := h.images.fetch(ctx, key, func(context.Context) (artwork, error) {
		h.images.resizes <- struct{}{}
		defer func() { <-h.images.resizes }()
		return drawBadges(original, b, size, width)
	})
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Debug("Badges could not be drawn", "error", err)
		}
		return original
	}
	return image
}

// drawBadges draws badges over an image, first scaled down to width when
// it is not 0: as a PNG when it was one, else as a JPEG.
func drawBadges(original artwork, b mosaic.Badges, size mosaic.BadgeSize, width int) (artwork, error) {
	source, format, err := image.Decode(bytes.NewReader(original.body))
	if err != nil {
		return artwork{}, err
	}
	bounds := source.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return artwork{}, fmt.Errorf("empty image")
	}
	var canvas *image.RGBA
	if width > 0 && width < bounds.Dx() {
		height := max(1, int(math.Round(float64(bounds.Dy())*float64(width)/float64(bounds.Dx()))))
		canvas = image.NewRGBA(image.Rect(0, 0, width, height))
		xdraw.BiLinear.Scale(canvas, canvas.Bounds(), source, bounds, xdraw.Src, nil)
	} else {
		canvas = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		xdraw.Draw(canvas, canvas.Bounds(), source, bounds.Min, xdraw.Src)
	}
	mosaic.DrawBadges(canvas, b, size)
	var out bytes.Buffer
	if format == "png" {
		if err := png.Encode(&out, canvas); err != nil {
			return artwork{}, err
		}
		return artwork{body: out.Bytes(), contentType: "image/png"}, nil
	}
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 88}); err != nil {
		return artwork{}, err
	}
	return artwork{body: out.Bytes(), contentType: "image/jpeg"}, nil
}

// PosterPreview is a title whose poster the admin app shows with and
// without badges: its Primary image's own tag, and the tag of a preview
// with badges, the same when it has none to show.
type PosterPreview struct {
	ID        accounts.ID
	Name      string
	Tag       string
	BadgedTag string
}

// PosterPreviews returns up to count movies and series of user's
// libraries, as the catalog pages Polyfin keeps list them, asking no
// addon, with the tags of their posters without badges and with badges
// at size, one of accounts.PosterBadgeSizes, whether the settings draw
// them or not: those with badges to show first.
func (h *Handler) PosterPreviews(ctx context.Context, user accounts.User, size string, count int) ([]PosterPreview, error) {
	pages, err := h.Library.KnownPages(ctx, user, 50)
	if err != nil {
		return nil, err
	}
	var titles []library.Item
	seen := map[accounts.ID]bool{}
	for _, page := range pages {
		for _, item := range page.Items {
			if (item.Kind == library.KindMovie || item.Kind == library.KindSeries) && !seen[item.ID] {
				seen[item.ID] = true
				titles = append(titles, item)
			}
		}
	}
	titles = h.Library.Overridden(titles...)
	titles = slices.DeleteFunc(titles, func(item library.Item) bool { return item.Images.Primary == "" })
	badges := h.badgesOf(ctx, titles, size)
	slices.SortStableFunc(titles, func(a, b library.Item) int {
		_, aShows := badges[a.ID]
		_, bShows := badges[b.ID]
		switch {
		case aShows == bShows:
			return 0
		case aShows:
			return -1
		}
		return 1
	})
	result := []PosterPreview{}
	for _, item := range titles[:min(count, len(titles))] {
		tag := library.ImageTag(item.Images.Primary)
		preview := PosterPreview{ID: item.ID, Name: item.Name, Tag: tag, BadgedTag: tag}
		if shown, ok := badges[item.ID]; ok {
			preview.BadgedTag = h.badgedTag(posterPreviewPurpose, item.ID, tag, shown)
		}
		result = append(result, preview)
	}
	return result, nil
}
