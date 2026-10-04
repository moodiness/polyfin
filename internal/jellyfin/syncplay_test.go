package jellyfin

import (
	"bufio"
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// syncPlayApp is a session that watches with others: its sign-in and its
// socket.
type syncPlayApp struct {
	device, token string
	socket        *appSocket
}

func (s testServer) syncPlayApp(t *testing.T, user, device string) syncPlayApp {
	t.Helper()
	a := syncPlayApp{device: device, token: s.signIn(user, device)}
	var err error
	if a.socket, _, err = s.openSocket(t, a.token); err != nil {
		t.Fatal(err)
	}
	if message := a.socket.next(t); message.MessageType != "ForceKeepAlive" {
		t.Fatalf("first message of %s: %+v", device, message)
	}
	return a
}

// send sends a request as the app: no body when body is nil, an empty one
// when it is "", else body in JSON.
func (a syncPlayApp) send(t *testing.T, s testServer, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded := []byte{}
		if body != "" {
			encoded, _ = json.Marshal(body)
		}
		reader = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, s.url+path, reader)
	request.Header.Set("Authorization", app(a.device, a.token))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(response.Body)
	return response.StatusCode, answer
}

// post sends a SyncPlay request that Jellyfin answers without content.
func (a syncPlayApp) post(t *testing.T, s testServer, path string, body any) {
	t.Helper()
	if status, answer := a.send(t, s, http.MethodPost, "/SyncPlay/"+path, body); status != http.StatusNoContent {
		t.Fatalf("%s of %s: %d %s", path, a.device, status, answer)
	}
}

// next is the app's next SyncPlay message.
func (a syncPlayApp) next(t *testing.T) socketMessage {
	t.Helper()
	for {
		if message := a.socket.next(t); strings.HasPrefix(message.MessageType, "SyncPlay") {
			return message
		}
	}
}

// quiet checks that no SyncPlay message comes for a while.
func (a syncPlayApp) quiet(t *testing.T) {
	t.Helper()
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case message := <-a.socket.messages:
			if strings.HasPrefix(message.MessageType, "SyncPlay") {
				t.Errorf("unexpected message to %s: %s %+v", a.device, message.MessageType, message.Data)
			}
		case <-deadline:
			return
		}
	}
}

// ended checks that the server closed the app's socket, without telling
// it anything of SyncPlay first.
func (a syncPlayApp) ended(t *testing.T) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case message := <-a.socket.messages:
			if strings.HasPrefix(message.MessageType, "SyncPlay") {
				t.Errorf("unexpected message to %s: %s %+v", a.device, message.MessageType, message.Data)
			}
		case <-a.socket.closed:
			return
		case <-deadline:
			t.Fatalf("the socket of %s stays open", a.device)
		}
	}
}

// drain reads the app's messages until the test ends, for an app whose
// messages a test does not look at.
func (a syncPlayApp) drain(t *testing.T) {
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case <-a.socket.messages:
			case <-done:
				return
			}
		}
	}()
}

// syncPlayEvent is what a SyncPlay message says, in short: the command,
// or the type of update with the state it tells.
func syncPlayEvent(message socketMessage) string {
	data, _ := message.Data.(map[string]any)
	if message.MessageType == "SyncPlayCommand" {
		return fmt.Sprint(data["Command"])
	}
	if data["Type"] == "StateUpdate" {
		state, _ := data["Data"].(map[string]any)
		return fmt.Sprintf("StateUpdate %v %v", state["State"], state["Reason"])
	}
	return fmt.Sprint(data["Type"])
}

// expect reads the app's next SyncPlay messages, which must say events.
func (a syncPlayApp) expect(t *testing.T, events ...string) []socketMessage {
	t.Helper()
	messages := make([]socketMessage, 0, len(events))
	for _, want := range events {
		message := a.next(t)
		if got := syncPlayEvent(message); got != want {
			t.Fatalf("%s received %s (%+v), want %s", a.device, got, message.Data, want)
		}
		messages = append(messages, message)
	}
	return messages
}

func decodeData[T any](t *testing.T, message socketMessage) T {
	t.Helper()
	encoded, _ := json.Marshal(message.Data)
	var data T
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatalf("%s: %v", encoded, err)
	}
	return data
}

// playQueueOf decodes the data of a PlayQueue update.
func playQueueOf(t *testing.T, message socketMessage) PlayQueueUpdate {
	t.Helper()
	update := decodeData[struct{ Data PlayQueueUpdate }](t, message)
	return update.Data
}

// syncPlayers sets up two users' sessions in a group, on the catalog
// addon's episodes.
type syncPlayers struct {
	tracking
	alice, bob syncPlayApp
	group      string
}

func newSyncPlayers(t *testing.T) syncPlayers {
	t.Helper()
	tr := newTracking(t)
	tr.testServer.user("alice", nil)
	tr.testServer.user("bob", nil)
	p := syncPlayers{tracking: tr, alice: tr.syncPlayApp(t, "alice", "alice-tv"), bob: tr.syncPlayApp(t, "bob", "bob-tv")}
	status, answer := p.alice.send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": " Film night "})
	var info GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &info) != nil || info.GroupName != "Film night" {
		t.Fatalf("new group: %d %s", status, answer)
	}
	p.group = info.GroupId
	p.alice.expect(t, "GroupJoined", "Stop")
	p.bob.post(t, tr.testServer, "Join", map[string]any{"GroupId": hyphenated(mustID(t, p.group))})
	p.bob.expect(t, "GroupJoined", "Stop")
	p.alice.expect(t, "UserJoined")
	return p
}

// ready reports an app ready at position, paused, on the entry given.
func (p syncPlayers) ready(t *testing.T, a syncPlayApp, entry string, position int64) {
	t.Helper()
	a.post(t, p.testServer, "Ready", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano),
		"PositionTicks": position, "IsPlaying": false, "PlaylistItemId": entry})
}

// play sets a queue of the first episode, and has both apps ready: the
// group plays. It returns the queue's entry.
func (p syncPlayers) play(t *testing.T) string {
	t.Helper()
	p.alice.post(t, p.testServer, "SetNewQueue", map[string]any{"PlayingQueue": []string{p.episodes[0]}})
	update := playQueueOf(t, p.alice.expect(t, "PlayQueue")[0])
	p.bob.expect(t, "PlayQueue")
	entry := update.Playlist[0].PlaylistItemId
	p.ready(t, p.alice, entry, 0)
	p.alice.expect(t, "Pause")
	p.ready(t, p.bob, entry, 0)
	p.alice.expect(t, "Unpause", "StateUpdate Playing Ready")
	p.bob.expect(t, "Unpause", "StateUpdate Playing Ready")
	return entry
}

// delay is how far ahead of its emission a command is to be carried out.
func delay(t *testing.T, message socketMessage) time.Duration {
	t.Helper()
	command := decodeData[SendCommand](t, message)
	return time.Time(command.When).Sub(time.Time(command.EmittedAt))
}

// TestSyncPlaySessionMatchesJellyfin replays the session recorded from
// Jellyfin 12.1 by scripts/jellyfin-fixtures.sh: A creates a group, B
// joins, they set a queue, get ready, play, pause, seek, wait for B
// buffering, change items, and C, who may see none of them, is kept out.
// Every answer and every message each session receives must be Jellyfin's,
// identifiers, dates and the positions playing moves on aside.
func TestSyncPlaySessionMatchesJellyfin(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "syncplay-session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var steps []struct {
		Step, Client, Method, Path string
		Body, Answer               any
		Status                     int
		Messages                   map[string][]any
	}
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatal(err)
	}
	tr := newTracking(t)
	for _, name := range []string{"fixtures", "partner", "restricted"} {
		tr.testServer.user(name, nil)
	}
	// The restricted user sees no addon, hence none of the episodes.
	restricted, _ := tr.store.Authenticate(t.Context(), "restricted", "correct horse")
	if err := tr.addons.SetUsesSharedAddons(t.Context(), restricted.ID, false); err != nil {
		t.Fatal(err)
	}
	apps := map[string]syncPlayApp{
		"A": tr.syncPlayApp(t, "fixtures", "syncplay-a"),
		"B": tr.syncPlayApp(t, "partner", "syncplay-b"),
		"C": tr.syncPlayApp(t, "restricted", "syncplay-c"),
	}
	l := newSyncPlayLabels(map[string]string{"title-1": tr.episodes[0], "title-2": tr.episodes[1], "title-3": tr.episodes[2],
		"unknown-group": randomID().String()})
	var position any = 0
	resolve := func(value any) any {
		var walk func(any) any
		walk = func(value any) any {
			switch v := value.(type) {
			case map[string]any:
				resolved := map[string]any{}
				for key, item := range v {
					resolved[key] = walk(item)
				}
				return resolved
			case []any:
				resolved := make([]any, 0, len(v))
				for _, item := range v {
					resolved = append(resolved, walk(item))
				}
				return resolved
			case string:
				name, isPlaceholder := strings.CutPrefix(v, "{")
				if name, isPlaceholder = strings.CutSuffix(name, "}"); !isPlaceholder {
					return v
				}
				switch name {
				case "now":
					return time.Now().UTC().Format(time.RFC3339Nano)
				case "group-position":
					return position
				}
				return l.raw[name]
			}
			return value
		}
		return walk(value)
	}
	placeholder := regexp.MustCompile(`\{([^}]+)\}`)

	for _, step := range steps {
		path := placeholder.ReplaceAllStringFunc(step.Path, func(match string) string { return l.raw[match[1:len(match)-1]] })
		body := step.Body
		if body != "" {
			body = resolve(body)
		}
		status, answer := apps[step.Client].send(t, tr.testServer, step.Method, path, body)
		if status != step.Status {
			t.Fatalf("%s: %d %s, want %d", step.Step, status, answer, step.Status)
		}
		var got any
		if len(answer) > 0 {
			if json.Unmarshal(answer, &got) != nil {
				got = string(answer)
			}
			if step.Step == "A creates a group" {
				id := got.(map[string]any)["GroupId"].(string)
				l.names[id], l.raw["group"] = "group", id
			}
			got = l.normalize(got, "")
		}
		if !reflect.DeepEqual(got, step.Answer) {
			t.Errorf("%s: answered %s, want %s", step.Step, encode(got), encode(step.Answer))
		}
		for _, client := range []string{"A", "B", "C"} {
			for i, want := range step.Messages[client] {
				message := apps[client].next(t)
				if message.MessageType == "SyncPlayCommand" {
					position = message.Data.(map[string]any)["PositionTicks"]
				}
				got := l.normalize(asAny(message), "")
				if !reflect.DeepEqual(timeless(got), timeless(want)) {
					t.Errorf("%s: message %d to %s is %s, want %s", step.Step, i, client, encode(got), encode(want))
				}
			}
		}
	}
	for _, a := range apps {
		a.quiet(t)
	}
}

func asAny(value any) any {
	encoded, _ := json.Marshal(value)
	var decoded any
	_ = json.Unmarshal(encoded, &decoded)
	return decoded
}

func encode(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// timeless leaves out what depends on the time requests take: positions,
// which move on while the group plays, and the order of a shuffled queue
// after the entry playing, which comes first.
func timeless(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, item := range v {
			switch key {
			case "PositionTicks", "StartPositionTicks":
				result[key] = "ticks"
			default:
				result[key] = timeless(item)
			}
		}
		if playlist, ok := result["Playlist"].([]any); ok && v["ShuffleMode"] == "Shuffle" && len(playlist) > 1 {
			rest := slices.Clone(playlist[1:])
			slices.SortFunc(rest, func(a, b any) int { return strings.Compare(encode(a), encode(b)) })
			result["Playlist"] = append([]any{playlist[0]}, rest...)
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			result = append(result, timeless(item))
		}
		return result
	}
	return value
}

// syncPlayLabels names identifiers as the fixture script does: known ones
// by their label, playlist entries and others by order of appearance.
type syncPlayLabels struct {
	names, raw map[string]string
	counters   map[string]int
}

func newSyncPlayLabels(known map[string]string) *syncPlayLabels {
	l := &syncPlayLabels{names: map[string]string{}, raw: map[string]string{}, counters: map[string]int{}}
	for name, value := range known {
		l.names[value], l.raw[name] = name, value
	}
	return l
}

var (
	plainGUID     = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hyphenGUID    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	dateTimeValue = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d`)
)

func (l *syncPlayLabels) label(value, key string) string {
	value = strings.ToLower(value)
	if value == strings.Repeat("0", 32) {
		return value
	}
	if _, known := l.names[value]; !known {
		prefix := "id"
		if key == "PlaylistItemId" {
			prefix = "playlist-item"
		}
		l.counters[prefix]++
		name := fmt.Sprintf("%s-%d", prefix, l.counters[prefix])
		l.names[value], l.raw[name] = name, value
	}
	return l.names[value]
}

func (l *syncPlayLabels) normalize(value any, key string) any {
	switch v := value.(type) {
	case map[string]any:
		result := map[string]any{}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			result[k] = l.normalize(v[k], k)
		}
		return result
	case []any:
		result := make([]any, 0, len(v))
		for _, item := range v {
			result = append(result, l.normalize(item, key))
		}
		return result
	case string:
		switch {
		case key == "MessageId":
			return "message-id"
		case key == "traceId":
			return "trace-id"
		case plainGUID.MatchString(strings.ToLower(v)):
			return l.label(v, key)
		case hyphenGUID.MatchString(strings.ToLower(v)):
			return l.label(strings.ReplaceAll(v, "-", ""), key) + " (hyphenated)"
		case dateTimeValue.MatchString(v):
			return "date"
		}
	}
	return value
}

// Playback starts ahead of the moment it is asked, by twice the highest
// latency members report, half a second at least, so that the command
// reaches everyone in time; a pause stops the group where it played to.
func TestSyncPlaySchedulesPlaybackOnTheServersClock(t *testing.T) {
	p := newSyncPlayers(t)
	entry := p.play(t)
	// Until members report their latency, half a second is assumed of
	// each.
	p.alice.post(t, p.testServer, "Pause", nil)
	p.alice.expect(t, "Pause", "StateUpdate Paused Pause")
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
	p.alice.post(t, p.testServer, "Unpause", nil)
	if got := delay(t, p.alice.expect(t, "Unpause", "StateUpdate Playing Unpause")[0]); got <= 900*time.Millisecond || got > time.Second {
		t.Errorf("unpause %v ahead with the default latency, want a second", got)
	}
	p.bob.expect(t, "Unpause", "StateUpdate Playing Unpause")

	p.alice.post(t, p.testServer, "Ping", map[string]any{"Ping": 0})
	p.bob.post(t, p.testServer, "Ping", map[string]any{"Ping": "20"})
	p.alice.post(t, p.testServer, "Pause", nil)
	p.alice.expect(t, "Pause", "StateUpdate Paused Pause")
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
	p.alice.post(t, p.testServer, "Unpause", nil)
	if got := delay(t, p.alice.expect(t, "Unpause", "StateUpdate Playing Unpause")[0]); got <= 400*time.Millisecond || got > 500*time.Millisecond {
		t.Errorf("unpause %v ahead with low latencies, want half a second", got)
	}
	p.bob.expect(t, "Unpause", "StateUpdate Playing Unpause")
	p.bob.post(t, p.testServer, "Ping", map[string]any{"Ping": 400})
	p.alice.post(t, p.testServer, "Pause", nil)
	p.alice.expect(t, "Pause", "StateUpdate Paused Pause")
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
	p.alice.post(t, p.testServer, "Unpause", nil)
	if got := delay(t, p.alice.expect(t, "Unpause", "StateUpdate Playing Unpause")[0]); got <= 700*time.Millisecond || got > 800*time.Millisecond {
		t.Errorf("unpause %v ahead with a 400 ms latency, want twice that", got)
	}
	p.bob.expect(t, "Unpause", "StateUpdate Playing Unpause")

	// The group pauses where it played to, once playback started.
	time.Sleep(time.Second)
	p.bob.post(t, p.testServer, "Pause", nil)
	paused := decodeData[SendCommand](t, p.alice.expect(t, "Pause", "StateUpdate Paused Pause")[0])
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
	if paused.PositionTicks < 1_000_000 || paused.PositionTicks > 8_000_000 || paused.PlaylistItemId != entry {
		t.Errorf("paused at %d on %s, want about 0.2 s on %s", paused.PositionTicks, paused.PlaylistItemId, entry)
	}

	// A seek has everyone seek and the group wait; a member ready
	// elsewhere is told to seek again, the others are not bothered.
	p.alice.post(t, p.testServer, "Seek", map[string]any{"PositionTicks": "600000000"})
	for _, a := range []syncPlayApp{p.alice, p.bob} {
		if seek := decodeData[SendCommand](t, a.expect(t, "Seek", "StateUpdate Waiting Seek")[0]); seek.PositionTicks != 600_000_000 {
			t.Errorf("%s seeks to %d", a.device, seek.PositionTicks)
		}
	}
	p.ready(t, p.alice, entry, 0)
	if seek := decodeData[SendCommand](t, p.alice.expect(t, "Seek", "StateUpdate Waiting Ready")[0]); seek.PositionTicks != 600_000_000 {
		t.Errorf("alice corrected to %d", seek.PositionTicks)
	}
	p.bob.expect(t, "StateUpdate Waiting Ready")
	p.ready(t, p.alice, entry, 600_000_000)
	p.alice.quiet(t)
	// Paused before the seek, the group pauses once everyone is ready.
	p.ready(t, p.bob, entry, 600_000_000)
	p.alice.expect(t, "Pause", "StateUpdate Paused Ready")
	p.bob.expect(t, "Pause", "StateUpdate Paused Ready")

	// Positions stay within bounds, however far a member asks.
	for _, tc := range []struct{ asked, want int64 }{{math.MaxInt64, maxPositionTicks}, {-5, 0}} {
		p.alice.post(t, p.testServer, "Seek", map[string]any{"PositionTicks": tc.asked})
		for _, a := range []syncPlayApp{p.alice, p.bob} {
			if seek := decodeData[SendCommand](t, a.expect(t, "Seek", "StateUpdate Waiting Seek")[0]); seek.PositionTicks != tc.want {
				t.Errorf("seeking to %d: %s seeks to %d, want %d", tc.asked, a.device, seek.PositionTicks, tc.want)
			}
		}
	}
}

// A queue holds maxQueueEntries entries at most: a queue or an addition
// that would make it longer is refused.
func TestSyncPlayQueuesAreBounded(t *testing.T) {
	p := newSyncPlayers(t)
	long := slices.Repeat([]string{p.episodes[0]}, maxQueueEntries+1)
	p.alice.post(t, p.testServer, "SetNewQueue", map[string]any{"PlayingQueue": long})
	p.alice.quiet(t)
	p.bob.quiet(t)
	p.alice.post(t, p.testServer, "SetNewQueue", map[string]any{"PlayingQueue": long[:maxQueueEntries-1]})
	if update := playQueueOf(t, p.alice.expect(t, "PlayQueue")[0]); len(update.Playlist) != maxQueueEntries-1 {
		t.Fatalf("queue of %d entries", len(update.Playlist))
	}
	p.bob.expect(t, "PlayQueue")
	p.alice.post(t, p.testServer, "Queue", map[string]any{"ItemIds": p.episodes[:2]})
	p.alice.quiet(t)
	p.bob.quiet(t)
	p.alice.post(t, p.testServer, "Queue", map[string]any{"ItemIds": p.episodes[:1], "Mode": "QueueNext"})
	if update := playQueueOf(t, p.alice.expect(t, "PlayQueue")[0]); len(update.Playlist) != maxQueueEntries || update.Reason != "QueueNext" {
		t.Fatalf("queue of %d entries after %s", len(update.Playlist), update.Reason)
	}
	p.bob.expect(t, "PlayQueue")
}

// stuckSocket opens a socket on which the app never reads what it is
// sent, as an app that stopped responding.
func (s testServer) stuckSocket(t *testing.T, device, token string) {
	t.Helper()
	address := strings.TrimPrefix(s.url, "http://")
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// The little the system buffers for the app soon fills.
	_ = conn.(*net.TCPConn).SetReadBuffer(4096)
	var key [16]byte
	_, _ = cryptorand.Read(key[:])
	_, _ = fmt.Fprintf(conn, "GET /socket HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nAuthorization: %s\r\n\r\n",
		address, base64.StdEncoding.EncodeToString(key[:]), app(device, token))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("stuck socket: %v %v", response, err)
	}
}

// An app that stops reading its socket holds back its own messages only:
// the requests of its group, and everyone else's, are answered at once.
func TestSyncPlayAppsThatDoNotReadHoldNothingBack(t *testing.T) {
	tr := newTracking(t)
	for _, name := range []string{"alice", "bob", "carol", "dave"} {
		tr.testServer.user(name, nil)
	}
	alice := tr.syncPlayApp(t, "alice", "alice-tv")
	alice.drain(t)
	bob := syncPlayApp{device: "bob-tv", token: tr.signIn("bob", "bob-tv")}
	tr.stuckSocket(t, bob.device, bob.token)
	carol := tr.syncPlayApp(t, "carol", "carol-tv")
	dave := tr.syncPlayApp(t, "dave", "dave-tv")
	quick := func(what string, call func()) {
		t.Helper()
		start := time.Now()
		call()
		// Writing to an app is given ten seconds.
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("%s took %v", what, took)
		}
	}
	status, answer := alice.send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Stuck"})
	var stuck GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &stuck) != nil {
		t.Fatalf("new group: %d %s", status, answer)
	}
	bob.post(t, tr.testServer, "Join", map[string]any{"GroupId": stuck.GroupId})
	// Every change sends the whole queue to every member: with a long one,
	// a few fill what the system buffers for bob.
	queue := slices.Repeat([]string{tr.episodes[0]}, maxQueueEntries)
	quick("setting the queue", func() { alice.post(t, tr.testServer, "SetNewQueue", map[string]any{"PlayingQueue": queue}) })
	for i := range 40 {
		quick("changing the repeat mode", func() {
			alice.post(t, tr.testServer, "SetRepeatMode", map[string]any{"Mode": repeatModes[i%len(repeatModes)]})
		})
	}
	quick("leaving the group", func() { alice.post(t, tr.testServer, "Leave", nil) })
	var other GroupInfoDto
	quick("creating a group", func() {
		status, answer := carol.send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Other"})
		if status != http.StatusOK || json.Unmarshal(answer, &other) != nil {
			t.Fatalf("other group: %d %s", status, answer)
		}
	})
	quick("joining it", func() { dave.post(t, tr.testServer, "Join", map[string]any{"GroupId": other.GroupId}) })
	quick("listing groups", func() {
		var groups []GroupInfoDto
		if status := tr.get(t, "/SyncPlay/List", carol.token, &groups); status != http.StatusOK || len(groups) != 2 {
			t.Errorf("groups: %d %+v", status, groups)
		}
	})
	carol.expect(t, "GroupJoined", "Stop", "UserJoined")
	dave.expect(t, "GroupJoined", "Stop")
}

// Access is checked without holding the groups: a slow library holds back
// the request that waits on it only. A check that a change made stale
// meanwhile is made again.
func TestSyncPlayChecksAccessWithoutHoldingGroups(t *testing.T) {
	alice, bob, carol, dave := randomID(), randomID(), randomID(), randomID()
	item1, item2, item3 := randomID(), randomID(), randomID()
	var mu sync.Mutex
	// Lookups for a user in gates wait until it closes; hidden are the
	// items each user may not see.
	gates := map[accounts.ID]chan struct{}{}
	hidden := map[accounts.ID][]accounts.ID{bob: {item2}, dave: {item3}}
	looking := make(chan accounts.ID, 10)
	canPlay := func(_ context.Context, user accounts.ID, items []accounts.ID) bool {
		mu.Lock()
		gate := gates[user]
		mu.Unlock()
		if gate != nil {
			looking <- user
			<-gate
		}
		return !slices.ContainsFunc(items, func(item accounts.ID) bool { return slices.Contains(hidden[user], item) })
	}
	block := func(user accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		gates[user] = make(chan struct{})
	}
	// release lets the lookup waiting go on; later ones do not wait.
	release := func(user accounts.ID) {
		mu.Lock()
		defer mu.Unlock()
		close(gates[user])
		delete(gates, user)
	}
	messages := make(chan syncMessage, 100)
	sp := newSyncPlay(canPlay, func(m syncMessage) { messages <- m }, slog.New(slog.DiscardHandler))
	member := func(user accounts.ID) *groupMember {
		return &groupMember{device: user, user: user, name: user.String(), ping: defaultPing}
	}
	quickly := func(what string, call func()) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			defer close(done)
			call()
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s waits on another's library lookup", what)
		}
	}
	queued := func(g *syncGroup) []accounts.ID {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.queue.items()
	}
	ctx := t.Context()
	info := sp.create(member(alice), "group", nil)
	id, _ := accounts.ParseID(info.GroupId)
	sp.handle(ctx, alice, syncRequest{kind: "Play", items: []accounts.ID{item1}})
	g := sp.groupOf(alice)

	// Bob's lookups wait: meanwhile, groups are created, played and
	// listed, and an item bob may not see is queued.
	block(bob)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		sp.join(ctx, member(bob), id)
	}()
	<-looking
	quickly("creating a group", func() { sp.create(member(carol), "other", nil) })
	quickly("pausing", func() { sp.handle(ctx, alice, syncRequest{kind: "Pause"}) })
	quickly("listing groups", func() { sp.visible(ctx, alice, nil) })
	quickly("queueing", func() { sp.handle(ctx, alice, syncRequest{kind: "Queue", items: []accounts.ID{item2}}) })
	release(bob)
	<-joined
	// Bob was found to see the queue as it was, not as it is.
	if sp.groupOf(bob) != nil {
		t.Errorf("bob joined a group whose queue he may not see")
	}
	for denied := false; !denied; {
		select {
		case m := <-messages:
			update, _ := m.data.(SyncPlayGroupUpdate)
			denied = m.device == bob && update.Type == "LibraryAccessDenied"
		case <-time.After(3 * time.Second):
			t.Fatal("bob was not told he may not join")
		}
	}

	// Alice's lookups wait: meanwhile dave, who may not see item3, joins.
	block(alice)
	added := make(chan struct{})
	go func() {
		defer close(added)
		sp.handle(ctx, alice, syncRequest{kind: "Queue", items: []accounts.ID{item3}})
	}()
	<-looking
	quickly("joining", func() { sp.join(ctx, member(dave), id) })
	release(alice)
	<-added
	if got := queued(g); !slices.Equal(got, []accounts.ID{item1, item2}) {
		t.Errorf("queue %v: an item a member who joined meanwhile may not see was queued", got)
	}
}

// A member that buffers holds the group back, but not for ever: after a
// while, playback goes on without it.
func TestSyncPlayWaitsForBufferingMembersForAWhile(t *testing.T) {
	p := newSyncPlayers(t)
	p.handler.syncPlay.waitTimeout = 300 * time.Millisecond
	entry := p.play(t)
	p.bob.post(t, p.testServer, "Buffering", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano),
		"PositionTicks": 0, "IsPlaying": true, "PlaylistItemId": entry})
	p.alice.expect(t, "Pause", "StateUpdate Waiting Buffer")
	p.bob.expect(t, "StateUpdate Waiting Buffer")
	// Bob never gets ready: the group plays without him.
	p.alice.expect(t, "Unpause", "StateUpdate Playing Unpause")
	p.bob.expect(t, "Unpause", "StateUpdate Playing Unpause")
	// Forced to play, the group ignores buffering until its state changes.
	p.bob.post(t, p.testServer, "Buffering", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano),
		"PositionTicks": 0, "IsPlaying": true, "PlaylistItemId": entry})
	p.alice.quiet(t)

	// Paused before the wait, the group stays paused when it gives up.
	p.alice.post(t, p.testServer, "Pause", nil)
	p.alice.expect(t, "Pause", "StateUpdate Paused Pause")
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
	p.alice.post(t, p.testServer, "Seek", map[string]any{"PositionTicks": 100_000_000})
	p.alice.expect(t, "Seek", "StateUpdate Waiting Seek")
	p.bob.expect(t, "Seek", "StateUpdate Waiting Seek")
	p.ready(t, p.alice, entry, 100_000_000)
	p.alice.expect(t, "Pause", "StateUpdate Paused Pause")
	p.bob.expect(t, "Pause", "StateUpdate Paused Pause")
}

// A session leaves its group when its app disconnects, or when it is
// signed out, however it is; the group goes once empty.
func TestSyncPlayMembersLeaveWithTheirSession(t *testing.T) {
	p := newSyncPlayers(t)
	entry := p.play(t)
	p.alice.post(t, p.testServer, "Seek", map[string]any{"PositionTicks": 50_000_000})
	p.alice.expect(t, "Seek", "StateUpdate Waiting Seek")
	p.bob.expect(t, "Seek", "StateUpdate Waiting Seek")
	p.ready(t, p.alice, entry, 50_000_000)
	p.alice.expect(t, "Pause")
	// Bob, whom the group waits for, disconnects: the group plays on.
	_ = p.bob.socket.conn.CloseNow()
	p.alice.expect(t, "Unpause", "StateUpdate Playing Unpause", "UserLeft")
	if status, _ := p.bob.send(t, p.testServer, http.MethodPost, "/SyncPlay/Pause", nil); status != http.StatusForbidden {
		t.Errorf("bob pausing once out of the group: %d", status)
	}
	var groups []GroupInfoDto
	if status := p.get(t, "/SyncPlay/List", p.alice.token, &groups); status != http.StatusOK || len(groups) != 1 ||
		!slices.Equal(groups[0].Participants, []string{"alice"}) || groups[0].State != "Playing" {
		t.Fatalf("groups once bob left: %d %+v", status, groups)
	}
	p.alice.post(t, p.testServer, "Stop", nil)
	p.alice.expect(t, "Stop")

	join := func(user string) syncPlayApp {
		t.Helper()
		p.testServer.user(user, nil)
		a := p.syncPlayApp(t, user, user+"-tv")
		a.post(t, p.testServer, "Join", map[string]any{"GroupId": p.group})
		a.expect(t, "GroupJoined", "Stop")
		p.alice.expect(t, "UserJoined")
		return a
	}
	device := func(user accounts.User) accounts.ID {
		t.Helper()
		devices, err := p.store.Devices(t.Context(), user.ID)
		if err != nil || len(devices) != 1 {
			t.Fatalf("devices of %s: %v %v", user.Name, devices, err)
		}
		return devices[0].ID
	}
	// A password changed on another of the user's devices.
	carol := join("carol")
	phone := p.signIn("carol", "carol-phone")
	if status, answer := p.call(http.MethodPost, "/Users/Password", app("carol-phone", phone),
		map[string]string{"CurrentPw": "correct horse", "NewPw": "battery staple"}); status/100 != 2 {
		t.Fatalf("password change: %d %s", status, answer)
	}
	carol.ended(t)
	p.alice.expect(t, "UserLeft")
	// A device an administrator revokes.
	dave := join("dave")
	daveUser, _ := p.store.Authenticate(t.Context(), "dave", "correct horse")
	if err := p.store.RevokeDevice(t.Context(), daveUser.ID, device(daveUser)); err != nil {
		t.Fatal(err)
	}
	dave.ended(t)
	p.alice.expect(t, "UserLeft")
	// An account deleted.
	erin := join("erin")
	erinUser, _ := p.store.Authenticate(t.Context(), "erin", "correct horse")
	if err := p.store.DeleteUser(t.Context(), erinUser.ID); err != nil {
		t.Fatal(err)
	}
	erin.ended(t)
	p.alice.expect(t, "UserLeft")
	if status := p.get(t, "/SyncPlay/List", p.alice.token, &groups); status != http.StatusOK || len(groups) != 1 ||
		!slices.Equal(groups[0].Participants, []string{"alice"}) {
		t.Fatalf("groups once the others were signed out: %d %+v", status, groups)
	}

	// The app signs out itself.
	if status, _ := p.alice.send(t, p.testServer, http.MethodPost, "/Sessions/Logout", nil); status != http.StatusNoContent {
		t.Fatalf("sign-out: %d", status)
	}
	p.alice.ended(t)
	if status := p.get(t, "/SyncPlay/List", p.token, &groups); status != http.StatusOK || len(groups) != 0 {
		t.Errorf("groups once empty: %d %+v", status, groups)
	}
	if status, _ := p.call(http.MethodGet, "/SyncPlay/"+p.group, app("tv", p.token), nil); status != http.StatusNotFound {
		t.Errorf("an empty group: %d", status)
	}
}

// Members only ever get items they may see: a group is hidden from those
// who may not see its queue, and a queue a member may not see is refused.
func TestSyncPlayKeepsToWhatMembersMaySee(t *testing.T) {
	p := newSyncPlayers(t)
	p.testServer.user("carol", nil)
	carol, _ := p.store.Authenticate(t.Context(), "carol", "correct horse")
	if err := p.addons.SetUsesSharedAddons(t.Context(), carol.ID, false); err != nil {
		t.Fatal(err)
	}
	c := p.syncPlayApp(t, "carol", "carol-tv")
	// The group has nothing queued yet: carol sees it and joins.
	var groups []GroupInfoDto
	if p.get(t, "/SyncPlay/List", c.token, &groups); len(groups) != 1 {
		t.Fatalf("carol's groups: %+v", groups)
	}
	c.post(t, p.testServer, "Join", map[string]any{"GroupId": p.group})
	c.expect(t, "GroupJoined", "Stop")
	p.alice.expect(t, "UserJoined")
	p.bob.expect(t, "UserJoined")
	// Carol cannot see the episodes: neither a queue nor an addition to
	// it is taken.
	p.alice.post(t, p.testServer, "SetNewQueue", map[string]any{"PlayingQueue": []string{p.episodes[0]}, "PlayingItemPosition": 0})
	p.alice.post(t, p.testServer, "Queue", map[string]any{"ItemIds": []string{p.episodes[1]}, "Mode": "QueueNext"})
	for _, a := range []syncPlayApp{p.alice, p.bob, c} {
		a.quiet(t)
	}
	var group GroupInfoDto
	if p.get(t, "/SyncPlay/"+p.group, p.alice.token, &group); group.State != "Idle" {
		t.Errorf("group after a refused queue: %+v", group)
	}

	// Once carol left, the others play an episode; carol may no longer
	// see the group, nor join it.
	c.post(t, p.testServer, "Leave", nil)
	c.expect(t, "GroupLeft")
	p.alice.expect(t, "UserLeft")
	p.bob.expect(t, "UserLeft")
	p.play(t)
	if p.get(t, "/SyncPlay/List", c.token, &groups); len(groups) != 0 {
		t.Errorf("carol lists %+v", groups)
	}
	if status, _ := p.call(http.MethodGet, "/SyncPlay/"+p.group, app("tv", c.token), nil); status != http.StatusNotFound {
		t.Errorf("carol reading the group: %d", status)
	}
	c.post(t, p.testServer, "Join", map[string]any{"GroupId": p.group})
	if update := decodeData[SyncPlayGroupUpdate](t, c.expect(t, "LibraryAccessDenied")[0]); update.GroupId != p.group {
		t.Errorf("access denied to %s", update.GroupId)
	}
	p.alice.quiet(t)
}

// A session playing a title brings it to the group it creates, which goes
// on from where it is once the session is ready.
func TestSyncPlayGroupStartsWithWhatItsCreatorPlays(t *testing.T) {
	tr := newTracking(t)
	tr.testServer.user("alice", nil)
	alice := tr.syncPlayApp(t, "alice", "alice-tv")
	if status, answer := alice.send(t, tr.testServer, http.MethodPost, "/Sessions/Playing",
		map[string]any{"ItemId": tr.episodes[1], "PositionTicks": 300_000_000, "IsPaused": false}); status != http.StatusNoContent {
		t.Fatalf("playback report: %d %s", status, answer)
	}
	status, answer := alice.send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{})
	var info GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &info) != nil || info.State != "Waiting" || info.GroupName != "" {
		t.Fatalf("new group: %d %s", status, answer)
	}
	messages := alice.expect(t, "GroupJoined", "PlayQueue")
	queue := playQueueOf(t, messages[1])
	if len(queue.Playlist) != 1 || queue.Playlist[0].ItemId != tr.episodes[1] || queue.PlayingItemIndex != 0 ||
		queue.StartPositionTicks < 300_000_000 || queue.StartPositionTicks > 310_000_000 {
		t.Fatalf("queue of the new group: %+v", queue)
	}
	alice.post(t, tr.testServer, "Ready", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano),
		"PositionTicks": queue.StartPositionTicks, "IsPlaying": true, "PlaylistItemId": queue.Playlist[0].PlaylistItemId})
	alice.expect(t, "Unpause", "StateUpdate Playing Ready")
}

// Requests are refused as Jellyfin refuses them, before anything changes.
func TestSyncPlayRequestsThatDoNotBind(t *testing.T) {
	p := newSyncPlayers(t)
	for _, tc := range []struct {
		path string
		body any
	}{
		{"SetRepeatMode", map[string]any{"Mode": "RepeatForEver"}},
		{"SetShuffleMode", map[string]any{"Mode": 7}},
		{"Seek", map[string]any{"PositionTicks": "soon"}},
		{"Join", map[string]any{"GroupId": "not a group"}},
		{"Ready", nil},
		{"New", map[string]any{"GroupName": nil}},
		{"New", map[string]any{"GroupName": strings.Repeat("é", 201)}},
	} {
		status, answer := p.alice.send(t, p.testServer, http.MethodPost, "/SyncPlay/"+tc.path, tc.body)
		want := http.StatusBadRequest
		if tc.body == nil {
			want = http.StatusUnsupportedMediaType
		}
		if status != want {
			t.Errorf("%s with %v: %d %s, want %d", tc.path, tc.body, status, answer, want)
		}
	}
	// Enumerations bind by value as well.
	p.alice.post(t, p.testServer, "SetRepeatMode", map[string]any{"Mode": 1})
	update := playQueueOf(t, p.alice.expect(t, "PlayQueue")[0])
	if update.RepeatMode != "RepeatAll" || update.Reason != "RepeatMode" {
		t.Errorf("repeat mode: %+v", update)
	}
	p.bob.expect(t, "PlayQueue")
	p.alice.quiet(t)
	p.bob.quiet(t)
}

// Requests of many sessions at once keep groups consistent.
func TestSyncPlayGroupsTakeConcurrentRequests(t *testing.T) {
	tr := newTracking(t)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	var apps []syncPlayApp
	for i := range 6 {
		name := fmt.Sprintf("user-%d", i%3)
		if i < 3 {
			tr.testServer.user(name, nil)
		}
		a := tr.syncPlayApp(t, name, fmt.Sprintf("device-%d", i))
		apps = append(apps, a)
		// Messages are read as they come, so that no app holds others
		// back.
		go func() {
			for {
				select {
				case <-a.socket.messages:
				case <-done:
					return
				}
			}
		}()
	}
	status, answer := apps[0].send(t, tr.testServer, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "busy"})
	var info GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &info) != nil {
		t.Fatalf("new group: %d %s", status, answer)
	}
	var wg sync.WaitGroup
	for i, a := range apps {
		wg.Go(func() {
			random := rand.New(rand.NewPCG(uint64(i), 1))
			for range 25 {
				var body any
				path := "Join"
				switch random.IntN(9) {
				case 0:
					body = map[string]any{"GroupId": info.GroupId}
				case 1:
					path, body = "SetNewQueue", map[string]any{"PlayingQueue": tr.episodes, "PlayingItemPosition": random.IntN(3)}
				case 2:
					path = "Pause"
				case 3:
					path = "Unpause"
				case 4:
					path, body = "Seek", map[string]any{"PositionTicks": random.Int64N(1_000_000_000)}
				case 5:
					path, body = "Ready", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano), "PositionTicks": 0, "IsPlaying": random.IntN(2) == 0}
				case 6:
					path, body = "Buffering", map[string]any{"When": time.Now().UTC().Format(time.RFC3339Nano), "IsPlaying": true}
				case 7:
					path, body = "NextItem", map[string]any{}
				case 8:
					path = "Leave"
				}
				if status, answer := a.send(t, tr.testServer, http.MethodPost, "/SyncPlay/"+path, body); status != http.StatusNoContent && status != http.StatusForbidden {
					t.Errorf("%s of %s: %d %s", path, a.device, status, answer)
				}
			}
		})
	}
	wg.Wait()
	for _, a := range apps {
		_, _ = a.send(t, tr.testServer, http.MethodPost, "/SyncPlay/Leave", nil)
	}
	var groups []GroupInfoDto
	if status := tr.get(t, "/SyncPlay/List", tr.token, &groups); status != http.StatusOK || len(groups) != 0 {
		t.Errorf("groups once everyone left: %d %+v", status, groups)
	}
	for i := range 3 {
		user, _ := tr.store.Authenticate(t.Context(), fmt.Sprintf("user-%d", i), "correct horse")
		if tr.handler.syncPlay.isActive(user.ID) {
			t.Errorf("user-%d still counts as in a group", i)
		}
	}
}

// The queue keeps the entry playing through changes, as Jellyfin's does.
func TestSyncPlayQueueKeepsTheEntryPlaying(t *testing.T) {
	items := []accounts.ID{{1}, {2}, {3}, {4}}
	entries := func(q *playQueue) []accounts.ID {
		result := []accounts.ID{}
		for _, e := range q.entries() {
			result = append(result, e.item)
		}
		return result
	}
	playing := func(q *playQueue) accounts.ID {
		current, _ := q.current()
		return current.item
	}
	entryOf := func(q *playQueue, item accounts.ID) accounts.ID {
		for _, e := range q.entries() {
			if e.item == item {
				return e.entry
			}
		}
		t.Fatalf("no entry of %v", item)
		return accounts.ID{}
	}

	q := newPlayQueue()
	q.set(items)
	q.playIndex(2)
	// Removing the entry playing plays the one before, counting those
	// removed before it.
	if !q.remove([]accounts.ID{entryOf(&q, items[1]), entryOf(&q, items[2])}) || playing(&q) != items[0] {
		t.Errorf("after removing the entry playing: %v", playing(&q))
	}
	if q.remove([]accounts.ID{entryOf(&q, items[3])}) || playing(&q) != items[0] {
		t.Errorf("after removing another entry: %v", playing(&q))
	}
	// The first one removed, the next one plays; nothing once empty.
	q.set(items[:2])
	q.playIndex(0)
	if !q.remove([]accounts.ID{entryOf(&q, items[0])}) || playing(&q) != items[1] {
		t.Errorf("after removing the first: %v", playing(&q))
	}
	if !q.remove([]accounts.ID{entryOf(&q, items[1])}) || q.playing != -1 {
		t.Errorf("after removing the last: %d", q.playing)
	}

	// Shuffled, the entry playing comes first; queued next, items follow
	// it in both orders; sorted again, it is found back.
	q.set(items[:3])
	q.playIndex(1)
	q.setShuffle(true)
	if q.playing != 0 || playing(&q) != items[1] || len(q.entries()) != 3 {
		t.Fatalf("shuffled: %v playing %v", entries(&q), playing(&q))
	}
	q.addNext([]accounts.ID{items[3]})
	if q.entries()[1].item != items[3] {
		t.Errorf("queued next while shuffled: %v", entries(&q))
	}
	q.setShuffle(false)
	if !slices.Equal(entries(&q), []accounts.ID{items[0], items[1], items[3], items[2]}) || playing(&q) != items[1] {
		t.Errorf("sorted again: %v playing %v", entries(&q), playing(&q))
	}
	// A move keeps what plays, within bounds.
	if !q.move(entryOf(&q, items[1]), 99) || playing(&q) != items[1] || q.playing != 3 {
		t.Errorf("after a move: %v playing index %d", entries(&q), q.playing)
	}
	if q.move(randomID(), 0) {
		t.Error("moved an unknown entry")
	}

	// At either end, skipping wraps around only when repeating all.
	q.set(items[:2])
	q.playIndex(1)
	if q.next() || q.playing != 1 {
		t.Errorf("next after the last: %d", q.playing)
	}
	q.setRepeat("RepeatAll")
	if !q.next() || q.playing != 0 || !q.previous() || q.playing != 1 {
		t.Errorf("wrapping around: %d", q.playing)
	}
	q.setRepeat("RepeatOne")
	if !q.next() || q.playing != 1 {
		t.Errorf("repeating one: %d", q.playing)
	}
	q.setRepeat("RepeatNone")
	q.playIndex(0)
	if q.previous() || q.playing != 0 {
		t.Errorf("previous before the first: %d", q.playing)
	}
}
