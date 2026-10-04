package jellyfin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/coder/websocket"

	"github.com/moodiness/polyfin/internal/accounts"
)

// groupWaitTimeout is how long a SyncPlay group waits for members that
// stay buffering, as Jellyfin waits.
const groupWaitTimeout = 30 * time.Second

// GroupInfoDto describes a SyncPlay group.
type GroupInfoDto struct {
	GroupId       string
	GroupName     string
	State         string
	Participants  []string
	LastUpdatedAt Time
}

// SyncPlayGroupUpdate is the data of a SyncPlayGroupUpdate message: what
// changed in a group, or why a request about one failed.
type SyncPlayGroupUpdate struct {
	GroupId string
	Data    any
	Type    string
}

// GroupStateUpdate is the data of a StateUpdate: the group's state, and
// the request that led to it.
type GroupStateUpdate struct {
	State  string
	Reason string
}

// PlayQueueUpdate is the data of a PlayQueue update.
type PlayQueueUpdate struct {
	Reason             string
	LastUpdate         Time
	Playlist           []SyncPlayQueueItem
	PlayingItemIndex   int
	StartPositionTicks int64
	IsPlaying          bool
	ShuffleMode        string
	RepeatMode         string
}

// SyncPlayQueueItem is an entry of a group's queue.
type SyncPlayQueueItem struct {
	ItemId         string
	PlaylistItemId string
}

// SendCommand is the data of a SyncPlayCommand message: what members are
// to do, from which position, when on the server's clock.
type SendCommand struct {
	GroupId        string
	PlaylistItemId string
	When           Time
	PositionTicks  int64
	Command        string
	EmittedAt      Time
}

// UtcTimeResponse is the server's time, which SyncPlay members keep their
// clocks in step with.
type UtcTimeResponse struct {
	RequestReceptionTime     Time
	ResponseTransmissionTime Time
}

// syncPlay holds the server's SyncPlay groups. Like Jellyfin's, they live
// in memory, as long as they have members.
type syncPlay struct {
	// joining serializes creating, joining and leaving groups, which may
	// change two groups at once. It is taken before any group's lock, and
	// never held while the library is looked up or apps are written to.
	joining sync.Mutex
	// mu guards the maps, and is held only briefly.
	mu sync.Mutex
	// groups by identifier, and the group of each session.
	groups  map[accounts.ID]*syncGroup
	members map[accounts.ID]*syncGroup
	// active counts the sessions of each user in a group.
	active      map[accounts.ID]int
	waitTimeout time.Duration
	// canPlay reports whether a user may play every one of items.
	canPlay func(ctx context.Context, user accounts.ID, items []accounts.ID) bool
	// write sends a message to its session.
	write  func(syncMessage)
	logger *slog.Logger
	// outboxMu guards outbox: the messages waiting for each session. A
	// session is listed while a goroutine sends it its messages.
	outboxMu sync.Mutex
	outbox   map[accounts.ID][]syncMessage
}

// maxWaiting bounds the messages waiting for a session: past it, an app
// that stopped reading misses the next ones.
const maxWaiting = 64

func newSyncPlay(canPlay func(context.Context, accounts.ID, []accounts.ID) bool, write func(syncMessage), logger *slog.Logger) *syncPlay {
	return &syncPlay{groups: map[accounts.ID]*syncGroup{}, members: map[accounts.ID]*syncGroup{},
		active: map[accounts.ID]int{}, waitTimeout: groupWaitTimeout, canPlay: canPlay, write: write, logger: logger,
		outbox: map[accounts.ID][]syncMessage{}}
}

// deliver queues messages for their sessions, which receive them in the
// order they are queued. Writing to an app may take long: each session's
// messages go out from a goroutine of its own, so that a slow app holds
// back its own messages only, and never a group or the server's locks.
func (sp *syncPlay) deliver(messages []syncMessage) {
	sp.outboxMu.Lock()
	defer sp.outboxMu.Unlock()
	for _, m := range messages {
		waiting, sending := sp.outbox[m.device]
		if len(waiting) >= maxWaiting {
			sp.logger.Debug("A SyncPlay message was dropped for an app that does not read", "kind", m.kind)
			continue
		}
		sp.outbox[m.device] = append(waiting, m)
		if !sending {
			go sp.send(m.device)
		}
	}
}

// send writes the messages waiting for a session until none is left.
func (sp *syncPlay) send(device accounts.ID) {
	for {
		sp.outboxMu.Lock()
		waiting := sp.outbox[device]
		if len(waiting) == 0 {
			delete(sp.outbox, device)
			sp.outboxMu.Unlock()
			return
		}
		sp.outbox[device] = []syncMessage{}
		sp.outboxMu.Unlock()
		for _, m := range waiting {
			sp.write(m)
		}
	}
}

func (sp *syncPlay) groupOf(device accounts.ID) *syncGroup {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.members[device]
}

// isActive reports whether one of a user's sessions is in a group, which
// Jellyfin requires of most SyncPlay requests.
func (sp *syncPlay) isActive(user accounts.ID) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.active[user] > 0
}

// allCanPlay reports whether all users may play items.
func (sp *syncPlay) allCanPlay(ctx context.Context, users, items []accounts.ID) bool {
	for _, user := range users {
		if !sp.canPlay(ctx, user, items) {
			return false
		}
	}
	return true
}

// failed tells a session why its request did nothing.
func (sp *syncPlay) failed(device, group accounts.ID, kind string) {
	sp.deliver([]syncMessage{{device: device, kind: "SyncPlayGroupUpdate",
		data: SyncPlayGroupUpdate{GroupId: group.String(), Type: kind, Data: ""}}})
}

// nowPlayingTitle is what a session creating a group plays.
type nowPlayingTitle struct {
	item     accounts.ID
	position int64
	paused   bool
}

// create makes a group of a session, which leaves its group first.
func (sp *syncPlay) create(member *groupMember, groupName string, playing *nowPlayingTitle) GroupInfoDto {
	sp.joining.Lock()
	defer sp.joining.Unlock()
	if current := sp.groupOf(member.device); current != nil {
		sp.leave(current, member.device)
	}
	g := &syncGroup{sp: sp, id: randomGUID(), name: groupName, created: time.Now(), queue: newPlayQueue()}
	g.mu.Lock()
	defer g.unlock()
	sp.mu.Lock()
	sp.groups[g.id] = g
	sp.members[member.device] = g
	sp.active[member.user]++
	sp.mu.Unlock()
	g.create(member, playing)
	return g.info()
}

// join adds a session to a group whose queue its user may play. Library
// lookups may be slow: they are made without holding any lock, and made
// again if items were queued meanwhile.
func (sp *syncPlay) join(ctx context.Context, member *groupMember, id accounts.ID) {
	for {
		sp.mu.Lock()
		g := sp.groups[id]
		sp.mu.Unlock()
		if g == nil {
			sp.failed(member.device, accounts.ID{}, "GroupDoesNotExist")
			return
		}
		g.mu.Lock()
		items, additions := g.queue.items(), g.queue.additions
		g.mu.Unlock()
		if sp.joinChecked(member, g, additions, sp.canPlay(ctx, member.user, items)) {
			return
		}
	}
}

// joinChecked adds a session to g, or tells it it may not play its queue,
// as allowed says. It reports false, doing nothing, when g ended or items
// were queued since the check, at the queue's additions given.
func (sp *syncPlay) joinChecked(member *groupMember, g *syncGroup, additions int, allowed bool) bool {
	sp.joining.Lock()
	defer sp.joining.Unlock()
	g.mu.Lock()
	defer g.unlock()
	sp.mu.Lock()
	ended := sp.groups[g.id] != g
	sp.mu.Unlock()
	switch {
	case ended || g.queue.additions != additions:
		return false
	case !allowed:
		sp.failed(member.device, g.id, "LibraryAccessDenied")
		return true
	}
	if current := sp.groupOf(member.device); current != g {
		if current != nil {
			sp.leave(current, member.device)
		}
		sp.mu.Lock()
		sp.members[member.device] = g
		sp.active[member.user]++
		sp.mu.Unlock()
	}
	g.join(member)
	return true
}

// sessionLeft takes a session out of its group, and reports whether it
// was in one.
func (sp *syncPlay) sessionLeft(device accounts.ID) bool {
	sp.joining.Lock()
	defer sp.joining.Unlock()
	g := sp.groupOf(device)
	if g == nil {
		return false
	}
	sp.leave(g, device)
	return true
}

// leave takes a session out of g, which goes once empty. The caller holds
// joining.
func (sp *syncPlay) leave(g *syncGroup, device accounts.ID) {
	g.mu.Lock()
	defer g.unlock()
	member := g.member(device)
	sp.mu.Lock()
	delete(sp.members, device)
	if sp.active[member.user]--; sp.active[member.user] == 0 {
		delete(sp.active, member.user)
	}
	sp.mu.Unlock()
	g.leave(member)
	if len(g.members) == 0 {
		sp.mu.Lock()
		delete(sp.groups, g.id)
		sp.mu.Unlock()
	}
}

// handle applies a playback request of a session to its group.
func (sp *syncPlay) handle(ctx context.Context, device accounts.ID, r syncRequest) {
	g := sp.groupOf(device)
	if g == nil {
		sp.failed(device, accounts.ID{}, "NotInGroup")
		return
	}
	g.mu.Lock()
	defer g.unlock()
	if (r.kind == "Play" || r.kind == "Queue") && len(r.items) > 0 && len(r.items) <= maxQueueEntries {
		// Every member must be able to play the items. Library lookups may
		// be slow: they are made without holding the group, and made again
		// if its members changed meanwhile.
		for {
			users := g.users()
			g.mu.Unlock()
			allowed := sp.allCanPlay(ctx, users, r.items)
			g.mu.Lock()
			if slices.Equal(users, g.users()) {
				r.allowed = allowed
				break
			}
		}
	}
	// The session may have left while the request waited.
	if g.member(device) != nil {
		g.handle(device, r)
	}
}

// visible lists the groups whose queue user may play, the oldest first,
// or only the one of id when id is set.
func (sp *syncPlay) visible(ctx context.Context, user accounts.ID, id *accounts.ID) []GroupInfoDto {
	sp.mu.Lock()
	groups := make([]*syncGroup, 0, len(sp.groups))
	for _, g := range sp.groups {
		if id == nil || g.id == *id {
			groups = append(groups, g)
		}
	}
	sp.mu.Unlock()
	slices.SortFunc(groups, func(a, b *syncGroup) int { return a.created.Compare(b.created) })
	infos := []GroupInfoDto{}
	for _, g := range groups {
		// Library lookups may be slow: they are made without holding the
		// group.
		g.mu.Lock()
		info, items := g.info(), g.queue.items()
		g.mu.Unlock()
		if sp.canPlay(ctx, user, items) {
			infos = append(infos, info)
		}
	}
	return infos
}

func randomGUID() accounts.ID {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id
}

// syncPlayRoutes serves SyncPlay, with which apps watch together. Like
// Jellyfin, most requests need one of the user's sessions to be in a group,
// and the user's SyncPlay access decides who may create, list and join
// groups (see syncPlayAllowed).
func (h *Handler) syncPlayRoutes(rt *router) {
	signedIn := func(method, pattern string, need syncPlayNeed, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(h.syncPlayAccess(need, handler)))
	}
	inGroup := func(pattern string, handler http.HandlerFunc) {
		signedIn(http.MethodPost, pattern, syncPlayInGroup, handler)
	}
	rt.handle(http.MethodGet, "/GetUtcTime", http.HandlerFunc(utcTime))
	signedIn(http.MethodPost, "/SyncPlay/New", syncPlayCreate, h.syncPlayNew)
	signedIn(http.MethodPost, "/SyncPlay/Join", syncPlayJoinGroups, h.syncPlayJoin)
	inGroup("/SyncPlay/Leave", h.syncPlayLeave)
	signedIn(http.MethodGet, "/SyncPlay/List", syncPlayJoinGroups, h.syncPlayGroups)
	signedIn(http.MethodGet, "/SyncPlay/{groupId}", syncPlayJoinGroups, h.syncPlayGroup)
	inGroup("/SyncPlay/SetNewQueue", h.syncPlayRequest(syncPlaySetNewQueue))
	inGroup("/SyncPlay/SetPlaylistItem", h.syncPlayRequest(syncPlayEntry("SetPlaylistItem")))
	inGroup("/SyncPlay/RemoveFromPlaylist", h.syncPlayRequest(syncPlayRemove))
	inGroup("/SyncPlay/MovePlaylistItem", h.syncPlayRequest(syncPlayMove))
	inGroup("/SyncPlay/Queue", h.syncPlayRequest(syncPlayQueue))
	inGroup("/SyncPlay/Unpause", h.syncPlayRequest(syncPlayBare("Unpause")))
	inGroup("/SyncPlay/Pause", h.syncPlayRequest(syncPlayBare("Pause")))
	inGroup("/SyncPlay/Stop", h.syncPlayRequest(syncPlayBare("Stop")))
	inGroup("/SyncPlay/Seek", h.syncPlayRequest(syncPlaySeek))
	inGroup("/SyncPlay/Buffering", h.syncPlayRequest(syncPlayReport("Buffer")))
	inGroup("/SyncPlay/Ready", h.syncPlayRequest(syncPlayReport("Ready")))
	inGroup("/SyncPlay/SetIgnoreWait", h.syncPlayRequest(syncPlayIgnoreWait))
	inGroup("/SyncPlay/NextItem", h.syncPlayRequest(syncPlayEntry("NextItem")))
	inGroup("/SyncPlay/PreviousItem", h.syncPlayRequest(syncPlayEntry("PreviousItem")))
	inGroup("/SyncPlay/SetRepeatMode", h.syncPlayRequest(syncPlayRepeat))
	inGroup("/SyncPlay/SetShuffleMode", h.syncPlayRequest(syncPlayShuffle))
	// A session out of any group may ping: it is then told so.
	signedIn(http.MethodPost, "/SyncPlay/Ping", syncPlayAnyAccess, h.syncPlayRequest(syncPlayPing))
}

// utcTime gives the server's time, for apps to measure how far their
// clock is from it.
func utcTime(w http.ResponseWriter, _ *http.Request) {
	received := time.Now()
	writeJSON(w, http.StatusOK, UtcTimeResponse{RequestReceptionTime: Time(received), ResponseTransmissionTime: Time(time.Now())})
}

// syncPlayMember is the caller's session, as a group member.
func syncPlayMember(r *http.Request) *groupMember {
	c := callerFrom(r.Context())
	return &groupMember{device: c.Device.ID, user: c.User.ID, name: c.User.Name, ping: defaultPing}
}

// syncPlayNew creates a group of the caller's session. A session playing a
// title brings it to the group. Jellyfin brings the queue the app reported
// along; Polyfin does not keep it, and starts with the title alone.
func (h *Handler) syncPlayNew(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupName json.RawMessage
	}
	if !requiredBody(w, r, "requestData", &body) {
		return
	}
	name := ""
	if body.GroupName != nil {
		text, problem := jsonString(body.GroupName)
		switch {
		case problem != "":
			validationProblem(w, map[string][]string{"$.GroupName": {problem}, "requestData": {"The requestData field is required."}})
			return
		case text == nil:
			validationProblem(w, map[string][]string{"GroupName": {"The GroupName field is required."}})
			return
		}
		name = *text
	}
	// Jellyfin counts the name's length in UTF-16 units.
	if len(utf16.Encode([]rune(name))) > 200 {
		validationProblem(w, map[string][]string{"GroupName": {"Group name must not exceed 200 characters."}})
		return
	}
	member := syncPlayMember(r)
	var playing *nowPlayingTitle
	if now, ok := h.sessions.Playing(member.device); ok {
		if title, err := h.title(r.Context(), callerFrom(r.Context()).User, now.Item); err == nil {
			playing = &nowPlayingTitle{item: title.ID, position: boundedPosition(ticks(now.Position)), paused: now.Paused}
		}
	}
	writeJSON(w, http.StatusOK, h.syncPlay.create(member, strings.TrimSpace(name), playing))
}

func (h *Handler) syncPlayJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupId syncPlayID
	}
	if !requiredBody(w, r, "requestData", &body) {
		return
	}
	h.syncPlay.join(context.WithoutCancel(r.Context()), syncPlayMember(r), accounts.ID(body.GroupId))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) syncPlayLeave(w http.ResponseWriter, r *http.Request) {
	device := callerFrom(r.Context()).Device.ID
	if !h.syncPlay.sessionLeft(device) {
		h.syncPlay.failed(device, accounts.ID{}, "NotInGroup")
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncPlayGroups lists the groups whose queue the user may play.
func (h *Handler) syncPlayGroups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.syncPlay.visible(r.Context(), callerFrom(r.Context()).User.ID, nil))
}

func (h *Handler) syncPlayGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(r.PathValue("groupId"))
	if !ok {
		// Jellyfin's route takes identifiers only: anything else matches
		// no route.
		w.WriteHeader(http.StatusNotFound)
		return
	}
	groups := h.syncPlay.visible(r.Context(), callerFrom(r.Context()).User.ID, &id)
	if len(groups) == 0 {
		notFoundProblem(w)
		return
	}
	writeJSON(w, http.StatusOK, groups[0])
}

// syncPlayRequest serves a playback request, which read decodes from the
// request's body.
func (h *Handler) syncPlayRequest(read func(http.ResponseWriter, *http.Request) (syncRequest, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, ok := read(w, r)
		if !ok {
			return
		}
		// The request is carried out even if the app hangs up meanwhile.
		h.syncPlay.handle(context.WithoutCancel(r.Context()), callerFrom(r.Context()).Device.ID, request)
		w.WriteHeader(http.StatusNoContent)
	}
}

// syncPlayBare reads a request without a body.
func syncPlayBare(kind string) func(http.ResponseWriter, *http.Request) (syncRequest, bool) {
	return func(http.ResponseWriter, *http.Request) (syncRequest, bool) { return syncRequest{kind: kind}, true }
}

func syncPlaySetNewQueue(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		PlayingQueue        []syncPlayID
		PlayingItemPosition looseInt
		StartPositionTicks  looseInt
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "Play", items: syncPlayIDs(body.PlayingQueue), index: int(body.PlayingItemPosition.value),
		position: boundedPosition(body.StartPositionTicks.value)}, ok
}

// syncPlayEntry reads a request naming an entry of the queue.
func syncPlayEntry(kind string) func(http.ResponseWriter, *http.Request) (syncRequest, bool) {
	return func(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
		var body struct {
			PlaylistItemId syncPlayID
		}
		ok := requiredBody(w, r, "requestData", &body)
		return syncRequest{kind: kind, entry: accounts.ID(body.PlaylistItemId)}, ok
	}
}

func syncPlayRemove(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		PlaylistItemIds  []syncPlayID
		ClearPlaylist    bool
		ClearPlayingItem bool
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "RemoveFromPlaylist", entries: syncPlayIDs(body.PlaylistItemIds),
		clearPlaylist: body.ClearPlaylist, clearPlaying: body.ClearPlayingItem}, ok
}

func syncPlayMove(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		PlaylistItemId syncPlayID
		NewIndex       looseInt
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "MovePlaylistItem", entry: accounts.ID(body.PlaylistItemId), index: int(body.NewIndex.value)}, ok
}

func syncPlayQueue(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		ItemIds []syncPlayID
		Mode    syncPlayEnum
	}
	ok := requiredBody(w, r, "requestData", &body) && body.Mode.in(w, "Mode", queueModes)
	return syncRequest{kind: "Queue", items: syncPlayIDs(body.ItemIds), queueNext: body.Mode.name == queueModes[1]}, ok
}

func syncPlaySeek(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		PositionTicks looseInt
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "Seek", position: boundedPosition(body.PositionTicks.value)}, ok
}

// syncPlayReport reads what a member reports when it buffers or is ready.
func syncPlayReport(kind string) func(http.ResponseWriter, *http.Request) (syncRequest, bool) {
	return func(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
		var body struct {
			When           Time
			PositionTicks  looseInt
			IsPlaying      bool
			PlaylistItemId syncPlayID
		}
		ok := requiredBody(w, r, "requestData", &body)
		return syncRequest{kind: kind, when: time.Time(body.When), position: boundedPosition(body.PositionTicks.value), playing: body.IsPlaying,
			entry: accounts.ID(body.PlaylistItemId)}, ok
	}
}

func syncPlayIgnoreWait(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		IgnoreWait bool
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "IgnoreWait", ignoreWait: body.IgnoreWait}, ok
}

func syncPlayRepeat(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		Mode syncPlayEnum
	}
	ok := requiredBody(w, r, "requestData", &body) && body.Mode.in(w, "Mode", repeatModes)
	return syncRequest{kind: "SetRepeatMode", mode: body.Mode.name}, ok
}

func syncPlayShuffle(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		Mode syncPlayEnum
	}
	ok := requiredBody(w, r, "requestData", &body) && body.Mode.in(w, "Mode", shuffleModes)
	return syncRequest{kind: "SetShuffleMode", mode: body.Mode.name}, ok
}

func syncPlayPing(w http.ResponseWriter, r *http.Request) (syncRequest, bool) {
	var body struct {
		Ping looseInt
	}
	ok := requiredBody(w, r, "requestData", &body)
	return syncRequest{kind: "Ping", ping: body.Ping.value}, ok
}

// The queue modes of Jellyfin's GroupQueueMode, in the order of their
// values.
var queueModes = []string{"Queue", "QueueNext"}

// syncPlayID reads an identifier as Jellyfin does: in any form .NET
// parses, null being the empty identifier.
type syncPlayID accounts.ID

func (id *syncPlayID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*id = syncPlayID{}
		return nil
	}
	text, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	parsed, ok := parseGUID(text)
	if !ok {
		return accounts.ErrInvalidID
	}
	*id = syncPlayID(parsed)
	return nil
}

func syncPlayIDs(ids []syncPlayID) []accounts.ID {
	result := make([]accounts.ID, 0, len(ids))
	for _, id := range ids {
		result = append(result, accounts.ID(id))
	}
	return result
}

// syncPlayEnum reads an enumeration as Jellyfin does: by name in any
// letter case, or by value. Left out, it takes the first value.
type syncPlayEnum struct {
	raw  json.RawMessage
	name string
}

func (e *syncPlayEnum) UnmarshalJSON(data []byte) error {
	e.raw = slices.Clone(data)
	return nil
}

// in converts the enumeration to one of names, answering as Jellyfin does
// when it cannot.
func (e *syncPlayEnum) in(w http.ResponseWriter, field string, names []string) bool {
	e.name = names[0]
	if e.raw == nil {
		return true
	}
	var text string
	var value int
	switch {
	case json.Unmarshal(e.raw, &text) == nil:
		for _, name := range names {
			if strings.EqualFold(name, strings.TrimSpace(text)) {
				e.name = name
				return true
			}
		}
	case json.Unmarshal(e.raw, &value) == nil && value >= 0 && value < len(names):
		e.name = names[value]
		return true
	}
	validationProblem(w, map[string][]string{"$." + field: {"The JSON value could not be converted."}, "requestData": {"The requestData field is required."}})
	return false
}

// canPlay reports whether a user may play every one of items, as players
// open them: through the titles the user can reach. Jellyfin queues any
// item the user sees; Polyfin plays movies and episodes only.
func (h *Handler) canPlay(ctx context.Context, userID accounts.ID, items []accounts.ID) bool {
	if len(items) == 0 {
		return true
	}
	user, err := h.Accounts.User(ctx, userID)
	if err != nil {
		return false
	}
	checked := map[accounts.ID]bool{}
	for _, item := range items {
		if checked[item] {
			continue
		}
		checked[item] = true
		if _, err := h.title(ctx, user, item); err != nil {
			return false
		}
	}
	return true
}

// writeSyncPlay sends a SyncPlay message on the socket its session's
// device opened last, as Jellyfin sends it to one socket of a session. A
// session without a socket misses it, as with Jellyfin. An app that does
// not read gets its socket closed once the write times out, which ends
// its session.
func (h *Handler) writeSyncPlay(m syncMessage) {
	conn := h.sockets.latest(m.device)
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), socketWrite)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, socketPayload(m.kind, m.data)); err != nil {
		h.Logger.Debug("A SyncPlay message could not reach an app", "kind", m.kind, "error", err)
	}
}
