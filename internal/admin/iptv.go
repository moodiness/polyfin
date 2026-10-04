package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// sourceRequest adds or changes an IPTV source: its name; its account,
// a playlist address (M3U) or a server address, username and password
// (Xtream Codes); on adding, the XMLTV guide of its catalog, or the one
// the Xtream server publishes for the account with ProviderGuide; its
// import options, fields left out keeping their values.
type sourceRequest struct {
	Name          *string            `json:"name"`
	Kind          string             `json:"kind"`
	URL           string             `json:"url"`
	Server        string             `json:"server"`
	Username      string             `json:"username"`
	Password      string             `json:"password"`
	GuideURL      string             `json:"guideUrl"`
	ProviderGuide bool               `json:"providerGuide"`
	Options       *iptv.OptionsPatch `json:"options"`
	// Groups, from v0.8, gave the groups shown: categories replaced them,
	// and a request that still sends them is refused.
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
	if body.Groups != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
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
	addon, err := h.IPTV.Add(ctx, scope, iptv.NewSource{Name: name, Account: account, Guide: guide, Options: body.Options}, confined(r))
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
// fetched first, or its import options. A password left empty keeps the
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
	if body.Groups != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	changes := iptv.Changes{Name: body.Name, Options: body.Options}
	if body.URL != "" || body.Server != "" || body.Username != "" || body.Password != "" {
		account := body.account("")
		changes.Account = &account
	}
	err := h.IPTV.Update(context.WithoutCancel(r.Context()), scope, id, changes, confined(r))
	addon, findErr := h.Addons.Find(r.Context(), id)
	if err == nil {
		err = findErr
	}
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}

type previewJSON struct {
	Total      int                   `json:"total"`
	Categories []previewCategoryJSON `json:"categories"`
}

type previewCategoryJSON struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Channels int    `json:"channels"`
	Excluded bool   `json:"excluded"`
}

func validPreview(w http.ResponseWriter, by string) bool {
	if by != iptv.PreviewGroups && by != iptv.PreviewCountries {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func (h *handler) answerPreview(w http.ResponseWriter, r *http.Request, total int, categories []iptv.PreviewCategory, err error) {
	if h.answerFailure(w, r, err) {
		return
	}
	result := previewJSON{Total: total, Categories: make([]previewCategoryJSON, 0, len(categories))}
	for _, c := range categories {
		result.Categories = append(result.Categories, previewCategoryJSON(c))
	}
	writeJSON(w, http.StatusOK, result)
}

// previewAccount groups a new account's list by group or by country,
// downloading it and storing nothing.
func (h *handler) previewAccount(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	var body struct {
		sourceRequest
		By string `json:"by"`
		Q  string `json:"q"`
	}
	if !decode(w, r, &body) || !validPreview(w, body.By) {
		return
	}
	if body.Kind != addons.KindM3U && body.Kind != addons.KindXtream {
		writeError(w, http.StatusBadRequest, "invalid_source_address")
		return
	}
	total, categories, err := h.IPTV.PreviewAccount(context.WithoutCancel(r.Context()), body.account(body.Kind), body.By, body.Q, confined(r))
	h.answerPreview(w, r, total, categories, err)
}

// previewSource groups an IPTV source's stored list by group or by
// country.
func (h *handler) previewSource(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, false)
	if !ok {
		return
	}
	by := r.URL.Query().Get("by")
	if !validPreview(w, by) {
		return
	}
	total, categories, err := h.IPTV.Preview(r.Context(), scope, source, by, r.URL.Query().Get("q"))
	h.answerPreview(w, r, total, categories, err)
}

// lineupTarget resolves the scope and the {id} source of a line-up route;
// write refuses a user's own scope while their addons are turned off.
func (h *handler) lineupTarget(w http.ResponseWriter, r *http.Request, write bool) (addons.Scope, accounts.ID, bool) {
	scope, ok := h.scope(w, r)
	if !ok || write && h.personalAddonsRefused(w, r, scope) {
		return addons.Scope{}, accounts.ID{}, false
	}
	source, ok := pathID(w, r, "id")
	return scope, source, ok
}

// answerFailure answers a known error, or an internal one, and reports
// whether there was one.
func (h *handler) answerFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if !addonError(w, err) {
		h.internalError(w, r, err)
	}
	return true
}

type categoryJSON struct {
	ID              string `json:"id"`
	Key             string `json:"key"`
	Name            string `json:"name"`
	ProviderName    string `json:"providerName"`
	Custom          bool   `json:"custom"`
	Enabled         bool   `json:"enabled"`
	Position        int    `json:"position"`
	Channels        int    `json:"channels"`
	EnabledChannels int    `json:"enabledChannels"`
}

func newCategoryJSON(c iptv.Category) categoryJSON {
	return categoryJSON{ID: c.ID.String(), Key: c.Key, Name: c.Name, ProviderName: c.ProviderName, Custom: c.Custom, Enabled: c.Enabled,
		Position: c.Position, Channels: c.Channels, EnabledChannels: c.EnabledChannels}
}

func (h *handler) answerCategory(w http.ResponseWriter, r *http.Request, status int, c iptv.Category, err error) {
	if !h.answerFailure(w, r, err) {
		writeJSON(w, status, newCategoryJSON(c))
	}
}

func (h *handler) listCategories(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, false)
	if !ok {
		return
	}
	categories, err := h.IPTV.Categories(r.Context(), scope, source, r.URL.Query().Get("q"))
	if h.answerFailure(w, r, err) {
		return
	}
	items := make([]categoryJSON, 0, len(categories))
	for _, c := range categories {
		items = append(items, newCategoryJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) createCategory(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := h.IPTV.CreateCategory(r.Context(), scope, source, body.Name)
	h.answerCategory(w, r, http.StatusCreated, c, err)
}

// nullable reads a field a change may set to a value or to null; a field
// left out (nil raw) changes nothing.
func nullable[T any](raw json.RawMessage) (*iptv.Nullable[T], bool) {
	if raw == nil {
		return nil, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return &iptv.Nullable[T]{}, true
	}
	value := new(T)
	if err := json.Unmarshal(raw, value); err != nil {
		return nil, false
	}
	return &iptv.Nullable[T]{Value: value}, true
}

func (h *handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var body struct {
		Name    json.RawMessage `json:"name"`
		Enabled *bool           `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, ok := nullable[string](body.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_category_name")
		return
	}
	c, err := h.IPTV.UpdateCategory(r.Context(), scope, source, id, iptv.CategoryChanges{Name: name, Enabled: body.Enabled})
	h.answerCategory(w, r, http.StatusOK, c, err)
}

func (h *handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	if !h.answerFailure(w, r, h.IPTV.DeleteCategory(r.Context(), scope, source, id)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// idList reads ids of a request; one that is not an id refuses it with
// code.
func idList(w http.ResponseWriter, raw []string, code string) ([]accounts.ID, bool) {
	if raw == nil {
		return nil, true
	}
	ids := make([]accounts.ID, 0, len(raw))
	for _, value := range raw {
		id, err := accounts.ParseID(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, code)
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func (h *handler) orderCategories(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	ids, ok := idList(w, body.IDs, "invalid_order")
	if !ok {
		return
	}
	if !h.answerFailure(w, r, h.IPTV.OrderCategories(r.Context(), scope, source, ids)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) bulkCategories(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Enabled *bool    `json:"enabled"`
		IDs     []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_bulk")
		return
	}
	ids, ok := idList(w, body.IDs, "invalid_bulk")
	if !ok {
		return
	}
	changed, err := h.IPTV.BulkCategories(r.Context(), scope, source, *body.Enabled, ids)
	if !h.answerFailure(w, r, err) {
		writeJSON(w, http.StatusOK, map[string]int{"changed": changed})
	}
}

type channelJSON struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	ProviderName       string              `json:"providerName"`
	Renamed            bool                `json:"renamed"`
	Logo               string              `json:"logo"`
	ProviderLogo       string              `json:"providerLogo"`
	Description        string              `json:"description"`
	Category           channelCategoryJSON `json:"category"`
	ProviderCategoryID string              `json:"providerCategoryId"`
	Moved              bool                `json:"moved"`
	Enabled            bool                `json:"enabled"`
	Shown              bool                `json:"shown"`
	Number             *int                `json:"number"`
	ProviderNumber     *int                `json:"providerNumber"`
	FixedNumber        *int                `json:"fixedNumber"`
	GuideID            string              `json:"guideId"`
	Mapping            *mappingJSON        `json:"mapping"`
	Streams            []lineupStreamJSON  `json:"streams"`
}

type channelCategoryJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// mappingJSON is the guide channel a channel takes; GuideID and
// GuideChannelID are null for a manual "no guide".
type mappingJSON struct {
	GuideID          *string `json:"guideId"`
	GuideChannelID   *string `json:"guideChannelId"`
	GuideChannelName string  `json:"guideChannelName"`
	Manual           bool    `json:"manual"`
}

type lineupStreamJSON struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Enabled bool    `json:"enabled"`
	Custom  bool    `json:"custom"`
	Address *string `json:"address"`
}

func newMappingJSON(guide *accounts.ID, guideChannel *string, name string, manual bool) *mappingJSON {
	result := &mappingJSON{GuideChannelID: guideChannel, GuideChannelName: name, Manual: manual}
	if guide != nil {
		result.GuideID = new(guide.String())
	}
	return result
}

func newChannelJSON(c iptv.Channel) channelJSON {
	result := channelJSON{ID: c.ID.String(), Name: c.Name, ProviderName: c.ProviderName, Renamed: c.Renamed, Logo: c.Logo,
		ProviderLogo: c.ProviderLogo, Description: c.Description, Category: channelCategoryJSON{ID: c.Category.String(), Name: c.CategoryName},
		ProviderCategoryID: c.ProviderCategory.String(), Moved: c.Moved, Enabled: c.Enabled, Shown: c.Shown, Number: c.Number,
		ProviderNumber: c.ProviderNumber, FixedNumber: c.FixedNumber, GuideID: c.GuideID, Streams: make([]lineupStreamJSON, 0, len(c.Streams))}
	if m := c.Mapping; m != nil {
		result.Mapping = newMappingJSON(m.Guide, m.GuideChannel, m.GuideChannelName, m.Manual)
	}
	for _, s := range c.Streams {
		stream := lineupStreamJSON{ID: s.ID, Label: s.Label, Enabled: s.Enabled, Custom: s.Custom}
		if s.Custom {
			stream.Address = new(s.Address)
		}
		result.Streams = append(result.Streams, stream)
	}
	return result
}

func (h *handler) answerChannel(w http.ResponseWriter, r *http.Request, status int, c iptv.Channel, err error) {
	if !h.answerFailure(w, r, err) {
		writeJSON(w, status, newChannelJSON(c))
	}
}

// pageJSON is a page of a list: total counts every item matching.
type pageJSON[T any] struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Items  []T `json:"items"`
}

// Paged lists answer defaultPage items, at most maxPage.
const (
	defaultPage = 100
	maxPage     = 500
)

// paging reads a paged list's offset and limit.
func paging(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	offset, limit := 0, defaultPage
	for _, p := range []struct {
		name  string
		into  *int
		least int
	}{{"offset", &offset, 0}, {"limit", &limit, 1}} {
		if value := r.URL.Query().Get(p.name); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < p.least {
				writeError(w, http.StatusBadRequest, "invalid_request")
				return 0, 0, false
			}
			*p.into = n
		}
	}
	return offset, min(limit, maxPage), true
}

// queryBool reads an optional true or false query parameter.
func queryBool(w http.ResponseWriter, r *http.Request, name string) (*bool, bool) {
	switch r.URL.Query().Get(name) {
	case "":
		return nil, true
	case "true":
		return new(true), true
	case "false":
		return new(false), true
	}
	writeError(w, http.StatusBadRequest, "invalid_request")
	return nil, false
}

func (h *handler) listChannels(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, false)
	if !ok {
		return
	}
	offset, limit, ok := paging(w, r)
	if !ok {
		return
	}
	filter := iptv.ChannelFilter{Q: r.URL.Query().Get("q"), Offset: offset, Limit: limit}
	if value := r.URL.Query().Get("category"); value != "" {
		id, err := accounts.ParseID(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_category")
			return
		}
		filter.Category = &id
	}
	for name, into := range map[string]**bool{"enabled": &filter.Enabled, "shown": &filter.Shown, "mapped": &filter.Mapped} {
		if *into, ok = queryBool(w, r, name); !ok {
			return
		}
	}
	total, channels, err := h.IPTV.ListChannels(r.Context(), scope, source, filter)
	if h.answerFailure(w, r, err) {
		return
	}
	page := pageJSON[channelJSON]{Total: total, Offset: offset, Limit: limit, Items: make([]channelJSON, 0, len(channels))}
	for _, c := range channels {
		page.Items = append(page.Items, newChannelJSON(c))
	}
	writeJSON(w, http.StatusOK, page)
}

// channelTarget resolves a channel route's scope, source and channel.
func (h *handler) channelTarget(w http.ResponseWriter, r *http.Request, write bool) (addons.Scope, accounts.ID, accounts.ID, bool) {
	scope, source, ok := h.lineupTarget(w, r, write)
	if !ok {
		return addons.Scope{}, accounts.ID{}, accounts.ID{}, false
	}
	channel, ok := pathID(w, r, "chid")
	return scope, source, channel, ok
}

func (h *handler) getChannel(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, false)
	if !ok {
		return
	}
	c, err := h.IPTV.Channel(r.Context(), scope, source, channel)
	h.answerChannel(w, r, http.StatusOK, c, err)
}

func (h *handler) updateChannel(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Enabled     *bool           `json:"enabled"`
		Name        json.RawMessage `json:"name"`
		Logo        json.RawMessage `json:"logo"`
		Description *string         `json:"description"`
		Category    json.RawMessage `json:"category"`
		Number      json.RawMessage `json:"number"`
	}
	if !decode(w, r, &body) {
		return
	}
	changes := iptv.ChannelChanges{Enabled: body.Enabled, Description: body.Description}
	if changes.Name, ok = nullable[string](body.Name); !ok {
		writeError(w, http.StatusBadRequest, "invalid_channel_name")
		return
	}
	if changes.Logo, ok = nullable[string](body.Logo); !ok {
		writeError(w, http.StatusBadRequest, "invalid_logo")
		return
	}
	if changes.Number, ok = nullable[int](body.Number); !ok {
		writeError(w, http.StatusBadRequest, "invalid_number")
		return
	}
	category, ok := nullable[string](body.Category)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_category")
		return
	}
	if category != nil {
		changes.Category = &iptv.Nullable[accounts.ID]{}
		if category.Value != nil {
			id, err := accounts.ParseID(*category.Value)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_category")
				return
			}
			changes.Category.Value = &id
		}
	}
	c, err := h.IPTV.UpdateChannel(r.Context(), scope, source, channel, changes)
	h.answerChannel(w, r, http.StatusOK, c, err)
}

func (h *handler) moveChannel(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Before *string `json:"before"`
	}
	if !decode(w, r, &body) {
		return
	}
	var before *accounts.ID
	if body.Before != nil {
		id, err := accounts.ParseID(*body.Before)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_move")
			return
		}
		before = &id
	}
	if !h.answerFailure(w, r, h.IPTV.MoveChannel(r.Context(), scope, source, channel, before)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) bulkChannels(w http.ResponseWriter, r *http.Request) {
	scope, source, ok := h.lineupTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Enabled    *bool    `json:"enabled"`
		DryRun     bool     `json:"dryRun"`
		IDs        []string `json:"ids"`
		Category   *string  `json:"category"`
		Q          string   `json:"q"`
		InCategory *string  `json:"inCategory"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_bulk")
		return
	}
	bulk := iptv.Bulk{Enabled: *body.Enabled, DryRun: body.DryRun, Q: body.Q}
	if bulk.IDs, ok = idList(w, body.IDs, "invalid_bulk"); !ok {
		return
	}
	for _, p := range []struct {
		raw  *string
		into **accounts.ID
	}{{body.Category, &bulk.Category}, {body.InCategory, &bulk.InCategory}} {
		if p.raw != nil {
			id, err := accounts.ParseID(*p.raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_bulk")
				return
			}
			*p.into = &id
		}
	}
	matched, changed, err := h.IPTV.BulkChannels(r.Context(), scope, source, bulk)
	if !h.answerFailure(w, r, err) {
		writeJSON(w, http.StatusOK, map[string]int{"matched": matched, "changed": changed})
	}
}

func (h *handler) setStreams(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Streams []struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		} `json:"streams"`
	}
	if !decode(w, r, &body) {
		return
	}
	settings := make([]iptv.StreamSetting, 0, len(body.Streams))
	for _, s := range body.Streams {
		if s.Enabled == nil {
			writeError(w, http.StatusBadRequest, "invalid_streams")
			return
		}
		settings = append(settings, iptv.StreamSetting{ID: s.ID, Enabled: *s.Enabled})
	}
	c, err := h.IPTV.SetStreams(r.Context(), scope, source, channel, settings)
	h.answerChannel(w, r, http.StatusOK, c, err)
}

func (h *handler) addStream(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, true)
	if !ok {
		return
	}
	var body struct {
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := h.IPTV.AddStream(r.Context(), scope, source, channel, body.URL, body.Label, confined(r))
	h.answerChannel(w, r, http.StatusCreated, c, err)
}

func (h *handler) deleteStream(w http.ResponseWriter, r *http.Request) {
	scope, source, channel, ok := h.channelTarget(w, r, true)
	if !ok {
		return
	}
	c, err := h.IPTV.DeleteStream(r.Context(), scope, source, channel, r.PathValue("sid"))
	h.answerChannel(w, r, http.StatusOK, c, err)
}
