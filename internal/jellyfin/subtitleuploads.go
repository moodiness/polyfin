package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Subtitle files users add.
//
// Jellyfin apps let users who may manage subtitles add a subtitle file to a
// movie or an episode, which Jellyfin saves next to the media. Polyfin
// keeps it in the database for the title, every version included, and
// offers it first among the title's subtitle files (see
// library.Service.Subtitles); administrators delete them, as in Jellyfin.

// uploadedCodecs are the formats a subtitle file may be added in, which
// Polyfin reads, and the codec its stream reports, as Jellyfin names its
// external files' codecs.
var uploadedCodecs = map[string]string{"srt": "subrip", "vtt": "webvtt", "ass": "ass", "ssa": "ssa"}

// maxUploadBody bounds the body of a subtitle upload: a file of
// maxSubtitleBytes in base64, inside JSON.
const maxUploadBody = maxSubtitleBytes/3*4 + 64<<10

func (h *Handler) subtitleUploadRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodPost, "/Videos/{itemId}/Subtitles", h.uploadSubtitle)
	signedIn(http.MethodDelete, "/Videos/{itemId}/Subtitles/{index}", h.deleteSubtitle)
}

// uploadSubtitleDto is Jellyfin's UploadSubtitleDto. Data is the file in
// base64.
type uploadSubtitleDto struct {
	Language          string
	Format            string
	IsForced          bool
	IsHearingImpaired bool
	Data              string
}

// subtitleOwner is the title a subtitle request names: the item, or the
// title of the version it is.
func (h *Handler) subtitleOwner(id accounts.ID) accounts.ID {
	if owner, ok := h.Library.VersionOwner(id); ok {
		return owner
	}
	return id
}

// uploadSubtitle adds a subtitle file to a movie or an episode. Like
// Jellyfin, it is refused to users who may not manage subtitles before
// anything is read, API keys being allowed everything, and answers 400 for
// a format it does not take; Polyfin also refuses a file it cannot read,
// which Jellyfin would save and fail to serve.
func (h *Handler) uploadSubtitle(w http.ResponseWriter, r *http.Request) {
	caller := callerFrom(r.Context())
	user := caller.User
	if !user.SubtitleManagement && caller.APIKey == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUploadBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	var body uploadSubtitleDto
	if err := json.Unmarshal(raw, &body); err != nil {
		b.add("$", "The JSON value could not be converted.")
		b.add("body", "The body field is required.")
	} else {
		for name, value := range map[string]string{"Language": body.Language, "Format": body.Format, "Data": body.Data} {
			if strings.TrimSpace(value) == "" {
				b.add(name, "The "+name+" field is required.")
			}
		}
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	format := strings.ToLower(strings.TrimSpace(body.Format))
	language := strings.TrimSpace(body.Language)
	data, decodeErr := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body.Data), ""))
	item := h.subtitleOwner(id)
	if _, known := uploadedCodecs[format]; !known || len(language) > 32 || strings.ContainsAny(language, `/\`) ||
		decodeErr != nil || len(data) == 0 || len(data) > maxSubtitleBytes {
		// Checked after the item in Jellyfin, which answers 404 first.
		if _, err := h.Library.Item(r.Context(), user, item); errors.Is(err, library.ErrNotFound) {
			notFoundProblem(w)
			return
		}
		processingError(w, http.StatusBadRequest)
		return
	}
	if text, err := readSubtitleText(data); err != nil || len(text.cues) == 0 {
		processingError(w, http.StatusBadRequest)
		return
	}
	err = h.Library.UploadSubtitle(r.Context(), user, item, library.UploadedSubtitle{
		Language: language, Format: format, Forced: body.IsForced, HearingImpaired: body.IsHearingImpaired, Data: data,
	})
	switch {
	case errors.Is(err, library.ErrNotFound):
		notFoundProblem(w)
	case errors.Is(err, library.ErrTooManySubtitles):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		// The title's subtitle files are listed again when next played.
		h.subtitleFiles.Delete(item)
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteSubtitle deletes a subtitle file a user added, the index-th stream
// of the title. Like Jellyfin, only administrators may, and a stream that
// is not such a file is refused with 400: the addons' files and the tracks
// inside versions cannot be deleted.
func (h *Handler) deleteSubtitle(w http.ResponseWriter, r *http.Request) {
	user := callerFrom(r.Context()).User
	if !user.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	index, message := convertInt32(r.PathValue("index"))
	if message != "" {
		b.add("index", message)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item := h.subtitleOwner(id)
	deleted, err := h.Library.DeleteUploadedSubtitle(r.Context(), user, item, index)
	switch {
	case errors.Is(err, library.ErrNotFound):
		notFoundProblem(w)
	case errors.Is(err, library.ErrNotUploaded):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		h.subtitleFiles.Delete(item)
		h.subtitleCache.Delete(deleted)
		w.WriteHeader(http.StatusNoContent)
	}
}
