package jellyfin

import (
	"bufio"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// moonfinMessages creates messages in the store of s, and fails the test
// on an error.
func moonfinMessages(t *testing.T, s testServer, messages ...moonfin.Message) []moonfin.Message {
	t.Helper()
	created := make([]moonfin.Message, len(messages))
	for i, m := range messages {
		if m.Color == "" {
			m.Color, m.Delivery = moonfin.ColorBlue, moonfin.DeliveryInbox
		}
		var err error
		if created[i], err = s.handler.Moonfin.CreateMessage(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	return created
}

// shownMoonfinMessages calls GET /Moonfin/Messages as auth, and returns
// the status and the messages answered.
func shownMoonfinMessages(t *testing.T, s testServer, auth string) (int, []map[string]any) {
	t.Helper()
	status, data := s.call(http.MethodGet, "/Moonfin/Messages", auth, nil)
	var answer struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(data, &answer)
	return status, answer.Items
}

// messageTitles lists the titles of messages as the apps read them.
func messageTitles(items []map[string]any) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i], _ = item["title"].(string)
	}
	return result
}

// The Ping tells Moonfin's apps that messages are served; each user is
// given the messages of their audience that show now, as the apps read
// them, without whom else they are meant for; a user the companion is not
// served to gets 404, and a disabled user nothing.
func TestMoonfinMessagesReachTheirAudienceAlone(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	root, alice, bob := app("dashboard", s.signIn("root", "dashboard")), app("phone", s.signIn("alice", "phone")), app("tv", s.signIn("bob", "tv"))
	if _, ping := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Ping", alice, nil); ping["messagesSupported"] != true {
		t.Errorf("messagesSupported: %v", ping["messagesSupported"])
	}
	moonfinMessages(t, s,
		moonfin.Message{Title: "Everyone", Body: "Hello **all**.", Audience: moonfin.AudienceAll},
		moonfin.Message{Title: "Administrators", Audience: moonfin.AudienceAdmins},
		moonfin.Message{Title: "Alice", Audience: moonfin.AudienceUsers, Users: []accounts.ID{users["alice"].ID}},
		moonfin.Message{Title: "Later", Audience: moonfin.AudienceAll, Start: time.Now().Add(time.Hour)},
		moonfin.Message{Title: "Over", Audience: moonfin.AudienceAll, End: time.Now().Add(-time.Hour)},
		moonfin.Message{Title: "Bob", Body: "See this.", Color: moonfin.ColorRed, Delivery: moonfin.DeliveryPopup, Audience: moonfin.AudienceUsers,
			Users: []accounts.ID{users["bob"].ID}, LinkLabel: "Open", LinkURL: "https://example.com/news"},
	)
	for _, c := range []struct {
		name, auth string
		want       []string
	}{
		{"root", root, []string{"Administrators", "Everyone"}},
		{"alice", alice, []string{"Alice", "Everyone"}},
		{"bob", bob, []string{"Bob", "Everyone"}},
	} {
		status, items := shownMoonfinMessages(t, s, c.auth)
		if got := messageTitles(items); status != http.StatusOK || !slices.Equal(got, c.want) {
			t.Errorf("%s's messages: %d %q, want %q", c.name, status, got, c.want)
		}
	}
	_, items := shownMoonfinMessages(t, s, bob)
	if len(items) == 0 {
		t.Fatal("bob has no messages")
	}
	message := items[0]
	if message["body"] != "See this." || message["color"] != "red" || message["delivery"] != "popup" ||
		message["actionLabel"] != "Open" || message["actionUrl"] != "https://example.com/news" || message["id"] == "" {
		t.Errorf("bob's message: %v", message)
	}
	if created, err := time.Parse(time.RFC3339, message["createdUtc"].(string)); err != nil || time.Since(created) > time.Minute {
		t.Errorf("createdUtc: %v %v", message["createdUtc"], err)
	}
	for _, kept := range []string{"audience", "users", "start", "end", "startUtc", "endUtc", "targetUserIds"} {
		if _, ok := message[kept]; ok {
			t.Errorf("an app is given %s", kept)
		}
	}
	if _, ok := items[1]["actionUrl"]; ok {
		t.Errorf("a message without a link has an action: %v", items[1])
	}

	off := accounts.MoonfinOff
	if _, err := s.store.UpdateUser(t.Context(), users["bob"].ID, accounts.UserChanges{Moonfin: &off}, nil); err != nil {
		t.Fatal(err)
	}
	if status, items := shownMoonfinMessages(t, s, bob); status != http.StatusNotFound || len(items) != 0 {
		t.Errorf("bob, not served the companion: %d %v", status, items)
	}
	if _, err := s.store.UpdateUser(t.Context(), users["alice"].ID, accounts.UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, items := shownMoonfinMessages(t, s, alice); status != http.StatusUnauthorized || len(items) != 0 {
		t.Errorf("alice, disabled: %d %v", status, items)
	}
	if status, _ := s.call(http.MethodGet, "/Moonfin/Messages", "", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}

// An app's open stream is told "messagesChanged" when a message meant for
// its user is saved, and not when one meant for another is.
func TestMoonfinStreamTellsOfMessages(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	tv := app("tv", s.signIn("alice", "tv"))
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+"/Moonfin/Settings/Stream", nil)
	request.Header.Set("Authorization", tv)
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	if line := <-lines; !strings.HasPrefix(line, ":") {
		t.Fatalf("first line: %q", line)
	}
	// Bob's message first: the next event is alice's.
	moonfinMessages(t, s,
		moonfin.Message{Title: "Bob", Audience: moonfin.AudienceUsers, Users: []accounts.ID{users["bob"].ID}},
		moonfin.Message{Title: "Alice", Audience: moonfin.AudienceUsers, Users: []accounts.ID{users["alice"].ID}},
	)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("the stream ended")
			}
			if line == `data: {"type":"messagesChanged"}` {
				return
			}
			if strings.HasPrefix(line, "data:") {
				t.Fatalf("event: %q", line)
			}
		case <-deadline:
			t.Fatal("no messagesChanged")
		}
	}
}

// A Moonfin app signed in through Silo's API reads its user's messages
// under the plugin's content path.
func TestSiloAppsReadMoonfinMessages(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	s.user("bob", nil)
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "alice")
	moonfinMessages(t, s,
		moonfin.Message{Title: "Alice", Audience: moonfin.AudienceUsers, Users: []accounts.ID{alice.ID}},
		moonfin.Message{Title: "Administrators", Audience: moonfin.AudienceAdmins},
	)
	status, data := a.do(t, http.MethodGet, "/api/v2/plugin-content/plugins/moonfin/Moonfin/Messages", token, profile, nil)
	var answer struct {
		Items []map[string]any `json:"items"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &answer) != nil || !slices.Equal(messageTitles(answer.Items), []string{"Alice"}) {
		t.Errorf("messages through Silo: %d %s", status, data)
	}
	bobToken, bobProfile := a.signIn(t, "bob")
	if status, data = a.do(t, http.MethodGet, "/api/v2/plugin-content/plugins/moonfin/Moonfin/Messages", bobToken, bobProfile, nil); status != http.StatusOK ||
		json.Unmarshal(data, &answer) != nil || len(answer.Items) != 0 {
		t.Errorf("bob's messages through Silo: %d %s", status, data)
	}
}
