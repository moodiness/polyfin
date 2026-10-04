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
// the thumbnails of the versions the user is offered, by media source and
// width, as Jellyfin lists them, keyed as the item's MediaSources name
// them: versions, in their order, the first under the identifier the item
// was opened with (see sourceID). Apps look a playing source's thumbnails
// up by its identifier. When the versions are not known, as in listings
// before the title opened, the thumbnails are keyed by version, those
// taller than the user's quality group left out while one fits. Empty when
// there are none, or when the settings turn them off.
func (h *Handler) trickplayManifest(ctx context.Context, user accounts.User, item library.Item, versions []library.Version, opened accounts.ID) *map[string]map[int]TrickplayInfo {
	manifest := map[string]map[int]TrickplayInfo{}
	if h.Thumbnails == nil {
		return &manifest
	}
	sets, err := h.Thumbnails.Manifest(ctx, item.ID)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The thumbnails of a title could not be listed", "error", err)
		}
		return &manifest
	}
	describe := func(widths map[int]thumbnails.Info) map[int]TrickplayInfo {
		described := map[int]TrickplayInfo{}
		for width, info := range widths {
			described[width] = TrickplayInfo(info)
		}
		return described
	}
	if len(versions) > 0 {
		for i, version := range versions {
			if widths, ok := sets[version.ID]; ok {
				manifest[sourceID(opened, version, i == 0).String()] = describe(widths)
			}
		}
		return &manifest
	}
	fitting := map[accounts.ID]bool{}
	for version := range sets {
		if analysis, ok := h.Playback.Analyzed(ctx, version); !ok || user.FitsGroup(videoHeight(analysis)) {
			fitting[version] = true
		}
	}
	for version, widths := range sets {
		if fitting[version] || len(fitting) == 0 {
			manifest[version.String()] = describe(widths)
		}
	}
	return &manifest
}

// trickplayVersion is the version whose thumbnails a request reads: the
// media source it names, else the version the item was opened as. The
// title's own identifier names the version it plays first for the user,
// as its first media source carries it: the first of its versions known,
// those taller than their quality group left out; zero when none is known,
// for the title's version whose thumbnails were used last.
func (h *Handler) trickplayVersion(ctx context.Context, user accounts.User, item library.Item, opened, mediaSource accounts.ID) accounts.ID {
	named := mediaSource
	if named == (accounts.ID{}) {
		named = opened
	}
	if named != item.ID {
		return named
	}
	if versions := h.cachedPlayable(ctx, user, item).versions; len(versions) > 0 {
		return versions[0].ID
	}
	if user.QualityGroup > 0 {
		if first := h.firstWorkingVersion(ctx, user, item); first != item.ID {
			return first
		}
	}
	return accounts.ID{}
}

// queueImages asks, in the background, for the thumbnails and chapter
// images of the version a playback on device started with, when the
// settings turn them on: they are made once it stopped.
func (h *Handler) queueImages(ctx context.Context, user accounts.User, device accounts.ID, item library.Item, mediaSource string) {
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
	// With a quality group, the title's own identifier stands for its
	// first version that fits, as in PlaybackInfo: the others are not read.
	if id == item.ID && user.QualityGroup > 0 {
		id = h.firstWorkingVersion(ctx, user, item)
	}
	version, err := h.Library.Version(ctx, user, item.ID, id)
	if err != nil {
		return
	}
	h.Thumbnails.Queue(version, device)
}

// playingStale is how long a playback no report came for still counts as
// under way: apps report every few seconds, and some never report a stop.
const playingStale = 5 * time.Minute

// thumbnailPlaybacks lists the playbacks under way for the thumbnails,
// which wait for them, with the URL of their version when it is known.
func (h *Handler) thumbnailPlaybacks() []thumbnails.Playing {
	var playing []thumbnails.Playing
	for device, now := range h.sessions.All() {
		if time.Since(now.CheckedIn) > playingStale {
			continue
		}
		p := thumbnails.Playing{Device: device}
		if id, ok := parseGUID(now.MediaSourceID); ok {
			if version, known := h.Library.KnownVersion(id); known {
				p.URL = version.URL
			}
		}
		playing = append(playing, p)
	}
	return playing
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
	if h.Thumbnails == nil {
		notFoundProblem(w)
		return
	}
	named := h.trickplayVersion(r.Context(), user, item, opened, mediaSource)
	version, info, err := h.Thumbnails.Trickplay(r.Context(), item.ID, named, width)
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
