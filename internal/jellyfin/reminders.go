package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/reminders"
)

// Jellyfin has no programme reminders, and its apps no button for one.
// Polyfin's own routes set them, for its web player's script (see
// internal/server/webclient.js) and any app, under the Live TV permission
// as Jellyfin's Live TV routes:
//
//	GET /Polyfin/Reminders lists the user's reminders and series reminders;
//	GET /Polyfin/Reminders/Programs/{programId} tells how a programme
//	  stands: started or not, its reminder and series reminder, and whether
//	  a reminder would reach the user;
//	POST /Polyfin/Reminders sets a reminder of a programme, or of every
//	  airing of it (Series), and answers as the previous route;
//	DELETE /Polyfin/Reminders/{id} deletes a reminder or a series reminder.
//
// The messages go to the user's own notification targets, and to their
// apps when they chose it (see reminders.Service).

// notificationsPage is the admin app's page of the user's notifications,
// where they choose the targets that receive reminders.
const notificationsPage = "/admin/me/account#notifications"

func (h *Handler) reminderRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodGet, "/Polyfin/Reminders", h.listReminders)
	signedIn(http.MethodGet, "/Polyfin/Reminders/Programs/{programId}", h.programReminder)
	signedIn(http.MethodPost, "/Polyfin/Reminders", h.setReminder)
	signedIn(http.MethodDelete, "/Polyfin/Reminders/{reminderId}", h.deleteReminder)
}

// ProgramReminderDto is how a programme stands for the caller's reminders.
// ReminderId is its reminder's, SeriesReminderId the series reminder's that
// made it, empty without one. Reaches tells whether a reminder would reach
// the user, NotificationsUrl the page where they choose what does.
type ProgramReminderDto struct {
	ProgramId        string
	Name             string
	ChannelId        string
	ChannelName      string
	StartDate        Time
	Started          bool
	ReminderId       string
	SeriesReminderId string
	SeriesStartDate  *Time
	BeforeMinutes    int
	Reaches          bool
	NotificationsUrl string
}

// ReminderDto is a reminder set by hand.
type ReminderDto struct {
	Id          string
	ProgramId   string
	Name        string
	ChannelId   string
	ChannelName string
	StartDate   Time
	EndDate     Time
}

// SeriesReminderDto is a series reminder: StartDate gives the time of day
// of its airings, NextDate the next one, absent while none is in the guide.
type SeriesReminderDto struct {
	Id          string
	Name        string
	ChannelId   string
	ChannelName string
	StartDate   Time
	NextDate    *Time `json:",omitempty"`
}

// reminderError answers the errors of the reminders routes.
func (h *Handler) reminderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reminders.ErrNotFound):
		notFoundProblem(w)
	case errors.Is(err, reminders.ErrForbidden):
		w.WriteHeader(http.StatusForbidden)
	case errors.Is(err, reminders.ErrStarted):
		badRequestProblem(w)
	default:
		h.internalError(w, r, err)
	}
}

// remindersOn reports whether the server keeps reminders, answering 404
// when it does not.
func (h *Handler) remindersOn(w http.ResponseWriter) bool {
	if h.Reminders == nil {
		notFoundProblem(w)
		return false
	}
	return true
}

func (h *Handler) listReminders(w http.ResponseWriter, r *http.Request) {
	if !h.remindersOn(w) {
		return
	}
	user := callerFrom(r.Context()).User
	list, series, err := h.Reminders.List(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Items  []ReminderDto
		Series []SeriesReminderDto
	}{Items: []ReminderDto{}, Series: []SeriesReminderDto{}}
	for _, reminder := range list {
		result.Items = append(result.Items, ReminderDto{Id: reminder.ID.String(), ProgramId: reminder.Program.String(), Name: reminder.Name,
			ChannelId: reminder.Channel.String(), ChannelName: reminder.ChannelName, StartDate: Time(reminder.Start), EndDate: Time(reminder.End)})
	}
	for _, sr := range series {
		dto := SeriesReminderDto{Id: sr.ID.String(), Name: sr.Name, ChannelId: sr.Channel.String(), ChannelName: sr.ChannelName,
			StartDate: Time(sr.Start)}
		if sr.Next != nil {
			next := Time(*sr.Next)
			dto.NextDate = &next
		}
		result.Series = append(result.Series, dto)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) programReminder(w http.ResponseWriter, r *http.Request) {
	if !h.remindersOn(w) {
		return
	}
	id, ok := parseGUID(r.PathValue("programId"))
	if !ok {
		notFoundProblem(w)
		return
	}
	user := callerFrom(r.Context()).User
	state, err := h.Reminders.State(r.Context(), user, id)
	if err != nil {
		h.reminderError(w, r, err)
		return
	}
	h.writeReminderState(w, r, user, state)
}

func (h *Handler) setReminder(w http.ResponseWriter, r *http.Request) {
	if !h.remindersOn(w) {
		return
	}
	var body struct {
		ProgramId string
		Series    bool
		// Language is the client's, such as "fr", which the message is
		// written in; English or French, else the server's.
		Language string
	}
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil || json.Unmarshal(raw, &body) != nil {
		badRequestProblem(w)
		return
	}
	id, ok := parseGUID(body.ProgramId)
	if !ok {
		badRequestProblem(w)
		return
	}
	user := callerFrom(r.Context()).User
	state, err := h.Reminders.Create(r.Context(), user, id, body.Series, body.Language)
	if err != nil {
		h.reminderError(w, r, err)
		return
	}
	h.writeReminderState(w, r, user, state)
}

func (h *Handler) deleteReminder(w http.ResponseWriter, r *http.Request) {
	if !h.remindersOn(w) {
		return
	}
	id, ok := parseGUID(r.PathValue("reminderId"))
	if !ok {
		notFoundProblem(w)
		return
	}
	if err := h.Reminders.Delete(r.Context(), callerFrom(r.Context()).User.ID, id); err != nil {
		h.reminderError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeReminderState answers how a programme stands for user.
func (h *Handler) writeReminderState(w http.ResponseWriter, r *http.Request, user accounts.User, state reminders.State) {
	settings, err := h.Reminders.Settings(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	program := state.Program
	dto := ProgramReminderDto{ProgramId: program.ID.String(), Name: program.Name, ChannelId: program.Channel.ID.String(),
		ChannelName: program.Channel.Name, StartDate: Time(*program.StartDate), Started: state.Started, BeforeMinutes: settings.Before,
		Reaches: state.Reaches, NotificationsUrl: notificationsPage}
	if state.Reminder != nil {
		dto.ReminderId = state.Reminder.ID.String()
	}
	if state.Series != nil {
		dto.SeriesReminderId = state.Series.ID.String()
		start := Time(state.Series.Start)
		dto.SeriesStartDate = &start
	}
	writeJSON(w, http.StatusOK, dto)
}

// ShowMessage shows a message, with a header, on every app of user that
// accepts messages, as Send a message does: the apps that keep a socket
// open and declared media control. It shows until dismissed. It returns
// how many apps were sent it.
func (h *Handler) ShowMessage(ctx context.Context, user accounts.ID, header, text string) int {
	devices, err := h.Accounts.Devices(ctx, user)
	if err != nil {
		h.Logger.Warn("A user's devices could not be read for a message", "error", err)
		return 0
	}
	shown := 0
	by := accounts.User{ID: user}
	for _, device := range devices {
		if !device.Capabilities.SupportsMediaControl || h.sockets.latest(device.ID) == nil {
			continue
		}
		command := GeneralCommand{Name: "DisplayMessage", ControllingUserId: user.String(),
			Arguments: map[string]string{"Header": header, "Text": text}}
		if delivered, err := h.deliver(ctx, by, device.ID, "GeneralCommand", command); err == nil && delivered {
			shown++
		}
	}
	return shown
}
