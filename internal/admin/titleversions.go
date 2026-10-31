package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// The Versions page (Content › Versions) explains, for a movie or an
// episode, every stream each addon listed and what became of it, as a user
// is offered them: the administrator signed in, or another user the page
// names. It pins a version for every user, blocks files, analyzes a version
// now and asks the addons again.

// Versions explains titles' versions and acts on them (see
// jellyfin.Handler.ExplainVersions).
type Versions interface {
	ExplainVersions(ctx context.Context, user accounts.User, id accounts.ID) (jellyfin.VersionsExplanation, error)
	PinVersion(ctx context.Context, by, user accounts.User, item, id accounts.ID) error
	UnpinVersion(ctx context.Context, item accounts.ID) error
	BlockVersion(ctx context.Context, by, user accounts.User, item, id accounts.ID, everywhere bool, reason string) (library.VersionBlock, error)
	UnblockVersion(ctx context.Context, id accounts.ID) error
	VersionBlocks(ctx context.Context) []library.VersionBlock
	AnalyzeVersion(ctx context.Context, user accounts.User, item, id accounts.ID) (string, error)
	AskVersionsAgain(ctx context.Context, by, user accounts.User, item accounts.ID) (time.Duration, error)
}

// Titles finds the movies, series and episodes the Versions page explains.
type Titles interface {
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
	Search(ctx context.Context, user accounts.User, term string, kinds []library.Kind, limit int) ([]library.Item, error)
	Episodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]library.Item, error)
}

// maxTitleResults bounds the titles a search of the Versions page lists.
const maxTitleResults = 30

// versionsUser is the user the Versions page explains versions for: the
// one ?user names, else the administrator signed in. It answers the error
// otherwise.
func (h *handler) versionsUser(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	text := r.URL.Query().Get("user")
	if text == "" {
		return sessionFrom(r.Context()).User, true
	}
	id, err := accounts.ParseID(text)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user")
		return accounts.User{}, false
	}
	user, err := h.Accounts.User(r.Context(), id)
	if errors.Is(err, accounts.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_user")
		return accounts.User{}, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return accounts.User{}, false
	}
	return user, true
}

// titleError answers a title or version the user cannot reach as not
// found, and reports whether err was one.
func titleError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return true
	}
	return false
}

// versionTitles searches the movies and series the user sees, as the web
// player's search finds them.
func (h *handler) versionTitles(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	term := strings.TrimSpace(r.URL.Query().Get("search"))
	if term == "" {
		writeJSON(w, http.StatusOK, []*liveItemJSON{})
		return
	}
	items, err := h.Titles.Search(r.Context(), user, term, []library.Kind{library.KindMovie, library.KindSeries}, maxTitleResults)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]*liveItemJSON, 0, len(items))
	for _, item := range items[:min(len(items), maxTitleResults)] {
		result = append(result, newLiveItemJSON(item))
	}
	writeJSON(w, http.StatusOK, result)
}

// versionTitle describes a title, as the user sees it.
func (h *handler) versionTitle(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := h.Titles.Item(r.Context(), user, id)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newLiveItemJSON(item))
}

// versionEpisodes lists the episodes of a series the user sees, in order.
func (h *handler) versionEpisodes(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	episodes, err := h.Titles.Episodes(r.Context(), user, id, nil)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]*liveItemJSON, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Kind == library.KindEpisode {
			result = append(result, newLiveItemJSON(episode))
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// versionsJSON is a title's versions explained (see
// jellyfin.VersionsExplanation).
type versionsJSON struct {
	Sources   []versionSourceExplainedJSON `json:"sources"`
	Pending   int                          `json:"pending"`
	Criteria  []string                     `json:"criteria"`
	Group     int                          `json:"qualityGroup"`
	KeptAll   bool                         `json:"keptAll"`
	Closest   bool                         `json:"closest"`
	Pin       *versionPinJSON              `json:"pin"`
	Blocks    []versionBlockJSON           `json:"blocks"`
	Decisions []playDecisionJSON           `json:"decisions"`
}

type versionSourceExplainedJSON struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Embedded bool                `json:"embedded"`
	State    string              `json:"state"`
	Streams  []versionStreamJSON `json:"streams"`
}

// versionStreamJSON is a stream an addon listed and what became of it;
// version is empty for a stream with nothing to play.
type versionStreamJSON struct {
	Name      string            `json:"name"`
	Filename  string            `json:"filename"`
	Size      int64             `json:"size"`
	Torrent   bool              `json:"torrent"`
	Version   string            `json:"version"`
	Step      string            `json:"step"`
	Rule      string            `json:"rule"`
	Detail    string            `json:"detail"`
	Place     int               `json:"place"`
	Pinned    bool              `json:"pinned"`
	Unstarted bool              `json:"unstarted"`
	Block     *versionBlockJSON `json:"block"`
	Labels    versionLabelsJSON `json:"labels"`
	Facts     versionFactsJSON  `json:"facts"`
	Analyzed  bool              `json:"analyzed"`
}

// versionLabelsJSON is what the stream's labels tell: its height, video
// codec, range ("sdr", "hdr" or empty), audio languages, whether they say
// several, the kinds of low-quality release they tag, the other title
// they name, its source tier, audio format and edition.
type versionLabelsJSON struct {
	Height    int              `json:"height"`
	Codec     string           `json:"codec"`
	Range     string           `json:"range"`
	Languages []string         `json:"languages"`
	Multi     bool             `json:"multi"`
	Releases  []string         `json:"releases"`
	Elsewhere string           `json:"elsewhere"`
	Tier      string           `json:"tier"`
	Audio     versionAudioJSON `json:"audio"`
	Edition   string           `json:"edition"`
}

// versionFactsJSON is what the version rules know of the version, with
// its size in bytes a minute of runtime, its score under the label rules
// and the text of those matching it.
type versionFactsJSON struct {
	Height     int              `json:"height"`
	Size       int64            `json:"size"`
	Codec      string           `json:"codec"`
	Range      string           `json:"range"`
	Languages  []string         `json:"languages"`
	Multi      bool             `json:"multi"`
	Tier       string           `json:"tier"`
	Audio      versionAudioJSON `json:"audio"`
	Edition    string           `json:"edition"`
	PerMinute  int64            `json:"perMinute"`
	Score      int              `json:"score"`
	LabelRules []string         `json:"labelRules"`
}

// versionAudioJSON is an audio format: lossless, object based, and its
// channels, 0 when unknown.
type versionAudioJSON struct {
	Lossless bool `json:"lossless"`
	Object   bool `json:"object"`
	Channels int  `json:"channels"`
}

func newVersionAudioJSON(audio library.AudioFormat) versionAudioJSON {
	return versionAudioJSON{Lossless: audio.Lossless, Object: audio.Object, Channels: audio.Channels}
}

type versionPinJSON struct {
	Version  string    `json:"version"`
	Name     string    `json:"name"`
	Filename string    `json:"filename"`
	Size     int64     `json:"size"`
	By       string    `json:"by"`
	At       time.Time `json:"at"`
	Missing  bool      `json:"missing"`
}

type versionBlockJSON struct {
	ID         string    `json:"id"`
	Item       string    `json:"item"`
	Everywhere bool      `json:"everywhere"`
	Name       string    `json:"name"`
	Filename   string    `json:"filename"`
	Size       int64     `json:"size"`
	Reason     string    `json:"reason"`
	By         string    `json:"by"`
	At         time.Time `json:"at"`
}

type playDecisionJSON struct {
	At       time.Time     `json:"at"`
	User     string        `json:"user"`
	Device   string        `json:"device"`
	App      string        `json:"app"`
	Picked   bool          `json:"picked"`
	Tries    []playTryJSON `json:"tries"`
	Chosen   string        `json:"chosen"`
	Delivery string        `json:"delivery"`
}

type playTryJSON struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

// rangeName names a dynamic range as the admin API writes it.
func rangeName(r library.DynamicRange) string {
	switch r {
	case library.StandardRange:
		return "sdr"
	case library.HighRange:
		return "hdr"
	}
	return ""
}

// idText writes an identifier, empty for zero.
func idText(id accounts.ID) string {
	if id == (accounts.ID{}) {
		return ""
	}
	return id.String()
}

// userName names the user id names, empty when unknown.
func (h *handler) userName(ctx context.Context, id accounts.ID, names map[accounts.ID]string) string {
	if id == (accounts.ID{}) {
		return ""
	}
	if name, ok := names[id]; ok {
		return name
	}
	user, err := h.Accounts.User(ctx, id)
	names[id] = user.Name
	if err != nil {
		names[id] = ""
	}
	return names[id]
}

func (h *handler) newVersionBlockJSON(ctx context.Context, block library.VersionBlock, names map[accounts.ID]string) versionBlockJSON {
	return versionBlockJSON{ID: block.ID.String(), Item: idText(block.Item), Everywhere: block.Everywhere(), Name: block.Name,
		Filename: block.Filename, Size: block.Size, Reason: block.Reason, By: h.userName(ctx, block.By, names), At: block.At}
}

// titleVersions explains a movie's or an episode's versions for the user.
func (h *handler) titleVersions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	explained, err := h.Versions.ExplainVersions(r.Context(), user, id)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	names := map[accounts.ID]string{}
	result := versionsJSON{Pending: explained.Pending, Criteria: explained.Criteria, Group: explained.Group, KeptAll: explained.KeptAll,
		Closest: explained.Closest, Sources: []versionSourceExplainedJSON{}, Blocks: []versionBlockJSON{}, Decisions: []playDecisionJSON{}}
	if result.Criteria == nil {
		result.Criteria = []string{}
	}
	for _, source := range explained.Sources {
		streams := make([]versionStreamJSON, 0, len(source.Streams))
		for _, stream := range source.Streams {
			listed := versionStreamJSON{Name: stream.Name, Filename: stream.Filename, Size: stream.Size, Torrent: stream.Torrent,
				Version: idText(stream.Version), Step: stream.Step, Rule: stream.Rule, Detail: stream.Detail, Place: stream.Place,
				Pinned: stream.Pinned, Unstarted: stream.Unstarted, Analyzed: stream.Analyzed,
				Labels: versionLabelsJSON{Height: stream.Height, Codec: stream.Labels.Codec, Range: rangeName(stream.Labels.Range),
					Languages: nonNil(stream.Labels.Languages), Multi: stream.Labels.Multi, Releases: nonNil(stream.Labels.Releases),
					Elsewhere: stream.Labels.Elsewhere, Tier: stream.Labels.Tier, Audio: newVersionAudioJSON(stream.Labels.Audio),
					Edition: stream.Labels.Edition},
				Facts: versionFactsJSON{Height: stream.Facts.Height, Size: stream.Facts.Size, Codec: stream.Facts.Codec,
					Range: rangeName(stream.Facts.Range), Languages: nonNil(stream.Facts.Languages), Multi: stream.Facts.Multi,
					Tier: stream.Facts.Tier, Audio: newVersionAudioJSON(stream.Facts.Audio), Edition: stream.Facts.Edition,
					PerMinute: stream.Facts.PerMinute, Score: stream.Facts.Score, LabelRules: nonNil(stream.Facts.LabelRules)}}
			if stream.Block != nil {
				block := h.newVersionBlockJSON(r.Context(), *stream.Block, names)
				listed.Block = &block
			}
			streams = append(streams, listed)
		}
		result.Sources = append(result.Sources, versionSourceExplainedJSON{ID: source.Addon.String(), Name: source.Name,
			Embedded: source.Embedded, State: source.State, Streams: streams})
	}
	if pin := explained.Pin; pin != nil {
		result.Pin = &versionPinJSON{Version: pin.Version.String(), Name: pin.Name, Filename: pin.Filename, Size: pin.Size,
			By: h.userName(r.Context(), pin.By, names), At: pin.At, Missing: pin.Missing}
	}
	for _, block := range explained.Blocks {
		result.Blocks = append(result.Blocks, h.newVersionBlockJSON(r.Context(), block, names))
	}
	for _, decision := range explained.Decisions {
		tries := make([]playTryJSON, 0, len(decision.Tries))
		for _, try := range decision.Tries {
			tries = append(tries, playTryJSON{Version: try.Version.String(), Name: try.Name, Outcome: try.Outcome, Reason: try.Reason})
		}
		result.Decisions = append(result.Decisions, playDecisionJSON{At: decision.At, User: decision.User, Device: decision.Device,
			App: decision.App, Picked: decision.Picked, Tries: tries, Chosen: idText(decision.Chosen), Delivery: decision.Delivery})
	}
	writeJSON(w, http.StatusOK, result)
}

// nonNil returns values, empty rather than nil, as JSON lists them.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// askVersionsAgain asks the user's addons again for a title's streams; a
// title asked again lately is answered 429, with Retry-After.
func (h *handler) askVersionsAgain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	wait, err := h.Versions.AskVersionsAgain(r.Context(), sessionFrom(r.Context()).User, user, id)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
		writeError(w, http.StatusTooManyRequests, "asked_lately")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// versionChoiceRequest names a version of the title, as the user is
// offered it; everywhere and reason are a block's.
type versionChoiceRequest struct {
	Version    string `json:"version"`
	User       string `json:"user"`
	Everywhere bool   `json:"everywhere"`
	Reason     string `json:"reason"`
}

// versionChoice reads a pin's or a block's request: the title, the
// version, and the user it is offered to, the administrator signed in
// unless the request names another.
func (h *handler) versionChoice(w http.ResponseWriter, r *http.Request) (versionChoiceRequest, accounts.User, accounts.ID, accounts.ID, bool) {
	item, ok := pathID(w, r, "id")
	if !ok {
		return versionChoiceRequest{}, accounts.User{}, accounts.ID{}, accounts.ID{}, false
	}
	var request versionChoiceRequest
	if !decode(w, r, &request) {
		return versionChoiceRequest{}, accounts.User{}, accounts.ID{}, accounts.ID{}, false
	}
	version, err := accounts.ParseID(request.Version)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_version")
		return versionChoiceRequest{}, accounts.User{}, accounts.ID{}, accounts.ID{}, false
	}
	if request.User != "" {
		query := r.URL.Query()
		query.Set("user", request.User)
		r.URL.RawQuery = query.Encode()
	}
	user, ok := h.versionsUser(w, r)
	return request, user, item, version, ok
}

// pinVersion pins a version of the title for every user.
func (h *handler) pinVersion(w http.ResponseWriter, r *http.Request) {
	_, user, item, version, ok := h.versionChoice(w, r)
	if !ok {
		return
	}
	err := h.Versions.PinVersion(r.Context(), sessionFrom(r.Context()).User, user, item, version)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unpinVersion unpins the title's version, if any.
func (h *handler) unpinVersion(w http.ResponseWriter, r *http.Request) {
	item, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.Versions.UnpinVersion(r.Context(), item); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// blockVersion blocks the file of a version of the title, for the title
// or every title, with a reason of at most library.MaxBlockReason
// characters.
func (h *handler) blockVersion(w http.ResponseWriter, r *http.Request) {
	request, user, item, version, ok := h.versionChoice(w, r)
	if !ok {
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if utf8.RuneCountInString(reason) > library.MaxBlockReason {
		writeError(w, http.StatusBadRequest, "invalid_reason")
		return
	}
	block, err := h.Versions.BlockVersion(r.Context(), sessionFrom(r.Context()).User, user, item, version, request.Everywhere, reason)
	if titleError(w, err) {
		return
	}
	if errors.Is(err, library.ErrNotBlockable) {
		writeError(w, http.StatusBadRequest, "not_blockable")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.newVersionBlockJSON(r.Context(), block, map[accounts.ID]string{}))
}

// unblockVersion lifts a block.
func (h *handler) unblockVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Versions.UnblockVersion(r.Context(), id)
	if titleError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// versionBlocks lists the blocks of every title, the latest first.
func (h *handler) versionBlocks(w http.ResponseWriter, r *http.Request) {
	names := map[accounts.ID]string{}
	result := []versionBlockJSON{}
	for _, block := range h.Versions.VersionBlocks(r.Context()) {
		result = append(result, h.newVersionBlockJSON(r.Context(), block, names))
	}
	writeJSON(w, http.StatusOK, result)
}

// analyzeVersion analyzes a version of the title now, and answers what
// kept it from being analyzed, empty when it was. A torrent's version,
// which only plays read, is refused.
func (h *handler) analyzeVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.versionsUser(w, r)
	if !ok {
		return
	}
	item, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	version, ok := pathID(w, r, "versionId")
	if !ok {
		return
	}
	failure, err := h.Versions.AnalyzeVersion(r.Context(), user, item, version)
	if titleError(w, err) {
		return
	}
	if errors.Is(err, jellyfin.ErrTorrentAnalysis) {
		writeError(w, http.StatusConflict, "torrent")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"failure": failure})
}
