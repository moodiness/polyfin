package jellyfin

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// The files attached to versions, above all the fonts their ASS tracks
// use: listed in MediaAttachments and served by Jellyfin's attachment
// route, from which jellyfin-web loads them to render ASS.

// MediaAttachment is a file attached to a version, as Jellyfin lists it.
type MediaAttachment struct {
	Codec   string `json:",omitempty"`
	Comment string `json:",omitempty"`
	Index   int
	// FileName and MimeType describe the file; jellyfin-web loads the
	// fonts it recognizes by their MIME type.
	FileName string `json:",omitempty"`
	MimeType string `json:",omitempty"`
	// DeliveryUrl is set in PlaybackInfo answers only.
	DeliveryUrl string `json:",omitempty"`
}

// mediaAttachments lists a version's attached files, as Jellyfin does from
// ffprobe: attachments, and cover art FFmpeg turns into a picture.
func mediaAttachments(analysis media.Analysis) []MediaAttachment {
	attachments := []MediaAttachment{}
	for _, stream := range analysis.Streams {
		if playback.Attached(stream) {
			attachments = append(attachments, MediaAttachment{Codec: stream.Codec, Comment: stream.Comment, Index: stream.Index,
				FileName: stream.FileName, MimeType: attachmentMimeType(stream)})
		}
	}
	return attachments
}

// attachmentURL is an attached file's DeliveryUrl: Jellyfin's, with the
// caller's token, as Polyfin signs every media URL it hands out where
// Jellyfin serves attached files to anyone.
func attachmentURL(r *http.Request, item, sourceID accounts.ID, index int) string {
	target := "/Videos/" + hyphenated(item) + "/" + sourceID.String() + "/Attachments/" + strconv.Itoa(index)
	if token := callerFrom(r.Context()).Token; token != "" {
		target += "?ApiKey=" + url.QueryEscape(token)
	}
	return target
}

// fontMimeTypes are the MIME types jellyfin-web loads attached fonts by.
var fontMimeTypes = []string{"application/vnd.ms-opentype", "application/x-truetype-font", "font/otf", "font/ttf", "font/woff", "font/woff2"}

// fontFormats are the MIME types of font files by extension, among those
// jellyfin-web loads. Collections are read like the fonts they gather.
var fontFormats = map[string]string{
	".ttf": "font/ttf", ".ttc": "font/ttf",
	".otf": "font/otf", ".otc": "font/otf",
	".woff": "font/woff", ".woff2": "font/woff2",
}

// attachmentMimeType is the MIME type an attached file is listed and
// served with. Jellyfin repeats the one the file gives, and jellyfin-web
// then skips fonts a muxer named otherwise (application/x-font-ttf,
// font/collection, application/octet-stream...), rendering their text in
// another font: Polyfin names such fonts by the type of their format.
func attachmentMimeType(stream media.Stream) string {
	given := strings.ToLower(stream.MimeType)
	if slices.Contains(fontMimeTypes, given) {
		// jellyfin-web compares types as written.
		return given
	}
	format, named := fontFormats[strings.ToLower(path.Ext(stream.FileName))]
	font := stream.Codec == "ttf" || stream.Codec == "otf" || named && (given == "" || given == "application/octet-stream") ||
		strings.Contains(given, "font") || strings.Contains(given, "truetype") || strings.Contains(given, "opentype")
	switch {
	case !font:
		return stream.MimeType
	case named:
		return format
	case stream.Codec == "otf":
		return "font/otf"
	default:
		return "font/ttf"
	}
}

// attachment serves a file attached to a version, by its stream index.
// Jellyfin serves them to anyone; Polyfin asks for the token its
// DeliveryUrls carry, as for subtitles inside files. Jellyfin also serves
// a file as the type the file gives it; Polyfin serves only fonts so, which
// is all jellyfin-web loads from here.
func (h *Handler) attachment(w http.ResponseWriter, r *http.Request) {
	opened, okItem := parseGUID(r.PathValue("itemId"))
	source := r.PathValue("mediaSourceId")
	wanted, okSource := parseGUID(source)
	index, err := strconv.Atoi(r.PathValue("index"))
	if !okItem || err != nil {
		notFoundProblem(w)
		return
	}
	user, _, signedIn := h.streamAccess(r)
	if !signedIn {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	item := opened
	if owner, ok := h.Library.VersionOwner(opened); ok {
		item = owner
	}
	if _, err := h.title(r.Context(), user, item); err != nil {
		notFoundProblem(w)
		return
	}
	if !okSource {
		writeJSON(w, http.StatusNotFound, fmt.Sprintf("MediaSource %s not found", source))
		return
	}
	version, err := h.Library.Version(r.Context(), user, item, wanted)
	if err != nil {
		writeJSON(w, http.StatusNotFound, fmt.Sprintf("MediaSource %s not found", source))
		return
	}
	analysis, analyzed := h.Playback.Analyzed(r.Context(), version.ID)
	at := slices.IndexFunc(analysis.Streams, func(stream media.Stream) bool { return stream.Index == index })
	if !analyzed || at < 0 || !playback.Attached(analysis.Streams[at]) {
		writeJSON(w, http.StatusNotFound, fmt.Sprintf("MediaSource %s has no attachment with stream index %d", wanted, index))
		return
	}
	stream := analysis.Streams[at]
	cannotExtract := func() {
		writeJSON(w, http.StatusNotFound, fmt.Sprintf("Attachment with stream index %d can't be extracted for MediaSource %s", index, wanted))
	}
	if stream.AttachedPicture {
		// Jellyfin lists cover art, which FFmpeg makes a video stream, among
		// a version's attachments, but does not extract it.
		cannotExtract()
		return
	}
	data, err := h.Playback.Attachment(r.Context(), version, analysis, index)
	if err != nil {
		if r.Context().Err() == nil && !errors.Is(err, playback.ErrNoAttachment) {
			h.Logger.Warn("An attached file could not be read", "addon", version.Addon, "error", err)
		}
		cannotExtract()
		return
	}
	// Anything else a file carries goes out as a download, sandboxed: a
	// page or a script in a file must not run as Polyfin's own.
	contentType := attachmentMimeType(stream)
	if !slices.Contains(fontMimeTypes, contentType) {
		contentType = "application/octet-stream"
		w.Header().Set("Content-Disposition", "attachment")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}
