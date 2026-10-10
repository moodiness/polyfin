package notifications

import (
	"context"
	"html"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/plays"
)

// fakeFiles are the titles added to the local folders, whatever the week.
type fakeFiles struct {
	added []localfiles.Added
}

func (f fakeFiles) Added(_ context.Context, _, _ time.Time, limit int) ([]localfiles.Added, int, error) {
	total := 0
	for _, a := range f.added {
		total += a.Files
	}
	return f.added[:min(len(f.added), limit)], total, nil
}

// withHistory has the summaries read the playback history of the
// harness's database.
func (h harness) withHistory() {
	h.Service.history = plays.NewHistory(h.db, h.store.Settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// play keeps a video user played in the history: a movie, or an episode of
// series, started at started, played for played.
func (h harness) play(t *testing.T, user accounts.User, item accounts.ID, name string, series *accounts.ID, seriesName string,
	started time.Time, played time.Duration) {
	t.Helper()
	kind, season, episode := plays.Movie, (*int)(nil), (*int)(nil)
	if series != nil {
		kind, season, episode = plays.Episode, new(1), new(2)
	}
	if _, err := h.db.Exec(t.Context(), `INSERT INTO playback_history (user_id, item_id, kind, name, series_id, series_name, season, episode,
			started_at, ended_at, played_seconds) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		user.ID, item, kind, name, series, seriesName, season, episode, started, started.Add(played), int(played.Seconds())); err != nil {
		t.Fatal(err)
	}
}

// summary is the summary a request carries.
func summary(t *testing.T, r received) map[string]any {
	t.Helper()
	sum, ok := r.body["summary"].(map[string]any)
	if !ok || r.body["type"] != WeeklySummary {
		t.Fatalf("not a weekly summary: %v", r.body)
	}
	return sum
}

// names lists the names of a summary's list.
func names(list any) []string {
	var found []string
	items, _ := list.([]any)
	for _, item := range items {
		found = append(found, item.(map[string]any)["name"].(string))
	}
	return found
}

// The server's targets and administrators' own receive the server's week:
// its playbacks and their top movies and series, the new episodes found,
// the users who joined through an invite, the files added to the local
// folders and the problems System › Health found. A member's targets
// receive their own week: their playbacks and titles, and the new episodes
// of the series they follow. Targets that did not choose the summary
// receive none, nor a member with nothing to tell.
func TestWeeklySummaryOfTheServerAndOfEachUser(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.Service.files = fakeFiles{added: []localfiles.Added{{Name: "Tides", Kind: localfiles.KindShows, Files: 3},
		{Name: "Old Mill", Kind: localfiles.KindMovies, Files: 1}}}
	quiet, err := h.store.CreateUser(ctx, accounts.NewUser{Name: "quiet", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	h.add(t, nil, h.webhook("/server", WeeklySummary))
	h.add(t, &h.admin, h.webhook("/admin", WeeklySummary))
	h.add(t, &h.member, h.webhook("/member", WeeklySummary))
	h.add(t, &h.member, h.webhook("/member-episodes", NewEpisode))
	h.add(t, &quiet, h.webhook("/quiet", WeeklySummary))

	// The playbacks: the member's, the admin's, and one from before the
	// week.
	now := time.Now()
	harbour, tides, mill := accounts.ID{0x31}, accounts.ID{0x32}, accounts.ID{0x33}
	h.play(t, h.member, harbour, "Harbour Lights", nil, "", now.Add(-3*24*time.Hour), 90*time.Minute)
	h.play(t, h.member, harbour, "Harbour Lights", nil, "", now.Add(-2*24*time.Hour), 30*time.Minute)
	h.play(t, h.member, accounts.ID{0x34}, "Pilot", &tides, "Tides", now.Add(-24*time.Hour), 45*time.Minute)
	h.play(t, h.admin, mill, "Old Mill", nil, "", now.Add(-time.Hour), 20*time.Minute)
	h.play(t, h.admin, accounts.ID{0x35}, "Last Year", nil, "", now.Add(-8*24*time.Hour), 3*time.Hour)

	// A new episode of a series the member follows, found by the
	// new-episode check: the series is first seen with its first episode,
	// then has a second.
	h.userData.played(h.member.ID, tides)
	episode := func(number int, released time.Duration) library.Item {
		date := now.Add(released)
		return library.Item{ID: accounts.ID{0x36, byte(number)}, Kind: library.KindEpisode, Name: "Low Water", SeriesID: tides,
			SeriesName: "Tides", ParentIndexNumber: 1, IndexNumber: number, PremiereDate: &date, Available: true}
	}
	h.library.set(tides, episode(1, -48*time.Hour))
	h.CheckEpisodes(ctx)
	h.library.set(tides, episode(1, -48*time.Hour), episode(2, -time.Minute))
	h.CheckEpisodes(ctx)

	// A guest who joined through the admin's invite.
	_, token, err := h.store.CreateInvite(ctx, accounts.NewInvite{MaxUses: 1, CreatedBy: h.admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	guest, invite, err := h.store.AcceptInvite(ctx, token, "guest", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	h.UserJoined(ctx, guest, invite)

	// A problem System › Health found, in two checks.
	*h.problems = []Problem{{Key: "backup", Severity: SeverityError, Text: "The last backup failed.", Page: "/system/health#backups"}}
	h.CheckHealth(ctx)
	h.CheckHealth(ctx)

	if err := h.SendSummary(ctx); err != nil {
		t.Fatal(err)
	}
	server := h.targets.wait(t, "/server", 1)[0]
	admin := h.targets.wait(t, "/admin", 1)[0]
	member := h.targets.wait(t, "/member", 1)[0]
	h.idle(t)
	if got := len(h.targets.at("/quiet")); got != 0 {
		t.Errorf("a user with nothing to tell received %d summaries", got)
	}
	if got := types(h.targets.at("/member-episodes")); len(got) != 1 || got[0] != "new_episode Low Water" {
		t.Errorf("a target that did not choose the summary received %v", got)
	}

	for path, r := range map[string]received{"/server": server, "/admin": admin} {
		sum := summary(t, r)
		users, _ := sum["users"].([]any)
		var invitedBy map[string]any
		if len(users) == 1 {
			invitedBy, _ = users[0].(map[string]any)["invitedBy"].(map[string]any)
		}
		problems, _ := sum["problems"].([]any)
		episodes, _ := sum["episodes"].([]any)
		if sum["scope"] != ScopeServer || r.body["user"] != nil || r.body["title"] != "The week on Home" ||
			r.body["url"] != "https://media.example.org/admin/system/statistics" {
			t.Errorf("%s: %v", path, r.body)
		}
		if sum["plays"] != 4.0 || sum["played"] != float64((90+30+45+20)*60) || strings.Join(names(sum["movies"]), ", ") != "Harbour Lights, Old Mill" ||
			strings.Join(names(sum["series"]), ", ") != "Tides" {
			t.Errorf("%s: the playbacks of the week: %v", path, sum)
		}
		if sum["newEpisodes"] != 1.0 || len(episodes) != 1 || episodes[0].(map[string]any)["number"] != 2.0 {
			t.Errorf("%s: new episodes %v %v", path, sum["newEpisodes"], episodes)
		}
		if strings.Join(names(sum["users"]), ", ") != "guest" || invitedBy["name"] != "admin" {
			t.Errorf("%s: users who joined %v", path, users)
		}
		if sum["addedFiles"] != 4.0 || strings.Join(names(sum["added"]), ", ") != "Tides, Old Mill" {
			t.Errorf("%s: added %v %v", path, sum["addedFiles"], sum["added"])
		}
		if len(problems) != 1 || problems[0].(map[string]any)["key"] != "backup" || problems[0].(map[string]any)["severity"] != SeverityError {
			t.Errorf("%s: problems %v", path, problems)
		}
	}
	if want := "Week of "; !strings.HasPrefix(server.body["message"].(string), want) {
		t.Errorf("message: %q", server.body["message"])
	}
	for _, line := range []string{"3.1 hours watched, 4 playbacks.", "Movies: Harbour Lights, Old Mill.", "Series: Tides.",
		"1 new episode of the series users follow.", "New user: guest.", "4 files added to the local folders.",
		"1 problem found by System › Health."} {
		if !strings.Contains(server.body["message"].(string), line+"\n") && !strings.HasSuffix(server.body["message"].(string), line) {
			t.Errorf("the server's message lacks %q: %q", line, server.body["message"])
		}
	}

	sum := summary(t, member)
	who, _ := member.body["user"].(map[string]any)
	if sum["scope"] != ScopeUser || who["name"] != "member" || member.body["title"] != "Your week on Home" ||
		member.body["url"] != "https://media.example.org/admin/me/account#statistics" {
		t.Errorf("the member's summary: %v", member.body)
	}
	if sum["plays"] != 3.0 || strings.Join(names(sum["movies"]), ", ") != "Harbour Lights" || strings.Join(names(sum["series"]), ", ") != "Tides" ||
		sum["newEpisodes"] != 1.0 {
		t.Errorf("the member's week: %v", sum)
	}
	if len(sum["users"].([]any))+len(sum["problems"].([]any))+len(sum["added"].([]any)) != 0 || sum["addedFiles"] != 0.0 {
		t.Errorf("the member's week tells the server's: %v", sum)
	}
	if want := "2.8 hours watched, 3 playbacks.\nMovies: Harbour Lights.\nSeries: Tides.\n1 new episode of the series you follow."; !strings.HasSuffix(
		member.body["message"].(string), want) {
		t.Errorf("the member's message: %q, want it to end with %q", member.body["message"], want)
	}
}

// Nothing is sent to a recipient with nothing to tell: no playback, no new
// episode, and for the server no user who joined, no file added and no
// problem.
func TestWeeklySummaryWithNothingToTell(t *testing.T) {
	h := newHarness(t)
	h.withHistory()
	h.add(t, nil, h.webhook("/server", WeeklySummary))
	h.add(t, &h.admin, h.webhook("/admin", WeeklySummary))
	h.add(t, &h.member, h.webhook("/member", WeeklySummary))
	if err := h.SendSummary(t.Context()); err != nil {
		t.Fatal(err)
	}
	h.idle(t)
	if got := len(h.targets.got); got != 0 {
		t.Fatalf("%d summaries of an empty week", got)
	}

	// A problem alone is something to tell the server, not the member.
	*h.problems = []Problem{{Key: "database", Severity: SeverityError, Text: "The database does not answer."}}
	h.CheckHealth(t.Context())
	h.CheckHealth(t.Context())
	if err := h.SendSummary(t.Context()); err != nil {
		t.Fatal(err)
	}
	h.targets.wait(t, "/server", 1)
	h.targets.wait(t, "/admin", 1)
	h.idle(t)
	if got := len(h.targets.at("/member")); got != 0 {
		t.Errorf("the member received %d summaries of a week with nothing of theirs", got)
	}
	if message := h.targets.at("/server")[0].body["message"].(string); !strings.Contains(message, "Nothing was watched.") {
		t.Errorf("message: %q", message)
	}
}

// The week ends at the last day and hour of the settings in the server's
// time zone, and starts seven days before at the same hour there, across a
// change of daylight saving time.
func TestWeeklySummaryWeekInTheServerTimeZone(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone database:", err)
	}
	at := func(zone *time.Location, value string) time.Time {
		t.Helper()
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, zone)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	for _, c := range []struct {
		now, want string
		day, hour int
	}{
		{"2026-10-12 08:59", "2026-10-05 09:00", 1, 9},  // Monday, before the hour
		{"2026-10-12 09:00", "2026-10-12 09:00", 1, 9},  // Monday, at the hour
		{"2026-10-14 15:00", "2026-10-12 09:00", 1, 9},  // Wednesday after
		{"2026-10-11 23:30", "2026-10-11 23:00", 0, 23}, // Sunday evening
		{"2026-10-11 22:59", "2026-10-04 23:00", 0, 23},
		{"2026-10-17 00:00", "2026-10-17 00:00", 6, 0}, // Saturday at midnight
	} {
		if got := summaryDue(at(newYork, c.now), newYork, c.day, c.hour); !got.Equal(at(newYork, c.want)) {
			t.Errorf("at %s, day %d at %d: %s, want %s", c.now, c.day, c.hour, got.In(newYork), c.want)
		}
	}
	// The same instant is Monday 13:30 in UTC, past the hour, and Monday
	// 9:30 in New York.
	instant := at(newYork, "2026-10-12 09:30")
	if got := summaryDue(instant, time.UTC, 1, 12); !got.Equal(at(time.UTC, "2026-10-12 12:00")) {
		t.Errorf("in UTC: %s", got)
	}
	if got := summaryDue(instant, newYork, 1, 12); !got.Equal(at(newYork, "2026-10-05 12:00")) {
		t.Errorf("in New York: %s", got)
	}
	// Daylight saving time ends on 1 November 2026: the week has an hour
	// more, and starts at 9:00 still.
	end := at(newYork, "2026-11-02 09:00")
	if start := weekBefore(end, newYork); !start.Equal(at(newYork, "2026-10-26 09:00")) || end.Sub(start) != 7*24*time.Hour+time.Hour {
		t.Errorf("the week across the change starts at %s", start.In(newYork))
	}
}

// restarted is another service on the harness's database, as after a
// restart, at the clock now.
func (h harness) restarted(t *testing.T, now func() time.Time) *Service {
	t.Helper()
	s := New(Options{DB: h.db, Accounts: h.store, Library: h.library, UserData: h.userData, Secrets: h.box, Version: "1.2.3",
		ServerID: h.serverID, Logger: h.logger, Zone: h.zone})
	s.history, s.now = h.history, now
	s.checkPublic, s.confined = h.checkPublic, h.trusted
	t.Cleanup(s.Close)
	return s
}

// The summary is sent once the day and hour come, in the server's time
// zone, once a week: checking again, or after a restart, sends nothing
// more. A server off at the hour sends it when it starts again that week,
// for the week before the hour. The end of the week sent is kept in the
// database.
func TestWeeklySummaryOncePerWeekAcrossRestarts(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone database:", err)
	}
	h.Service.zone = paris
	h.withHistory()
	at := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, paris)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	clock := at("2030-01-14 08:59")
	h.Service.now = func() time.Time { return clock }
	// The last week sent ended on Monday 7 January 2030 at 9:00.
	if _, err := h.db.Exec(ctx, "UPDATE notification_summary SET week_end = $1", at("2030-01-07 09:00")); err != nil {
		t.Fatal(err)
	}
	h.add(t, nil, h.webhook("/server", WeeklySummary))
	h.add(t, &h.member, h.webhook("/member", WeeklySummary))
	h.play(t, h.member, accounts.ID{0x41}, "Harbour Lights", nil, "", at("2030-01-10 20:00"), time.Hour)
	h.play(t, h.member, accounts.ID{0x42}, "Old Mill", nil, "", at("2030-01-16 20:00"), time.Hour)

	// Monday 8:59 in Paris, 7:59 in UTC: not yet.
	h.CheckSummary(ctx)
	h.idle(t)
	if got := len(h.targets.got); got != 0 {
		t.Fatalf("%d summaries before the hour", got)
	}
	clock = at("2030-01-14 09:00")
	h.CheckSummary(ctx)
	sum := summary(t, h.targets.wait(t, "/server", 1)[0])
	if sum["start"] != "2030-01-07T08:00:00Z" || sum["end"] != "2030-01-14T08:00:00Z" || strings.Join(names(sum["movies"]), ", ") != "Harbour Lights" {
		t.Errorf("the week: %v", sum)
	}
	h.targets.wait(t, "/member", 1)
	// A sink has a message before Polyfin records its delivery, which reads
	// the clock: wait for the queues before moving it.
	h.idle(t)
	clock = at("2030-01-14 09:30")
	h.CheckSummary(ctx)
	restarted := h.restarted(t, func() time.Time { return at("2030-01-14 10:00") })
	restarted.CheckSummary(ctx)
	h.idle(t)
	harness{Service: restarted}.idle(t)
	if got := len(h.targets.at("/server")) + len(h.targets.at("/member")); got != 2 {
		t.Errorf("%d summaries after checking again and restarting, want 2", got)
	}

	// Off from Sunday to Wednesday 23 January: the week that ended on
	// Monday 21 is sent at the next start.
	late := h.restarted(t, func() time.Time { return at("2030-01-23 12:00") })
	late.CheckSummary(ctx)
	got := h.targets.wait(t, "/server", 2)
	if sum := summary(t, got[1]); sum["start"] != "2030-01-14T08:00:00Z" || sum["end"] != "2030-01-21T08:00:00Z" ||
		strings.Join(names(sum["movies"]), ", ") != "Old Mill" {
		t.Errorf("the week caught up: %v", sum)
	}
	late.CheckSummary(ctx)
	harness{Service: late}.idle(t)
	late.Close()
	var end time.Time
	if err := h.db.QueryRow(ctx, "SELECT week_end FROM notification_summary").Scan(&end); err != nil || !end.Equal(at("2030-01-21 09:00")) {
		t.Errorf("the week kept: %s, %v", end, err)
	}
	if got := len(h.targets.at("/server")); got != 2 {
		t.Errorf("%d summaries to the server, want 2", got)
	}
}

// System › Schedule's task sends the summary of the last seven days now,
// each time it runs, without changing the week the schedule sends next.
func TestWeeklySummarySentNow(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	h.withHistory()
	h.add(t, nil, h.webhook("/server", WeeklySummary))
	now := time.Now()
	h.play(t, h.member, accounts.ID{0x51}, "Recent", nil, "", now.Add(-6*24*time.Hour), time.Hour)
	h.play(t, h.member, accounts.ID{0x52}, "Too Old", nil, "", now.Add(-8*24*time.Hour), time.Hour)
	var before time.Time
	if err := h.db.QueryRow(ctx, "SELECT week_end FROM notification_summary").Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := h.SendSummary(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got := h.targets.wait(t, "/server", 2)
	for _, r := range got {
		if sum := summary(t, r); strings.Join(names(sum["movies"]), ", ") != "Recent" || sum["plays"] != 1.0 {
			t.Errorf("the last seven days: %v", sum)
		}
	}
	var after time.Time
	if err := h.db.QueryRow(ctx, "SELECT week_end FROM notification_summary").Scan(&after); err != nil || !after.Equal(before) {
		t.Errorf("the week the schedule sends next changed from %s to %s (%v)", before, after, err)
	}
}

// Discord, ntfy, Telegram, Gotify and Pushover receive the summary as a
// short text, with the key figures and the link to the statistics; a
// webhook the JSON summary.
func TestWeeklySummaryFormats(t *testing.T) {
	h := newHarness(t)
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.add(t, nil, Draft{Kind: Discord, Name: new("Channel"), Address: new(h.targets.url + "/api/webhooks/1/discord-token"), Events: []string{WeeklySummary}})
	h.add(t, nil, Draft{Kind: Ntfy, Name: new("Phone"), Address: new(h.targets.url + "/ntfy/"), Topic: new("polyfin-alerts"),
		Events: []string{WeeklySummary}})
	h.chatServices(t, WeeklySummary)
	h.play(t, h.member, accounts.ID{0x61}, "Harbour Lights", nil, "", time.Now().Add(-time.Hour), 2*time.Hour)
	if err := h.SendSummary(t.Context()); err != nil {
		t.Fatal(err)
	}
	link := "https://media.example.org/admin/system/statistics"
	message := h.targets.wait(t, "/api/webhooks/1/discord-token", 1)[0].body
	embeds, _ := message["embeds"].([]any)
	embed, _ := embeds[0].(map[string]any)
	if embed["title"] != "The week on Home" || !strings.Contains(embed["description"].(string), "2 hours watched, 1 playback.\nMovies: Harbour Lights.") ||
		embed["url"] != link || embed["color"] != float64(0x6366f1) {
		t.Errorf("Discord: %v", embed)
	}
	ntfy := h.targets.wait(t, "/ntfy/", 1)[0].body
	if tags, _ := ntfy["tags"].([]any); ntfy["title"] != "The week on Home" || ntfy["click"] != link || len(tags) != 1 || tags[0] != "bar_chart" ||
		ntfy["priority"] != nil {
		t.Errorf("ntfy: %v", ntfy)
	}
	telegram := h.targets.wait(t, telegramPath, 1)[0].body["text"].(string)
	if !strings.HasPrefix(telegram, "<b>The week on Home</b>\nWeek of ") || !strings.HasSuffix(telegram, "<a href=\""+html.EscapeString(link)+"\">Open</a>") {
		t.Errorf("Telegram: %q", telegram)
	}
	gotify := h.targets.wait(t, "/gotify/message", 1)[0].body
	if gotify["title"] != "The week on Home" || gotify["priority"] != 5.0 || !strings.Contains(gotify["message"].(string), "Movies: Harbour Lights.") {
		t.Errorf("Gotify: %v", gotify)
	}
	pushover := h.targets.wait(t, pushoverPath, 1)[0].body
	if pushover["title"] != "The week on Home" || pushover["url"] != link || pushover["priority"] != 0.0 {
		t.Errorf("Pushover: %v", pushover)
	}
}

// mailParts reads a message the SMTP sink received: its subject, and its
// parts by type, decoded.
func mailParts(t *testing.T, data string) (string, map[string]string) {
	t.Helper()
	message, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	_, params, _ := mime.ParseMediaType(message.Header.Get("Content-Type"))
	parts := map[string]string{}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextRawPart()
		if err != nil {
			break
		}
		kind, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		var body io.Reader = part
		if part.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
			body = quotedprintable.NewReader(part)
		}
		decoded, _ := io.ReadAll(body)
		parts[kind] = string(decoded)
	}
	return subject, parts
}

// An email target receives the summary whole, in the server language, in
// plain text and in HTML laid out as the other emails: the week, the
// figures, and a list for each part, each title linking to its page.
func TestWeeklySummaryEmail(t *testing.T) {
	h := newHarness(t)
	sink := newSMTPSink(t, false, "PLAIN LOGIN")
	h.useSMTP(t, sink, sink.password)
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName, settings.Language = "https://media.example.org", "Home", "fr"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.add(t, nil, Draft{Kind: Email, Name: new("Inbox"), Address: new("sam@example.org"), Events: []string{WeeklySummary}})
	movie := accounts.ID{0x71}
	h.play(t, h.member, movie, "Late <Show>", nil, "", time.Now().Add(-2*time.Hour), 2*time.Hour)
	h.play(t, h.member, movie, "Late <Show>", nil, "", time.Now().Add(-26*time.Hour), time.Hour)
	*h.problems = []Problem{{Key: "backup", Severity: SeverityError, Text: "La dernière sauvegarde a échoué.", Page: "/system/health#backups"}}
	h.CheckHealth(t.Context())
	h.CheckHealth(t.Context())
	if err := h.SendSummary(t.Context()); err != nil {
		t.Fatal(err)
	}
	subject, parts := mailParts(t, sink.wait(1)[0].data)
	link := "https://media.example.org/web/#/details?id=" + movie.String() + "&serverId=fedcba9876543210fedcba9876543210"
	if subject != "La semaine sur Home" {
		t.Errorf("subject: %q", subject)
	}
	text := parts["text/plain"]
	for _, want := range []string{"Semaine du ", "3 heures de visionnage, 2 lectures.", "Films les plus regardés\n- Late <Show> · 3 heures, 2 lectures\n  " + link,
		"1 problème détecté par Système › Santé\n- La dernière sauvegarde a échoué. · Erreur\n  https://media.example.org/admin/system/health#backups",
		"Ouvrir les statistiques : https://media.example.org/admin/system/statistics", "-- \nEnvoyé par Polyfin depuis Home."} {
		if !strings.Contains(text, want) {
			t.Errorf("plain text lacks %q: %q", want, text)
		}
	}
	page := parts["text/html"]
	for _, want := range []string{`<html lang="fr">`, `<h1 style="font-size: 18px;">La semaine sur Home</h1>`, "Films les plus regardés</h2>",
		`<a href="` + strings.ReplaceAll(link, "&", "&amp;") + `">Late &lt;Show&gt;</a>`,
		`<a href="https://media.example.org/admin/system/statistics">Ouvrir les statistiques</a>`, "Envoyé par Polyfin depuis Home."} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML lacks %q: %q", want, page)
		}
	}
	if strings.Contains(page, "<Show>") {
		t.Errorf("HTML does not escape names: %q", page)
	}
}
