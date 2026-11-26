package moonfin

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// titles lists the titles of messages.
func titles(messages []Message) []string {
	result := make([]string, len(messages))
	for i, m := range messages {
		result[i] = m.Title
	}
	return result
}

// A user is given the messages of their audience that show now, in the
// administrator's order, and no other: not another user's, not the
// administrators' to a member, not one before its start or after its end.
// A deleted user's messages go with them.
func TestMessagesReachTheirAudienceBetweenTheirDates(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "sam", "robin", "boss")
	sam := accounts.User{ID: ids[0]}
	robin := accounts.User{ID: ids[1]}
	boss := accounts.User{ID: ids[2], IsAdministrator: true}
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	ctx := t.Context()
	for _, m := range []Message{
		{Title: "everyone", Audience: AudienceAll},
		{Title: "administrators", Audience: AudienceAdmins},
		{Title: "sam", Audience: AudienceUsers, Users: []accounts.ID{sam.ID}},
		{Title: "robin", Audience: AudienceUsers, Users: []accounts.ID{robin.ID}},
		{Title: "sam and robin", Audience: AudienceUsers, Users: []accounts.ID{robin.ID, sam.ID, sam.ID}},
		{Title: "tomorrow", Audience: AudienceAll, Start: now.Add(24 * time.Hour)},
		{Title: "yesterday", Audience: AudienceAll, Start: now.Add(-48 * time.Hour), End: now.Add(-24 * time.Hour)},
		{Title: "this week", Audience: AudienceAll, Start: now.Add(-time.Hour), End: now.Add(7 * 24 * time.Hour)},
		{Title: "ends now", Audience: AudienceAll, End: now},
	} {
		m.Color, m.Delivery = ColorBlue, DeliveryInbox
		if _, err := s.CreateMessage(ctx, m); err != nil {
			t.Fatalf("%s: %v", m.Title, err)
		}
	}
	shown := func(user accounts.User) []string {
		t.Helper()
		messages, err := s.MessagesFor(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		return titles(messages)
	}
	// The newest first.
	if got, want := shown(sam), []string{"this week", "sam and robin", "sam", "everyone"}; !slices.Equal(got, want) {
		t.Errorf("sam's: %q, want %q", got, want)
	}
	if got, want := shown(robin), []string{"this week", "sam and robin", "robin", "everyone"}; !slices.Equal(got, want) {
		t.Errorf("robin's: %q, want %q", got, want)
	}
	if got, want := shown(boss), []string{"this week", "administrators", "everyone"}; !slices.Equal(got, want) {
		t.Errorf("the administrator's: %q, want %q", got, want)
	}
	all, err := s.Messages(ctx)
	if err != nil || len(all) != 9 {
		t.Fatalf("every message: %d %v", len(all), err)
	}
	both := all[slices.IndexFunc(all, func(m Message) bool { return m.Title == "sam and robin" })]
	if len(both.Users) != 2 {
		t.Errorf("users kept once each: %v", both.Users)
	}

	now = now.Add(25 * time.Hour)
	if got := shown(sam); !slices.Contains(got, "tomorrow") {
		t.Errorf("after its start: %q", got)
	}

	if _, err := s.db.Exec(ctx, "DELETE FROM users WHERE id = $1", robin.ID); err != nil {
		t.Fatal(err)
	}
	if got := shown(robin); slices.Contains(got, "robin") || slices.Contains(got, "sam and robin") {
		t.Errorf("a deleted user's: %q", got)
	}
	if got := shown(sam); !slices.Contains(got, "sam and robin") {
		t.Errorf("sam's after robin left: %q", got)
	}
}

// Messages are kept as written, cleaned of spaces, and refused when Valid
// says so; their order changes, and an edit keeps a message's place.
func TestMessagesAreValidatedOrderedAndEdited(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "sam")
	ctx := t.Context()
	valid := Message{Title: "Maintenance", Body: "Back **soon**.", Color: ColorYellow, Delivery: DeliveryPopup, Audience: AudienceAll,
		LinkLabel: "Status", LinkURL: "https://status.example.com/now"}
	start := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	for name, change := range map[string]func(*Message){
		"neither title nor body":  func(m *Message) { m.Title, m.Body = " ", "" },
		"a title too long":        func(m *Message) { m.Title = strings.Repeat("é", MaxTitleLength+1) },
		"a body too long":         func(m *Message) { m.Body = strings.Repeat("a", MaxBodyLength+1) },
		"a NUL character":         func(m *Message) { m.Body = "a\x00b" },
		"an unknown color":        func(m *Message) { m.Color = "purple" },
		"an unknown delivery":     func(m *Message) { m.Delivery = "push" },
		"an unknown audience":     func(m *Message) { m.Audience = "guests" },
		"users without users":     func(m *Message) { m.Audience = AudienceUsers },
		"an end at the start":     func(m *Message) { m.Start, m.End = start, start },
		"an end before the start": func(m *Message) { m.Start, m.End = start, start.Add(-time.Hour) },
		"a label without a link":  func(m *Message) { m.LinkURL = "" },
		"a link without a label":  func(m *Message) { m.LinkLabel = "" },
		"a javascript link":       func(m *Message) { m.LinkURL = "javascript:alert(1)" },
		"a file link":             func(m *Message) { m.LinkURL = "file:///etc/passwd" },
		"a link with no host":     func(m *Message) { m.LinkURL = "https:///path" },
		"a relative link":         func(m *Message) { m.LinkURL = "/web/index.html" },
		"a label too long":        func(m *Message) { m.LinkLabel = strings.Repeat("a", MaxLinkLabelLength+1) },
	} {
		m := valid
		change(&m)
		if _, err := s.CreateMessage(ctx, m); !errors.Is(err, ErrInvalidMessage) {
			t.Errorf("%s: %v", name, err)
		}
	}
	spaced := valid
	spaced.Title, spaced.Body, spaced.LinkURL = "  Maintenance ", "\nBack **soon**.\n", " https://status.example.com/now "
	first, err := s.CreateMessage(ctx, spaced)
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != "Maintenance" || first.Body != "Back **soon**." || first.LinkURL != "https://status.example.com/now" ||
		first.Created.IsZero() || !first.Updated.Equal(first.Created) {
		t.Errorf("kept: %+v", first)
	}
	second, err := s.CreateMessage(ctx, Message{Body: "Only a body", Color: ColorWhite, Delivery: DeliveryInbox, Audience: AudienceUsers,
		Users: []accounts.ID{ids[0]}, Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Start.Equal(start) || !second.End.Equal(start.Add(time.Hour)) || len(second.Users) != 1 {
		t.Errorf("dates and users: %+v", second)
	}
	third, err := s.CreateMessage(ctx, Message{Title: "Third", Color: ColorRed, Delivery: DeliveryInbox, Audience: AudienceAdmins,
		Users: []accounts.ID{ids[0]}})
	if err != nil || third.Users != nil {
		t.Fatalf("users of another audience dropped: %+v %v", third.Users, err)
	}
	order := func() []accounts.ID {
		t.Helper()
		all, err := s.Messages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		result := make([]accounts.ID, len(all))
		for i, m := range all {
			result[i] = m.ID
		}
		return result
	}
	if got := order(); !slices.Equal(got, []accounts.ID{third.ID, second.ID, first.ID}) {
		t.Errorf("newest first: %v", got)
	}
	if err := s.OrderMessages(ctx, []accounts.ID{first.ID}); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []accounts.ID{first.ID, third.ID, second.ID}) {
		t.Errorf("moved first, the others after it: %v", got)
	}
	edited := second
	edited.Title, edited.Audience, edited.Users = "Edited", AudienceAll, nil
	if updated, err := s.UpdateMessage(ctx, edited); err != nil || updated.Title != "Edited" || updated.Users != nil ||
		!updated.Created.Equal(second.Created) {
		t.Errorf("edited: %+v %v", updated, err)
	}
	if got := order(); !slices.Equal(got, []accounts.ID{first.ID, third.ID, second.ID}) {
		t.Errorf("an edit keeps its place: %v", got)
	}
	missing := edited
	missing.ID = accounts.ID{1}
	if _, err := s.UpdateMessage(ctx, missing); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("editing an unknown message: %v", err)
	}
	if err := s.DeleteMessage(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMessage(ctx, third.ID); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("deleting it again: %v", err)
	}
	for range MaxMessages - 2 {
		if _, err := s.CreateMessage(ctx, valid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateMessage(ctx, valid); !errors.Is(err, ErrTooManyMessages) {
		t.Errorf("past the most: %v", err)
	}
}

// Those who watch a user's messages are told when a message meant for the
// user is saved or deleted, before and after a change of its audience,
// when the messages are ordered, and when one starts showing; never of a
// message meant for others.
func TestMessageWatchersAreToldOfTheirMessages(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "sam", "robin", "boss")
	ctx := t.Context()
	if _, err := s.db.Exec(ctx, "UPDATE users SET is_administrator = true WHERE id = $1", ids[2]); err != nil {
		t.Fatal(err)
	}
	sam, stopSam := s.WatchMessages(ctx, ids[0])
	defer stopSam()
	robin, stopRobin := s.WatchMessages(ctx, ids[1])
	defer stopRobin()
	boss, stopBoss := s.WatchMessages(ctx, ids[2])
	defer stopBoss()
	settings, stopSettings := s.Watch(ids[0])
	defer stopSettings()
	expect := func(step string, samTold, robinTold, bossTold bool) {
		t.Helper()
		for _, w := range []struct {
			name    string
			changed <-chan struct{}
			want    bool
		}{{"sam", sam, samTold}, {"robin", robin, robinTold}, {"boss", boss, bossTold}} {
			if w.want {
				if !toldNow(w.changed) {
					t.Errorf("%s: %s not told", step, w.name)
				}
				continue
			}
			select {
			case <-w.changed:
				t.Errorf("%s: %s told", step, w.name)
			default:
			}
		}
	}
	m, err := s.CreateMessage(ctx, Message{Title: "For sam", Color: ColorGreen, Delivery: DeliveryPopup, Audience: AudienceUsers,
		Users: []accounts.ID{ids[0]}})
	if err != nil {
		t.Fatal(err)
	}
	expect("created for sam", true, false, false)
	m.Title = "Still for sam"
	if m, err = s.UpdateMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	expect("edited for sam", true, false, false)
	m.Audience, m.Users = AudienceAdmins, nil
	if m, err = s.UpdateMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	expect("moved to the administrators", true, false, true)
	m.Audience = AudienceAll
	if m, err = s.UpdateMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	expect("moved to everyone", true, true, true)
	m.Audience, m.Users = AudienceUsers, []accounts.ID{ids[1]}
	if m, err = s.UpdateMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	expect("moved from everyone to robin", true, true, true)
	if err := s.DeleteMessage(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	expect("deleted", false, true, false)
	if err := s.OrderMessages(ctx, nil); err != nil {
		t.Fatal(err)
	}
	expect("ordered", true, true, true)
	select {
	case <-settings:
		t.Error("the settings' watcher was told of messages")
	default:
	}

	// A message starting in a moment, which the timer tells of.
	if _, err := s.CreateMessage(ctx, Message{Title: "Soon", Color: ColorWhite, Delivery: DeliveryPopup, Audience: AudienceUsers,
		Users: []accounts.ID{ids[1]}, Start: time.Now().Add(300 * time.Millisecond)}); err != nil {
		t.Fatal(err)
	}
	expect("created for later", false, true, false)
	select {
	case <-robin:
	case <-time.After(5 * time.Second):
		t.Error("robin not told when the message started")
	}
	expect("started", false, false, false)
}
