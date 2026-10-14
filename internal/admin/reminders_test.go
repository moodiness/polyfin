package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/reminders"
)

// oneProgramme is a guide of a single programme.
type oneProgramme struct{ program library.Item }

func (g oneProgramme) Guide(context.Context, accounts.User, library.GuideQuery) ([]library.Item, error) {
	return []library.Item{g.program}, nil
}

func (g oneProgramme) Item(_ context.Context, _ accounts.User, id accounts.ID) (library.Item, error) {
	if id != g.program.ID {
		return library.Item{}, library.ErrNotFound
	}
	return g.program, nil
}

func (g oneProgramme) Channels(context.Context, accounts.User) ([]library.Item, error) {
	return []library.Item{*g.program.Channel}, nil
}

// My account lists the user's own reminders, deletes them, and keeps
// their reminder settings: 5 minutes before by default, or another of the
// choices, and the apps off by default. A user without Live TV has none.
func TestMyAccountListsRemindersAndKeepsTheirSettings(t *testing.T) {
	t.Parallel()
	channel := library.Item{ID: accounts.ID{0xc1}, Kind: library.KindChannel, Name: "Example TV"}
	start := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	show := library.Item{ID: accounts.ID{0xd1}, Kind: library.KindProgram, Name: "Example Show", StartDate: &start, EndDate: &end, Channel: &channel}
	var service *reminders.Service
	api := newTestAPI(t, 10, func(options *Options, deps testDeps) {
		service = reminders.New(reminders.Config{DB: deps.pool, Guide: oneProgramme{show}, Users: options.Accounts,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		options.Reminders = service
	})
	member := api.signedIn("member", false)
	users, err := api.store.Users(t.Context())
	if err != nil || len(users) != 1 {
		t.Fatalf("users: %+v %v", users, err)
	}
	user := users[0]
	status, settings, _ := member.call(http.MethodGet, "/account/reminders/settings", nil)
	if status != http.StatusOK || settings["available"] != true || settings["beforeMinutes"] != 5.0 || settings["onApps"] != false ||
		len(settings["choices"].([]any)) != 6 {
		t.Fatalf("the default settings: %d %v", status, settings)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/reminders/settings", map[string]any{"beforeMinutes": 7}); status != http.StatusBadRequest ||
		body["error"] != "invalid_reminder_settings" {
		t.Errorf("7 minutes before: %d %v", status, body)
	}
	status, settings, _ = member.call(http.MethodPut, "/account/reminders/settings", map[string]any{"beforeMinutes": 15, "onApps": true})
	if status != http.StatusOK || settings["beforeMinutes"] != 15.0 || settings["onApps"] != true {
		t.Errorf("settings saved: %d %v", status, settings)
	}

	single, err := service.Create(t.Context(), user, show.ID, false, "")
	if err != nil {
		t.Fatal(err)
	}
	status, list, _ := member.call(http.MethodGet, "/account/reminders", nil)
	items, _ := list["reminders"].([]any)
	if status != http.StatusOK || list["available"] != true || len(items) != 1 || len(list["series"].([]any)) != 0 {
		t.Fatalf("the reminders listed: %d %v", status, list)
	}
	if got := items[0].(map[string]any); got["name"] != "Example Show" || got["channelName"] != "Example TV" || got["id"] != single.Reminder.ID.String() {
		t.Errorf("the reminder listed: %v", got)
	}

	other := api.signedIn("other", false)
	if status, _, _ := other.call(http.MethodDelete, "/account/reminders/"+single.Reminder.ID.String(), nil); status != http.StatusNotFound {
		t.Errorf("another user deleting the reminder: %d", status)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/reminders/"+single.Reminder.ID.String(), nil); status != http.StatusNoContent {
		t.Errorf("deleting the reminder: %d", status)
	}
	if _, list, _ := member.call(http.MethodGet, "/account/reminders", nil); len(list["reminders"].([]any)) != 0 {
		t.Errorf("a reminder deleted is listed: %v", list)
	}

	off := false
	if _, err := api.store.UpdateUser(t.Context(), user.ID, accounts.UserChanges{LiveTv: &off}, nil); err != nil {
		t.Fatal(err)
	}
	if _, list, _ := member.call(http.MethodGet, "/account/reminders", nil); list["available"] != false {
		t.Errorf("reminders without Live TV: %v", list)
	}
}
