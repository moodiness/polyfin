package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// The administrator's messages to the users of Moonfin's apps (Settings ›
// General, under Serve Moonfin's companion; see package moonfin).

// moonfinMessageRoutes adds the routes of the administrator's messages,
// for administrators alone.
func (h *handler) moonfinMessageRoutes(mux *http.ServeMux) {
	mux.Handle("GET /admin/api/moonfin/messages", h.administrator(h.moonfinMessages))
	mux.Handle("POST /admin/api/moonfin/messages", h.administrator(h.createMoonfinMessage))
	mux.Handle("PUT /admin/api/moonfin/messages/order", h.administrator(h.orderMoonfinMessages))
	mux.Handle("PATCH /admin/api/moonfin/messages/{id}", h.administrator(h.updateMoonfinMessage))
	mux.Handle("DELETE /admin/api/moonfin/messages/{id}", h.administrator(h.deleteMoonfinMessage))
}

// moonfinMessageJSON is a message: its title and Markdown body, its color
// (white, blue, green, yellow or red), its delivery (inbox or popup), its
// audience (all, users or admins) with the users of users, from start
// until end, RFC 3339, each empty for no bound, and its link button,
// linkLabel and linkUrl, both empty for none. In answers only: when it was
// created and last changed, and its state at the time: scheduled before
// its start, ended after its end, shown in between.
type moonfinMessageJSON struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Color     string   `json:"color"`
	Delivery  string   `json:"delivery"`
	Audience  string   `json:"audience"`
	Users     []string `json:"users"`
	Start     string   `json:"start"`
	End       string   `json:"end"`
	LinkLabel string   `json:"linkLabel"`
	LinkURL   string   `json:"linkUrl"`
	CreatedAt string   `json:"createdAt,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
	State     string   `json:"state,omitempty"`
}

// The states of a message.
const (
	moonfinMessageScheduled = "scheduled"
	moonfinMessageShown     = "shown"
	moonfinMessageEnded     = "ended"
)

func newMoonfinMessageJSON(m moonfin.Message, now time.Time) moonfinMessageJSON {
	date := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	state := moonfinMessageShown
	switch {
	case !m.Start.IsZero() && now.Before(m.Start):
		state = moonfinMessageScheduled
	case !m.End.IsZero() && !now.Before(m.End):
		state = moonfinMessageEnded
	}
	result := moonfinMessageJSON{ID: m.ID.String(), Title: m.Title, Body: m.Body, Color: string(m.Color), Delivery: string(m.Delivery),
		Audience: string(m.Audience), Users: []string{}, Start: date(m.Start), End: date(m.End), LinkLabel: m.LinkLabel,
		LinkURL: m.LinkURL, CreatedAt: date(m.Created), UpdatedAt: date(m.Updated), State: state}
	for _, user := range m.Users {
		result.Users = append(result.Users, user.String())
	}
	return result
}

// message reads a message from its JSON, and reports whether it could:
// users and dates that are not ones cannot be read.
func (j moonfinMessageJSON) message() (moonfin.Message, bool) {
	m := moonfin.Message{Title: j.Title, Body: j.Body, Color: moonfin.MessageColor(j.Color), Delivery: moonfin.Delivery(j.Delivery),
		Audience: moonfin.Audience(j.Audience), LinkLabel: j.LinkLabel, LinkURL: j.LinkURL}
	ok := true
	date := func(raw string) time.Time {
		if raw == "" {
			return time.Time{}
		}
		t, err := time.Parse(time.RFC3339, raw)
		ok = ok && err == nil
		return t
	}
	m.Start, m.End = date(j.Start), date(j.End)
	for _, raw := range j.Users {
		user, err := accounts.ParseID(raw)
		ok = ok && err == nil
		m.Users = append(m.Users, user)
	}
	return m, ok && m.Cleaned().Valid()
}

// moonfinMessageError answers the errors of the messages' store, and
// reports whether it did.
func moonfinMessageError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, moonfin.ErrMessageNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, moonfin.ErrTooManyMessages):
		writeError(w, http.StatusConflict, "too_many_moonfin_messages")
	case errors.Is(err, moonfin.ErrInvalidMessage):
		writeError(w, http.StatusBadRequest, "invalid_moonfin_message")
	default:
		return false
	}
	return true
}

// moonfinMessages answers every message, in order, whatever its dates.
func (h *handler) moonfinMessages(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	messages, err := h.Moonfin.Messages(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	result := make([]moonfinMessageJSON, 0, len(messages))
	for _, m := range messages {
		result = append(result, newMoonfinMessageJSON(m, now))
	}
	writeJSON(w, http.StatusOK, struct {
		Messages []moonfinMessageJSON `json:"messages"`
	}{result})
}

// moonfinMessageFrom reads the message of the body, and answers 400 when
// it is invalid. It reports whether it read one.
func moonfinMessageFrom(w http.ResponseWriter, r *http.Request) (moonfin.Message, bool) {
	var body moonfinMessageJSON
	if !decode(w, r, &body) {
		return moonfin.Message{}, false
	}
	m, ok := body.message()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_moonfin_message")
	}
	return m, ok
}

// savedMoonfinMessage answers err, or else m with status.
func (h *handler) savedMoonfinMessage(w http.ResponseWriter, r *http.Request, status int, m moonfin.Message, err error) {
	if moonfinMessageError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, status, newMoonfinMessageJSON(m, h.now()))
}

// createMoonfinMessage adds a message before the others, as the newest:
// 201 with the message. Open apps of those it is meant for are told.
func (h *handler) createMoonfinMessage(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	m, ok := moonfinMessageFrom(w, r)
	if !ok {
		return
	}
	created, err := h.Moonfin.CreateMessage(r.Context(), m)
	h.savedMoonfinMessage(w, r, http.StatusCreated, created, err)
}

// updateMoonfinMessage changes a message, its place kept. Open apps of
// those it was or is meant for are told.
func (h *handler) updateMoonfinMessage(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	m, ok := moonfinMessageFrom(w, r)
	if !ok {
		return
	}
	m.ID = id
	updated, err := h.Moonfin.UpdateMessage(r.Context(), m)
	h.savedMoonfinMessage(w, r, http.StatusOK, updated, err)
}

// deleteMoonfinMessage deletes a message: 204. Open apps of those it was
// meant for are told.
func (h *handler) deleteMoonfinMessage(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Moonfin.DeleteMessage(r.Context(), id)
	if moonfinMessageError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// orderMoonfinMessages puts the messages in the order of the identifiers
// given; the others keep their order after them. It answers the messages.
func (h *handler) orderMoonfinMessages(w http.ResponseWriter, r *http.Request) {
	if !h.moonfinOffered(w) {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	ids := make([]accounts.ID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := accounts.ParseID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		ids = append(ids, id)
	}
	if err := h.Moonfin.OrderMessages(r.Context(), ids); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.moonfinMessages(w, r)
}
