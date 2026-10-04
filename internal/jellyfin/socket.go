package jellyfin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

const (
	// socketLost is how long an app's socket may go without a keep-alive
	// before it is dropped, as Jellyfin does. Apps learn it when they
	// connect, and send a keep-alive about every half of it.
	socketLost = 60 * time.Second
	// socketBatch is how long changes to a user's data gather before they
	// are pushed, counted from the first: a season marked played goes out
	// as one message.
	socketBatch = 500 * time.Millisecond
	// socketWrite bounds the delivery of a message to an app that stopped
	// reading.
	socketWrite = 10 * time.Second
)

// socketMessage is a message on an app's socket, in Jellyfin's form. Those
// the server sends carry an ID of their own.
type socketMessage struct {
	Data        any    `json:",omitempty"`
	MessageId   string `json:",omitempty"`
	MessageType string
}

// userDataChange is the data of a UserDataChanged message.
type userDataChange struct {
	UserId       string
	UserDataList []UserItemData
}

// socketPayload encodes a message of type kind, carrying data unless nil.
func socketPayload(kind string, data any) []byte {
	var id [16]byte
	_, _ = rand.Read(id[:])
	payload, _ := json.Marshal(socketMessage{Data: data, MessageId: hex.EncodeToString(id[:]), MessageType: kind})
	return payload
}

// sockets are the sockets users' apps keep open, and the changes to users'
// data waiting to be pushed to them. The same sockets are also known by the
// device that opened them, in the order they opened, for the commands one
// app sends another.
type sockets struct {
	mu      sync.Mutex
	open    map[accounts.ID]map[*websocket.Conn]bool
	devices map[accounts.ID][]*websocket.Conn
	pending map[accounts.ID]*changedData
}

func newSockets() *sockets {
	return &sockets{
		open:    map[accounts.ID]map[*websocket.Conn]bool{},
		devices: map[accounts.ID][]*websocket.Conn{},
		pending: map[accounts.ID]*changedData{},
	}
}

func (s *sockets) add(user, device accounts.ID, conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[user] == nil {
		s.open[user] = map[*websocket.Conn]bool{}
	}
	s.open[user][conn] = true
	s.devices[device] = append(s.devices[device], conn)
}

// remove forgets a socket, and reports whether its device holds no other.
func (s *sockets) remove(user, device accounts.ID, conn *websocket.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.open[user], conn)
	if len(s.open[user]) == 0 {
		delete(s.open, user)
	}
	s.devices[device] = slices.DeleteFunc(s.devices[device], func(open *websocket.Conn) bool { return open == conn })
	if len(s.devices[device]) == 0 {
		delete(s.devices, device)
		return true
	}
	return false
}

// latest is the socket a device opened last, or nil when it holds none.
// An app reconnecting, or open in two browser tabs sharing a device, holds
// several: like Jellyfin, a command goes to one of them only, so that it is
// not carried out twice.
func (s *sockets) latest(device accounts.ID) *websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	open := s.devices[device]
	if len(open) == 0 {
		return nil
	}
	return open[len(open)-1]
}

// close closes the sockets of a device, whose serving then ends.
func (s *sockets) close(device accounts.ID) {
	s.mu.Lock()
	open := slices.Clone(s.devices[device])
	s.mu.Unlock()
	for _, conn := range open {
		_ = conn.CloseNow()
	}
}

// changedData is what changed in a user's data since the last push: the
// items, in the order they changed, each followed by the season or series
// above it, whose counts of played episodes changed with it. Parents are
// known by their ID only, and looked up when pushed.
type changedData struct {
	user  accounts.User
	order []accounts.ID
	items map[accounts.ID]*library.Item
}

func (c *changedData) add(id accounts.ID, item *library.Item) {
	if id == (accounts.ID{}) {
		return
	}
	known, listed := c.items[id]
	if !listed {
		c.order = append(c.order, id)
	}
	if known == nil {
		c.items[id] = item
	}
}

// isSocket reports whether r asks to open a WebSocket. Jellyfin opens one
// on any path; apps ask on /socket.
func isSocket(r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, value := range r.Header.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// socket serves an app's WebSocket as Jellyfin does. The app learns how
// long a silent socket lasts, its keep-alives are answered, a socket left
// silent that long is dropped, the user's data is pushed to it as it
// changes, and the commands other apps send its device reach it (see
// remote.go), as do the messages of its SyncPlay group. Other messages,
// which ask for what Polyfin does not offer, such as the sessions of the
// server for its dashboard, are left unanswered.
func (h *Handler) socket(w http.ResponseWriter, r *http.Request) {
	c, ok, err := h.signedInCaller(r)
	switch {
	case err != nil:
		h.internalError(w, r, err)
		return
	case !ok || c.APIKey != nil:
		// Jellyfin refuses a socket without a valid token with a 403,
		// where other requests get a 401. Polyfin's sockets are those of
		// signed-in devices: an API key, which has none, is refused too.
		processingError(w, http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Apps authenticate with a token in the request, never with a cookie
		// that another site's page could send: any origin may connect, as
		// with the rest of the API.
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	h.sockets.add(c.User.ID, c.Device.ID, conn)
	defer func() {
		// The socket closes first, so that no message still on its way to
		// it waits on it. Like Jellyfin, a session whose last socket
		// closes has ended: it leaves its SyncPlay group.
		_ = conn.CloseNow()
		if h.sockets.remove(c.User.ID, c.Device.ID, conn) {
			h.syncPlay.sessionLeft(c.Device.ID)
		}
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	kinds := make(chan string)
	go func() {
		defer close(kinds)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message socketMessage
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			select {
			case kinds <- message.MessageType:
			case <-ctx.Done():
				return
			}
		}
	}()
	send := func(kind string, data any) bool {
		ctx, cancel := context.WithTimeout(ctx, socketWrite)
		defer cancel()
		return conn.Write(ctx, websocket.MessageText, socketPayload(kind, data)) == nil
	}
	lost := int(socketLost / time.Second)
	if !send("ForceKeepAlive", lost) {
		return
	}
	alive := time.Now()
	// Like Jellyfin, sockets are checked five times in the time they last,
	// and reminded once three quarters of it went by in silence.
	check := time.NewTicker(socketLost / 5)
	defer check.Stop()
	for {
		select {
		case kind, open := <-kinds:
			if !open {
				return
			}
			if strings.EqualFold(kind, "KeepAlive") {
				alive = time.Now()
				if !send("KeepAlive", nil) {
					return
				}
			}
		case <-check.C:
			switch silent := time.Since(alive); {
			case silent >= socketLost:
				return
			case silent > socketLost*3/4:
				if !send("ForceKeepAlive", lost) {
					return
				}
			}
		}
	}
}

// userDataChanged has the user's data of items, and of the season or series
// above each, pushed to the sockets the user's apps keep open, so that they
// show it as it is. Changes gather for a moment first.
func (h *Handler) userDataChanged(user accounts.User, items []library.Item) {
	s := h.sockets
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.open[user.ID]) == 0 {
		return
	}
	changed := s.pending[user.ID]
	if changed == nil {
		changed = &changedData{user: user, items: map[accounts.ID]*library.Item{}}
		s.pending[user.ID] = changed
		time.AfterFunc(socketBatch, func() { h.pushUserData(user.ID) })
	}
	for _, item := range items {
		changed.add(item.ID, &item)
		switch {
		case item.Kind == library.KindEpisode && item.SeasonID != (accounts.ID{}):
			changed.add(item.SeasonID, nil)
		case item.Kind == library.KindEpisode || item.Kind == library.KindSeason:
			changed.add(item.SeriesID, nil)
		}
	}
}

// pushUserData sends what changed in a user's data to the user's sockets.
func (h *Handler) pushUserData(user accounts.ID) {
	s := h.sockets
	s.mu.Lock()
	changed := s.pending[user]
	delete(s.pending, user)
	conns := make([]*websocket.Conn, 0, len(s.open[user]))
	for conn := range s.open[user] {
		conns = append(conns, conn)
	}
	s.mu.Unlock()
	if changed == nil || len(conns) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), socketWrite)
	defer cancel()
	items := make([]library.Item, 0, len(changed.order))
	for _, id := range changed.order {
		item := changed.items[id]
		if item == nil {
			found, err := h.Library.Item(ctx, changed.user, id)
			if err != nil {
				continue
			}
			item = &found
		}
		items = append(items, *item)
	}
	state, err := h.userState(ctx, changed.user, items)
	if err != nil {
		h.Logger.Warn("Changed user data could not be pushed to apps", "error", err)
		return
	}
	change := userDataChange{UserId: user.String(), UserDataList: make([]UserItemData, 0, len(items))}
	for _, item := range items {
		change.UserDataList = append(change.UserDataList, state.of(item))
	}
	payload := socketPayload("UserDataChanged", change)
	for _, conn := range conns {
		_ = conn.Write(ctx, websocket.MessageText, payload)
	}
}
