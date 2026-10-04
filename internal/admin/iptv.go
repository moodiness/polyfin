package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// sourceRequest adds or changes an IPTV source: its name; its account,
// a playlist address (M3U) or a server address, username and password
// (Xtream Codes); on adding, the XMLTV guide of its catalog, or the one
// the Xtream server publishes for the account with ProviderGuide.
type sourceRequest struct {
	Name          *string `json:"name"`
	Kind          string  `json:"kind"`
	URL           string  `json:"url"`
	Server        string  `json:"server"`
	Username      string  `json:"username"`
	Password      string  `json:"password"`
	GuideURL      string  `json:"guideUrl"`
	ProviderGuide bool    `json:"providerGuide"`
	// Groups, on changing a source, are the groups shown; null shows them
	// all, those added later included. Left out, they do not change.
	Groups json.RawMessage `json:"groups"`
}

func (body sourceRequest) account(kind string) iptv.Account {
	return iptv.Account{Kind: kind, URL: body.URL, Server: body.Server, Username: body.Username, Password: body.Password}
}

// addSource adds an IPTV source to the scope, as installing an addon does,
// once its channel list could be read, then fetches its guide.
func (h *handler) addSource(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body sourceRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Kind != addons.KindM3U && body.Kind != addons.KindXtream {
		writeError(w, http.StatusBadRequest, "invalid_source_address")
		return
	}
	account := body.account(body.Kind)
	guide := body.GuideURL
	if body.ProviderGuide && body.Kind == addons.KindXtream {
		guide = iptv.ProviderGuide(account)
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	// The list is worth keeping when the admin app stops waiting.
	ctx := context.WithoutCancel(r.Context())
	addon, err := h.IPTV.Add(ctx, scope, iptv.NewSource{Name: name, Account: account, Guide: guide}, confined(r))
	if err == nil {
		h.Activity.AddonInstalled(ctx, sessionFrom(ctx).User, addon.Manifest.Name, addon.Manifest.Version, scope.Owner == nil)
		if guide != "" {
			if err := h.fetchGuide(r, scope, addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: addon.Manifest.Catalogs[0].ID}); err != nil {
				h.internalError(w, r, err)
				return
			}
		}
	}
	h.answerAddon(w, r, scope, http.StatusCreated, addon, err)
}

// updateSource changes an IPTV source of the scope: its name, its account,
// fetched first, or the groups it shows. A password left empty keeps the
// current one.
func (h *handler) updateSource(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body sourceRequest
	if !decode(w, r, &body) {
		return
	}
	changes := iptv.Changes{Name: body.Name}
	if body.URL != "" || body.Server != "" || body.Username != "" || body.Password != "" {
		account := body.account("")
		changes.Account = &account
	}
	if len(body.Groups) > 0 {
		if string(body.Groups) == "null" {
			changes.AllGroups = true
		} else if err := json.Unmarshal(body.Groups, &changes.Groups); err != nil || changes.Groups == nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	err := h.IPTV.Update(context.WithoutCancel(r.Context()), scope, id, changes, confined(r))
	addon, findErr := h.Addons.Find(r.Context(), id)
	if err == nil {
		err = findErr
	}
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}
