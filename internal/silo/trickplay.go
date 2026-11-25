package silo

import (
	"bytes"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// The scrubbing thumbnails Jellyfin apps get (see docs/scrubbing-thumbnails.md)
// are the seek-bar previews of Silo's apps: a version's tiles of Jellyfin's
// grid are Silo's sheets, laid out the same way.

func (s *Server) trickplayRoutes() {
	s.mux.HandleFunc("GET /api/v2/watch/{id}/trickplay", s.profiled(s.watchTrickplay))
	// The sheets' addresses carry their own authorization, as Silo's do:
	// the apps load them as images, without credentials.
	s.mux.HandleFunc("GET /api/v2/artwork/trickplay/{version}/{width}/{sheet}", s.trickplaySheet)
}

// trickplayInfo describes a version's thumbnails at a width, as the
// Jellyfin API's Trickplay field does: the size of a thumbnail, the
// thumbnails across and down a tile, how many there are, and the time
// each covers, in milliseconds.
type trickplayInfo struct {
	Width, Height, TileWidth, TileHeight, ThumbnailCount, Interval int
}

// trickplayLifetime is how long the addresses of the sheets work: the
// apps read the previews again for fresh ones once it has passed.
const trickplayLifetime = time.Hour

// trickplayPurpose is what the addresses of the sheets are signed for.
const trickplayPurpose = "silo.trickplay"

type trickplaySheet struct {
	Index int    `json:"index"`
	URL   string `json:"url"`
}

type watchTrickplay struct {
	FileID          string           `json:"file_id"`
	IntervalMS      int              `json:"interval_ms"`
	ThumbnailWidth  int              `json:"thumbnail_width"`
	ThumbnailHeight int              `json:"thumbnail_height"`
	TileColumns     int              `json:"tile_columns"`
	TileRows        int              `json:"tile_rows"`
	ThumbnailCount  int              `json:"thumbnail_count"`
	Sheets          []trickplaySheet `json:"sheets"`
	ExpiresAt       string           `json:"expires_at"`
}

// watchTrickplay answers the seek-bar previews of the version file_id
// names of a movie or an episode: its thumbnails at their widest, as
// Silo serves one width. A version without any, or while the settings
// turn them off, has none, as Silo answers it: not found.
func (s *Server) watchTrickplay(w http.ResponseWriter, r *http.Request, v viewer) {
	file := r.URL.Query().Get("file_id")
	if file == "" {
		validationProblem(w, "query.file_id", "required", "The file_id parameter names the file played.")
		return
	}
	i, versions, ok := s.watchable(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	at := slices.IndexFunc(versions, func(version fileVersion) bool { return version.FileID == file })
	if at < 0 {
		problem(w, http.StatusNotFound, "not_found", "The item has no such file.")
		return
	}
	version := versions[at]
	widths := i.Trickplay[version.sourceID]
	if len(widths) == 0 {
		problem(w, http.StatusNotFound, "not_found", "This file has no seek-bar previews.")
		return
	}
	width := slices.Max(slices.Collect(maps.Keys(widths)))
	info := widths[width]
	perSheet := info.TileWidth * info.TileHeight
	if info.ThumbnailCount <= 0 || perSheet <= 0 {
		problem(w, http.StatusNotFound, "not_found", "This file has no seek-bar previews.")
		return
	}
	expires := s.options.now().Add(trickplayLifetime).Truncate(time.Second)
	exp := strconv.FormatInt(expires.Unix(), 10)
	folder := "/api/v2/artwork/trickplay/" + version.versionID + "/" + strconv.Itoa(width) + "/"
	signature := s.options.Signer.Mark(trickplayPurpose, sheetsSigned(version.versionID, strconv.Itoa(width), exp))
	answer := watchTrickplay{FileID: version.FileID, IntervalMS: info.Interval, ThumbnailWidth: info.Width, ThumbnailHeight: info.Height,
		TileColumns: info.TileWidth, TileRows: info.TileHeight, ThumbnailCount: info.ThumbnailCount, Sheets: []trickplaySheet{},
		ExpiresAt: instant(expires)}
	for n := range (info.ThumbnailCount + perSheet - 1) / perSheet {
		answer.Sheets = append(answer.Sheets, trickplaySheet{Index: n, URL: folder + strconv.Itoa(n) + ".jpg?exp=" + exp + "&sig=" + signature})
	}
	writeJSON(w, http.StatusOK, answer)
}

// sheetsSigned is what the addresses of a version's sheets at a width
// sign, until exp.
func sheetsSigned(version, width, exp string) string {
	return version + "/" + width + "/" + exp
}

// trickplaySheet serves a sheet of a version's thumbnails, a tile of
// Jellyfin's grid, at an address watchTrickplay signed, until it expires.
func (s *Server) trickplaySheet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	exp, sig := query.Get("exp"), query.Get("sig")
	expires, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || !s.options.now().Before(time.Unix(expires, 0)) ||
		!s.options.Signer.VerifyMark(trickplayPurpose, sheetsSigned(r.PathValue("version"), r.PathValue("width"), exp), sig) {
		problem(w, http.StatusForbidden, "permission_denied", "This address expired; read the previews again.")
		return
	}
	version, errVersion := accounts.ParseID(r.PathValue("version"))
	width, errWidth := strconv.Atoi(r.PathValue("width"))
	name, jpeg := strings.CutSuffix(r.PathValue("sheet"), ".jpg")
	index, errIndex := strconv.Atoi(name)
	if errVersion != nil || errWidth != nil || errIndex != nil || !jpeg || index < 0 || s.options.Tiles == nil {
		problem(w, http.StatusNotFound, "not_found", "No such image.")
		return
	}
	tile, err := s.options.Tiles(r.Context(), version, width, index)
	if errors.Is(err, thumbnails.ErrNotFound) {
		problem(w, http.StatusNotFound, "not_found", "No such image.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(int(trickplayLifetime/time.Second)))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(tile))
}
