package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Invitations to watch together. The web player's "Watch together" lists
// the people its SyncPlay group may invite, then invites some of them; each
// invitee's web player shows a prompt to join it (see the tenth part of
// webclient.js), which Join answers with Jellyfin's POST /SyncPlay/Join.
// They live in memory, like the groups.
const (
	// inviteMessage is the type of the socket message that tells an
	// invitee's web players of an invitation, and of its end.
	inviteMessage = "PolyfinSyncPlayInvite"
	// inviteLasts is how long an invitation lasts, unless its group ends
	// before.
	inviteLasts = 5 * time.Minute
	// inviteEvery is how often a user may be invited, by anyone.
	inviteEvery = time.Minute
)

// The states a PolyfinSyncPlayInvite tells: an invitation sent, then how
// it ended, which closes its prompt.
const (
	inviteSent     = "Invited"
	inviteJoined   = "Joined"
	inviteDeclined = "Declined"
	inviteExpired  = "Expired"
	inviteEnded    = "Ended"
)

// InvitePush is the data of a PolyfinSyncPlayInvite message. An invitation
// sent names its group, who sent it, the title the group plays, and until
// when it lasts; the message telling its end gives its Id, its GroupId and
// its State only.
type InvitePush struct {
	Id         string
	State      string
	GroupId    string
	GroupName  string `json:",omitempty"`
	HostName   string `json:",omitempty"`
	ItemId     string `json:",omitempty"`
	ItemName   string `json:",omitempty"`
	SeriesName string `json:",omitempty"`
	ExpiresAt  *Time  `json:",omitempty"`
}

// SyncPlayInvitee is a user a group may invite.
type SyncPlayInvitee struct {
	Id   string
	Name string
}

// SyncPlayInvitees answers GET /Polyfin/SyncPlay/Invitees: the caller's
// group, and the users it may invite, by name.
type SyncPlayInvitees struct {
	GroupId string
	Items   []SyncPlayInvitee
}

type syncPlayInvite struct {
	id, group, invitee accounts.ID
	timer              *time.Timer
}

// syncPlayInvites holds the invitations under way, and when each user was
// last invited.
type syncPlayInvites struct {
	mu sync.Mutex
	// lasts is how long an invitation lasts, inviteLasts but in tests.
	lasts time.Duration
	open  map[accounts.ID]*syncPlayInvite
	sent  map[accounts.ID]time.Time
}

func newSyncPlayInvites() *syncPlayInvites {
	return &syncPlayInvites{lasts: inviteLasts, open: map[accounts.ID]*syncPlayInvite{}, sent: map[accounts.ID]time.Time{}}
}

// take removes the invitations matching, and gives them, their timers
// stopped.
func (in *syncPlayInvites) take(match func(*syncPlayInvite) bool) []*syncPlayInvite {
	in.mu.Lock()
	defer in.mu.Unlock()
	var taken []*syncPlayInvite
	for id, invite := range in.open {
		if match(invite) {
			invite.timer.Stop()
			delete(in.open, id)
			taken = append(taken, invite)
		}
	}
	return taken
}

// syncPlayInviteRoutes serves the invitations of the caller's group.
func (h *Handler) syncPlayInviteRoutes(rt *router) {
	rt.handle(http.MethodGet, "/Polyfin/SyncPlay/Invitees", h.authenticated(h.syncPlayAccess(syncPlayInGroup, h.syncPlayInvitees)))
	rt.handle(http.MethodPost, "/Polyfin/SyncPlay/Invitations", h.authenticated(h.syncPlayAccess(syncPlayInGroup, h.syncPlayInvite)))
	rt.handle(http.MethodPost, "/Polyfin/SyncPlay/Invitations/{inviteId}/Decline", h.authenticated(h.syncPlayDecline))
}

// invitees lists the users g may invite, by name: other users, neither
// disabled nor ended, nor kept off sign-in screens, who may join groups,
// are inside their allowed hours, have a web player open, whose sockets
// passed their network rules, and may play every title of g's queue.
// Members of g are left out.
func (h *Handler) invitees(ctx context.Context, caller accounts.ID, g *syncGroup) []accounts.User {
	g.mu.Lock()
	items, members := g.queue.items(), g.users()
	g.mu.Unlock()
	now := h.now()
	var users []accounts.User
	for _, id := range h.sockets.webUsers() {
		if id == caller || slices.Contains(members, id) {
			continue
		}
		user, err := h.Accounts.User(ctx, id)
		if err != nil || user.Refused(now) || user.IsHidden || !user.SyncPlay.MayJoinGroups() || h.outsideHours(user) ||
			!h.canPlay(ctx, id, items) {
			continue
		}
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b accounts.User) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return users
}

func (h *Handler) syncPlayInvitees(w http.ResponseWriter, r *http.Request) {
	c := callerFrom(r.Context())
	g := h.syncPlay.groupOf(c.Device.ID)
	if g == nil {
		// Another of the user's sessions is in a group, not this one.
		w.WriteHeader(http.StatusForbidden)
		return
	}
	answer := SyncPlayInvitees{GroupId: g.id.String(), Items: []SyncPlayInvitee{}}
	for _, user := range h.invitees(r.Context(), c.User.ID, g) {
		answer.Items = append(answer.Items, SyncPlayInvitee{Id: user.ID.String(), Name: user.Name})
	}
	writeJSON(w, http.StatusOK, answer)
}

// syncPlayInvite invites a user to the caller's group, which the request
// names: one that is no longer the session's group is refused. A user
// the group may not invite is not found, and one invited less than a
// minute ago, by anyone, waits.
func (h *Handler) syncPlayInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupId syncPlayID
		UserId  syncPlayID
	}
	if !requiredBody(w, r, "requestData", &body) {
		return
	}
	c := callerFrom(r.Context())
	g := h.syncPlay.groupOf(c.Device.ID)
	if g == nil || g.id != accounts.ID(body.GroupId) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var invitee *accounts.User
	for _, user := range h.invitees(r.Context(), c.User.ID, g) {
		if user.ID == accounts.ID(body.UserId) {
			invitee = &user
			break
		}
	}
	if invitee == nil {
		notFoundProblem(w)
		return
	}
	g.mu.Lock()
	groupName, playing := g.name, g.queue.items()
	if current, ok := g.queue.current(); ok {
		playing = []accounts.ID{current.item}
	}
	g.mu.Unlock()

	in := h.invites
	in.mu.Lock()
	now := h.now()
	if wait := in.sent[invitee.ID].Add(inviteEvery).Sub(now); wait > 0 {
		in.mu.Unlock()
		// Whole seconds, rounded up.
		w.Header().Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
		processingError(w, http.StatusTooManyRequests)
		return
	}
	for id, last := range in.sent {
		if now.Sub(last) >= inviteEvery {
			delete(in.sent, id)
		}
	}
	in.sent[invitee.ID] = now
	// A new invitation to the same group replaces the one before.
	for id, before := range in.open {
		if before.group == g.id && before.invitee == invitee.ID {
			before.timer.Stop()
			delete(in.open, id)
		}
	}
	invite := &syncPlayInvite{id: randomGUID(), group: g.id, invitee: invitee.ID}
	in.open[invite.id] = invite
	lasts := in.lasts
	invite.timer = time.AfterFunc(lasts, func() { h.inviteEnded(invite, inviteExpired) })
	in.mu.Unlock()
	// The group may have ended meanwhile, before the invitation was there
	// to end with it.
	if !h.syncPlay.exists(g) {
		h.invites.take(func(i *syncPlayInvite) bool { return i == invite })
		notFoundProblem(w)
		return
	}

	expires := Time(now.Add(lasts))
	push := InvitePush{Id: invite.id.String(), State: inviteSent, GroupId: g.id.String(), GroupName: groupName,
		HostName: c.User.Name, ExpiresAt: &expires}
	if len(playing) > 0 {
		if title, err := h.title(r.Context(), *invitee, playing[0]); err == nil {
			push.ItemId, push.ItemName, push.SeriesName = playing[0].String(), title.Name, title.SeriesName
		}
	}
	h.pushInvite(invitee.ID, push)
	w.WriteHeader(http.StatusNoContent)
}

// syncPlayDecline ends an invitation of the caller, who chose Not now.
func (h *Handler) syncPlayDecline(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGUID(r.PathValue("inviteId"))
	user := callerFrom(r.Context()).User.ID
	if ok {
		for _, invite := range h.invites.take(func(i *syncPlayInvite) bool { return i.id == id && i.invitee == user }) {
			h.pushInvite(user, InvitePush{Id: invite.id.String(), State: inviteDeclined, GroupId: invite.group.String()})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	notFoundProblem(w)
}

// inviteEnded ends an invitation, as it expires, unless it ended already.
func (h *Handler) inviteEnded(invite *syncPlayInvite, state string) {
	if len(h.invites.take(func(i *syncPlayInvite) bool { return i == invite })) > 0 {
		h.pushInvite(invite.invitee, InvitePush{Id: invite.id.String(), State: state, GroupId: invite.group.String()})
	}
}

// joinedGroup ends a user's invitations to a group one of their sessions
// joined, which closes their prompts on the user's other devices.
func (h *Handler) joinedGroup(user, group accounts.ID) {
	for _, invite := range h.invites.take(func(i *syncPlayInvite) bool { return i.invitee == user && i.group == group }) {
		h.pushInvite(user, InvitePush{Id: invite.id.String(), State: inviteJoined, GroupId: group.String()})
	}
}

// groupEnded ends the invitations to a group that ended. It is called
// with the group's locks held: the invitees are told from a goroutine.
func (h *Handler) groupEnded(group accounts.ID) {
	ended := h.invites.take(func(i *syncPlayInvite) bool { return i.group == group })
	if len(ended) == 0 {
		return
	}
	go func() {
		for _, invite := range ended {
			h.pushInvite(invite.invitee, InvitePush{Id: invite.id.String(), State: inviteEnded, GroupId: group.String()})
		}
	}()
}

// pushInvite tells a user's web players of an invitation. Other apps are
// told nothing: they would not know the message (see webSockets).
func (h *Handler) pushInvite(user accounts.ID, push InvitePush) {
	ctx, cancel := context.WithTimeout(context.Background(), socketWrite)
	defer cancel()
	payload := socketPayload(inviteMessage, push)
	for _, conn := range h.webSockets(user) {
		_ = conn.Write(ctx, websocket.MessageText, payload)
	}
}
