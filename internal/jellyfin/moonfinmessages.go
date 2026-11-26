package jellyfin

import (
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/moonfin"
)

// The administrator's messages to the users of Moonfin's apps (see
// package moonfin): an app asks for the caller's with GET
// /Moonfin/Messages when it signs in, and again each time its settings
// stream tells it "messagesChanged" (see moonfinStream). Which messages it
// read it keeps itself.

// moonfinMessageRoutes adds the routes of the administrator's messages.
func (h *Handler) moonfinMessageRoutes(route func(method, pattern string, handler http.HandlerFunc)) {
	route(http.MethodGet, "/Moonfin/Messages", h.moonfinMessages)
}

// moonfinMessageJSON is a message as Moonfin's apps read it: its color
// and delivery as moonfin names them, its link button, if any, as an
// action, and when it was written.
type moonfinMessageJSON struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Color       string    `json:"color"`
	Delivery    string    `json:"delivery"`
	ActionLabel string    `json:"actionLabel,omitempty"`
	ActionURL   string    `json:"actionUrl,omitempty"`
	CreatedUTC  time.Time `json:"createdUtc"`
}

// moonfinMessages answers, in the administrator's order, the messages
// meant for the caller that show now, and those alone: whom else a
// message is meant for, and its dates, stay on the server.
func (h *Handler) moonfinMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.Moonfin.MessagesFor(r.Context(), callerFrom(r.Context()).User)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items := make([]moonfinMessageJSON, 0, len(messages))
	for _, m := range messages {
		items = append(items, newMoonfinMessageJSON(m))
	}
	writeJSON(w, http.StatusOK, struct {
		Items []moonfinMessageJSON `json:"items"`
	}{items})
}

func newMoonfinMessageJSON(m moonfin.Message) moonfinMessageJSON {
	return moonfinMessageJSON{ID: m.ID.String(), Title: m.Title, Body: m.Body, Color: string(m.Color), Delivery: string(m.Delivery),
		ActionLabel: m.LinkLabel, ActionURL: m.LinkURL, CreatedUTC: m.Created.UTC()}
}
