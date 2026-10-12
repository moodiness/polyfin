package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localfiles"
)

// reviewOf is the summary a request carries, of the event wanted.
func reviewOf(t *testing.T, r received, event string) map[string]any {
	t.Helper()
	sum, ok := r.body["summary"].(map[string]any)
	if !ok || r.body["type"] != event {
		t.Fatalf("not a %s: %v", event, r.body)
	}
	return sum
}

// inParis parses a time of day in Paris, skipping the test without a time
// zone database.
func inParis(t *testing.T) func(string) time.Time {
	t.Helper()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone database:", err)
	}
	return func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, paris)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
}

// A month's review is due on the 1st of the month after it at the hour,
// and a year's on the 1st of January after it, in the server's time zone;
// either is sent within the month it was due in, never later.
func TestReviewDueOnTheFirstWithinTheMonth(t *testing.T) {
	at := inParis(t)
	paris := at("2030-01-01 00:00").Location()
	for _, c := range []struct {
		event, now  string
		hour        int
		start, end  string
		withinMonth bool
	}{
		{MonthlySummary, "2030-01-01 08:59", 9, "2029-11-01 00:00", "2029-12-01 00:00", false}, // November's, due in December
		{MonthlySummary, "2030-01-01 09:00", 9, "2029-12-01 00:00", "2030-01-01 00:00", true},
		{MonthlySummary, "2030-01-31 23:59", 9, "2029-12-01 00:00", "2030-01-01 00:00", true},
		{MonthlySummary, "2030-02-01 08:00", 9, "2029-12-01 00:00", "2030-01-01 00:00", false},
		{MonthlySummary, "2030-03-01 00:00", 0, "2030-02-01 00:00", "2030-03-01 00:00", true},
		{YearlySummary, "2030-01-01 08:59", 9, "2028-01-01 00:00", "2029-01-01 00:00", false},
		{YearlySummary, "2030-01-01 09:00", 9, "2029-01-01 00:00", "2030-01-01 00:00", true},
		{YearlySummary, "2030-01-31 23:59", 9, "2029-01-01 00:00", "2030-01-01 00:00", true},
		{YearlySummary, "2030-02-01 00:00", 9, "2029-01-01 00:00", "2030-01-01 00:00", false},
		{YearlySummary, "2030-12-31 23:00", 9, "2029-01-01 00:00", "2030-01-01 00:00", false},
	} {
		start, end, ok := reviewDue(c.event, at(c.now), paris, c.hour)
		if !start.Equal(at(c.start)) || !end.Equal(at(c.end)) || ok != c.withinMonth {
			t.Errorf("%s at %s, hour %d: %s to %s, %v; want %s to %s, %v", c.event, c.now, c.hour, start.In(paris), end.In(paris), ok,
				c.start, c.end, c.withinMonth)
		}
	}
}

// The month in review is sent on the 1st of each month at the hour, and
// the year in review on the 1st of January, once: checking again, or after
// a restart, sends nothing more. A server off on the 1st sends them when it
// starts within that month, not later. The last month and year sent are
// kept in the database.
func TestReviewsOnceAcrossRestarts(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	at := inParis(t)
	h.Service.zone = at("2030-01-01 00:00").Location()
	h.withHistory()
	clock := at("2030-01-01 08:59")
	h.Service.now = func() time.Time { return clock }
	if _, err := h.db.Exec(ctx, "UPDATE notification_summary SET month_sent = '2029-11-01', year_sent = '2028-01-01'"); err != nil {
		t.Fatal(err)
	}
	h.add(t, nil, h.webhook("/server", MonthlySummary, YearlySummary))
	h.add(t, &h.member, h.webhook("/member", MonthlySummary))
	h.play(t, h.member, accounts.ID{0x81}, "Harbour Lights", nil, "", at("2029-12-10 20:00"), time.Hour)
	h.play(t, h.member, accounts.ID{0x82}, "Old Mill", nil, "", at("2030-01-15 20:00"), time.Hour)
	h.play(t, h.member, accounts.ID{0x83}, "Late Frost", nil, "", at("2030-02-15 20:00"), time.Hour)
	h.play(t, h.member, accounts.ID{0x84}, "Spring Tide", nil, "", at("2030-12-15 20:00"), time.Hour)

	// Tuesday 1 January 2030, 8:59 in Paris: not yet.
	h.CheckSummary(ctx)
	h.idle(t)
	if got := len(h.targets.got); got != 0 {
		t.Fatalf("%d reviews before the hour", got)
	}
	clock = at("2030-01-01 09:00")
	h.CheckSummary(ctx)
	server := h.targets.wait(t, "/server", 2)
	byType := map[string]map[string]any{}
	for _, r := range server {
		byType[r.body["type"].(string)] = r.body["summary"].(map[string]any)
	}
	if month := byType[MonthlySummary]; month["start"] != "2029-11-30T23:00:00Z" || month["end"] != "2029-12-31T23:00:00Z" ||
		strings.Join(names(month["movies"]), ", ") != "Harbour Lights" {
		t.Errorf("December: %v", month)
	}
	if year := byType[YearlySummary]; year["start"] != "2028-12-31T23:00:00Z" || year["end"] != "2029-12-31T23:00:00Z" ||
		strings.Join(names(year["movies"]), ", ") != "Harbour Lights" {
		t.Errorf("2029: %v", year)
	}
	reviewOf(t, h.targets.wait(t, "/member", 1)[0], MonthlySummary)
	h.idle(t)
	clock = at("2030-01-01 09:30")
	h.CheckSummary(ctx)
	restarted := h.restarted(t, func() time.Time { return at("2030-01-01 10:00") })
	restarted.CheckSummary(ctx)
	harness{Service: restarted}.idle(t)
	h.idle(t)
	if got := len(h.targets.at("/server")) + len(h.targets.at("/member")); got != 3 {
		t.Errorf("%d reviews after checking again and restarting, want 3", got)
	}

	// Off on 1 February, started on the 20th: January's review is sent,
	// and the year's not again.
	late := h.restarted(t, func() time.Time { return at("2030-02-20 12:00") })
	late.CheckSummary(ctx)
	if sum := reviewOf(t, h.targets.wait(t, "/server", 3)[2], MonthlySummary); strings.Join(names(sum["movies"]), ", ") != "Old Mill" {
		t.Errorf("January caught up: %v", sum)
	}
	h.targets.wait(t, "/member", 2)
	harness{Service: late}.idle(t)

	// Off from 1 March to 1 April at 8:00: February's review is not sent
	// in April, and stays unsent.
	sent := func() (month, year string) {
		t.Helper()
		var m, y time.Time
		if err := h.db.QueryRow(ctx, "SELECT month_sent, year_sent FROM notification_summary").Scan(&m, &y); err != nil {
			t.Fatal(err)
		}
		return m.Format(time.DateOnly), y.Format(time.DateOnly)
	}
	april := h.restarted(t, func() time.Time { return at("2030-04-01 08:00") })
	april.CheckSummary(ctx)
	harness{Service: april}.idle(t)
	if month, year := sent(); month != "2030-01-01" || year != "2029-01-01" {
		t.Errorf("kept in April: month %s, year %s", month, year)
	}
	// Off on 1 January and on 1 February 2031: 2030's review is not sent
	// in February.
	february := h.restarted(t, func() time.Time { return at("2031-02-01 10:00") })
	february.CheckSummary(ctx)
	harness{Service: february}.idle(t)
	if _, year := sent(); year != "2029-01-01" {
		t.Errorf("kept in February 2031: year %s", year)
	}
	if got := len(h.targets.at("/server")) + len(h.targets.at("/member")); got != 5 {
		t.Errorf("%d reviews, want 5: none sent late", got)
	}
}

// The server's targets and administrators' own receive the server's month
// and year, a member's targets their own, which holds only their
// playbacks. The year tells the ten titles played longest and the month
// with the most hours, the users who joined and the files added, but
// neither the new episodes nor the problems found. Send the month in
// review and Send the year in review send the last month and year that
// ended.
func TestReviewsOfTheServerAndOfAMember(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	at := inParis(t)
	h.Service.zone = at("2030-01-01 00:00").Location()
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.Service.files = fakeFiles{added: []localfiles.Added{{Name: "Tides", Kind: localfiles.KindShows, Files: 3}}}
	h.Service.now = func() time.Time { return at("2030-03-15 12:00") }
	h.add(t, nil, h.webhook("/server", MonthlySummary, YearlySummary))
	h.add(t, &h.admin, h.webhook("/admin", YearlySummary))
	h.add(t, &h.member, h.webhook("/member", MonthlySummary, YearlySummary))

	tides := accounts.ID{0x90}
	// February 2030, the month sent, and January, before it.
	h.play(t, h.member, accounts.ID{0x91}, "Harbour Lights", nil, "", at("2030-02-03 20:00"), 2*time.Hour)
	h.play(t, h.member, accounts.ID{0x92}, "Pilot", &tides, "Tides", at("2030-02-28 23:30"), time.Hour)
	h.play(t, h.admin, accounts.ID{0x93}, "Old Mill", nil, "", at("2030-02-10 20:00"), 30*time.Minute)
	h.play(t, h.member, accounts.ID{0x94}, "Before", nil, "", at("2030-01-31 23:30"), time.Hour)
	// 2029, the year sent.
	h.play(t, h.member, accounts.ID{0x95}, "Late Frost", nil, "", at("2029-03-05 20:00"), 3*time.Hour)
	h.play(t, h.member, accounts.ID{0x96}, "Spring Tide", nil, "", at("2029-07-05 20:00"), time.Hour)
	h.play(t, h.admin, accounts.ID{0x97}, "Long Night", nil, "", at("2029-08-31 23:00"), 4*time.Hour)
	// A guest who joined in 2029.
	guest, err := h.store.CreateUser(ctx, accounts.NewUser{Name: "guest", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(ctx, "INSERT INTO notification_joins (user_id, invite_id, joined_at) VALUES ($1, gen_random_uuid(), $2)",
		guest.ID, at("2029-06-01 10:00")); err != nil {
		t.Fatal(err)
	}

	if err := h.SendSummary(ctx, MonthlySummary); err != nil {
		t.Fatal(err)
	}
	server := h.targets.wait(t, "/server", 1)[0]
	member := h.targets.wait(t, "/member", 1)[0]
	h.idle(t)
	if got := len(h.targets.at("/admin")); got != 0 {
		t.Errorf("a target that did not choose the month in review received %d", got)
	}
	sum := reviewOf(t, server, MonthlySummary)
	if server.body["title"] != "The month on Home" || sum["scope"] != ScopeServer || sum["start"] != "2030-01-31T23:00:00Z" ||
		sum["end"] != "2030-02-28T23:00:00Z" || sum["plays"] != 3.0 || strings.Join(names(sum["movies"]), ", ") != "Harbour Lights, Old Mill" ||
		strings.Join(names(sum["series"]), ", ") != "Tides" || sum["topMonth"] != nil {
		t.Errorf("the server's February: %v", server.body)
	}
	if !strings.HasPrefix(server.body["message"].(string), "Month of February 2030.\n3.5 hours watched, 3 playbacks.\n") {
		t.Errorf("the server's message: %q", server.body["message"])
	}
	sum = reviewOf(t, member, MonthlySummary)
	if member.body["title"] != "Your month on Home" || sum["scope"] != ScopeUser || sum["plays"] != 2.0 ||
		strings.Join(names(sum["movies"]), ", ") != "Harbour Lights" || strings.Join(names(sum["series"]), ", ") != "Tides" {
		t.Errorf("the member's February: %v", member.body)
	}

	if err := h.SendSummary(ctx, YearlySummary); err != nil {
		t.Fatal(err)
	}
	server = h.targets.wait(t, "/server", 2)[1]
	admin := h.targets.wait(t, "/admin", 1)[0]
	member = h.targets.wait(t, "/member", 2)[1]
	for path, r := range map[string]received{"/server": server, "/admin": admin} {
		sum := reviewOf(t, r, YearlySummary)
		top, _ := sum["topMonth"].(map[string]any)
		if r.body["title"] != "The year on Home" || sum["scope"] != ScopeServer || sum["start"] != "2028-12-31T23:00:00Z" ||
			sum["end"] != "2029-12-31T23:00:00Z" || sum["plays"] != 3.0 ||
			strings.Join(names(sum["movies"]), ", ") != "Long Night, Late Frost, Spring Tide" {
			t.Errorf("%s: the server's 2029: %v", path, r.body)
		}
		if top["month"] != "2029-08" || top["played"] != float64(4*3600) || top["plays"] != 1.0 {
			t.Errorf("%s: the server's top month: %v", path, sum["topMonth"])
		}
		if strings.Join(names(sum["users"]), ", ") != "guest" || sum["addedFiles"] != 3.0 || sum["newEpisodes"] != 0.0 {
			t.Errorf("%s: the server's year: %v", path, sum)
		}
	}
	if want := "Year 2029.\n8 hours watched, 3 playbacks.\nMost watched month: August, 4 hours.\n" +
		"Movies: Long Night, Late Frost, Spring Tide.\nNew user: guest.\n3 files added to the local folders."; server.body["message"] != want {
		t.Errorf("the server's year message: %q, want %q", server.body["message"], want)
	}
	sum = reviewOf(t, member, YearlySummary)
	top, _ := sum["topMonth"].(map[string]any)
	if member.body["title"] != "Your year on Home" || sum["plays"] != 2.0 || strings.Join(names(sum["movies"]), ", ") != "Late Frost, Spring Tide" ||
		top["month"] != "2029-03" || len(sum["users"].([]any)) != 0 || sum["addedFiles"] != 0.0 {
		t.Errorf("the member's 2029: %v", member.body)
	}
}

// Nothing is sent for a month or a year with nothing to tell, to the
// server nor to a member.
func TestReviewsWithNothingToTell(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	at := inParis(t)
	h.Service.zone = at("2030-01-01 00:00").Location()
	h.withHistory()
	h.Service.now = func() time.Time { return at("2030-03-15 12:00") }
	h.add(t, nil, h.webhook("/server", MonthlySummary, YearlySummary))
	h.add(t, &h.member, h.webhook("/member", MonthlySummary, YearlySummary))
	// Played this month, neither in February nor in 2029.
	h.play(t, h.member, accounts.ID{0xa1}, "Harbour Lights", nil, "", at("2030-03-01 00:30"), time.Hour)
	for _, event := range []string{MonthlySummary, YearlySummary} {
		if err := h.SendSummary(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	h.idle(t)
	if got := len(h.targets.got); got != 0 {
		t.Errorf("%d reviews of a month and a year with nothing to tell", got)
	}
}

// An email holds the whole year, in the server language: its figures, the
// month with the most hours, and the titles linking to their pages.
func TestYearInReviewEmail(t *testing.T) {
	h := newHarness(t)
	sink := newSMTPSink(t, false, "PLAIN LOGIN")
	h.useSMTP(t, sink, sink.password)
	at := inParis(t)
	h.Service.zone = at("2030-01-01 00:00").Location()
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName, settings.Language = "https://media.example.org", "Home", "fr"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.withHistory()
	h.Service.now = func() time.Time { return at("2030-01-01 09:00") }
	h.add(t, nil, Draft{Kind: Email, Name: new("Inbox"), Address: new("sam@example.org"), Events: []string{YearlySummary}})
	movie := accounts.ID{0xb1}
	h.play(t, h.member, movie, "Late Frost", nil, "", at("2029-08-05 20:00"), 3*time.Hour)
	h.play(t, h.member, movie, "Late Frost", nil, "", at("2029-04-05 20:00"), time.Hour)
	if err := h.SendSummary(t.Context(), YearlySummary); err != nil {
		t.Fatal(err)
	}
	subject, parts := mailParts(t, sink.wait(1)[0].data)
	link := "https://media.example.org/web/#/details?id=" + movie.String() + "&serverId=fedcba9876543210fedcba9876543210"
	if subject != "L’année sur Home" {
		t.Errorf("subject: %q", subject)
	}
	text := parts["text/plain"]
	for _, want := range []string{"Année 2029.\n4 heures de visionnage, 2 lectures.", "Films les plus regardés\n- Late Frost · 4 heures, 2 lectures\n  " + link,
		"Mois le plus regardé\n- août 2029, 3 heures · 1 lecture"} {
		if !strings.Contains(text, want) {
			t.Errorf("plain text lacks %q: %q", want, text)
		}
	}
	if page := parts["text/html"]; !strings.Contains(page, "<p>Année 2029.<br><strong>4 heures de visionnage, 2 lectures.</strong></p>") {
		t.Errorf("HTML: %q", page)
	}
}
