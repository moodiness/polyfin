package jellyfin

import (
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// groupState is what a SyncPlay group does, as Jellyfin's GroupStateType.
type groupState int

const (
	// stateIdle: nothing plays, apps are stopped.
	stateIdle groupState = iota
	// stateWaiting: the group waits for every member to be ready, having
	// loaded the item or buffered, before it plays or pauses.
	stateWaiting
	// statePaused: members have the item loaded, paused.
	statePaused
	// statePlaying: members play the item.
	statePlaying
)

var groupStateNames = [...]string{"Idle", "Waiting", "Paused", "Playing"}

func (s groupState) String() string { return groupStateNames[s] }

// Jellyfin's timing of SyncPlay groups, in milliseconds.
const (
	// defaultPing is the latency assumed of a member that reported none.
	defaultPing = 500
	// maxPing bounds the latency a member reports.
	maxPing = 10000
	// timeSyncOffset is how far the time a member reports may be from the
	// server's before it is not trusted.
	timeSyncOffset = 2000
	// maxPlaybackOffset is how far a ready member may be from the group's
	// position.
	maxPlaybackOffset = 500
	// maxCatchUpOffset is how far behind a member playing on after
	// buffering may be.
	maxCatchUpOffset = 60000
)

// ticksPerMillisecond converts milliseconds to the ticks of positions.
const ticksPerMillisecond = 10_000

func ticks(d time.Duration) int64 { return int64(d / 100) }

// Bounds of what members ask, which Jellyfin does not have.
const (
	// maxQueueEntries bounds a group's queue, which every change sends
	// whole to every member. It holds the longest series.
	maxQueueEntries = 5000
	// maxPositionTicks bounds positions, 30 days, far past any title, so
	// that the time played added to them stays within int64.
	maxPositionTicks = int64(30 * 24 * time.Hour / 100)
)

// boundedPosition keeps a position members ask within 0 and
// maxPositionTicks.
func boundedPosition(position int64) int64 {
	return min(max(position, 0), maxPositionTicks)
}

// broadcast chooses the members a message goes to, relative to the
// session a change comes from.
type broadcast int

const (
	toAll broadcast = iota
	toSession
	toOthers
	// toReady are the members not buffering.
	toReady
)

// groupMember is a session in a group.
type groupMember struct {
	device, user accounts.ID
	name         string
	// ping is the member's latency, in milliseconds.
	ping       int64
	buffering  bool
	ignoreWait bool
}

// syncMessage is a SyncPlay message for one session.
type syncMessage struct {
	device accounts.ID
	kind   string
	data   any
}

// syncRequest is a playback request of a member, named after Jellyfin's
// PlaybackRequestType, with what the request carries.
type syncRequest struct {
	kind string
	// items are the items a Play or Queue request plays or queues, and
	// allowed whether every member may play them, checked beforehand:
	// false for no items or more than maxQueueEntries.
	items   []accounts.ID
	allowed bool
	// index is the item to play first, or where to move an entry.
	index int
	// position is the position of Play, Seek, Buffer and Ready requests.
	position int64
	// entry is the entry a request names; entries those it removes.
	entry   accounts.ID
	entries []accounts.ID
	// clearPlaylist and clearPlaying ask a removal to empty the queue.
	clearPlaylist, clearPlaying bool
	queueNext                   bool
	// when and playing are the time and state of a Buffer or Ready
	// request.
	when    time.Time
	playing bool
	// mode is a repeat or shuffle mode.
	mode       string
	ping       int64
	ignoreWait bool
}

// syncGroup is a SyncPlay group, run as Jellyfin's state machine: the
// server keeps the state of playback and tells members what to do and
// when, on its clock; members keep in step by themselves.
type syncGroup struct {
	sp      *syncPlay
	id      accounts.ID
	name    string
	created time.Time

	// mu guards what follows. Messages that changes produce gather in
	// outbox, and are queued for their sessions before mu is released,
	// which keeps them in order.
	mu      sync.Mutex
	outbox  []syncMessage
	members []*groupMember
	state   groupState
	// While waiting: whether to play once everyone is ready, the state
	// the wait began in, and whether the position jumped meanwhile.
	resumePlaying   bool
	initialState    groupState
	initialStateSet bool
	positionJumped  bool
	// While playing: whether playback was forced to start, so that
	// buffering members do not stop it.
	ignoreBuffering bool
	queue           playQueue
	// position is where playback is, in ticks, at lastActivity, which is
	// in the future while members are to start playing.
	position     int64
	lastActivity time.Time
	// waitDeadline is when a wait for buffering members gives up.
	waitDeadline time.Time
	waitTimer    *time.Timer
}

// unlock queues the messages the group's changes produced, and releases
// the group.
func (g *syncGroup) unlock() {
	g.sp.deliver(g.outbox)
	g.outbox = nil
	g.mu.Unlock()
}

func (g *syncGroup) member(device accounts.ID) *groupMember {
	for _, m := range g.members {
		if m.device == device {
			return m
		}
	}
	return nil
}

// users lists the members' users, each once.
func (g *syncGroup) users() []accounts.ID {
	var users []accounts.ID
	for _, m := range g.members {
		if !slices.Contains(users, m.user) {
			users = append(users, m.user)
		}
	}
	return users
}

func (g *syncGroup) recipients(from accounts.ID, to broadcast) []accounts.ID {
	if to == toSession {
		return []accounts.ID{from}
	}
	var devices []accounts.ID
	for _, m := range g.members {
		if to == toAll || to == toOthers && m.device != from || to == toReady && !m.buffering {
			devices = append(devices, m.device)
		}
	}
	return devices
}

func (g *syncGroup) send(from accounts.ID, to broadcast, kind string, data any) {
	for _, device := range g.recipients(from, to) {
		g.outbox = append(g.outbox, syncMessage{device: device, kind: kind, data: data})
	}
}

// update sends a SyncPlayGroupUpdate.
func (g *syncGroup) update(from accounts.ID, to broadcast, kind string, data any) {
	g.send(from, to, "SyncPlayGroupUpdate", SyncPlayGroupUpdate{GroupId: g.id.String(), Type: kind, Data: data})
}

// command sends a SyncPlayCommand.
func (g *syncGroup) command(from accounts.ID, to broadcast, command SendCommand) {
	g.send(from, to, "SyncPlayCommand", command)
}

// newCommand is a command to carry out at lastActivity from the group's
// position.
func (g *syncGroup) newCommand(kind string) SendCommand {
	return SendCommand{GroupId: g.id.String(), PlaylistItemId: g.queue.playingEntry().String(), When: Time(g.lastActivity),
		PositionTicks: g.position, Command: kind, EmittedAt: Time(time.Now())}
}

// stateUpdate tells every member the group's state, and the request that
// led to it.
func (g *syncGroup) stateUpdate(from accounts.ID, reason string) {
	g.update(from, toAll, "StateUpdate", GroupStateUpdate{State: g.state.String(), Reason: reason})
}

// queueUpdate describes the queue, from the position playback reached.
func (g *syncGroup) queueUpdate(reason string) PlayQueueUpdate {
	start := g.position
	playing := g.state == statePlaying
	if playing {
		// Playback may not have started yet: lastActivity is then ahead.
		start += max(ticks(time.Since(g.lastActivity)), 0)
	}
	return g.queue.update(reason, start, playing)
}

func (g *syncGroup) info() GroupInfoDto {
	participants := []string{}
	for _, m := range g.members {
		if !slices.Contains(participants, m.name) {
			participants = append(participants, m.name)
		}
	}
	return GroupInfoDto{GroupId: g.id.String(), GroupName: g.name, State: g.state.String(), Participants: participants,
		LastUpdatedAt: Time(time.Now())}
}

// setState changes the group's state, the next one starting afresh.
func (g *syncGroup) setState(state groupState) {
	g.state = state
	switch state {
	case stateWaiting:
		g.resumePlaying, g.initialStateSet, g.positionJumped = false, false, false
	case statePlaying:
		g.ignoreBuffering = false
	}
	if state != stateWaiting {
		g.setWaitDeadline(time.Time{})
	}
}

// wait starts waiting, unless the group already does.
func (g *syncGroup) wait(prev groupState) {
	if prev != stateWaiting {
		g.setState(stateWaiting)
	}
}

// waitingSince remembers the state a wait began in.
func (g *syncGroup) waitingSince(prev groupState) {
	if !g.initialStateSet {
		g.initialState, g.initialStateSet = prev, true
	}
}

// fallBack returns to the state before a wait that changed nothing.
func (g *syncGroup) fallBack(prev groupState) {
	switch prev {
	case statePlaying, statePaused:
		g.setState(prev)
	default:
		g.setState(stateIdle)
	}
}

// restart plays the current item from its start.
func (g *syncGroup) restart() {
	g.position = 0
	g.lastActivity = time.Now()
}

// catchUp moves the position on by the time played since lastActivity.
func (g *syncGroup) catchUp() {
	now := time.Now()
	g.position += max(ticks(now.Sub(g.lastActivity)), 0)
	g.lastActivity = now
}

func (g *syncGroup) highestPing() int64 {
	if len(g.members) == 0 {
		return defaultPing
	}
	highest := int64(0)
	for _, m := range g.members {
		highest = max(highest, m.ping)
	}
	return highest
}

func (g *syncGroup) setBuffering(device accounts.ID, buffering bool) {
	if m := g.member(device); m != nil {
		m.buffering = buffering
	}
	g.updateWaitDeadline(false)
}

func (g *syncGroup) setAllBuffering(buffering bool) {
	for _, m := range g.members {
		m.buffering = buffering
	}
	// Making everyone buffer starts a new wait.
	g.updateWaitDeadline(buffering)
}

// isBuffering reports whether a member the group waits for is buffering.
func (g *syncGroup) isBuffering() bool {
	return slices.ContainsFunc(g.members, func(m *groupMember) bool { return m.buffering && !m.ignoreWait })
}

func (g *syncGroup) setIgnoreWait(device accounts.ID, ignore bool) {
	if m := g.member(device); m != nil {
		m.ignoreWait = ignore
	}
	g.updateWaitDeadline(false)
}

// updateWaitDeadline bounds a wait for buffering members. A running
// deadline covers the whole wait: members reporting buffering again do not
// push it back.
func (g *syncGroup) updateWaitDeadline(startNew bool) {
	if g.state != stateWaiting || !g.isBuffering() {
		g.setWaitDeadline(time.Time{})
		return
	}
	if g.waitDeadline.IsZero() || startNew {
		g.setWaitDeadline(time.Now().Add(g.sp.waitTimeout))
	}
}

func (g *syncGroup) setWaitDeadline(deadline time.Time) {
	if g.waitTimer != nil {
		g.waitTimer.Stop()
		g.waitTimer = nil
	}
	g.waitDeadline = deadline
	if !deadline.IsZero() {
		g.waitTimer = time.AfterFunc(time.Until(deadline), g.waitTimedOut)
	}
}

// waitTimedOut gives up waiting for members that stay buffering: playback
// starts without them if it was to, else the group pauses.
func (g *syncGroup) waitTimedOut() {
	g.mu.Lock()
	defer g.unlock()
	if g.waitDeadline.IsZero() || time.Now().Before(g.waitDeadline) {
		return
	}
	g.waitTimer, g.waitDeadline = nil, time.Time{}
	if g.state != stateWaiting {
		return
	}
	i := slices.IndexFunc(g.members, func(m *groupMember) bool { return m.buffering && !m.ignoreWait })
	if i < 0 {
		return
	}
	// What follows goes to the whole group: any of the members waited for
	// may act for it.
	from := g.members[i].device
	if g.resumePlaying {
		g.waitingUnpause(from, stateWaiting)
		return
	}
	g.setAllBuffering(false)
	g.setState(statePaused)
	g.command(from, toAll, g.newCommand("Pause"))
	g.stateUpdate(from, "Pause")
}

// create starts the group with its creator. A creator playing a title
// brings it: the group waits for the creator to be ready, and goes on
// playing or paused as the creator was.
func (g *syncGroup) create(creator *groupMember, playing *nowPlayingTitle) {
	g.members = append(g.members, creator)
	g.restart()
	if playing != nil {
		g.queue.reset()
		g.queue.set([]accounts.ID{playing.item})
		g.queue.playItem(playing.item)
		g.position = playing.position
		g.setState(stateWaiting)
		g.resumePlaying = !playing.paused
	}
	g.update(creator.device, toSession, "GroupJoined", g.info())
	g.sessionJoined(creator.device)
}

// join adds a session to the group, or lets a member that joins again
// learn the group's state anew.
func (g *syncGroup) join(member *groupMember) {
	if g.member(member.device) == nil {
		g.members = append(g.members, member)
	}
	g.update(member.device, toSession, "GroupJoined", g.info())
	g.update(member.device, toOthers, "UserJoined", member.name)
	g.sessionJoined(member.device)
}

// leave takes a member out of the group.
func (g *syncGroup) leave(member *groupMember) {
	g.sessionLeaving(member.device)
	g.members = slices.DeleteFunc(g.members, func(m *groupMember) bool { return m == member })
	g.updateWaitDeadline(false)
	// Jellyfin names the group in its usual form here, with hyphens.
	g.update(member.device, toSession, "GroupLeft", hyphenated(g.id))
	g.update(member.device, toOthers, "UserLeft", member.name)
}

func (g *syncGroup) sessionJoined(from accounts.ID) {
	switch prev := g.state; prev {
	case stateIdle:
		g.stopCommand(from, prev)
	case statePaused, statePlaying, stateWaiting:
		g.wait(prev)
		g.waitingSince(prev)
		if prev == statePlaying {
			g.resumePlaying = true
			g.catchUp()
		}
		g.update(from, toSession, "PlayQueue", g.queueUpdate("NewPlaylist"))
		g.setBuffering(from, true)
		g.command(from, toReady, g.newCommand("Pause"))
	}
}

func (g *syncGroup) sessionLeaving(from accounts.ID) {
	if g.state != stateWaiting {
		return
	}
	g.waitingSince(stateWaiting)
	g.setBuffering(from, false)
	if g.isBuffering() {
		return
	}
	// The member the group waited for leaves.
	if g.resumePlaying {
		g.setState(statePlaying)
		g.playingUnpause(from, stateWaiting)
	} else {
		g.setState(statePaused)
	}
}

// handle applies a member's request, as the group's state calls for.
func (g *syncGroup) handle(from accounts.ID, r syncRequest) {
	prev := g.state
	switch r.kind {
	case "Play":
		g.wait(prev)
		g.waitingPlay(from, prev, r)
	case "SetPlaylistItem":
		g.wait(prev)
		g.waitingSetPlaylistItem(from, prev, r)
	case "RemoveFromPlaylist":
		g.removeFromPlaylist(from, r)
	case "MovePlaylistItem":
		if g.queue.move(r.entry, r.index) {
			g.update(from, toAll, "PlayQueue", g.queueUpdate("MoveItem"))
		}
	case "Queue":
		g.addToQueue(from, r)
	case "Unpause":
		switch prev {
		case stateIdle:
			g.setState(stateWaiting)
			g.waitingUnpause(from, prev)
		case statePaused:
			g.setState(statePlaying)
			g.playingUnpause(from, prev)
		case statePlaying:
			g.playingUnpause(from, prev)
		case stateWaiting:
			g.waitingUnpause(from, prev)
		}
	case "Pause":
		switch prev {
		case stateIdle:
			g.stopCommand(from, prev)
		case statePaused:
			g.pausedPause(from, prev)
		case statePlaying:
			g.setState(statePaused)
			g.pausedPause(from, prev)
		case stateWaiting:
			g.waitingSince(prev)
			g.resumePlaying = false
			g.stateUpdate(from, "Pause")
		}
	case "Stop":
		if prev != stateIdle {
			g.setState(stateIdle)
		}
		g.stopCommand(from, prev)
	case "Seek":
		if prev == stateIdle {
			g.stopCommand(from, prev)
			return
		}
		g.wait(prev)
		g.waitingSeek(from, prev, r)
	case "Buffer":
		switch prev {
		case stateIdle:
			g.stopCommand(from, prev)
		case statePlaying:
			if g.ignoreBuffering {
				return
			}
			fallthrough
		default:
			g.wait(prev)
			g.waitingBuffer(from, prev, r)
		}
	case "Ready":
		switch prev {
		case stateIdle:
			g.stopCommand(from, prev)
		case statePaused:
			g.pausedReady(from, prev)
		case statePlaying:
			g.playingReady(from, prev)
		case stateWaiting:
			g.waitingReady(from, r)
		}
	case "NextItem", "PreviousItem":
		g.wait(prev)
		g.waitingSkip(from, prev, r)
	case "SetRepeatMode":
		g.queue.setRepeat(r.mode)
		g.update(from, toAll, "PlayQueue", g.queueUpdate("RepeatMode"))
	case "SetShuffleMode":
		g.queue.setShuffle(r.mode == "Shuffle")
		g.update(from, toAll, "PlayQueue", g.queueUpdate("ShuffleMode"))
	case "Ping":
		// Latencies delay the start of playback, so that it reaches all.
		if m := g.member(from); m != nil {
			m.ping = min(max(r.ping, 0), maxPing)
		}
	case "IgnoreWait":
		g.setIgnoreWait(from, r.ignoreWait)
		if prev != stateWaiting || g.isBuffering() {
			return
		}
		// The member the group waited for stopped following playback.
		if g.resumePlaying {
			g.setState(statePlaying)
			g.playingUnpause(from, prev)
		} else {
			g.setState(statePaused)
		}
	}
}

// stopCommand stops the whole group when it just stopped, else reminds
// the member asking that it is stopped.
func (g *syncGroup) stopCommand(from accounts.ID, prev groupState) {
	to := toSession
	if prev != stateIdle {
		to = toAll
	}
	g.command(from, to, g.newCommand("Stop"))
}

func (g *syncGroup) pausedPause(from accounts.ID, prev groupState) {
	if prev == statePaused {
		// The member got lost: it is told the group's state.
		g.command(from, toSession, g.newCommand("Pause"))
		return
	}
	g.catchUp()
	g.command(from, toAll, g.newCommand("Pause"))
	g.stateUpdate(from, "Pause")
}

func (g *syncGroup) pausedReady(from accounts.ID, prev groupState) {
	switch prev {
	case statePaused:
		g.command(from, toSession, g.newCommand("Pause"))
	case stateWaiting:
		g.command(from, toAll, g.newCommand("Pause"))
		g.stateUpdate(from, "Ready")
	}
}

// playingUnpause starts playback a little ahead, so that the command
// reaches every member in time, the latencies they report allowing.
func (g *syncGroup) playingUnpause(from accounts.ID, prev groupState) {
	if prev == statePlaying {
		g.command(from, toSession, g.newCommand("Unpause"))
		return
	}
	delay := max(g.highestPing()*2, defaultPing)
	g.lastActivity = time.Now().Add(time.Duration(delay) * time.Millisecond)
	g.command(from, toAll, g.newCommand("Unpause"))
	g.stateUpdate(from, "Unpause")
}

func (g *syncGroup) playingReady(from accounts.ID, prev groupState) {
	switch prev {
	case statePlaying:
		g.command(from, toSession, g.newCommand("Unpause"))
	case stateWaiting:
		g.stateUpdate(from, "Ready")
	}
}

// waitingPlay replaces the queue. Jellyfin takes queues of any length;
// Polyfin refuses those longer than maxQueueEntries.
func (g *syncGroup) waitingPlay(from accounts.ID, prev groupState, r syncRequest) {
	g.waitingSince(prev)
	g.resumePlaying = true
	ok := r.allowed && r.index >= 0 && r.index < len(r.items)
	g.positionJumped = ok
	if !ok {
		g.fallBack(prev)
		return
	}
	g.queue.reset()
	g.queue.set(r.items)
	g.queue.playIndex(r.index)
	g.position = r.position
	g.lastActivity = time.Now()
	g.update(from, toAll, "PlayQueue", g.queueUpdate("NewPlaylist"))
	g.setAllBuffering(true)
}

func (g *syncGroup) waitingSetPlaylistItem(from accounts.ID, prev groupState, r syncRequest) {
	g.waitingSince(prev)
	g.resumePlaying = true
	found := g.queue.playEntry(r.entry)
	g.restart()
	g.positionJumped = found
	if !found {
		g.fallBack(prev)
		return
	}
	g.update(from, toAll, "PlayQueue", g.queueUpdate("SetCurrentItem"))
	g.setAllBuffering(true)
}

func (g *syncGroup) removeFromPlaylist(from accounts.ID, r syncRequest) {
	var removedPlaying bool
	if r.clearPlaylist {
		g.queue.clear(!r.clearPlaying)
		removedPlaying = r.clearPlaying
	} else {
		removedPlaying = g.queue.remove(r.entries)
	}
	if removedPlaying {
		g.restart()
	}
	g.update(from, toAll, "PlayQueue", g.queueUpdate("RemoveItems"))
	if removedPlaying && g.queue.playing < 0 {
		// Nothing is left to play.
		prev := g.state
		g.setState(stateIdle)
		g.stopCommand(from, prev)
	}
}

// addToQueue queues items, unless the queue would grow longer than
// maxQueueEntries.
func (g *syncGroup) addToQueue(from accounts.ID, r syncRequest) {
	if !r.allowed || len(g.queue.sorted)+len(r.items) > maxQueueEntries {
		return
	}
	reason := "Queue"
	if r.queueNext {
		reason = "QueueNext"
		g.queue.addNext(r.items)
	} else {
		g.queue.add(r.items)
	}
	g.update(from, toAll, "PlayQueue", g.queueUpdate(reason))
}

func (g *syncGroup) waitingUnpause(from accounts.ID, prev groupState) {
	g.waitingSince(prev)
	switch {
	case prev == stateIdle:
		g.resumePlaying = true
		g.restart()
		g.positionJumped = true
		g.update(from, toAll, "PlayQueue", g.queueUpdate("NewPlaylist"))
		g.setAllBuffering(true)
	case g.resumePlaying:
		// Playback is forced to start without the members not ready, who
		// may not stop it until the group's state changes.
		g.setAllBuffering(false)
		g.setState(statePlaying)
		g.ignoreBuffering = true
		g.playingUnpause(from, stateWaiting)
	default:
		// The group would have paused once ready; it plays instead.
		g.resumePlaying = true
		g.stateUpdate(from, "Unpause")
	}
}

// waitingSeek moves the group's position: every member seeks, and the
// group waits for them. Jellyfin also caps positions at the item's
// runtime. Polyfin knows a title's runtime from its metadata only, which
// rounds it and may not be the version's: positions are only kept within
// maxPositionTicks, so that a seek into the last minutes of a longer
// version holds.
func (g *syncGroup) waitingSeek(from accounts.ID, prev groupState, r syncRequest) {
	g.waitingSince(prev)
	switch prev {
	case statePlaying:
		g.resumePlaying = true
	case statePaused:
		g.resumePlaying = false
	}
	g.position = r.position
	g.lastActivity = time.Now()
	g.positionJumped = true
	g.command(from, toAll, g.newCommand("Seek"))
	g.setAllBuffering(true)
	g.stateUpdate(from, "Seek")
}

// waitingBuffer has the group wait for a member that buffers, the others
// pausing meanwhile.
func (g *syncGroup) waitingBuffer(from accounts.ID, prev groupState, r syncRequest) {
	g.waitingSince(prev)
	if r.entry != g.queue.playingEntry() {
		// The member plays another item than the group's.
		g.update(from, toSession, "PlayQueue", g.queueUpdate("SetCurrentItem"))
		g.setBuffering(from, true)
		return
	}
	switch prev {
	case statePlaying:
		g.resumePlaying = true
		g.setBuffering(from, true)
		g.catchUp()
		g.command(from, toReady, g.newCommand("Pause"))
	case statePaused:
		g.resumePlaying = false
		g.setBuffering(from, true)
		g.command(from, toSession, g.newCommand("Pause"))
	case stateWaiting:
		g.setBuffering(from, true)
		if !g.resumePlaying {
			g.command(from, toSession, g.newCommand("Pause"))
		}
	}
	g.stateUpdate(from, "Buffer")
}

// waitingReady counts a member ready at the position it reports. Once
// everyone is, the group plays or pauses as it was to; a member too far
// from the group's position is told to seek first.
func (g *syncGroup) waitingReady(from accounts.ID, r syncRequest) {
	g.waitingSince(stateWaiting)
	if r.entry != g.queue.playingEntry() {
		g.update(from, toSession, "PlayQueue", g.queueUpdate("SetCurrentItem"))
		g.setBuffering(from, true)
		return
	}
	// The member's position moved on since it reported it, if it plays.
	// A report too far from the server's clock is not trusted.
	now := time.Now()
	elapsed := now.Sub(r.when)
	if elapsed.Abs() > timeSyncOffset*time.Millisecond || !r.playing {
		elapsed = 0
	}
	reported := r.position
	delay := g.position - (reported + ticks(elapsed))
	if !g.resumePlaying {
		if abs(g.position-reported) > maxPlaybackOffset*ticksPerMillisecond {
			g.setBuffering(from, true)
			g.command(from, toSession, g.newCommand("Seek"))
			g.stateUpdate(from, "Ready")
			return
		}
		g.setBuffering(from, false)
		if g.isBuffering() {
			return
		}
		initial := g.initialState
		g.setState(statePaused)
		switch initial {
		case statePlaying:
			// A pause was asked while waiting.
			g.pausedPause(from, stateWaiting)
		case statePaused:
			g.pausedReady(from, stateWaiting)
		}
		return
	}
	// A member playing on after buffering may lag behind, unless it was to
	// seek.
	maxOffset := int64(maxPlaybackOffset)
	if r.playing && !g.positionJumped {
		maxOffset = maxCatchUpOffset
	}
	if abs(delay) > maxOffset*ticksPerMillisecond {
		g.setBuffering(from, true)
		g.command(from, toSession, g.newCommand("Seek"))
		g.stateUpdate(from, "Ready")
		return
	}
	g.setBuffering(from, false)
	if g.isBuffering() {
		// The member pauses once it reaches the group's position.
		command := g.newCommand("Pause")
		command.When = Time(now.Add(time.Duration(delay) * 100))
		g.command(from, toSession, command)
		return
	}
	latency := g.highestPing() * 2 * ticksPerMillisecond
	if delay > latency {
		// The member that buffered is behind: the others resume as it
		// catches up.
		g.lastActivity = now.Add(time.Duration(delay) * 100)
		to := toOthers
		if !r.playing {
			to = toAll
		}
		g.command(from, to, g.newCommand("Unpause"))
	} else {
		delay = max(latency, defaultPing*ticksPerMillisecond)
		g.lastActivity = now.Add(time.Duration(delay) * 100)
		g.command(from, toAll, g.newCommand("Unpause"))
	}
	g.setState(statePlaying)
	g.playingReady(from, stateWaiting)
}

func (g *syncGroup) waitingSkip(from accounts.ID, prev groupState, r syncRequest) {
	g.waitingSince(prev)
	g.resumePlaying = true
	// A member that does not know what plays would skip twice.
	if r.entry != g.queue.playingEntry() {
		return
	}
	reason, moved := "NextItem", false
	if r.kind == "NextItem" {
		moved = g.queue.next()
	} else {
		reason, moved = "PreviousItem", g.queue.previous()
	}
	g.positionJumped = moved
	if !moved {
		g.fallBack(prev)
		return
	}
	g.restart()
	g.update(from, toAll, "PlayQueue", g.queueUpdate(reason))
	g.setAllBuffering(true)
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
