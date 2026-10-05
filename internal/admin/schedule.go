package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/tasks"
)

type taskJSON struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	// Interval is in seconds, 0 for a task run by hand only or daily.
	Interval int64 `json:"interval"`
	// Daily is the hour of the server's time zone a daily task runs at,
	// null for other tasks.
	Daily *int `json:"daily"`
	// State is Idle, Running or Cancelling.
	State string          `json:"state"`
	Last  *taskResultJSON `json:"last"`
	Next  *time.Time      `json:"next"`
}

type taskResultJSON struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Status is Completed, Failed or Cancelled.
	Status string `json:"status"`
	Error  string `json:"error"`
}

// scheduledTasks lists the server's tasks, named in the language asked,
// the server's by default.
func (h *handler) scheduledTasks(w http.ResponseWriter, r *http.Request) {
	result := []taskJSON{}
	if h.Tasks != nil {
		language := r.URL.Query().Get("language")
		if language == "" {
			language = h.Accounts.Settings().Language
		}
		for _, info := range h.Tasks.Tasks(language) {
			result = append(result, newTaskJSON(info))
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func newTaskJSON(info tasks.Info) taskJSON {
	result := taskJSON{ID: info.ID, Key: info.Key, Name: info.Name, Description: info.Description, Category: info.Category,
		Interval: int64(info.Interval.Seconds()), Daily: info.Daily, State: string(info.State), Next: info.Next}
	if info.Last != nil {
		result.Last = &taskResultJSON{Start: info.Last.Start, End: info.Last.End, Status: string(info.Last.Status), Error: info.Last.Error}
	}
	return result
}

// runTask starts a task now, unless it runs.
func (h *handler) runTask(w http.ResponseWriter, r *http.Request) {
	h.taskAction(w, r, func(id string) error { return h.Tasks.Run(id) })
}

// stopTask asks a running task to stop.
func (h *handler) stopTask(w http.ResponseWriter, r *http.Request) {
	h.taskAction(w, r, func(id string) error { return h.Tasks.Cancel(id) })
}

func (h *handler) taskAction(w http.ResponseWriter, r *http.Request, action func(id string) error) {
	if h.Tasks == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	id := r.PathValue("id")
	if errors.Is(action(id), tasks.ErrUnknown) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	info, _ := h.Tasks.Task(id, h.Accounts.Settings().Language)
	h.Logger.Info("An administrator acted on a task", "task", info.Key, "path", r.URL.Path, "by", sessionFrom(r.Context()).User.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ItemReader reads the items of the library as a user sees them: the
// library's.
type ItemReader interface {
	Item(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, error)
}

type timerJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Channel  string `json:"channel"`
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	// Start and End are the programme's; the recording runs from From to
	// Until, its padding included.
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	From  time.Time `json:"from"`
	Until time.Time `json:"until"`
	// Status is New, InProgress, Cancelled or Error.
	Status string `json:"status"`
	Series bool   `json:"series"`
}

// timers lists the Live TV recordings scheduled and under way, by start;
// none when recording is off.
func (h *handler) timers(w http.ResponseWriter, r *http.Request) {
	result := []timerJSON{}
	if h.Recordings == nil || !h.Recordings.Available() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "timers": result})
		return
	}
	list, err := h.Recordings.Timers(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	users := map[accounts.ID]accounts.User{}
	for _, t := range list {
		if t.Status == recordings.StatusCancelled {
			continue
		}
		user, ok := users[t.User]
		if !ok {
			if user, err = h.Accounts.User(r.Context(), t.User); err != nil && !errors.Is(err, accounts.ErrNotFound) {
				h.internalError(w, r, err)
				return
			}
			users[t.User] = user
		}
		timer := timerJSON{ID: t.ID.String(), Name: t.Name, UserID: t.User.String(), UserName: user.Name, Start: t.Start, End: t.End,
			From: t.From(), Until: t.Until(), Status: string(t.Status), Series: t.SeriesTimer != nil}
		// The channel is named as the user who scheduled it sees it.
		if h.Library != nil && user.ID == t.User {
			if channel, err := h.Library.Item(r.Context(), user, t.Channel); err == nil {
				timer.Channel = channel.Name
			}
		}
		result = append(result, timer)
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "timers": result})
}
