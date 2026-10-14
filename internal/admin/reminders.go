package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/reminders"
)

// reminderJSON is a reminder of one programme, set by hand.
type reminderJSON struct {
	ID          string    `json:"id"`
	ProgramID   string    `json:"programId"`
	Name        string    `json:"name"`
	ChannelID   string    `json:"channelId"`
	ChannelName string    `json:"channelName"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

// seriesReminderJSON is a series reminder: start gives the time of day of
// its airings, next the next one, null while none is in the guide.
type seriesReminderJSON struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ChannelID   string     `json:"channelId"`
	ChannelName string     `json:"channelName"`
	Start       time.Time  `json:"start"`
	Next        *time.Time `json:"next"`
}

// remindersJSON is My account › Reminders: available is false for a user
// without the Live TV permission, who sets none.
type remindersJSON struct {
	Available bool                 `json:"available"`
	Reminders []reminderJSON       `json:"reminders"`
	Series    []seriesReminderJSON `json:"series"`
}

// reminderSettingsJSON are the user's reminder settings, under My account ›
// Notifications: the minutes before a programme, one of choices, and
// whether reminders show on their apps too.
type reminderSettingsJSON struct {
	Available     bool  `json:"available"`
	BeforeMinutes int   `json:"beforeMinutes"`
	OnApps        bool  `json:"onApps"`
	Choices       []int `json:"choices"`
}

func (h *handler) ownReminders(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	result := remindersJSON{Available: h.Reminders != nil && user.LiveTv, Reminders: []reminderJSON{}, Series: []seriesReminderJSON{}}
	if !result.Available {
		writeJSON(w, http.StatusOK, result)
		return
	}
	list, series, err := h.Reminders.List(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, reminder := range list {
		result.Reminders = append(result.Reminders, reminderJSON{ID: reminder.ID.String(), ProgramID: reminder.Program.String(),
			Name: reminder.Name, ChannelID: reminder.Channel.String(), ChannelName: reminder.ChannelName, Start: reminder.Start, End: reminder.End})
	}
	for _, sr := range series {
		result.Series = append(result.Series, seriesReminderJSON{ID: sr.ID.String(), Name: sr.Name, ChannelID: sr.Channel.String(),
			ChannelName: sr.ChannelName, Start: sr.Start, Next: sr.Next})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) deleteOwnReminder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if h.Reminders == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	err := h.Reminders.Delete(r.Context(), sessionFrom(r.Context()).User.ID, id)
	switch {
	case errors.Is(err, reminders.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) ownReminderSettings(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	result := reminderSettingsJSON{Available: h.Reminders != nil && user.LiveTv, BeforeMinutes: reminders.DefaultBefore,
		Choices: reminders.Befores}
	if h.Reminders != nil {
		settings, err := h.Reminders.Settings(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		result.BeforeMinutes, result.OnApps = settings.Before, settings.OnApps
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) updateOwnReminderSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BeforeMinutes int  `json:"beforeMinutes"`
		OnApps        bool `json:"onApps"`
	}
	if !decode(w, r, &body) {
		return
	}
	if h.Reminders == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	user := sessionFrom(r.Context()).User
	err := h.Reminders.SetSettings(r.Context(), user.ID, reminders.Settings{Before: body.BeforeMinutes, OnApps: body.OnApps})
	switch {
	case errors.Is(err, reminders.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_reminder_settings")
		return
	case err != nil:
		h.internalError(w, r, err)
		return
	}
	h.ownReminderSettings(w, r)
}
