package jellyfin

import (
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Profile pictures.
//
// Jellyfin apps show each user's picture on the sign-in screen, in their
// menus and beside sessions, and let a user change their own, as
// administrators may anyone's. Apps send the picture encoded in base64,
// with its type as Content-Type. Polyfin keeps it in the database,
// normalized (see accounts.Store.SetImage), and tags it in UserDto's
// PrimaryImageTag and SessionInfo's UserPrimaryImageTag.

// imageTypes are Jellyfin's ImageType names, which the older routes name
// in their path. Users have one picture whatever the type named, as in
// Jellyfin.
var imageTypes = []string{"Primary", "Art", "Backdrop", "Banner", "Logo", "Thumb", "Disc", "Box", "Screenshot", "Menu", "Chapter", "BoxRear", "Profile"}

// maxImageBody bounds the body of an upload: a picture of
// accounts.MaxImageBytes in base64, with room for line breaks.
const maxImageBody = accounts.MaxImageBytes/3*4 + 64<<10

func (h *Handler) userImageRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	// Apps load pictures without credentials, as Jellyfin allows.
	rt.handle(http.MethodGet, "/UserImage", http.HandlerFunc(h.userImage))
	rt.handle(http.MethodGet, "/Users/{userId}/Images/{imageType}", http.HandlerFunc(h.userImage))
	rt.handle(http.MethodGet, "/Users/{userId}/Images/{imageType}/{imageIndex}", http.HandlerFunc(h.userImage))
	for _, pattern := range []string{"/UserImage", "/Users/{userId}/Images/{imageType}", "/Users/{userId}/Images/{imageType}/{index}"} {
		signedIn(http.MethodPost, pattern, h.setUserImage)
		signedIn(http.MethodDelete, pattern, h.deleteUserImage)
	}
}

// bindImageRoute binds what the older routes add to the user: the image
// type and index, which change nothing.
func bindImageRoute(r *http.Request, b bindErrors) {
	if raw := r.PathValue("imageType"); raw != "" {
		b.enum("imageType", raw, imageTypes)
	}
	for _, name := range []string{"imageIndex", "index"} {
		if raw := r.PathValue(name); raw != "" {
			if _, message := convertInt32(raw); message != "" {
				b.add(name, message)
			}
		}
	}
}

// userImage serves a user's picture: the userId one, else the caller's.
// Like Jellyfin, anyone may load it.
func (h *Handler) userImage(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id, set := b.userID(r)
	bindImageRoute(r, b)
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	if !set {
		caller, ok, err := h.signedInCaller(r)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		// An API key has no user of its own, as in Jellyfin.
		if !ok || caller.APIKey != nil {
			writeJSON(w, http.StatusBadRequest, "UserId is required if unauthenticated")
			return
		}
		id = caller.User.ID
	}
	picture, err := h.Accounts.Image(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	header := w.Header()
	header.Set("Content-Type", picture.ContentType)
	header.Set("Content-Length", strconv.Itoa(len(picture.Data)))
	header.Set("ETag", `"`+picture.Tag+`"`)
	// A picture asked for by its tag never changes; one asked for without
	// may change at any time.
	if query(r, "tag") == picture.Tag {
		header.Set("Cache-Control", "public, max-age=604800")
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	if r.Header.Get("If-None-Match") == `"`+picture.Tag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(picture.Data)
	}
}

// imageOwner resolves the user whose picture a request changes and checks
// that the caller may: the user themselves, or an administrator. ok is
// false once w was answered.
func (h *Handler) imageOwner(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	b := bindErrors{}
	id, set := b.userID(r)
	bindImageRoute(r, b)
	if len(b) > 0 {
		validationProblem(w, b)
		return accounts.User{}, false
	}
	// An API key has no user of its own: Jellyfin finds none.
	if !set && callerFrom(r.Context()).APIKey != nil {
		notFoundProblem(w)
		return accounts.User{}, false
	}
	return h.targetUser(w, r, id, set, notFoundProblem)
}

// setUserImage replaces a user's picture with the body, a JPEG, PNG or
// WebP image in base64. Jellyfin takes any image type and keeps the file
// as sent; Polyfin refuses other types as Jellyfin refuses a type that is
// not an image, and a body it cannot read as a picture with 400.
func (h *Handler) setUserImage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.imageOwner(w, r)
	if !ok {
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !slices.Contains(accounts.ImageTypes, strings.ToLower(contentType)) {
		writeJSON(w, http.StatusBadRequest, "Incorrect ContentType.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImageBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.Map(func(c rune) rune {
		if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
			return -1
		}
		return c
	}, string(body)))
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	_, err = h.Accounts.SetImage(r.Context(), user.ID, data)
	switch {
	case errors.Is(err, accounts.ErrInvalidImage):
		processingError(w, http.StatusBadRequest)
	case errors.Is(err, accounts.ErrNotFound):
		notFoundProblem(w)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteUserImage removes a user's picture; a user without one is left
// as is.
func (h *Handler) deleteUserImage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.imageOwner(w, r)
	if !ok {
		return
	}
	_, err := h.Accounts.DeleteImage(r.Context(), user.ID)
	switch {
	case errors.Is(err, accounts.ErrNotFound):
		notFoundProblem(w)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
