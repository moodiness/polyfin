package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// sharePath is where a share link's page is, followed by its token (see
// the jellyfin package's share.go).
const sharePath = "/share/"

// maxShareResults bounds the titles a search for one to share lists.
const maxShareResults = 20

// Sharing finds the titles users may give through share links: the
// library's.
type Sharing interface {
	Shareable(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
	Search(ctx context.Context, user accounts.User, term string, kinds []library.Kind, limit int) ([]library.Item, error)
	Episodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]library.Item, error)
	HiddenTitles(ctx context.Context, user accounts.User) (library.HiddenTitles, error)
}

// shareLinkJSON is a share link as the lists show it: its title, as it
// was named when the link was made, who made it, when it expires, how
// many guests may watch at once and how many times it was played, and its
// state: active, expired or revoked.
type shareLinkJSON struct {
	ID         string            `json:"id"`
	User       inviteUserJSON    `json:"user"`
	Item       shareLinkItemJSON `json:"item"`
	MaxViewers int               `json:"maxViewers"`
	CreatedAt  time.Time         `json:"createdAt"`
	ExpiresAt  time.Time         `json:"expiresAt"`
	RevokedAt  *time.Time        `json:"revokedAt"`
	Plays      int               `json:"plays"`
	State      string            `json:"state"`
}

type shareLinkItemJSON struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (h *handler) newShareLinkJSON(link accounts.ShareLink) shareLinkJSON {
	return shareLinkJSON{ID: link.ID.String(), User: inviteUserJSON{ID: link.User.ID.String(), Name: link.User.Name},
		Item:       shareLinkItemJSON{ID: link.Item.String(), Kind: link.Kind, Name: link.Title},
		MaxViewers: link.MaxViewers, CreatedAt: link.CreatedAt, ExpiresAt: link.ExpiresAt, RevokedAt: link.RevokedAt, Plays: link.Plays,
		State: link.State(h.now())}
}

func (h *handler) shareLinksJSON(links []accounts.ShareLink) []shareLinkJSON {
	result := make([]shareLinkJSON, 0, len(links))
	for _, link := range links {
		result = append(result, h.newShareLinkJSON(link))
	}
	return result
}

// ownShareLinks lists the links of the user signed in, with whether they
// may make one now: the server allows share links, and their account may
// share.
func (h *handler) ownShareLinks(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	links, err := h.Accounts.ShareLinks(r.Context(), &user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowed": h.mayShare(user), "links": h.shareLinksJSON(links)})
}

// mayShare reports whether user may make share links now.
func (h *handler) mayShare(user accounts.User) bool {
	return h.Sharing != nil && h.Accounts.MayShare(user)
}

// shareError answers the client-facing share link errors and reports
// whether err was one of them.
func shareError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{accounts.ErrInvalidShareExpiry, http.StatusBadRequest, "invalid_share_expiry"},
		{accounts.ErrInvalidShareViewers, http.StatusBadRequest, "invalid_share_viewers"},
		{accounts.ErrInvalidShareTitle, http.StatusBadRequest, "not_shareable"},
		{accounts.ErrShareLinksOff, http.StatusForbidden, "share_links_off"},
		{library.ErrNotShareable, http.StatusBadRequest, "not_shareable"},
		{library.ErrNotFound, http.StatusNotFound, "not_found"},
		{accounts.ErrNotFound, http.StatusNotFound, "not_found"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// createShareLink makes a link to a movie or an episode the user signed in
// may see. expiresInHours is 1, 24 (the default), 168 or 720; maxViewers 1
// (the default) to 5. The answer holds the link's token, given this once,
// and its address when the server's public address is set (null
// otherwise: the admin app completes it).
func (h *handler) createShareLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemID         string `json:"itemId"`
		ExpiresInHours *int   `json:"expiresInHours"`
		MaxViewers     *int   `json:"maxViewers"`
	}
	if !decode(w, r, &body) {
		return
	}
	user := sessionFrom(r.Context()).User
	if !h.mayShare(user) {
		writeError(w, http.StatusForbidden, "share_links_off")
		return
	}
	id, err := accounts.ParseID(body.ItemID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "not_shareable")
		return
	}
	item, err := h.Sharing.Shareable(r.Context(), user, id)
	if shareError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	expiry := accounts.DefaultShareLinkExpiry
	if hours := body.ExpiresInHours; hours != nil {
		expiry = time.Duration(*hours) * time.Hour
	}
	kind := accounts.ShareMovie
	if item.Kind == library.KindEpisode {
		kind = accounts.ShareEpisode
	}
	link, token, err := h.Accounts.CreateShareLink(r.Context(), accounts.NewShareLink{User: user.ID, Item: item.ID, Kind: kind,
		Title: library.ShareTitle(item), Expiry: expiry, MaxViewers: valueOr(body.MaxViewers, 1)})
	if shareError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Activity.ShareLinkMade(r.Context(), link)
	var address *string
	if base := h.Accounts.Settings().PublicAddress; base != "" {
		address = new(base + sharePath + token)
	}
	writeJSON(w, http.StatusCreated, struct {
		shareLinkJSON
		Token string  `json:"token"`
		URL   *string `json:"url"`
	}{h.newShareLinkJSON(link), token, address})
}

// revokeOwnShareLink revokes a link of the user signed in.
func (h *handler) revokeOwnShareLink(w http.ResponseWriter, r *http.Request) {
	h.revokeShareLink(w, r, new(sessionFrom(r.Context()).User.ID))
}

// revokeAnyShareLink revokes any user's link, for an administrator.
func (h *handler) revokeAnyShareLink(w http.ResponseWriter, r *http.Request) {
	h.revokeShareLink(w, r, nil)
}

// revokeShareLink revokes a link, owner's only unless nil. A link that
// already expired or was revoked is answered as it is.
func (h *handler) revokeShareLink(w http.ResponseWriter, r *http.Request, owner *accounts.ID) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	link, revoked, err := h.Accounts.RevokeShareLink(r.Context(), id, owner)
	if shareError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if revoked {
		h.Activity.ShareLinkRevokedBy(r.Context(), link, sessionFrom(r.Context()).User)
	}
	writeJSON(w, http.StatusOK, h.newShareLinkJSON(link))
}

// userShareLinks lists a user's links, for an administrator.
func (h *handler) userShareLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.Accounts.User(r.Context(), id); accountError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	links, err := h.Accounts.ShareLinks(r.Context(), &id)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.shareLinksJSON(links))
}

// shareTitles searches the libraries the user signed in sees for movies
// and series to share, up to maxShareResults: a series is shared one
// episode at a time (see shareEpisodes). Parental control, blocked genres
// and the titles they hid apply.
func (h *handler) shareTitles(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	if !h.mayShare(user) {
		writeError(w, http.StatusForbidden, "share_links_off")
		return
	}
	term := strings.TrimSpace(r.URL.Query().Get("search"))
	if term == "" {
		writeJSON(w, http.StatusOK, []*liveItemJSON{})
		return
	}
	items, err := h.Sharing.Search(r.Context(), user, term, []library.Kind{library.KindMovie, library.KindSeries}, maxShareResults)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	hidden, err := h.Sharing.HiddenTitles(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items = hidden.Without(items)
	result := make([]*liveItemJSON, 0, len(items))
	for _, item := range items[:min(len(items), maxShareResults)] {
		result = append(result, newLiveItemJSON(item))
	}
	writeJSON(w, http.StatusOK, result)
}

// shareTitle describes a title the user signed in may share, as the web
// client's Share opens the form with it.
func (h *handler) shareTitle(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	if !h.mayShare(user) {
		writeError(w, http.StatusForbidden, "share_links_off")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := h.Sharing.Shareable(r.Context(), user, id)
	if shareError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newLiveItemJSON(item))
}

// shareEpisodes lists the episodes of a series the user signed in may
// share, in order.
func (h *handler) shareEpisodes(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	if !h.mayShare(user) {
		writeError(w, http.StatusForbidden, "share_links_off")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	episodes, err := h.Sharing.Episodes(r.Context(), user, id, nil)
	if shareError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	hidden, err := h.Sharing.HiddenTitles(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	episodes = slices.DeleteFunc(hidden.Without(episodes), func(item library.Item) bool { return item.Kind != library.KindEpisode })
	result := make([]*liveItemJSON, 0, len(episodes))
	for _, episode := range episodes {
		result = append(result, newLiveItemJSON(episode))
	}
	writeJSON(w, http.StatusOK, result)
}
