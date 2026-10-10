package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

// received is a request a fake target received.
type received struct {
	path   string
	header http.Header
	body   map[string]any
	at     time.Time
}

// fakeTargets stands in for every target, each under its own path. Unless
// told otherwise, it accepts every message with 204, as Discord does.
type fakeTargets struct {
	mu       sync.Mutex
	got      []received
	answers  map[string][]func(w http.ResponseWriter)
	fallback map[string]func(w http.ResponseWriter)
	url      string
}

func newFakeTargets(t *testing.T) *fakeTargets {
	t.Helper()
	f := &fakeTargets{answers: map[string][]func(w http.ResponseWriter){}, fallback: map[string]func(w http.ResponseWriter){}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got := received{path: r.URL.Path, header: r.Header.Clone(), at: time.Now()}
		_ = json.Unmarshal(raw, &got.body)
		f.mu.Lock()
		f.got = append(f.got, got)
		answer := f.fallback[r.URL.Path]
		if queued := f.answers[r.URL.Path]; len(queued) > 0 {
			answer, f.answers[r.URL.Path] = queued[0], queued[1:]
		}
		f.mu.Unlock()
		if answer == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		answer(w)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// status answers with code.
func status(code int, header ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for i := 0; i+1 < len(header); i += 2 {
			w.Header().Set(header[i], header[i+1])
		}
		w.WriteHeader(code)
	}
}

// reply answers with code and a JSON body.
func reply(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// then has path answer the next requests with answers, in order.
func (f *fakeTargets) then(path string, answers ...func(w http.ResponseWriter)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers[path] = append(f.answers[path], answers...)
}

// always has path answer every request with answer.
func (f *fakeTargets) always(path string, answer func(w http.ResponseWriter)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallback[path] = answer
}

// at lists the requests path received.
func (f *fakeTargets) at(path string) []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []received
	for _, got := range f.got {
		if got.path == path {
			found = append(found, got)
		}
	}
	return found
}

// wait waits until path received n requests, and returns them.
func (f *fakeTargets) wait(t *testing.T, path string, n int) []received {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		found := f.at(path)
		if len(found) >= n {
			return found
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s received %d requests, want %d", path, len(found), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fakeLibrary lists the episodes of series, which tests change, and names
// channels.
type fakeLibrary struct {
	mu       sync.Mutex
	episodes map[accounts.ID][]library.Item
	asked    int
}

func (l *fakeLibrary) Episodes(_ context.Context, _ accounts.User, series accounts.ID, _ *accounts.ID) ([]library.Item, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked++
	episodes, ok := l.episodes[series]
	if !ok {
		return nil, library.ErrNotFound
	}
	return slices.Clone(episodes), nil
}

func (l *fakeLibrary) Item(_ context.Context, _ accounts.User, id accounts.ID) (library.Item, error) {
	return library.Item{ID: id, Kind: library.KindChannel, Name: "Channel Nine"}, nil
}

// set replaces the episodes of series.
func (l *fakeLibrary) set(series accounts.ID, episodes ...library.Item) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.episodes[series] = episodes
}

// fakeUserData tells which series users played and marked favorite.
type fakeUserData struct {
	mu                  sync.Mutex
	episodes, favorites map[accounts.ID][]userdata.Entry
}

func (u *fakeUserData) Episodes(_ context.Context, user accounts.ID) ([]userdata.Entry, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.episodes[user]), nil
}

func (u *fakeUserData) Favorites(_ context.Context, user accounts.ID) ([]userdata.Entry, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.favorites[user]), nil
}

// played records that user played an episode of series; favorite that
// they marked item favorite.
func (u *fakeUserData) played(user, series accounts.ID) {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := time.Now()
	u.episodes[user] = append(u.episodes[user], userdata.Entry{Item: accounts.ID{0xee, byte(len(u.episodes[user]))}, Series: series,
		Data: userdata.Data{Played: true, LastPlayed: &now}})
}

func (u *fakeUserData) favorite(user, item accounts.ID) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.favorites[user] = append(u.favorites[user], userdata.Entry{Item: item, Data: userdata.Data{Favorite: true}})
}

// lockedBuffer collects the log.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type harness struct {
	*Service
	store    *accounts.Store
	targets  *fakeTargets
	library  *fakeLibrary
	userData *fakeUserData
	log      *lockedBuffer
	problems *[]Problem
	// admin and member are users; admin is an administrator.
	admin, member accounts.User
}

// newHarness is a service on a fresh database whose secrets are sealed,
// sending to fake targets, retrying soon and giving up after a second.
func newHarness(t *testing.T) harness {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool, accounts.Sealing(box))
	if err != nil {
		t.Fatal(err)
	}
	h := harness{store: store, targets: newFakeTargets(t), library: &fakeLibrary{episodes: map[accounts.ID][]library.Item{}},
		userData: &fakeUserData{episodes: map[accounts.ID][]userdata.Entry{}, favorites: map[accounts.ID][]userdata.Entry{}},
		log:      &lockedBuffer{}, problems: &[]Problem{}}
	if h.admin, err = store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	if h.member, err = store.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"}); err != nil {
		t.Fatal(err)
	}
	h.Service = New(Options{DB: pool, Accounts: store, Library: h.library, UserData: h.userData, Secrets: box, Version: "1.2.3",
		ServerID: "fedcba9876543210fedcba9876543210", WebClient: true, Logger: slog.New(slog.NewTextHandler(h.log, nil)),
		Problems: func(context.Context) ([]Problem, error) { return slices.Clone(*h.problems), nil }})
	h.timing.retryFirst, h.timing.retryMax, h.timing.giveUp = 20*time.Millisecond, 80*time.Millisecond, time.Second
	// The fake targets are on the local network: members' targets reach
	// them here, unlike in TestMembersTargetsStayOnPublicAddresses.
	h.checkPublic, h.confined = func(context.Context, string) error { return nil }, h.trusted
	t.Cleanup(h.Close)
	return h
}

// add adds a target for owner, nil for the server, failing the test on an
// error.
func (h harness) add(t *testing.T, owner *accounts.User, d Draft) Target {
	t.Helper()
	target, err := h.Create(t.Context(), owner, d)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

// webhook is a webhook target at path of the fake targets.
func (h harness) webhook(path string, events ...string) Draft {
	return Draft{Kind: Webhook, Name: new("Hook " + path), Address: new(h.targets.url + path), Events: events}
}

// idle waits until no message waits to be sent.
func (h harness) idle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.mu.Lock()
		empty := len(h.lanes) == 0
		h.mu.Unlock()
		if empty {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("messages still wait")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// types lists the event types the requests carry, as webhooks receive them,
// with their episode's or recording's name.
func types(requests []received) []string {
	var found []string
	for _, r := range requests {
		name := ""
		for _, object := range []string{"episode", "recording", "problem"} {
			if o, ok := r.body[object].(map[string]any); ok {
				name, _ = o["name"].(string)
				if object == "problem" {
					name, _ = o["key"].(string)
				}
			}
		}
		found = append(found, r.body["type"].(string)+" "+name)
	}
	return found
}

// Each kind of target receives a message formatted for it: a webhook the
// versioned JSON event, Discord an embed that mentions no one, ntfy a JSON
// publication to its server with its topic, its tags, its access token and
// the address a click opens.
func TestTargetsReceiveTheirFormat(t *testing.T) {
	h := newHarness(t)
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	events := []string{RecordingFinished, RecordingFailed}
	h.add(t, nil, h.webhook("/hook", events...))
	h.add(t, nil, Draft{Kind: Discord, Name: new("Channel"), Address: new(h.targets.url + "/api/webhooks/1/discord-token"), Events: events})
	h.add(t, nil, Draft{Kind: Ntfy, Name: new("Phone"), Address: new(h.targets.url + "/ntfy/"), Topic: new("polyfin-alerts"),
		Token: new("tk_ntfy"), Events: events})

	recording := accounts.ID{0x42}
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: recording, User: &h.member.ID, Channel: accounts.ID{9}, Name: "Late Show",
		Start: time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC), Partial: true})
	link := "https://media.example.org/web/#/details?id=" + recording.String() + "&serverId=fedcba9876543210fedcba9876543210"

	hook := h.targets.wait(t, "/hook", 1)[0]
	if hook.header.Get("Content-Type") != "application/json" || hook.header.Get("User-Agent") != "Polyfin/1.2.3" ||
		hook.header.Get("X-Polyfin-Event") != RecordingFinished {
		t.Errorf("webhook headers: %v", hook.header)
	}
	body := hook.body
	rec, _ := body["recording"].(map[string]any)
	user, _ := body["user"].(map[string]any)
	server, _ := body["server"].(map[string]any)
	if body["version"] != 1.0 || body["type"] != RecordingFinished || body["id"] == "" || body["url"] != link ||
		body["title"] != "Recording finished: Late Show" || !strings.Contains(body["message"].(string), "Part of the programme is missing") ||
		rec["name"] != "Late Show" || rec["channelName"] != "Channel Nine" || rec["partial"] != true || rec["start"] != "2026-10-09T20:00:00Z" ||
		user["name"] != "member" || user["id"] != h.member.ID.String() ||
		server["name"] != "Home" || server["url"] != "https://media.example.org" || server["id"] != "fedcba9876543210fedcba9876543210" {
		t.Errorf("webhook event: %v", body)
	}

	discord := h.targets.wait(t, "/api/webhooks/1/discord-token", 1)[0].body
	embeds, _ := discord["embeds"].([]any)
	mentions, _ := discord["allowed_mentions"].(map[string]any)
	if len(embeds) != 1 || mentions == nil || len(mentions["parse"].([]any)) != 0 {
		t.Fatalf("Discord message: %v", discord)
	}
	embed := embeds[0].(map[string]any)
	if embed["title"] != "Recording finished: Late Show" || embed["url"] != link || embed["color"] != float64(0x22c55e) ||
		!strings.Contains(embed["description"].(string), "Channel Nine") || embed["footer"].(map[string]any)["text"] != "Home" {
		t.Errorf("Discord embed: %v", embed)
	}

	// ntfy takes JSON at its server's root, the topic inside.
	ntfy := h.targets.wait(t, "/ntfy/", 1)[0]
	tags, _ := ntfy.body["tags"].([]any)
	if ntfy.header.Get("Authorization") != "Bearer tk_ntfy" || ntfy.body["topic"] != "polyfin-alerts" ||
		ntfy.body["title"] != "Recording finished: Late Show" || ntfy.body["click"] != link ||
		!slices.Equal(tags, []any{"red_circle", "warning"}) {
		t.Errorf("ntfy message: %v %v", ntfy.header, ntfy.body)
	}

	// Without a public address, messages carry no link.
	settings.PublicAddress = ""
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: recording, User: &h.member.ID, Name: "Late Show", Failed: true})
	failed := h.targets.wait(t, "/ntfy/", 2)[1]
	if _, ok := failed.body["click"]; ok || failed.body["priority"] != 4.0 {
		t.Errorf("failed recording without an address: %v", failed.body)
	}
}

// The paths at which the fake targets stand for Telegram's Bot API, with
// the token chatServices gives, and for Pushover.
const (
	telegramPath = "/telegram/bot123456:telegram-very-secret-token/sendMessage"
	pushoverPath = "/pushover/1/messages.json"
)

// chatServices points Telegram and Pushover to the fake targets, and adds a
// Telegram, a Gotify and a Pushover target to the server, for events.
func (h harness) chatServices(t *testing.T, events ...string) (telegram, gotify, pushover Target) {
	t.Helper()
	h.Service.telegramAPI, h.Service.pushoverAPI = h.targets.url+"/telegram", h.targets.url+pushoverPath
	telegram = h.add(t, nil, Draft{Kind: Telegram, Name: new("Chat"), Chat: new("-1001234567890"), Token: new("123456:telegram-very-secret-token"),
		Events: events})
	gotify = h.add(t, nil, Draft{Kind: Gotify, Name: new("Gotify"), Address: new(h.targets.url + "/gotify/"), Token: new("gotify-very-secret-token"),
		Events: events})
	pushover = h.add(t, nil, Draft{Kind: Pushover, Name: new("Phone"), UserKey: new("uPushoverUser0"), Token: new("aPushoverApp0"), Events: events})
	return telegram, gotify, pushover
}

// Telegram, Gotify and Pushover receive each message formatted for them:
// Telegram in HTML, with the link, Gotify and Pushover with a priority by
// event and the address a click opens. A new Pushover token keeps the
// user key.
func TestChatServicesReceiveTheirFormat(t *testing.T) {
	h := newHarness(t)
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	_, _, pushover := h.chatServices(t, RecordingFinished, RecordingFailed)

	recording := accounts.ID{0x42}
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: recording, User: &h.member.ID, Channel: accounts.ID{9}, Name: "Late <Show> & Co"})
	link := "https://media.example.org/web/#/details?id=" + recording.String() + "&serverId=fedcba9876543210fedcba9876543210"
	telegram := h.targets.wait(t, telegramPath, 1)[0].body
	preview, _ := telegram["link_preview_options"].(map[string]any)
	if want := "<b>Recording finished: Late &lt;Show&gt; &amp; Co</b>\nRecorded on Channel Nine, for member.\n\n<a href=\"" +
		html.EscapeString(link) + "\">Open</a>"; telegram["chat_id"] != "-1001234567890" || telegram["parse_mode"] != "HTML" ||
		preview["is_disabled"] != true || telegram["text"] != want {
		t.Errorf("Telegram message: %v\nwant text %q", telegram, want)
	}
	gotify := h.targets.wait(t, "/gotify/message", 1)[0]
	extras, _ := gotify.body["extras"].(map[string]any)
	notification, _ := extras["client::notification"].(map[string]any)
	click, _ := notification["click"].(map[string]any)
	if gotify.header.Get("X-Gotify-Key") != "gotify-very-secret-token" || gotify.body["title"] != "Recording finished: Late <Show> & Co" ||
		gotify.body["message"] != "Recorded on Channel Nine, for member." || gotify.body["priority"] != 5.0 || click["url"] != link {
		t.Errorf("Gotify message: %v %v", gotify.header, gotify.body)
	}
	posted := h.targets.wait(t, pushoverPath, 1)[0].body
	if posted["token"] != "aPushoverApp0" || posted["user"] != "uPushoverUser0" || posted["title"] != "Recording finished: Late <Show> & Co" ||
		posted["message"] != "Recorded on Channel Nine, for member." || posted["url"] != link || posted["priority"] != 0.0 {
		t.Errorf("Pushover message: %v", posted)
	}

	// A failure is urgent, and links nowhere.
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: recording, User: &h.member.ID, Name: "Empty Show", Failed: true})
	if failed := h.targets.wait(t, "/gotify/message", 2)[1].body; failed["priority"] != 8.0 || failed["extras"] != nil {
		t.Errorf("a failure to Gotify: %v", failed)
	}
	if failed := h.targets.wait(t, pushoverPath, 2)[1].body; failed["priority"] != 1.0 || failed["url"] != nil {
		t.Errorf("a failure to Pushover: %v", failed)
	}
	if failed := h.targets.wait(t, telegramPath, 2)[1].body; strings.Contains(failed["text"].(string), "<a ") {
		t.Errorf("a failure to Telegram: %v", failed)
	}
	h.idle(t)

	if _, err := h.Update(t.Context(), nil, pushover.ID, Draft{Token: new("aPushoverApp1")}); err != nil {
		t.Fatal(err)
	}
	if result, err := h.Test(t.Context(), nil, pushover.ID); err != nil || !result.Delivered {
		t.Fatalf("a test: %+v, %v", result, err)
	}
	if test := h.targets.at(pushoverPath)[2].body; test["token"] != "aPushoverApp1" || test["user"] != "uPushoverUser0" {
		t.Errorf("after a new token: %v", test)
	}
}

// Telegram, Gotify and Pushover refusing Polyfin's token, key or chat show
// the target as Refused; refusing the message itself shows Message
// refused. A message they ask to wait for is sent again once the wait is
// over, as long as Telegram asks in its answer.
func TestChatServicesRefusingAndRateLimiting(t *testing.T) {
	h := newHarness(t)
	// Long enough for Telegram's wait below, short enough to wait for.
	h.timing.giveUp = 3 * time.Second
	telegram, gotify, pushover := h.chatServices(t, RecordingFinished)
	for _, refusal := range []struct {
		target  Target
		path    string
		status  int
		body    string
		problem string
	}{
		{telegram, telegramPath, 401, `{"ok":false,"error_code":401,"description":"Unauthorized"}`, ProblemRefused},
		{telegram, telegramPath, 400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, ProblemRefused},
		{telegram, telegramPath, 400, `{"ok":false,"error_code":400,"description":"Bad Request: message is too long"}`, ProblemRejected},
		{gotify, "/gotify/message", 401, `{"error":"Unauthorized","errorCode":401,"errorDescription":"provide a valid access token"}`, ProblemRefused},
		{pushover, pushoverPath, 400, `{"user":"invalid","errors":["user identifier is invalid"],"status":0}`, ProblemRefused},
		{pushover, pushoverPath, 400, `{"token":"invalid","errors":["application token is invalid"],"status":0}`, ProblemRefused},
	} {
		h.targets.then(refusal.path, reply(refusal.status, refusal.body))
		result, err := h.Test(t.Context(), nil, refusal.target.ID)
		if err != nil || result.Delivered || result.Status != refusal.status || result.Target.Problem != refusal.problem {
			t.Errorf("%s answering %s: %+v, %v", refusal.target.Kind, refusal.body, result, err)
		}
	}

	sent, posted := len(h.targets.at(telegramPath)), len(h.targets.at(pushoverPath))
	h.targets.then(telegramPath, reply(http.StatusTooManyRequests,
		`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`))
	h.targets.then(pushoverPath, reply(http.StatusTooManyRequests, `{"status":0,"errors":["too many messages"]}`))
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	got := h.targets.wait(t, telegramPath, sent+2)
	h.targets.wait(t, pushoverPath, posted+2)
	h.idle(t)
	if wait := got[sent+1].at.Sub(got[sent].at); wait < 900*time.Millisecond {
		t.Errorf("Telegram asked to wait 1 second: tried again after %v", wait)
	}
	targets, _ := h.Targets(t.Context(), nil)
	for _, target := range targets {
		if target.Problem != "" || target.LastSentAt == nil {
			t.Errorf("%s after waiting: %+v", target.Kind, target)
		}
	}
}

// The tokens and keys of Telegram, Gotify and Pushover targets are sealed
// in the database, never shown, and never logged, even when Telegram's
// address, which holds the bot's token, cannot be reached.
func TestChatServiceTokensAreSealedNeverShownNorLogged(t *testing.T) {
	h := newHarness(t)
	h.chatServices(t, RecordingFinished)
	h.Service.telegramAPI = "http://127.0.0.1:1"
	h.targets.always("/gotify/message", status(http.StatusUnauthorized))
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	h.targets.wait(t, "/gotify/message", 1)
	h.targets.wait(t, pushoverPath, 1)
	h.idle(t)

	rows, err := h.db.Query(t.Context(), "SELECT secret FROM notification_targets")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !secrets.Sealed(stored) || strings.Contains(stored, "very-secret") || strings.Contains(stored, "Pushover") {
			t.Errorf("stored as %q", stored)
		}
	}
	targets, err := h.Targets(t.Context(), nil)
	if err != nil || len(targets) != 3 {
		t.Fatalf("targets: %v, %v", targets, err)
	}
	if shown, _ := json.Marshal(targets); strings.Contains(string(shown), "very-secret") || strings.Contains(string(shown), "Pushover") {
		t.Errorf("shown: %s", shown)
	}
	for _, target := range targets {
		if !target.TokenSet {
			t.Errorf("%s shows no token", target.Kind)
		}
	}
	if log := h.log.String(); strings.Contains(log, "very-secret") || strings.Contains(log, "Pushover") ||
		!strings.Contains(log, "could not be delivered in time") || !strings.Contains(log, "refused a message") {
		t.Errorf("log: %s", log)
	}
}

// A new episode is told once, when it becomes available, for the series a
// user played or marked favorite: never for the episodes a series already
// had when it was first looked at, nor for those released long ago and
// added late, nor for series the user does not follow. Another user's
// targets are never told, and the server's are told once per episode,
// however many users follow its series.
func TestNewEpisodesAreToldOncePerEpisodeOfFollowedSeries(t *testing.T) {
	h := newHarness(t)
	h.add(t, &h.member, h.webhook("/member", NewEpisode))
	h.add(t, &h.admin, h.webhook("/admin", NewEpisode))
	h.add(t, nil, h.webhook("/server", NewEpisode))

	played, favorite, other := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	h.userData.played(h.member.ID, played)
	h.userData.favorite(h.member.ID, favorite)
	h.userData.played(h.admin.ID, played)
	now := time.Now()
	episode := func(series accounts.ID, number int, released time.Duration, available bool) library.Item {
		date := now.Add(released)
		return library.Item{ID: accounts.ID{series[0], byte(number)}, Kind: library.KindEpisode, Name: "Episode " + string(rune('A'+number)),
			SeriesID: series, SeriesName: "Series", ParentIndexNumber: 1, IndexNumber: number, PremiereDate: &date, Available: available}
	}
	h.library.set(played, episode(played, 1, -48*time.Hour, true), episode(played, 2, -time.Hour, true), episode(played, 3, 2*time.Hour, false))
	h.library.set(favorite, episode(favorite, 1, -10*24*time.Hour, true))
	h.library.set(other, episode(other, 1, -time.Hour, true))

	// The first look tells nothing: what the series have is not news.
	h.CheckEpisodes(t.Context())
	h.idle(t)
	if got := len(h.targets.at("/member")) + len(h.targets.at("/admin")) + len(h.targets.at("/server")); got != 0 {
		t.Fatalf("%d messages at the first look", got)
	}

	// Then the third episode is out, the favorite has a new one, an old
	// one is added late, and the series no one follows has a new one.
	h.library.set(played, episode(played, 1, -48*time.Hour, true), episode(played, 2, -time.Hour, true), episode(played, 3, -time.Minute, true))
	h.library.set(favorite, episode(favorite, 0, -30*24*time.Hour, true), episode(favorite, 1, -10*24*time.Hour, true),
		episode(favorite, 2, -time.Minute, true))
	h.library.set(other, episode(other, 1, -time.Hour, true), episode(other, 2, -time.Minute, true))
	h.CheckEpisodes(t.Context())
	h.idle(t)
	member := types(h.targets.at("/member"))
	slices.Sort(member)
	if want := []string{"new_episode Episode C", "new_episode Episode D"}; !slices.Equal(member, want) {
		t.Errorf("member's target: %v, want %v", member, want)
	}
	// The admin follows the first series only; the member's favorite is
	// not theirs.
	if got, want := types(h.targets.at("/admin")), []string{"new_episode Episode D"}; !slices.Equal(got, want) {
		t.Errorf("admin's target: %v, want %v", got, want)
	}
	// Both follow the first series: the server is told of its episode once,
	// with no user.
	server := h.targets.at("/server")
	got := types(server)
	slices.Sort(got)
	if want := []string{"new_episode Episode C", "new_episode Episode D"}; !slices.Equal(got, want) {
		t.Errorf("server's target: %v, want %v", got, want)
	}
	for _, r := range server {
		if r.body["user"] != nil {
			t.Errorf("a server message names a user: %v", r.body["user"])
		}
	}
	memberMessage := h.targets.at("/member")[0].body
	if memberMessage["user"].(map[string]any)["name"] != "member" || memberMessage["title"] != "New episode of Series" {
		t.Errorf("member's message: %v", memberMessage)
	}

	// Looking again tells nothing more.
	h.CheckEpisodes(t.Context())
	h.idle(t)
	if a, b, c := len(h.targets.at("/member")), len(h.targets.at("/admin")), len(h.targets.at("/server")); a != 2 || b != 1 || c != 2 {
		t.Errorf("after looking again: %d, %d and %d messages", a, b, c)
	}
}

// A recording that ended is told to the targets of the user who made it
// and to the server's, finished or failed, never to another user's.
func TestRecordingsEndedReachTheirOwnerAndTheServer(t *testing.T) {
	h := newHarness(t)
	events := []string{RecordingFinished, RecordingFailed}
	h.add(t, &h.member, h.webhook("/member", events...))
	h.add(t, &h.admin, h.webhook("/admin", events...))
	h.add(t, nil, h.webhook("/server", events...))
	// A target that did not choose recordings hears nothing of them.
	h.add(t, &h.member, h.webhook("/episodes", NewEpisode))

	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Finished Show"})
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{2}, User: &h.member.ID, Name: "Empty Show", Failed: true})
	h.targets.wait(t, "/member", 2)
	h.targets.wait(t, "/server", 2)
	h.idle(t)
	want := []string{"recording_failed Empty Show", "recording_finished Finished Show"}
	for _, path := range []string{"/member", "/server"} {
		got := types(h.targets.at(path))
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	if got := len(h.targets.at("/admin")) + len(h.targets.at("/episodes")); got != 0 {
		t.Errorf("%d messages reached targets of other users or events", got)
	}
}

// A problem System › Health shows is told once two checks in a row found
// it, to the server's targets and administrators' own, and once solved
// when two checks in a row no longer do; a brief one is never told, and a
// restart neither tells a problem again nor forgets it, nor the page that
// shows it, which both messages open.
func TestHealthProblemsFoundAndSolved(t *testing.T) {
	h := newHarness(t)
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	events := []string{HealthProblem, HealthSolved}
	h.add(t, nil, h.webhook("/server", events...))
	h.add(t, &h.admin, h.webhook("/admin", events...))
	// A member may not choose health events; an administrator's target
	// keeps them once its owner is no longer one, but they stop reaching it.
	if _, err := h.Create(t.Context(), &h.member, h.webhook("/member", events...)); err != ErrInvalidEvents {
		t.Errorf("a member choosing health events: %v", err)
	}
	demoted, err := h.store.CreateUser(t.Context(), accounts.NewUser{Name: "demoted", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	h.add(t, &demoted, h.webhook("/demoted", events...))
	if _, err := h.store.UpdateUser(t.Context(), demoted.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}

	disk := Problem{Key: "disk:cache", Severity: SeverityWarning, Text: "Little space left for the cache: 1.0 GB free.", Page: "/system/health#disks"}
	blip := Problem{Key: "addon:x", Severity: SeverityWarning, Text: "Addon: could not be reached"}
	check := func(problems ...Problem) {
		*h.problems = problems
		h.CheckHealth(t.Context())
		h.idle(t)
	}
	check(disk, blip)
	check(disk)
	check(disk)
	got := types(h.targets.at("/server"))
	if want := []string{"health_problem disk:cache"}; !slices.Equal(got, want) {
		t.Fatalf("after three checks: %v, want %v", got, want)
	}

	// A restart keeps what was told, and the page that shows it: a second
	// restart finds the problem gone without seeing it again.
	restart := func() *Service {
		s := New(Options{DB: h.db, Accounts: h.store, Library: h.library, UserData: h.userData, Secrets: h.box, Version: "1.2.3",
			Logger: h.logger, Problems: h.problems2()})
		t.Cleanup(s.Close)
		return s
	}
	restarted := restart()
	*h.problems = []Problem{disk}
	restarted.CheckHealth(t.Context())
	restarted = restart()
	*h.problems = nil
	restarted.CheckHealth(t.Context())
	if got := len(h.targets.at("/server")); got != 1 {
		t.Errorf("%d messages after a restart and one check without the problem", got)
	}
	restarted.CheckHealth(t.Context())
	h.targets.wait(t, "/server", 2)
	h.targets.wait(t, "/admin", 2)
	restarted.CheckHealth(t.Context())
	time.Sleep(50 * time.Millisecond)
	for _, path := range []string{"/server", "/admin"} {
		got := types(h.targets.at(path))
		if want := []string{"health_problem disk:cache", "health_solved disk:cache"}; !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	const page = "https://media.example.org/admin/system/health#disks"
	if found := h.targets.at("/server")[0].body; found["url"] != page {
		t.Errorf("found message opens %v, want %s", found["url"], page)
	}
	solved := h.targets.at("/server")[1].body
	if solved["title"] != "Problem solved" || solved["message"] != disk.Text || solved["user"] != nil || solved["url"] != page {
		t.Errorf("solved message: %v", solved)
	}
	if got := len(h.targets.at("/demoted")); got != 0 {
		t.Errorf("%d health messages reached a user who is no longer an administrator", got)
	}
}

// problems2 reads the problems the harness's test sets, for another
// service on the same database.
func (h harness) problems2() func(context.Context) ([]Problem, error) {
	return func(context.Context) ([]Problem, error) { return slices.Clone(*h.problems), nil }
}

// A message is tried again after server errors and network errors, waits
// as long as a target asks with Retry-After, and is given up after a
// while: the target then shows as unreachable.
func TestDeliveryRetriesWaitsAndGivesUp(t *testing.T) {
	h := newHarness(t)
	// Long enough for the Retry-After below, short enough to wait for.
	h.timing.giveUp = 2 * time.Second
	h.add(t, nil, h.webhook("/flaky", RecordingFinished))
	h.targets.then("/flaky", status(http.StatusServiceUnavailable), status(http.StatusBadGateway), status(http.StatusTooManyRequests, "Retry-After", "1"))
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	got := h.targets.wait(t, "/flaky", 4)
	h.idle(t)
	if len(got) != 4 {
		t.Fatalf("%d attempts, want 4", len(got))
	}
	if wait := got[3].at.Sub(got[2].at); wait < 900*time.Millisecond {
		t.Errorf("tried again %v after Retry-After: 1", wait)
	}
	if first, last := got[0].body["id"], got[3].body["id"]; first != last {
		t.Errorf("a retry is another event: %v, %v", first, last)
	}
	targets, _ := h.Targets(t.Context(), nil)
	if targets[0].Problem != "" || targets[0].LastSentAt == nil {
		t.Errorf("after delivering: problem %q, last sent %v", targets[0].Problem, targets[0].LastSentAt)
	}

	// A target that never answers well is given up on.
	h.add(t, nil, h.webhook("/down", RecordingFailed))
	h.targets.always("/down", status(http.StatusInternalServerError))
	start := time.Now()
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{2}, User: &h.member.ID, Name: "Show", Failed: true})
	h.targets.wait(t, "/down", 2)
	h.idle(t)
	if elapsed := time.Since(start); elapsed < 1500*time.Millisecond || elapsed > 4*time.Second {
		t.Errorf("given up after %v", elapsed)
	}
	attempts := len(h.targets.at("/down"))
	time.Sleep(200 * time.Millisecond)
	if len(h.targets.at("/down")) != attempts {
		t.Error("tried again after giving up")
	}
	targets, _ = h.Targets(t.Context(), nil)
	if targets[1].Problem != ProblemUnreachable || targets[1].ProblemStatus == nil || *targets[1].ProblemStatus != 500 {
		t.Errorf("a target given up on: %+v", targets[1])
	}
	if !strings.Contains(h.log.String(), "could not be delivered in time") {
		t.Error("giving up is not logged")
	}
}

// A target answering 401, 403 or 404 is marked failing at once, with the
// status, and the message is not tried again; "Send a test" tells the
// same, and a delivered message clears it.
func TestRefusingTargetsAreMarkedFailing(t *testing.T) {
	h := newHarness(t)
	target := h.add(t, nil, h.webhook("/gone", RecordingFinished))
	h.targets.then("/gone", status(http.StatusNotFound))
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	h.targets.wait(t, "/gone", 1)
	h.idle(t)
	if got := len(h.targets.at("/gone")); got != 1 {
		t.Errorf("a refused message was tried %d times", got)
	}
	targets, _ := h.Targets(t.Context(), nil)
	if targets[0].Problem != ProblemRefused || targets[0].ProblemStatus == nil || *targets[0].ProblemStatus != 404 {
		t.Errorf("a refusing target: %+v", targets[0])
	}

	h.targets.then("/gone", status(http.StatusForbidden))
	result, err := h.Test(t.Context(), nil, target.ID)
	if err != nil || result.Delivered || result.Status != 403 || result.Target.Problem != ProblemRefused {
		t.Errorf("a test refused: %+v, %v", result, err)
	}
	result, err = h.Test(t.Context(), nil, target.ID)
	if err != nil || !result.Delivered || result.Target.Problem != "" || result.Target.LastSentAt == nil {
		t.Errorf("a test delivered: %+v, %v", result, err)
	}
	test := h.targets.at("/gone")[2].body
	if test["type"] != Test || test["title"] != "Test message" {
		t.Errorf("test message: %v", test)
	}
	// Another owner's target cannot be tested.
	if _, err := h.Test(t.Context(), &h.member, target.ID); err != ErrNotFound {
		t.Errorf("testing the server's target as a member: %v", err)
	}
}

// Discord addresses and ntfy tokens are sealed in the database, never
// shown but for the address's host, and never logged, whatever happens
// to the messages sent with them.
func TestSecretsAreSealedNeverShownNorLogged(t *testing.T) {
	h := newHarness(t)
	discordPath := "/api/webhooks/123/very-secret-webhook-token"
	h.add(t, nil, Draft{Kind: Discord, Name: new("Channel"), Address: new(h.targets.url + discordPath), Events: []string{RecordingFinished}})
	h.add(t, &h.admin, Draft{Kind: Ntfy, Name: new("Phone"), Address: new(h.targets.url + "/ntfy"), Topic: new("topic"),
		Token: new("tk_very-secret-ntfy-token"), Events: []string{RecordingFinished}})
	h.targets.then(discordPath, status(http.StatusTooManyRequests, "Retry-After", "0"), status(http.StatusNotFound))
	h.targets.always("/ntfy/", status(http.StatusUnauthorized))
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.admin.ID, Name: "Show"})
	h.targets.wait(t, discordPath, 2)
	h.targets.wait(t, "/ntfy/", 1)
	h.idle(t)

	rows, err := h.db.Query(t.Context(), "SELECT secret FROM notification_targets")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !secrets.Sealed(stored) || strings.Contains(stored, "very-secret") {
			t.Errorf("stored as %q", stored)
		}
	}
	for _, owner := range []*accounts.User{nil, &h.admin} {
		targets, err := h.Targets(t.Context(), owner)
		if err != nil || len(targets) != 1 {
			t.Fatalf("targets: %v, %v", targets, err)
		}
		if shown, _ := json.Marshal(targets); strings.Contains(string(shown), "very-secret") {
			t.Errorf("shown: %s", shown)
		}
		if owner == nil && targets[0].Address != h.targets.url {
			t.Errorf("a Discord target shows %q", targets[0].Address)
		}
		if owner != nil && !targets[0].TokenSet {
			t.Error("an ntfy target with a token shows none")
		}
	}
	if log := h.log.String(); strings.Contains(log, "very-secret") || !strings.Contains(log, "refused a message") {
		t.Errorf("log: %s", log)
	}
}

// A member's target may only reach public addresses: one on the local
// network is refused when added, unlike an administrator's, and messages
// to one whose address now points there do not reach it.
func TestMembersTargetsStayOnPublicAddresses(t *testing.T) {
	h := newHarness(t)
	moved := h.add(t, &h.member, h.webhook("/moved", RecordingFinished))
	h.checkPublic, h.confined = stremio.CheckPublic, stremio.HTTPClient(true)
	if _, err := h.Create(t.Context(), &h.member, h.webhook("/member", NewEpisode)); err != ErrPrivateAddress {
		t.Errorf("a member's target on the local network: %v", err)
	}
	h.add(t, &h.admin, h.webhook("/admin", NewEpisode))

	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	time.Sleep(50 * time.Millisecond)
	h.idle(t)
	if result, err := h.Test(t.Context(), &h.member, moved.ID); err != nil || result.Delivered || result.Target.Problem != ProblemUnreachable {
		t.Errorf("testing it: %+v, %v", result, err)
	}
	if got := len(h.targets.at("/moved")); got != 0 {
		t.Errorf("%d messages reached a member's target on the local network", got)
	}
}
