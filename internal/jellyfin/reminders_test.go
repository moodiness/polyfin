package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/reminders"
)

// stubTeller tells nothing, and words a reminder for the apps.
type stubTeller struct{}

func (stubTeller) ProgrammeStarting(due reminders.Due) (string, string) {
	return "Starting soon: " + due.Reminder.Name, due.Reminder.Name + " starts on " + due.Reminder.ChannelName + "."
}

// reminding gives s a reminders service whose clock is now.
func reminding(t *testing.T, s testServer, now *atomic.Pointer[time.Time]) *reminders.Service {
	t.Helper()
	service := reminders.New(reminders.Config{DB: s.pool, Guide: s.library, Users: s.store, Teller: stubTeller{}, Apps: s.handler,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return *now.Load() }})
	s.handler.Reminders = service
	return service
}

// The web client's script sets a reminder of a programme to come through
// /Polyfin/Reminders, which only users with the Live TV permission reach:
// the programme's page then shows it set, and a started programme takes
// none. With the apps option on, a signed-in web client shows the message
// when it is due.
func TestProgrammeRemindersFromTheWebClient(t *testing.T) {
	t.Parallel()
	addon := newTVAddon(t, true, "")
	s, token, member := tuned(t, addon)
	now := &atomic.Pointer[time.Time]{}
	current := time.Now()
	now.Store(&current)
	service := reminding(t, s, now)
	programs := guidePrograms(t, s, token)
	next, started := programs["Next"], programs["Now"]
	nextID, _ := parseGUID(next.Id)
	item, err := s.library.Item(t.Context(), member, nextID)
	if err != nil {
		t.Fatal(err)
	}
	start := *item.StartDate

	var state ProgramReminderDto
	if status := s.get(t, "/Polyfin/Reminders/Programs/"+next.Id, token, &state); status != http.StatusOK || state.Started || state.ReminderId != "" ||
		state.Reaches || state.BeforeMinutes != 5 || state.ChannelName != "One" || state.NotificationsUrl != "/admin/me/account#notifications" {
		t.Fatalf("a programme to come: %d %+v", status, state)
	}
	status, body := s.call(http.MethodPost, "/Polyfin/Reminders", app("tv", token), map[string]any{"ProgramId": next.Id, "Language": "fr"})
	if status != http.StatusOK || json.Unmarshal(body, &state) != nil || state.ReminderId == "" || state.SeriesReminderId != "" {
		t.Fatalf("setting a reminder: %d %s", status, body)
	}
	if status, body := s.call(http.MethodPost, "/Polyfin/Reminders", app("tv", token), map[string]any{"ProgramId": started.Id}); status != http.StatusBadRequest {
		t.Errorf("a reminder of a programme started: %d %s", status, body)
	}
	var list struct {
		Items  []ReminderDto
		Series []SeriesReminderDto
	}
	if status := s.get(t, "/Polyfin/Reminders", token, &list); status != http.StatusOK || len(list.Items) != 1 || list.Items[0].Name != "Next" ||
		list.Items[0].ChannelName != "One" || list.Series == nil {
		t.Errorf("the reminders listed: %d %+v", status, list)
	}

	off := false
	s.user("guest", func(c *accounts.UserChanges) { c.LiveTv = &off })
	guest := s.signIn("guest", "tv")
	if status, body := s.call(http.MethodPost, "/Polyfin/Reminders", app("tv", guest), map[string]any{"ProgramId": next.Id}); status != http.StatusForbidden {
		t.Errorf("a reminder set without Live TV: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/Polyfin/Reminders", app("tv", guest), nil); status != http.StatusForbidden {
		t.Errorf("reminders listed without Live TV: %d", status)
	}
	// The script asks every page it shows: a user without Live TV, or an
	// item that is no programme, such as its channel, gets nothing, and no
	// error.
	if next.ChannelId == nil || *next.ChannelId == "" {
		t.Fatal("the programme has no channel")
	}
	for _, ask := range []struct{ token, id string }{{guest, next.Id}, {token, *next.ChannelId}} {
		if status, body := s.call(http.MethodGet, "/Polyfin/Reminders/Programs/"+ask.id, app("tv", ask.token), nil); status != http.StatusNoContent ||
			len(body) != 0 {
			t.Errorf("the state of %s: %d %s", ask.id, status, body)
		}
	}

	if err := service.SetSettings(t.Context(), member.ID, reminders.Settings{Before: 5, OnApps: true}); err != nil {
		t.Fatal(err)
	}
	if status := s.get(t, "/Polyfin/Reminders/Programs/"+next.Id, token, &state); status != http.StatusOK || !state.Reaches || state.ReminderId == "" {
		t.Fatalf("with the apps option on: %d %+v", status, state)
	}
	web := s.remoteApp(t, "member", "web", queryMediaControl, true)
	due := start.Add(-5 * time.Minute)
	now.Store(&due)
	service.Check(t.Context())
	var message GeneralCommand
	web.socket.command(t, "GeneralCommand", &message)
	if message.Name != "DisplayMessage" || message.Arguments["Header"] != "Starting soon: Next" || message.Arguments["Text"] != "Next starts on One." ||
		message.Arguments["TimeoutMs"] != "" {
		t.Errorf("the message shown: %+v", message)
	}

	if status, body := s.call(http.MethodDelete, "/Polyfin/Reminders/"+state.ReminderId, app("tv", token), nil); status != http.StatusNotFound {
		t.Errorf("deleting a reminder sent: %d %s", status, body)
	}
	if status, body := s.call(http.MethodPost, "/Polyfin/Reminders", app("tv", token), map[string]any{"ProgramId": next.Id, "Series": true}); status != http.StatusOK ||
		json.Unmarshal(body, &state) != nil || state.SeriesReminderId == "" || state.SeriesStartDate == nil {
		t.Fatalf("setting a series reminder: %d %s", status, body)
	}
	if status, body := s.call(http.MethodDelete, "/Polyfin/Reminders/"+state.SeriesReminderId, app("tv", token), nil); status != http.StatusNoContent {
		t.Errorf("deleting a series reminder: %d %s", status, body)
	}
	if status := s.get(t, "/Polyfin/Reminders/Programs/"+next.Id, token, &state); status != http.StatusOK || state.ReminderId != "" || state.SeriesReminderId != "" {
		t.Errorf("after deleting the series: %d %+v", status, state)
	}
}
