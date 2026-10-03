package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// appSocket is an app's socket, its messages read as they come.
type appSocket struct {
	conn     *websocket.Conn
	messages chan socketMessage
}

func (srv testServer) openSocket(t *testing.T, token string) (*appSocket, *http.Response, error) {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", app("phone", token))
	conn, response, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(srv.url, "http")+"/socket", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, response, err
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	s := &appSocket{conn: conn, messages: make(chan socketMessage, 16)}
	go func() {
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var message socketMessage
			if json.Unmarshal(data, &message) == nil {
				s.messages <- message
			}
		}
	}()
	return s, response, nil
}

// next is the next message within a few seconds.
func (s *appSocket) next(t *testing.T) socketMessage {
	t.Helper()
	select {
	case message := <-s.messages:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
	return socketMessage{}
}

// quiet checks that no message comes for longer than changes gather.
func (s *appSocket) quiet(t *testing.T) {
	t.Helper()
	select {
	case message := <-s.messages:
		t.Errorf("unexpected %s message: %+v", message.MessageType, message.Data)
	case <-time.After(3 * socketBatch):
	}
}

// changed reads the next message as a change of the user's data.
func (s *appSocket) changed(t *testing.T) userDataChange {
	t.Helper()
	message := s.next(t)
	if message.MessageType != "UserDataChanged" || message.MessageId == "" {
		t.Fatalf("message %s %q", message.MessageType, message.MessageId)
	}
	data, _ := json.Marshal(message.Data)
	var change userDataChange
	if err := json.Unmarshal(data, &change); err != nil {
		t.Fatal(err)
	}
	return change
}

func TestAppSocketsAreKeptAliveAndToldOfTheirUsersData(t *testing.T) {
	tr := newTracking(t)
	// Jellyfin answers a socket without a valid token with a 403, and a
	// plain request to /socket finds nothing.
	if _, response, err := tr.openSocket(t, "0123456789abcdef0123456789abcdef"); err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("socket with an unknown token: %v", err)
	}
	if status, _ := tr.call(http.MethodGet, "/socket", app("tv", tr.token), nil); status != http.StatusNotFound {
		t.Errorf("plain request to /socket: %d", status)
	}

	member, _, err := tr.openSocket(t, tr.token)
	if err != nil {
		t.Fatal(err)
	}
	// The app learns how long a silent socket lasts, and its keep-alives
	// are answered.
	if message := member.next(t); message.MessageType != "ForceKeepAlive" || message.Data != float64(60) || message.MessageId == "" {
		t.Fatalf("first message: %+v", message)
	}
	if err := member.conn.Write(t.Context(), websocket.MessageText, []byte(`{"MessageType":"KeepAlive"}`)); err != nil {
		t.Fatal(err)
	}
	if message := member.next(t); message.MessageType != "KeepAlive" || message.Data != nil {
		t.Fatalf("answer to a keep-alive: %+v", message)
	}
	tr.testServer.user("other", nil)
	other, _, err := tr.openSocket(t, tr.signIn("other", "phone"))
	if err != nil {
		t.Fatal(err)
	}
	other.next(t)

	// An episode marked played comes with its season, which has one
	// episode less to play.
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])
	change := member.changed(t)
	if change.UserId != tr.user || !slices.Equal(ids(change.UserDataList), []string{tr.episodes[0], tr.seasons[0]}) {
		t.Fatalf("change of %s: %v", change.UserId, ids(change.UserDataList))
	}
	if episode, season := change.UserDataList[0], change.UserDataList[1]; !episode.Played || season.UnplayedItemCount == nil || season.Played {
		t.Errorf("episode %+v, season %+v", episode, season)
	}
	other.quiet(t)

	// Positions reported while playing are not pushed; the stop is.
	tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.movie, "PositionTicks": 27_000_000_000})
	member.quiet(t)
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": 30_000_000_000})
	change = member.changed(t)
	if list := change.UserDataList; !slices.Equal(ids(list), []string{tr.movie}) || list[0].PlaybackPositionTicks != 30_000_000_000 {
		t.Errorf("after the stop: %+v", list)
	}
}

func ids(list []UserItemData) []string {
	result := make([]string, 0, len(list))
	for _, data := range list {
		result = append(result, data.ItemId)
	}
	return result
}

func TestSocketRequestsAreToldApart(t *testing.T) {
	for _, tc := range []struct {
		method, upgrade string
		connection      []string
		socket          bool
	}{
		{"GET", "websocket", []string{"Upgrade"}, true},
		// Some browsers keep the connection alive while upgrading it.
		{"GET", "WebSocket", []string{"keep-alive, Upgrade"}, true},
		{"GET", "websocket", []string{"keep-alive", "upgrade"}, true},
		{"GET", "websocket", []string{"keep-alive"}, false},
		{"GET", "h2c", []string{"Upgrade"}, false},
		{"POST", "websocket", []string{"Upgrade"}, false},
	} {
		r, _ := http.NewRequest(tc.method, "/socket", nil)
		r.Header.Set("Upgrade", tc.upgrade)
		for _, value := range tc.connection {
			r.Header.Add("Connection", value)
		}
		if got := isSocket(r); got != tc.socket {
			t.Errorf("%s with Upgrade %q and Connection %q: %v", tc.method, tc.upgrade, tc.connection, got)
		}
	}
}
