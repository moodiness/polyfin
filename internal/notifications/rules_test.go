package notifications

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/plays"
)

// tell dispatches an event of kind, titled title, to every target that
// chose it, as the services' events are.
func (h harness) tell(kind, title string) {
	ev := h.newEvent(kind)
	ev.Title = title
	h.dispatch(ev, func(target) bool { return true })
}

// A target that caps its messages a minute is sent no more: the others
// wait for the minute of the first to pass, in order, and all arrive.
func TestRateCapSpacesTheMessages(t *testing.T) {
	h := newHarness(t)
	h.timing.minute = time.Second
	h.add(t, nil, Draft{Kind: Webhook, Name: new("Capped"), Address: new(h.targets.url + "/capped"), Events: []string{PlaybackStarted},
		PerMinute: new(2)})
	for i := range 5 {
		h.tell(PlaybackStarted, fmt.Sprint("play ", i))
	}
	got := h.targets.wait(t, "/capped", 5)
	h.idle(t)
	for i, r := range got {
		if r.body["title"] != fmt.Sprint("play ", i) {
			t.Errorf("message %d: %v", i, r.body["title"])
		}
		// Each message waits for the minute of the one two before it.
		if i >= 2 {
			if gap := r.at.Sub(got[i-2].at); gap < h.timing.minute {
				t.Errorf("message %d came %s after message %d, under the cap's minute", i, gap, i-2)
			}
		}
	}
	if len(h.targets.at("/capped")) != 5 {
		t.Errorf("%d messages, want 5", len(h.targets.at("/capped")))
	}
}

// The caps and groupings are checked, and kept.
func TestPacingIsCheckedAndKept(t *testing.T) {
	h := newHarness(t)
	draft := h.webhook("/paced", PlaybackStarted)
	for _, bad := range []Draft{{PerMinute: new(61)}, {PerMinute: new(-1)}, {GroupMinutes: new(1441)}, {GroupMinutes: new(-5)}} {
		d := draft
		d.PerMinute, d.GroupMinutes = bad.PerMinute, bad.GroupMinutes
		if _, err := h.Create(t.Context(), nil, d); !errors.Is(err, ErrInvalidPerMinute) && !errors.Is(err, ErrInvalidGroupMinutes) {
			t.Errorf("%v per minute, %v minutes: %v", deref(bad.PerMinute), deref(bad.GroupMinutes), err)
		}
	}
	draft.PerMinute, draft.GroupMinutes = new(60), new(1440)
	target := h.add(t, nil, draft)
	if target.PerMinute != 60 || target.GroupMinutes != 1440 {
		t.Errorf("created: %+v", target)
	}
	changed, err := h.Update(t.Context(), nil, target.ID, Draft{PerMinute: new(0), GroupMinutes: new(15)})
	if err != nil || changed.PerMinute != 0 || changed.GroupMinutes != 15 {
		t.Errorf("changed: %+v, %v", changed, err)
	}
	listed, err := h.Targets(t.Context(), nil)
	if err != nil || len(listed) != 1 || listed[0].PerMinute != 0 || listed[0].GroupMinutes != 15 {
		t.Errorf("listed: %+v, %v", listed, err)
	}
}

func deref(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

// A target that groups holds the plays and new episodes for its minutes,
// then sends them as one message, each event whole for a webhook; a
// failure goes at once. A group of one event is sent as it is.
func TestGroupingHoldsPlaysAndSendsUrgentEventsAtOnce(t *testing.T) {
	h := newHarness(t)
	h.timing.minute = 2 * time.Second
	h.add(t, nil, Draft{Kind: Webhook, Name: new("Grouped"), Address: new(h.targets.url + "/grouped"),
		Events: []string{PlaybackStarted, PlaybackStopped, NewEpisode, RecordingFailed}, GroupMinutes: new(1)})
	discord := h.add(t, nil, Draft{Kind: Discord, Name: new("Chat"), Address: new(h.targets.url + "/discord"),
		Events: []string{PlaybackStarted, PlaybackStopped, NewEpisode}, GroupMinutes: new(1)})

	start := time.Now()
	h.tell(PlaybackStarted, "Sam is watching A")
	h.tell(NewEpisode, "New episode of B")
	h.tell(RecordingFailed, "Recording failed: C")
	h.tell(PlaybackStopped, "Sam stopped A")
	got := h.targets.wait(t, "/grouped", 2)
	h.idle(t)
	if len(got) != 2 {
		t.Fatalf("%d messages, want 2: %v", len(got), types(got))
	}
	var urgentAt, groupAt time.Time
	for _, r := range got {
		switch r.body["type"] {
		case RecordingFailed:
			urgentAt = r.at
		case Grouped:
			groupAt = r.at
			events, _ := r.body["events"].([]any)
			var titles []string
			for _, e := range events {
				titles = append(titles, e.(map[string]any)["title"].(string))
			}
			if strings.Join(titles, "|") != "Sam is watching A|New episode of B|Sam stopped A" {
				t.Errorf("grouped events: %q", titles)
			}
			if r.body["title"] != "3 notifications" ||
				r.body["message"] != "• Sam is watching A\n• New episode of B\n• Sam stopped A" {
				t.Errorf("grouped message: %q, %q", r.body["title"], r.body["message"])
			}
		default:
			t.Errorf("unexpected message: %v", r.body)
		}
	}
	if urgentAt.IsZero() || groupAt.IsZero() {
		t.Fatalf("messages: %v", types(got))
	}
	if groupAt.Sub(start) < h.timing.minute {
		t.Errorf("the group left %s after the first play, under its minute", groupAt.Sub(start))
	}
	chat := h.targets.wait(t, "/discord", 1)[0].body
	embed := chat["embeds"].([]any)[0].(map[string]any)
	if embed["title"] != "3 notifications" || !strings.Contains(embed["description"].(string), "• New episode of B") {
		t.Errorf("Discord's group: %v", embed)
	}

	// A group of one is sent as the event alone.
	h.tell(PlaybackStarted, "Sam is watching D")
	alone := h.targets.wait(t, "/grouped", 3)[2]
	if alone.body["type"] != PlaybackStarted || alone.body["events"] != nil {
		t.Errorf("a group of one: %v", alone.body)
	}
	h.targets.wait(t, "/discord", 2)
	h.idle(t)
	if deliveries, err := h.Deliveries(t.Context(), nil, discord.ID); err != nil || len(deliveries) != 2 || deliveries[1].Events != 3 ||
		deliveries[1].Type != Grouped || deliveries[0].Events != 1 {
		t.Errorf("Discord's deliveries: %+v, %v", deliveries, err)
	}
}

// Each target keeps what came of its messages, tests included, newest
// first, its last 100 for 30 days, for its owner only.
func TestRecentDeliveries(t *testing.T) {
	h := newHarness(t)
	ok := h.add(t, nil, h.webhook("/ok", PlaybackStarted))
	gone := h.add(t, nil, h.webhook("/gone", PlaybackStarted))
	h.targets.always("/gone", status(404))
	h.targets.then("/ok", reply(400, `{"error":"bad payload"}`))
	h.tell(PlaybackStarted, "first")
	h.tell(PlaybackStarted, "second")
	h.targets.wait(t, "/ok", 2)
	h.targets.wait(t, "/gone", 2)
	h.idle(t)
	if _, err := h.Test(t.Context(), nil, ok.ID); err != nil {
		t.Fatal(err)
	}

	list, err := h.Deliveries(t.Context(), nil, ok.ID)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, d := range list {
		seen = append(seen, d.Type+" "+d.Title+" "+d.Outcome+" "+fmt.Sprint(d.Status))
	}
	if strings.Join(seen, "|") != "test Test message delivered 204|playback_started second delivered 204|playback_started first rejected 400" {
		t.Errorf("deliveries: %q", seen)
	}
	if list[2].Error == "" || list[2].Attempts != 1 {
		t.Errorf("rejected delivery: %+v", list[2])
	}
	refused, err := h.Deliveries(t.Context(), nil, gone.ID)
	if err != nil || len(refused) != 2 || refused[0].Outcome != DeliveryRefused || refused[0].Status != 404 {
		t.Errorf("refused deliveries: %+v, %v", refused, err)
	}
	if _, err := h.Deliveries(t.Context(), &h.member, ok.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a member reading the server's deliveries: %v", err)
	}

	// Past 100, the oldest are forgotten; past 30 days, all.
	for i := range 105 {
		h.record(ok.ID, &delivery{event: Event{ID: fmt.Sprint(i), Type: PlaybackStarted, Title: fmt.Sprint("bulk ", i)}, queued: time.Now()},
			DeliveryDelivered, result{})
	}
	if list, err = h.Deliveries(t.Context(), nil, ok.ID); err != nil || len(list) != 100 || list[0].Title != "bulk 104" || list[99].Title != "bulk 5" {
		t.Fatalf("after 105 more: %d, %v", len(list), err)
	}
	var kept int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM notification_deliveries WHERE target_id = $1", ok.ID).Scan(&kept); err != nil || kept != 100 {
		t.Errorf("%d kept, %v", kept, err)
	}
	if _, err := h.db.Exec(t.Context(), "UPDATE notification_deliveries SET at = at - interval '31 days' WHERE target_id = $1", gone.ID); err != nil {
		t.Fatal(err)
	}
	h.pruneDeliveries(t.Context())
	if refused, err = h.Deliveries(t.Context(), nil, gone.ID); err != nil || len(refused) != 0 {
		t.Errorf("deliveries past 30 days: %+v, %v", refused, err)
	}
	if list, err = h.Deliveries(t.Context(), nil, ok.ID); err != nil || len(list) != 100 {
		t.Errorf("recent deliveries after the prune: %d, %v", len(list), err)
	}
}

// A failed play reaches the server's targets and administrators' own,
// once an hour per user and title, naming the user, the title, the app
// and the reason. Members cannot choose it.
func TestFailedPlaysAreToldOnceAnHour(t *testing.T) {
	h := newHarness(t)
	var mu sync.Mutex
	now := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	h.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	h.add(t, nil, h.webhook("/server", PlaybackFailed))
	h.add(t, &h.admin, h.webhook("/admin", PlaybackFailed))
	if _, err := h.Create(t.Context(), &h.member, h.webhook("/member", PlaybackFailed)); !errors.Is(err, ErrInvalidEvents) {
		t.Errorf("a member choosing failed plays: %v", err)
	}
	movie := plays.Title{Item: accounts.ID{0x51}, Kind: plays.Movie, Name: "Night Train"}
	episode := plays.Title{Item: accounts.ID{0x52}, Kind: plays.Episode, Name: "Pilot", SeriesID: accounts.ID{0x53}, SeriesName: "Harbour",
		Season: 1, Episode: 1}
	fail := func(title plays.Title, reason string) {
		h.PlaybackFailed(plays.Failure{User: h.member.ID, UserName: "member", App: "Test App", DeviceName: "Living room", Title: title,
			Reason: reason, At: h.now()})
	}
	fail(movie, plays.FailedNoStream)
	fail(movie, plays.FailedStream)
	fail(episode, plays.FailedLimit)
	got := h.targets.wait(t, "/server", 2)
	h.targets.wait(t, "/admin", 2)
	h.idle(t)
	if len(h.targets.at("/server")) != 2 {
		t.Fatalf("%d messages, want 2", len(h.targets.at("/server")))
	}
	first, second := got[0].body, got[1].body
	failure, _ := first["failure"].(map[string]any)
	if first["type"] != PlaybackFailed || first["title"] != "member could not play Night Train" ||
		first["message"] != "None of its versions plays on this app now. · Test App on Living room" ||
		failure["reason"] != plays.FailedNoStream || failure["app"] != "Test App" || failure["device"] != "Living room" ||
		failure["itemId"] != movie.Item.String() || failure["kind"] != plays.Movie {
		t.Errorf("first: %v", first)
	}
	failure, _ = second["failure"].(map[string]any)
	if second["title"] != "member could not play Harbour S01E01" || failure["reason"] != plays.FailedLimit || failure["seriesName"] != "Harbour" {
		t.Errorf("second: %v", second)
	}

	// An hour later, the movie's failure is told again.
	mu.Lock()
	now = now.Add(time.Hour)
	mu.Unlock()
	fail(movie, plays.FailedStream)
	again := h.targets.wait(t, "/server", 3)[2].body
	if failure, _ := again["failure"].(map[string]any); failure["reason"] != plays.FailedStream {
		t.Errorf("an hour later: %v", again)
	}
}

// The preview of a summary is the email a target receives: its subject,
// its plain text and its HTML.
func TestSummaryPreviewIsTheSentEmail(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	h.now = func() time.Time { return now }
	sink := newSMTPSink(t, false, "PLAIN LOGIN")
	h.useSMTP(t, sink, sink.password)
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.add(t, nil, Draft{Kind: Email, Name: new("Inbox"), Address: new("sam@example.org"), Events: []string{WeeklySummary}})
	h.play(t, h.member, accounts.ID{0x71}, "Late <Show>", nil, "", now.Add(-2*time.Hour), 2*time.Hour)

	preview, err := h.Preview(t.Context(), WeeklySummary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SendSummary(t.Context(), WeeklySummary); err != nil {
		t.Fatal(err)
	}
	subject, parts := mailParts(t, sink.wait(1)[0].data)
	if preview.Empty || preview.Subject != subject || preview.HTML != parts["text/html"] || preview.Text != parts["text/plain"] {
		t.Errorf("preview %+v\nsent %q %q", preview, subject, parts)
	}
	if !strings.Contains(preview.HTML, "Late &lt;Show&gt;") {
		t.Errorf("preview HTML: %s", preview.HTML)
	}

	// A user's own week, and their year.
	mine, err := h.Preview(t.Context(), WeeklySummary, &h.member.ID)
	if err != nil || mine.Subject == preview.Subject || !strings.Contains(mine.Text, "Late <Show>") {
		t.Errorf("the member's week: %+v, %v", mine, err)
	}
	if year, err := h.Preview(t.Context(), YearlySummary, &h.member.ID); err != nil || year.End.After(now) || year.HTML == "" {
		t.Errorf("the member's year: %+v, %v", year, err)
	}
	if _, err := h.Preview(t.Context(), NewEpisode, nil); !errors.Is(err, ErrNoPreview) {
		t.Errorf("a preview of new episodes: %v", err)
	}
	if _, err := h.Preview(t.Context(), WeeklySummary, &accounts.ID{0x99}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a preview of a missing user: %v", err)
	}
}
