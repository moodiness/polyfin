package jellyfin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The user's sound options (accounts.User BoostVolume and ConvertSound),
// for Polyfin's script in jellyfin-web:
//
//	GET /Polyfin/Sound answers them as SoundDto;
//	POST /Polyfin/Sound turns Boost the volume on or off, as the player's
//	  menu does, and answers as the previous route; a member who follows
//	  their group keeps the group's, refused with 409.
//
// An API key, without a user, may not. Either route marks the caller's
// device as one whose web player runs the script (see boostRelay).
func (h *Handler) soundRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodGet, "/Polyfin/Sound", h.sound)
	signedIn(http.MethodPost, "/Polyfin/Sound", h.setSound)
}

// SoundDto is how the caller's sound is set: BoostVolume makes the web
// player louder in the browser, ConvertSound converts and evens the audio
// on the server; FollowsGroup tells that their group holds both, which
// they cannot change.
type SoundDto struct {
	BoostVolume  bool
	ConvertSound bool
	FollowsGroup bool
}

func soundOf(user accounts.User) SoundDto {
	return SoundDto{BoostVolume: user.BoostVolume, ConvertSound: user.ConvertSound, FollowsGroup: user.GroupID != nil && user.FollowGroup}
}

func (h *Handler) sound(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	if c.APIKey != nil {
		processingError(w, http.StatusForbidden)
		return
	}
	h.scripted.Put(c.Device.ID, struct{}{})
	writeJSON(w, http.StatusOK, soundOf(c.User))
}

// boostRelay reports whether the direct plays the caller's device is about
// to get are relayed for Boost the volume: the script reads the sound only
// of media the page's own server sends, of media a browser fetched with
// CORS, or of HLS it feeds itself, else the browser plays it silent. So
// for a user who boosts, on a device that runs the script, a version is
// never sent to its source: its bytes go through the server, which costs
// the server their bandwidth. Its address then carries relayParameter,
// which tells the script.
func (h *Handler) boostRelay(r *http.Request) bool {
	c := callerFrom(r.Context())
	if !c.User.BoostVolume {
		return false
	}
	_, scripted := h.scripted.Get(c.Device.ID)
	return scripted
}

func (h *Handler) setSound(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	if c.APIKey != nil {
		processingError(w, http.StatusForbidden)
		return
	}
	h.scripted.Put(c.Device.ID, struct{}{})
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	var body struct {
		BoostVolume *bool
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil || json.Unmarshal(raw, &body) != nil || body.BoostVolume == nil {
		badRequestProblem(w)
		return
	}
	user, err := h.Accounts.UpdateUser(r.Context(), c.User.ID, accounts.UserChanges{BoostVolume: body.BoostVolume}, nil)
	if errors.Is(err, accounts.ErrFollowsGroup) {
		processingError(w, http.StatusConflict)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, soundOf(user))
}
