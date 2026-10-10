package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

// invitePath is where the admin app opens an invite's link, followed by
// its token.
const invitePath = "/admin/invite/"

// defaultInviteDays is how long an invite lasts when its creation does not
// say.
const defaultInviteDays = 7

// inviteUserJSON names a user an invite refers to.
type inviteUserJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newInviteUserJSON(user *accounts.InviteUser) *inviteUserJSON {
	if user == nil {
		return nil
	}
	return &inviteUserJSON{ID: user.ID.String(), Name: user.Name}
}

// inviteJSON is an invite link as the invites list shows it: how many
// accounts it may create and has created, when it expires (null for
// never), the user whose settings new accounts copy (null for a new user's
// defaults), who created it (null once deleted), and its state: active,
// used_up, expired or revoked.
type inviteJSON struct {
	ID        string          `json:"id"`
	MaxUses   int             `json:"maxUses"`
	Uses      int             `json:"uses"`
	ExpiresAt *time.Time      `json:"expiresAt"`
	CreatedAt time.Time       `json:"createdAt"`
	RevokedAt *time.Time      `json:"revokedAt"`
	Model     *inviteUserJSON `json:"model"`
	CreatedBy *inviteUserJSON `json:"createdBy"`
	State     string          `json:"state"`
}

func (h *handler) newInviteJSON(invite accounts.Invite) inviteJSON {
	return inviteJSON{
		ID:        invite.ID.String(),
		MaxUses:   invite.MaxUses,
		Uses:      invite.Uses,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
		RevokedAt: invite.RevokedAt,
		Model:     newInviteUserJSON(invite.Model),
		CreatedBy: newInviteUserJSON(invite.CreatedBy),
		State:     invite.State(h.now()),
	}
}

func (h *handler) invites(w http.ResponseWriter, r *http.Request) {
	invites, err := h.Accounts.Invites(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]inviteJSON, 0, len(invites))
	for _, invite := range invites {
		result = append(result, h.newInviteJSON(invite))
	}
	writeJSON(w, http.StatusOK, result)
}

// createInvite creates an invite link. maxUses defaults to 1, and
// expiresInDays to 7; null expiresInDays never expires. The answer holds
// the link's token, given this once, and its address when the server's
// public address is set (null otherwise: the admin app completes it).
func (h *handler) createInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MaxUses       *int            `json:"maxUses"`
		ExpiresInDays json.RawMessage `json:"expiresInDays"`
		ModelUserID   *string         `json:"modelUserId"`
	}
	if !decode(w, r, &body) {
		return
	}
	invite := accounts.NewInvite{MaxUses: valueOr(body.MaxUses, 1), Days: defaultInviteDays, CreatedBy: sessionFrom(r.Context()).User.ID}
	switch string(body.ExpiresInDays) {
	case "":
	case "null":
		invite.Days = 0
	default:
		// 0 would mean never: only null says so.
		if json.Unmarshal(body.ExpiresInDays, &invite.Days) != nil || invite.Days == 0 {
			writeError(w, http.StatusBadRequest, "invalid_invite_expiry")
			return
		}
	}
	if body.ModelUserID != nil {
		id, err := accounts.ParseID(*body.ModelUserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_invite_model")
			return
		}
		invite.Model = &id
	}
	created, token, err := h.Accounts.CreateInvite(r.Context(), invite)
	if inviteError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var link *string
	if base := h.Accounts.Settings().PublicAddress; base != "" {
		link = new(base + invitePath + token)
	}
	writeJSON(w, http.StatusCreated, struct {
		inviteJSON
		Token string  `json:"token"`
		URL   *string `json:"url"`
	}{h.newInviteJSON(created), token, link})
}

func (h *handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	invite, err := h.Accounts.RevokeInvite(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.newInviteJSON(invite))
}

// inviteError answers the client-facing invite errors and reports whether
// err was one of them.
func inviteError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{accounts.ErrInvalidInviteUses, http.StatusBadRequest, "invalid_invite_uses"},
		{accounts.ErrInvalidInviteExpiry, http.StatusBadRequest, "invalid_invite_expiry"},
		{accounts.ErrInvalidInviteModel, http.StatusBadRequest, "invalid_invite_model"},
		{accounts.ErrInviteUnknown, http.StatusNotFound, "invite_unknown"},
		{accounts.ErrInviteUsedUp, http.StatusGone, "invite_used_up"},
		{accounts.ErrInviteExpired, http.StatusGone, "invite_expired"},
		{accounts.ErrInviteRevoked, http.StatusGone, "invite_revoked"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// publicInvite answers whether an invite's link may still create an
// account, with the server's name, for the guest's page, which needs no
// session. A token that is no invite's counts as a wrong password does
// toward the client's failed attempts.
func (h *handler) publicInvite(w http.ResponseWriter, r *http.Request) {
	key := throttle.ClientKey(r)
	if h.throttled(w, key) {
		return
	}
	_, err := h.Accounts.InviteByToken(r.Context(), r.PathValue("token"))
	if errors.Is(err, accounts.ErrInviteUnknown) {
		h.SignIns.Fail(key)
	}
	if inviteError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"serverName": h.Accounts.Settings().ServerName})
}

// acceptInvite creates the member account a guest asks for through an
// invite's link: only its name and password come from the guest, its
// settings from the invite. With the web client, the guest's page then
// signs in to it through the Jellyfin API; without it, the guest is signed
// in to the admin app here.
func (h *handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := throttle.ClientKey(r)
	if h.throttled(w, key) {
		return
	}
	user, invite, err := h.Accounts.AcceptInvite(r.Context(), r.PathValue("token"), body.Name, body.Password)
	if errors.Is(err, accounts.ErrInviteUnknown) {
		h.SignIns.Fail(key)
	}
	if inviteError(w, err) || accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.SignIns.Succeed(key)
	h.Activity.UserJoined(r.Context(), user, invite, clientAddress(r))
	if h.Notifications != nil {
		h.Notifications.UserJoined(r.Context(), user, invite)
	}
	if h.WebClient {
		writeJSON(w, http.StatusCreated, map[string]any{"user": newSessionUser(user), "webClient": true})
		return
	}
	token, expires, err := h.Accounts.CreateAdminSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.SignedIn(r.Context(), user, clientAddress(r))
	setCookie(w, r, token, expires)
	writeJSON(w, http.StatusCreated, map[string]any{"user": newSessionUser(user), "webClient": false})
}
