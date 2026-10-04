package jellyfin

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// TrickplayInfo describes a version's scrubbing thumbnails at one width,
// as Jellyfin's TrickplayInfoDto does.
type TrickplayInfo struct {
	Width          int
	Height         int
	TileWidth      int
	TileHeight     int
	ThumbnailCount int
	Interval       int
	Bandwidth      int
}

// trickplayManifest is the Trickplay field of a movie's or episode's DTO:
// the thumbnails of its versions, by media source and width, as Jellyfin
// lists them; empty when there are none, or when the settings turn them
// off.
func (h *Handler) trickplayManifest(ctx context.Context, item accounts.ID) *map[string]map[int]TrickplayInfo {
	manifest := map[string]map[int]TrickplayInfo{}
	if h.Thumbnails == nil {
		return &manifest
	}
	sets, err := h.Thumbnails.Manifest(ctx, item)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The thumbnails of a title could not be listed", "error", err)
		}
		return &manifest
	}
	for version, widths := range sets {
		described := map[int]TrickplayInfo{}
		for width, info := range widths {
			described[width] = TrickplayInfo(info)
		}
		manifest[version.String()] = described
	}
	return &manifest
}

// queueImages asks, in the background, for the thumbnails and chapter
// images of the version a playback started with, when the settings turn
// them on.
func (h *Handler) queueImages(ctx context.Context, user accounts.User, item library.Item, mediaSource string) {
	// Recordings are Polyfin's own files, which may be deleted at any time:
	// they get no thumbnails or chapter images.
	if h.Thumbnails == nil || item.Kind == library.KindRecording {
		return
	}
	if settings := h.Accounts.Settings(); !settings.Trickplay && !settings.ChapterImages {
		return
	}
	id, ok := parseGUID(mediaSource)
	if !ok {
		return
	}
	version, err := h.Library.Version(ctx, user, item.ID, id)
	if err != nil {
		return
	}
	h.Thumbnails.Queue(version)
}

// trickplayFile serves a title's thumbnails at a width:
// /Videos/{itemId}/Trickplay/{width}/tiles.m3u8, the HLS playlist of its
// tiles, and /Videos/{itemId}/Trickplay/{width}/{index}.jpg, a tile. They
// are those of the version mediaSourceId names, else of the version the
// item was opened as, else of the title's version whose thumbnails were
// used last. The user, the caller or the one userId names, must be able
// to open the title.
func (h *Handler) trickplayFile(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	opened := b.pathID(r, "itemId")
	width, err := strconv.Atoi(r.PathValue("width"))
	if err != nil {
		b.add("width", notValid(r.PathValue("width")))
	}
	name, extension, _ := strings.Cut(r.PathValue("file"), ".")
	playlist := strings.EqualFold(r.PathValue("file"), "tiles.m3u8")
	index := 0
	switch {
	case playlist:
	case strings.EqualFold(extension, "jpg"):
		if index, err = strconv.Atoi(name); err != nil {
			b.add("index", notValid(name))
		}
	default:
		processingError(w, http.StatusNotFound)
		return
	}
	mediaSource, _ := b.guid(r, "mediaSourceId")
	userID, userSet := b.guid(r, "userId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	// The caller, or the user an administrator or an API key names: a key
	// without one reads as no one, as its other requests browse.
	user, ok := h.targetUser(w, r, userID, userSet, unknownListingUser)
	if !ok {
		return
	}
	c := callerFrom(r.Context())
	item, err := h.title(r.Context(), user, opened)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	if mediaSource == (accounts.ID{}) && opened != item.ID {
		mediaSource = opened
	}
	if h.Thumbnails == nil {
		notFoundProblem(w)
		return
	}
	version, info, err := h.Thumbnails.Trickplay(r.Context(), item.ID, mediaSource, width)
	switch {
	case errors.Is(err, thumbnails.ErrNotFound):
		notFoundProblem(w)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	if playlist {
		w.Header().Set("Content-Type", "application/x-mpegURL; charset=utf-8")
		_, _ = w.Write([]byte(trickplayPlaylist(info, version, c.Token)))
		return
	}
	tile, err := h.Thumbnails.Tile(r.Context(), version, width, index)
	switch {
	case errors.Is(err, thumbnails.ErrNotFound):
		notFoundProblem(w)
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "attachment")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(tile))
}

// trickplayPlaylist is the HLS playlist of a version's tiles, written as
// Jellyfin writes it: each tile a segment lasting the thumbnails it holds,
// a target duration of the number of tiles, and tile URLs carrying the
// media source and the caller's token.
func trickplayPlaylist(info thumbnails.Info, version accounts.ID, token string) string {
	var b strings.Builder
	perTile := info.TileWidth * info.TileHeight
	if info.ThumbnailCount <= 0 || perTile <= 0 {
		return ""
	}
	seconds := float64(info.Interval) / 1000
	tiles := (info.ThumbnailCount + perTile - 1) / perTile
	b.WriteString("#EXTM3U\n#EXT-X-TARGETDURATION:" + strconv.Itoa(tiles) + "\n#EXT-X-VERSION:7\n#EXT-X-MEDIA-SEQUENCE:1\n" +
		"#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-IMAGES-ONLY\n")
	resolution := strconv.Itoa(info.Width) + "x" + strconv.Itoa(info.Height)
	layout := strconv.Itoa(info.TileWidth) + "x" + strconv.Itoa(info.TileHeight)
	for i := range tiles {
		count := perTile
		if i == tiles-1 {
			count = info.ThumbnailCount - i*perTile
		}
		b.WriteString("#EXTINF:" + playlistDecimal(seconds*float64(count)) + ",\n")
		b.WriteString("#EXT-X-TILES:RESOLUTION=" + resolution + ",LAYOUT=" + layout + ",DURATION=" + playlistDecimal(seconds) + "\n")
		b.WriteString(strconv.Itoa(i) + ".jpg?MediaSourceId=" + version.String() + "&ApiKey=" + url.QueryEscape(token) + "\n")
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// playlistDecimal writes a number as .NET's "0.###" format does: at most
// three decimals, none trailing.
func playlistDecimal(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}
