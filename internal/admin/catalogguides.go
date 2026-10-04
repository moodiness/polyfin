package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
)

// Guides are what the guide routes ask the library: downloading the
// XMLTV guides of live TV catalogs and mapping their channels (see
// library.Service).
type Guides interface {
	RefreshGuide(ctx context.Context, scope addons.Scope, key addons.LibraryKey) error
	CatalogGuides(ctx context.Context, scope addons.Scope, key addons.LibraryKey) (library.CatalogGuides, error)
	SetCatalogGuides(ctx context.Context, scope addons.Scope, key addons.LibraryKey, list []addons.GuideAddress) (library.CatalogGuides, error)
	Automap(ctx context.Context, scope addons.Scope, key addons.LibraryKey, mode string) (library.AutomapResult, error)
	GuideChannels(ctx context.Context, scope addons.Scope, key addons.LibraryKey, q string, guide *accounts.ID, offset, limit int) (int, []library.GuideChannel, error)
	Mappings(ctx context.Context, scope addons.Scope, key addons.LibraryKey, state, q string, offset, limit int) (int, []library.ChannelMapping, error)
	SetMapping(ctx context.Context, scope addons.Scope, key addons.LibraryKey, channel accounts.ID, guide *accounts.ID, xmltvID *string) (library.ChannelMapping, error)
	ClearMapping(ctx context.Context, scope addons.Scope, key addons.LibraryKey, channel accounts.ID) (library.ChannelMapping, error)
}

type catalogGuidesJSON struct {
	Guides   []guideJSON `json:"guides"`
	Channels int         `json:"channels"`
	Mapped   int         `json:"mapped"`
	Manual   int         `json:"manual"`
}

func (h *handler) answerCatalogGuides(w http.ResponseWriter, r *http.Request, guides library.CatalogGuides, err error) {
	if h.answerFailure(w, r, err) {
		return
	}
	result := catalogGuidesJSON{Guides: newGuidesJSON(guides.Guides, h.Accounts.Settings().LiveTvRefreshHours), Channels: guides.Channels,
		Mapped: guides.Mapped, Manual: guides.Manual}
	if result.Guides == nil {
		result.Guides = []guideJSON{}
	}
	writeJSON(w, http.StatusOK, result)
}

// catalogQuery reads the catalog a guide read is for, from the query.
func catalogQuery(w http.ResponseWriter, r *http.Request) (addons.LibraryKey, bool) {
	query := r.URL.Query()
	return guideTarget(w, guideRequest{AddonID: query.Get("addonId"), CatalogType: query.Get("catalogType"), CatalogID: query.Get("catalogId")})
}

func (h *handler) catalogGuides(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	key, ok := catalogQuery(w, r)
	if !ok {
		return
	}
	guides, err := h.Guides.CatalogGuides(r.Context(), scope, key)
	h.answerCatalogGuides(w, r, guides, err)
}

// guideAddress is an entry of a guide list: a new address, or a guide of
// the list by its id.
type guideAddress addons.GuideAddress

func (g *guideAddress) UnmarshalJSON(data []byte) error {
	var address string
	if err := json.Unmarshal(data, &address); err == nil {
		*g = guideAddress{URL: address}
		return nil
	}
	var kept struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &kept); err != nil {
		return err
	}
	id, err := accounts.ParseID(kept.ID)
	if err != nil {
		return addons.ErrInvalidGuide
	}
	*g = guideAddress{ID: &id}
	return nil
}

// saveCatalogGuides replaces a live TV catalog's guides, downloading the
// new ones, to the end even when the admin app leaves.
func (h *handler) saveCatalogGuides(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body struct {
		guideRequest
		URLs []json.RawMessage `json:"urls"`
	}
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body.guideRequest)
	if !ok {
		return
	}
	if body.URLs == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	list := make([]addons.GuideAddress, 0, len(body.URLs))
	for _, raw := range body.URLs {
		var entry guideAddress
		if err := json.Unmarshal(raw, &entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_guide")
			return
		}
		list = append(list, addons.GuideAddress(entry))
	}
	guides, err := h.Guides.SetCatalogGuides(context.WithoutCancel(r.Context()), scope, key, list)
	h.answerCatalogGuides(w, r, guides, err)
}

// refreshCatalogGuides downloads every guide of a live TV catalog now.
func (h *handler) refreshCatalogGuides(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body guideRequest
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body)
	if !ok {
		return
	}
	if err := h.fetchGuide(r, scope, key); h.answerFailure(w, r, err) {
		return
	}
	guides, err := h.Guides.CatalogGuides(r.Context(), scope, key)
	h.answerCatalogGuides(w, r, guides, err)
}

func (h *handler) automapCatalog(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body struct {
		guideRequest
		Mode string `json:"mode"`
	}
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body.guideRequest)
	if !ok {
		return
	}
	result, err := h.Guides.Automap(context.WithoutCancel(r.Context()), scope, key, body.Mode)
	if !h.answerFailure(w, r, err) {
		writeJSON(w, http.StatusOK, map[string]int{"channels": result.Channels, "mapped": result.Mapped, "changed": result.Changed})
	}
}

type guideChannelJSON struct {
	GuideID       string            `json:"guideId"`
	GuidePosition int               `json:"guidePosition"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Names         []string          `json:"names"`
	Icon          string            `json:"icon"`
	Now           *guideProgramJSON `json:"now"`
}

type guideProgramJSON struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (h *handler) guideChannels(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	key, ok := catalogQuery(w, r)
	if !ok {
		return
	}
	offset, limit, ok := paging(w, r)
	if !ok {
		return
	}
	var guide *accounts.ID
	if value := r.URL.Query().Get("guide"); value != "" {
		id, err := accounts.ParseID(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_guide")
			return
		}
		guide = &id
	}
	total, channels, err := h.Guides.GuideChannels(r.Context(), scope, key, r.URL.Query().Get("q"), guide, offset, limit)
	if h.answerFailure(w, r, err) {
		return
	}
	page := pageJSON[guideChannelJSON]{Total: total, Offset: offset, Limit: limit, Items: make([]guideChannelJSON, 0, len(channels))}
	for _, c := range channels {
		item := guideChannelJSON{GuideID: c.Guide.String(), GuidePosition: c.GuidePosition, ID: c.ID, Names: c.Names, Icon: c.Icon}
		if len(c.Names) > 0 {
			item.Name = c.Names[0]
		} else {
			item.Name = c.ID
		}
		if c.Now != nil {
			item.Now = &guideProgramJSON{Title: c.Now.Title, Start: c.Now.Start, End: c.Now.End}
		}
		page.Items = append(page.Items, item)
	}
	writeJSON(w, http.StatusOK, page)
}

type channelMappingJSON struct {
	ChannelID string       `json:"channelId"`
	Name      string       `json:"name"`
	Number    int          `json:"number"`
	GuideID   string       `json:"guideId"`
	Mapping   *mappingJSON `json:"mapping"`
}

func newChannelMappingJSON(m library.ChannelMapping) channelMappingJSON {
	result := channelMappingJSON{ChannelID: m.Channel.String(), Name: m.Name, Number: m.Number, GuideID: m.GuideID}
	if m.Mapping != nil {
		result.Mapping = newMappingJSON(m.Mapping.Guide, m.Mapping.GuideChannel, m.Mapping.Name, m.Mapping.Manual)
	}
	return result
}

func (h *handler) answerMapping(w http.ResponseWriter, r *http.Request, m library.ChannelMapping, err error) {
	if !h.answerFailure(w, r, err) {
		writeJSON(w, http.StatusOK, newChannelMappingJSON(m))
	}
}

func (h *handler) catalogMappings(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	key, ok := catalogQuery(w, r)
	if !ok {
		return
	}
	offset, limit, ok := paging(w, r)
	if !ok {
		return
	}
	total, mappings, err := h.Guides.Mappings(r.Context(), scope, key, r.URL.Query().Get("state"), r.URL.Query().Get("q"), offset, limit)
	if h.answerFailure(w, r, err) {
		return
	}
	page := pageJSON[channelMappingJSON]{Total: total, Offset: offset, Limit: limit, Items: make([]channelMappingJSON, 0, len(mappings))}
	for _, m := range mappings {
		page.Items = append(page.Items, newChannelMappingJSON(m))
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *handler) setMapping(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body struct {
		guideRequest
		ChannelID      string  `json:"channelId"`
		GuideID        *string `json:"guideId"`
		GuideChannelID *string `json:"guideChannelId"`
	}
	if !decode(w, r, &body) {
		return
	}
	key, ok := guideTarget(w, body.guideRequest)
	if !ok {
		return
	}
	channel, err := accounts.ParseID(body.ChannelID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var guide *accounts.ID
	if body.GuideID != nil {
		id, err := accounts.ParseID(*body.GuideID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_mapping")
			return
		}
		guide = &id
	}
	m, err := h.Guides.SetMapping(r.Context(), scope, key, channel, guide, body.GuideChannelID)
	h.answerMapping(w, r, m, err)
}

func (h *handler) clearMapping(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	key, ok := catalogQuery(w, r)
	if !ok {
		return
	}
	channel, err := accounts.ParseID(r.URL.Query().Get("channelId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	m, err := h.Guides.ClearMapping(context.WithoutCancel(r.Context()), scope, key, channel)
	h.answerMapping(w, r, m, err)
}
