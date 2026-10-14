package notifications

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/reminders"
)

// guideOf is a guide that lists programmes, all on their channels.
type guideOf []library.Item

func (g guideOf) Guide(_ context.Context, _ accounts.User, q library.GuideQuery) ([]library.Item, error) {
	var found []library.Item
	for _, p := range g {
		if p.EndDate.After(q.From) && p.StartDate.Before(q.To) && (q.Channels == nil || slices.Contains(q.Channels, p.Channel.ID)) {
			found = append(found, p)
		}
	}
	return found, nil
}

func (g guideOf) Item(_ context.Context, _ accounts.User, id accounts.ID) (library.Item, error) {
	for _, p := range g {
		if p.ID == id {
			return p, nil
		}
	}
	return library.Item{}, library.ErrNotFound
}

func (g guideOf) Channels(context.Context, accounts.User) ([]library.Item, error) {
	var channels []library.Item
	for _, p := range g {
		channels = append(channels, *p.Channel)
	}
	return channels, nil
}

// A reminder sends one message to the user's own targets that chose the
// event, at the lead time, in the language it was set in, with the start
// in the server's time zone and a link to the channel; the server's
// targets, which cannot choose it, and administrators' never get it.
func TestProgrammeRemindersReachTheUsersOwnTargetsOnly(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	h.Service.zone = paris
	if _, err := h.Create(ctx, nil, h.webhook("/server", ProgrammeReminder)); !errors.Is(err, ErrInvalidEvents) {
		t.Errorf("a server target choosing programme reminders: %v", err)
	}
	if slices.Contains(EventsFor(nil), ProgrammeReminder) || !slices.Contains(EventsFor(&h.member), ProgrammeReminder) {
		t.Errorf("events the server's and members' targets choose from: %v, %v", EventsFor(nil), EventsFor(&h.member))
	}
	h.add(t, nil, h.webhook("/server", EventsFor(nil)...))
	h.add(t, &h.admin, h.webhook("/admin", EventsFor(&h.admin)...))
	h.add(t, &h.member, h.webhook("/member-episodes", NewEpisode))
	if h.Receives(h.member.ID, ProgrammeReminder) {
		t.Error("a member without a target choosing reminders is reached")
	}
	h.add(t, &h.member, h.webhook("/member", ProgrammeReminder))
	if !h.Receives(h.member.ID, ProgrammeReminder) {
		t.Error("a member with a target choosing reminders is not reached")
	}

	channel := library.Item{ID: accounts.ID{0xc4}, Kind: library.KindChannel, Name: "Example TV"}
	start := time.Date(2026, 10, 14, 18, 30, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	show := library.Item{ID: accounts.ID{0xd5}, Kind: library.KindProgram, Name: "Example Show", StartDate: &start, EndDate: &end, Channel: &channel}
	now := start.Add(-time.Hour)
	service := reminders.New(reminders.Config{DB: h.db, Guide: guideOf{show}, Users: h.store, Teller: h.Service, Logger: h.logger,
		Now: func() time.Time { return now }})
	if _, err := service.Create(ctx, h.member, show.ID, false, "en-US"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, h.admin, show.ID, false, "fr"); err != nil {
		t.Fatal(err)
	}
	now = start.Add(-5*time.Minute - time.Second)
	service.Check(ctx)
	h.idle(t)
	if got := len(h.targets.at("/member")) + len(h.targets.at("/admin")); got != 0 {
		t.Fatalf("%d messages before the lead time", got)
	}
	now = start.Add(-5 * time.Minute)
	service.Check(ctx)
	member := h.targets.wait(t, "/member", 1)[0].body
	admin := h.targets.wait(t, "/admin", 1)[0].body
	now = start.Add(-time.Minute)
	service.Check(ctx)
	h.idle(t)
	if got := len(h.targets.at("/member")); got != 1 {
		t.Errorf("the member was told %d times", got)
	}
	if got := len(h.targets.at("/server")) + len(h.targets.at("/member-episodes")); got != 0 {
		t.Errorf("%d messages to targets that are not told", got)
	}
	programme, _ := member["programme"].(map[string]any)
	who, _ := member["user"].(map[string]any)
	if member["type"] != ProgrammeReminder || member["title"] != "Starting at 20:30: Example Show" ||
		member["message"] != "Example Show starts in 5 minutes on Example TV." || who["name"] != "member" ||
		member["url"] != "https://media.example.org/web/#/details?id="+channel.ID.String()+"&serverId=fedcba9876543210fedcba9876543210" {
		t.Errorf("the member's message: %v", member)
	}
	if programme["id"] != show.ID.String() || programme["channelId"] != channel.ID.String() || programme["channelName"] != "Example TV" ||
		programme["start"] != "2026-10-14T18:30:00Z" || programme["end"] != "2026-10-14T19:30:00Z" {
		t.Errorf("the programme told: %v", programme)
	}
	if admin["title"] != "Commence à 20:30 : Example Show" || admin["message"] != "Example Show commence dans 5 minutes sur Example TV." {
		t.Errorf("the reminder set in French: %v", admin)
	}
}
