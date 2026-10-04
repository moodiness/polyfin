package jellyfin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The answers Jellyfin gives, as JSON strings, when a policy change would
// leave the server without an administrator or disable one.
const (
	lastAdministrator     = "There must be at least one user in the system with administrative access."
	administratorDisabled = "Administrators cannot be disabled."
)

// policyUpdate is the part of Jellyfin's UserPolicy Polyfin keeps. Apps
// post the whole policy, as they read it from the user; the capabilities
// Polyfin does not offer are left as they are reported.
type policyUpdate struct {
	IsAdministrator      bool
	IsHidden             bool
	IsDisabled           bool
	MaxParentalRating    *int
	MaxParentalSubRating *int
	BlockUnratedItems    []string
}

// updatePolicy sets a user's policy from an administrator's app: whether
// the user is an administrator, hidden or disabled, and their parental
// control. Like Jellyfin, the policy replaces the stored one whole, and
// disabling a user signs out their devices but the one that asked.
func (h *Handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	caller := callerFrom(r.Context())
	// Jellyfin checks the caller's rights before reading the request.
	if !caller.User.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	errs := bindErrors{}
	id := errs.pathID(r, "userId")
	var policy policyUpdate
	if len(bytes.TrimSpace(raw)) == 0 {
		errs.add("", "A non-empty request body is required.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if json.Unmarshal(raw, &policy) != nil {
		errs.add("$", "The JSON value could not be converted.")
		errs.add("newPolicy", "The newPolicy field is required.")
	} else if i := slices.IndexFunc(policy.BlockUnratedItems, unknownUnratedKind); i >= 0 {
		// Jellyfin reads these as an enumeration, which refuses other names;
		// its message also gives the position in the request.
		path := fmt.Sprintf("$.BlockUnratedItems[%d]", i)
		errs.add(path, "The JSON value could not be converted to Jellyfin.Data.Enums.UnratedItem. Path: "+path+".")
		errs.add("newPolicy", "The newPolicy field is required.")
	}
	if len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		notFoundProblem(w)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if policy.IsDisabled && user.IsAdministrator {
		writeJSON(w, http.StatusForbidden, administratorDisabled)
		return
	}
	_, err = h.Accounts.UpdateUserFromDevice(r.Context(), id, accounts.UserChanges{
		IsAdministrator: &policy.IsAdministrator,
		IsHidden:        &policy.IsHidden,
		IsDisabled:      &policy.IsDisabled,
		Parental: &accounts.ParentalControl{
			MaxRating:    policy.MaxParentalRating,
			MaxSubRating: policy.MaxParentalSubRating,
			BlockUnrated: policy.BlockUnratedItems,
		},
	}, caller.Device.ID)
	switch {
	case errors.Is(err, accounts.ErrLastAdministrator):
		writeJSON(w, http.StatusForbidden, lastAdministrator)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func unknownUnratedKind(kind string) bool {
	return !slices.ContainsFunc(accounts.UnratedKinds, func(known string) bool { return strings.EqualFold(known, kind) })
}
