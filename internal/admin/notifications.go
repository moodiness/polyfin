package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/notifications"
)

// notificationsJSON is what a notifications page shows: the targets, and
// the events and kinds of targets they may choose from.
type notificationsJSON struct {
	Targets []notificationTargetJSON `json:"targets"`
	Events  []string                 `json:"events"`
	Kinds   []string                 `json:"kinds"`
}

// notificationTargetJSON is a target. Its secret address and access token
// are never answered: Address is only the scheme and host of a webhook's
// or Discord target's address, and an ntfy target's server; TokenSet
// tells whether an ntfy target has an access token. Problem is null,
// "refused", "rejected", "unreachable" or "unreadable", ProblemStatus the
// HTTP status the target answered, when it answered.
type notificationTargetJSON struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Topic         string     `json:"topic"`
	TokenSet      bool       `json:"tokenSet"`
	Events        []string   `json:"events"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSentAt    *time.Time `json:"lastSentAt"`
	Problem       *string    `json:"problem"`
	ProblemStatus *int       `json:"problemStatus"`
	ProblemAt     *time.Time `json:"problemAt"`
}

func newNotificationTargetJSON(t notifications.Target) notificationTargetJSON {
	result := notificationTargetJSON{ID: t.ID.String(), Kind: t.Kind, Name: t.Name, Address: t.Address, Topic: t.Topic, TokenSet: t.TokenSet,
		Events: t.Events, Enabled: t.Enabled, CreatedAt: t.CreatedAt.UTC().Truncate(time.Second), LastSentAt: utcSeconds(t.LastSentAt),
		ProblemStatus: t.ProblemStatus, ProblemAt: utcSeconds(t.ProblemAt)}
	if t.Problem != "" {
		result.Problem = &t.Problem
	}
	return result
}

// notificationDraftJSON adds a target, or changes one: on a change, the
// fields left out keep their values, and kind is ignored. address is a
// webhook's or Discord target's address, or an ntfy target's server, null
// or empty for the default one; token an ntfy target's access token, empty
// for none.
type notificationDraftJSON struct {
	Kind    string    `json:"kind"`
	Name    *string   `json:"name"`
	Address *string   `json:"address"`
	Topic   *string   `json:"topic"`
	Token   *string   `json:"token"`
	Events  *[]string `json:"events"`
	Enabled *bool     `json:"enabled"`
}

func (d notificationDraftJSON) draft() notifications.Draft {
	result := notifications.Draft{Kind: d.Kind, Name: d.Name, Address: d.Address, Topic: d.Topic, Token: d.Token, Enabled: d.Enabled}
	if d.Events != nil {
		result.Events = *d.Events
		if result.Events == nil {
			result.Events = []string{}
		}
	}
	return result
}

// testResultJSON is what came of "Send a test": whether the target
// accepted it, the status it answered, null when it did not answer, and
// the target as it stands after.
type testResultJSON struct {
	Delivered bool                   `json:"delivered"`
	Status    *int                   `json:"status"`
	Target    notificationTargetJSON `json:"target"`
}

// notificationOwner is whose targets a route is about: nil for the
// server's, which only administrators reach, else the signed-in user.
type notificationOwner func(r *http.Request) *accounts.User

func serverTargets(*http.Request) *accounts.User { return nil }

func ownTargets(r *http.Request) *accounts.User {
	user := sessionFrom(r.Context()).User
	return &user
}

// notificationError answers the errors of the notification routes and
// reports whether err was one of them.
func notificationError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{notifications.ErrNotFound, http.StatusNotFound, "not_found"},
		{notifications.ErrInvalidKind, http.StatusBadRequest, "invalid_target_kind"},
		{notifications.ErrInvalidName, http.StatusBadRequest, "invalid_target_name"},
		{notifications.ErrInvalidAddress, http.StatusBadRequest, "invalid_target_address"},
		{notifications.ErrPrivateAddress, http.StatusBadRequest, "private_target_address"},
		{notifications.ErrInvalidTopic, http.StatusBadRequest, "invalid_topic"},
		{notifications.ErrInvalidToken, http.StatusBadRequest, "invalid_token"},
		{notifications.ErrInvalidEvents, http.StatusBadRequest, "invalid_events"},
		{notifications.ErrTooManyTargets, http.StatusConflict, "too_many_targets"},
		{notifications.ErrUnreadable, http.StatusConflict, "target_unreadable"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// notificationRoutes answers the routes of one owner's targets.
type notificationRoutes struct {
	h     *handler
	owner notificationOwner
}

// list answers the owner's targets, and the events and kinds they may
// choose from.
func (n notificationRoutes) list(w http.ResponseWriter, r *http.Request) {
	if n.h.Notifications == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	owner := n.owner(r)
	targets, err := n.h.Notifications.Targets(r.Context(), owner)
	if err != nil {
		n.h.internalError(w, r, err)
		return
	}
	result := notificationsJSON{Targets: []notificationTargetJSON{}, Events: notifications.EventsFor(owner), Kinds: notifications.Kinds}
	for _, t := range targets {
		result.Targets = append(result.Targets, newNotificationTargetJSON(t))
	}
	writeJSON(w, http.StatusOK, result)
}

// create adds a target: 201 with it.
func (n notificationRoutes) create(w http.ResponseWriter, r *http.Request) {
	var body notificationDraftJSON
	if n.h.Notifications == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if !decode(w, r, &body) {
		return
	}
	target, err := n.h.Notifications.Create(r.Context(), n.owner(r), body.draft())
	if notificationError(w, err) {
		return
	} else if err != nil {
		n.h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newNotificationTargetJSON(target))
}

// update changes a target.
func (n notificationRoutes) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if n.h.Notifications == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body notificationDraftJSON
	if !decode(w, r, &body) {
		return
	}
	target, err := n.h.Notifications.Update(r.Context(), n.owner(r), id, body.draft())
	if notificationError(w, err) {
		return
	} else if err != nil {
		n.h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newNotificationTargetJSON(target))
}

// remove deletes a target: 204.
func (n notificationRoutes) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if n.h.Notifications == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	err := n.h.Notifications.Delete(r.Context(), n.owner(r), id)
	if notificationError(w, err) {
		return
	} else if err != nil {
		n.h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// test sends a test message to a target, at once, and answers what came
// of it: 200 whether the target accepted it or not.
func (n notificationRoutes) test(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if n.h.Notifications == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	result, err := n.h.Notifications.Test(r.Context(), n.owner(r), id)
	if notificationError(w, err) {
		return
	} else if err != nil {
		n.h.internalError(w, r, err)
		return
	}
	answer := testResultJSON{Delivered: result.Delivered, Target: newNotificationTargetJSON(result.Target)}
	if result.Status != 0 {
		answer.Status = &result.Status
	}
	writeJSON(w, http.StatusOK, answer)
}
