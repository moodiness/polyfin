package admin

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// ChannelBuilder freezes the movies and episodes the sources of the
// channels Polyfin airs hold, and offers what they may be built from (see
// jellyfin.Handler).
type ChannelBuilder interface {
	ChannelEntries(ctx context.Context, user accounts.User, source virtualtv.Source) ([]virtualtv.Entry, error)
	ChannelTitles(ctx context.Context, user accounts.User, term string, limit int) ([]library.Item, error)
	HomeRowCollections(ctx context.Context, user accounts.User) ([]jellyfin.HomeRowCollection, error)
}

// Playouts stops the playout of a channel Polyfin airs (see
// playback.Service.StopPlayout).
type Playouts interface {
	StopPlayout(channel accounts.ID)
}

// channelTitlesLimit bounds the titles a search of the builder answers.
const channelTitlesLimit = 30

type channelItemJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type channelSourceJSON struct {
	Kind  string            `json:"kind"`
	Items []channelItemJSON `json:"items"`
}

type channelBlockJSON struct {
	Start   int               `json:"start"`
	End     int               `json:"end"`
	Source  channelSourceJSON `json:"source"`
	Shuffle bool              `json:"shuffle"`
	// Entries is how many movies and episodes the block airs, Runtime how
	// long they last in all, in seconds; read only.
	Entries int     `json:"entries"`
	Runtime float64 `json:"runtime"`
}

// airingJSON is a programme of a channel.
type airingJSON struct {
	Name    string    `json:"name"`
	Episode string    `json:"episode"`
	Season  int       `json:"season"`
	Number  int       `json:"number"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

// virtualChannelJSON is a channel Polyfin airs: what it is built from,
// how many movies and episodes it airs (Entries), how long they last in
// all (Runtime, in seconds), and what it airs now and next.
type virtualChannelJSON struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Number    *int               `json:"number"`
	Enabled   bool               `json:"enabled"`
	Source    channelSourceJSON  `json:"source"`
	Shuffle   bool               `json:"shuffle"`
	Blocks    []channelBlockJSON `json:"blocks"`
	Entries   int                `json:"entries"`
	Runtime   float64            `json:"runtime"`
	Airing    []airingJSON       `json:"airing"`
	Epoch     time.Time          `json:"epoch"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

// virtualChannelRequest is a channel as the admin app sends it. Rebuild
// freezes its sources' titles again, as a change of its sources, blocks
// or shuffle does: its schedule then starts again today.
type virtualChannelRequest struct {
	Name    string             `json:"name"`
	Number  *int               `json:"number"`
	Enabled bool               `json:"enabled"`
	Source  channelSourceJSON  `json:"source"`
	Shuffle bool               `json:"shuffle"`
	Blocks  []channelBlockJSON `json:"blocks"`
	Rebuild bool               `json:"rebuild"`
}

func newChannelSourceJSON(s virtualtv.Source) channelSourceJSON {
	result := channelSourceJSON{Kind: s.Kind, Items: make([]channelItemJSON, 0, len(s.Items))}
	for _, item := range s.Items {
		result.Items = append(result.Items, channelItemJSON{ID: item.ID.String(), Name: item.Name, Kind: item.Kind})
	}
	return result
}

func (s channelSourceJSON) source() (virtualtv.Source, bool) {
	source := virtualtv.Source{Kind: s.Kind}
	for _, item := range s.Items {
		id, err := accounts.ParseID(item.ID)
		if err != nil {
			return virtualtv.Source{}, false
		}
		source.Items = append(source.Items, virtualtv.SourceItem{ID: id, Name: strings.TrimSpace(item.Name), Kind: item.Kind})
	}
	return source, true
}

func runtimeOf(entries []virtualtv.Entry) float64 {
	var total time.Duration
	for _, e := range entries {
		total += e.Runtime
	}
	return total.Seconds()
}

func newVirtualChannelJSON(c virtualtv.Channel, now time.Time) virtualChannelJSON {
	result := virtualChannelJSON{ID: c.ID.String(), Name: c.Name, Enabled: c.Enabled, Source: newChannelSourceJSON(c.Source), Shuffle: c.Shuffle,
		Blocks: make([]channelBlockJSON, 0, len(c.Blocks)), Entries: len(c.Entries), Runtime: runtimeOf(c.Entries), Airing: []airingJSON{},
		Epoch: c.Epoch.UTC(), UpdatedAt: c.UpdatedAt.UTC()}
	if c.Number > 0 {
		result.Number = new(c.Number)
	}
	for _, b := range c.Blocks {
		result.Blocks = append(result.Blocks, channelBlockJSON{Start: b.Start, End: b.End, Source: newChannelSourceJSON(b.Source), Shuffle: b.Shuffle,
			Entries: len(b.Entries), Runtime: runtimeOf(b.Entries)})
	}
	// What airs now, and the two programmes after it.
	for _, a := range c.Airings(now, now.Add(12*time.Hour), time.Local) {
		if len(result.Airing) == 3 {
			break
		}
		result.Airing = append(result.Airing, airingJSON{Name: a.Entry.Name, Episode: a.Entry.Episode, Season: a.Entry.Season, Number: a.Entry.Number,
			Start: a.Start.UTC(), End: a.End.UTC()})
	}
	return result
}

// virtualChannelError answers the errors of channels and reports whether
// err was one of them.
func virtualChannelError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{virtualtv.ErrNotFound, http.StatusNotFound, "not_found"},
		{library.ErrNotFound, http.StatusBadRequest, "invalid_channel_source"},
		{virtualtv.ErrInvalidName, http.StatusBadRequest, "invalid_own_channel_name"},
		{virtualtv.ErrInvalidNumber, http.StatusBadRequest, "invalid_channel_number"},
		{virtualtv.ErrInvalidSource, http.StatusBadRequest, "invalid_channel_source"},
		{virtualtv.ErrNoEntries, http.StatusBadRequest, "channel_without_titles"},
		{virtualtv.ErrInvalidBlocks, http.StatusBadRequest, "invalid_channel_blocks"},
		{virtualtv.ErrTooMany, http.StatusConflict, "too_many_channels"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// virtualChannels answers the channels Polyfin airs, in the order they
// were made.
func (h *handler) virtualChannels(w http.ResponseWriter, r *http.Request) {
	all, err := h.VirtualChannels.Channels(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	result := make([]virtualChannelJSON, 0, len(all))
	for _, c := range all {
		result = append(result, newVirtualChannelJSON(c, now))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) virtualChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c, err := h.VirtualChannels.Channel(r.Context(), id)
	h.answerVirtualChannel(w, r, http.StatusOK, c, err)
}

// answerVirtualChannel answers a channel saved, or why it was not.
func (h *handler) answerVirtualChannel(w http.ResponseWriter, r *http.Request, status int, c virtualtv.Channel, err error) {
	if virtualChannelError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, newVirtualChannelJSON(c, h.now()))
}

// readVirtualChannel reads the channel a request sends, answering it when
// it is not one.
func readVirtualChannel(w http.ResponseWriter, r *http.Request) (virtualtv.Channel, bool, bool) {
	var body virtualChannelRequest
	if !decode(w, r, &body) {
		return virtualtv.Channel{}, false, false
	}
	c := virtualtv.Channel{Name: strings.TrimSpace(body.Name), Enabled: body.Enabled, Shuffle: body.Shuffle}
	if body.Number != nil {
		if *body.Number < 1 {
			writeError(w, http.StatusBadRequest, "invalid_channel_number")
			return virtualtv.Channel{}, false, false
		}
		c.Number = *body.Number
	}
	source, ok := body.Source.source()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_channel_source")
		return virtualtv.Channel{}, false, false
	}
	c.Source = source
	for _, b := range body.Blocks {
		source, ok := b.Source.source()
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_channel_source")
			return virtualtv.Channel{}, false, false
		}
		c.Blocks = append(c.Blocks, virtualtv.Block{Start: b.Start, End: b.End, Source: source, Shuffle: b.Shuffle})
	}
	return c, body.Rebuild, true
}

// freeze sets the movies and episodes a channel's sources hold now, its
// schedule starting today; the sources are checked first, so that a
// channel that cannot be saved reads no catalog.
func (h *handler) freeze(ctx context.Context, user accounts.User, c *virtualtv.Channel) error {
	probe := *c
	probe.Entries = []virtualtv.Entry{{Kind: virtualtv.EntryMovie, Runtime: time.Minute}}
	for i := range probe.Blocks {
		probe.Blocks[i].Entries = probe.Entries
	}
	if err := probe.Validate(); err != nil {
		return err
	}
	entries, err := h.ChannelBuilder.ChannelEntries(ctx, user, c.Source)
	if err != nil {
		return err
	}
	c.Entries = entries
	for i, b := range c.Blocks {
		if c.Blocks[i].Entries, err = h.ChannelBuilder.ChannelEntries(ctx, user, b.Source); err != nil {
			return err
		}
	}
	c.Epoch = virtualtv.Now(time.Local)
	return nil
}

func (h *handler) createVirtualChannel(w http.ResponseWriter, r *http.Request) {
	c, _, ok := readVirtualChannel(w, r)
	if !ok {
		return
	}
	c.Seed = virtualtv.NewSeed()
	err := h.freeze(r.Context(), sessionFrom(r.Context()).User, &c)
	if err == nil {
		c, err = h.VirtualChannels.Create(r.Context(), c)
	}
	h.answerVirtualChannel(w, r, http.StatusCreated, c, err)
}

// updateVirtualChannel saves a channel. A channel whose sources, blocks
// or shuffle changed, or asked to be rebuilt, freezes its titles again and
// starts its schedule again today, shuffled anew; its playout stops, as
// it does when the channel is turned off, so that it airs what changed at
// once.
func (h *handler) updateVirtualChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c, rebuild, ok := readVirtualChannel(w, r)
	if !ok {
		return
	}
	old, err := h.VirtualChannels.Channel(r.Context(), id)
	if err != nil {
		h.answerVirtualChannel(w, r, http.StatusOK, old, err)
		return
	}
	c.ID, c.Seed, c.Entries, c.Epoch = id, old.Seed, old.Entries, old.Epoch
	changed := rebuild || c.Shuffle != old.Shuffle || !reflect.DeepEqual(c.Source, old.Source) || len(c.Blocks) != len(old.Blocks)
	for i := range c.Blocks {
		if changed {
			break
		}
		b, was := c.Blocks[i], old.Blocks[i]
		changed = b.Start != was.Start || b.End != was.End || b.Shuffle != was.Shuffle || !reflect.DeepEqual(b.Source, was.Source)
		c.Blocks[i].Entries = was.Entries
	}
	if changed {
		c.Seed = virtualtv.NewSeed()
		err = h.freeze(r.Context(), sessionFrom(r.Context()).User, &c)
	}
	if err == nil {
		c, err = h.VirtualChannels.Update(r.Context(), c)
	}
	if err == nil && h.Playouts != nil && (changed || !c.Enabled) {
		h.Playouts.StopPlayout(id)
	}
	h.answerVirtualChannel(w, r, http.StatusOK, c, err)
}

func (h *handler) deleteVirtualChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.VirtualChannels.Delete(r.Context(), id)
	if virtualChannelError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if h.Playouts != nil {
		h.Playouts.StopPlayout(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelChoice is a collection or a smart list a channel may be built
// from.
type channelChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Source tells a collection made by users, of an addon, or a smart
	// list shown as a collection.
	Source string `json:"source"`
}

// virtualChannelChoices answers the collections and the smart lists a
// channel may be built from, as the administrator sees them.
func (h *handler) virtualChannelChoices(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	found, err := h.ChannelBuilder.HomeRowCollections(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	collections := []channelChoice{}
	for _, c := range found {
		if c.Source != "smart" {
			collections = append(collections, channelChoice{ID: c.ID.String(), Name: c.Name, Source: c.Source})
		}
	}
	lists, err := h.SmartLists.Lists(r.Context(), nil)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	smart := []channelChoice{}
	for _, l := range lists {
		smart = append(smart, channelChoice{ID: library.SmartListItemID(l.ID).String(), Name: l.Name, Source: l.ShownAs})
	}
	writeJSON(w, http.StatusOK, struct {
		Collections []channelChoice `json:"collections"`
		SmartLists  []channelChoice `json:"smartLists"`
	}{collections, smart})
}

// channelTitleJSON is a movie or a series found for the builder.
type channelTitleJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Year *int   `json:"year"`
}

// virtualChannelTitles answers the movies and series whose name holds q.
func (h *handler) virtualChannelTitles(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	result := []channelTitleJSON{}
	if term == "" {
		writeJSON(w, http.StatusOK, result)
		return
	}
	found, err := h.ChannelBuilder.ChannelTitles(r.Context(), sessionFrom(r.Context()).User, term, channelTitlesLimit)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, item := range found {
		title := channelTitleJSON{ID: item.ID.String(), Name: item.Name, Kind: string(item.Kind)}
		if item.ProductionYear > 0 {
			title.Year = new(item.ProductionYear)
		}
		result = append(result, title)
	}
	writeJSON(w, http.StatusOK, result)
}
