package jellyfin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/tasks"
)

// recordPlayback records in the activity log that user started or stopped
// playing item on device, as Jellyfin does: an episode is named after its
// series, and a track or an audiobook after its first artist, as audio.
// Progress reports are not recorded.
func (h *Handler) recordPlayback(ctx context.Context, user accounts.User, device string, event playbackEvent, item library.Item) {
	if user.ID == (accounts.ID{}) || event != playbackStarted && event != playbackStopped {
		return
	}
	title := item.Name
	if item.SeriesName != "" {
		title = item.SeriesName + " - " + title
	}
	if len(item.Artists) > 0 {
		title = item.Artists[0].Name + " - " + title
	}
	switch {
	case library.AudioKind(item.Kind) && event == playbackStarted:
		h.Activity.AudioStarted(ctx, user, title, item.ID.String(), device)
	case library.AudioKind(item.Kind):
		h.Activity.AudioStopped(ctx, user, title, item.ID.String(), device)
	case event == playbackStarted:
		h.Activity.PlaybackStarted(ctx, user, title, item.ID.String(), device)
	default:
		h.Activity.PlaybackStopped(ctx, user, title, item.ID.String(), device)
	}
}

// ActivityLogEntry is an entry of Jellyfin's activity log.
type ActivityLogEntry struct {
	Id            int64
	Name          string
	Overview      *string `json:",omitempty"`
	ShortOverview *string `json:",omitempty"`
	Type          string
	ItemId        *string `json:",omitempty"`
	Date          Time
	UserId        string
	Severity      string
}

// activityEntries lists the activity log, newest first, as Jellyfin's
// dashboard reads it: from startIndex, at most limit entries, from
// minDate, and those about a user or not (hasUserId).
func (h *Handler) activityEntries(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, startSet := b.int32(r, "startIndex")
	limit, _ := b.int32(r, "limit")
	var q activity.Query
	if raw, _ := queryParam(r, "minDate"); strings.TrimSpace(raw) != "" {
		if date, ok := parseTime(raw); ok {
			q.MinDate = date
		} else {
			b.add("minDate", notValid(raw))
		}
	}
	if hasUser, set := b.bool(r, "hasUserId"); set {
		q.HasUserID = &hasUser
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	q.Start, q.Limit = max(start, 0), max(limit, 0)
	page := activity.Page{}
	if h.Activity != nil {
		var err error
		if page, err = h.Activity.Entries(r.Context(), q); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	result := listResult[ActivityLogEntry]{Items: make([]ActivityLogEntry, 0, len(page.Entries)), TotalRecordCount: page.Total}
	if startSet {
		result.StartIndex = start
	}
	for _, e := range page.Entries {
		entry := ActivityLogEntry{Id: e.ID, Name: e.Name, Overview: e.Overview, ShortOverview: e.ShortOverview, Type: e.Type,
			ItemId: e.ItemID, Date: Time(e.Date), UserId: zeroID, Severity: e.Severity}
		if e.UserID != nil {
			entry.UserId = e.UserID.String()
		}
		result.Items = append(result.Items, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// zeroID is the empty identifier, as Jellyfin writes it for no user.
const zeroID = "00000000000000000000000000000000"

// TaskInfo describes a scheduled task as Jellyfin does.
type TaskInfo struct {
	Name                string
	State               tasks.State
	Id                  string
	LastExecutionResult *TaskResult `json:",omitempty"`
	Triggers            []TaskTriggerInfo
	Description         string
	Category            string
	IsHidden            bool
	Key                 string
}

// TaskResult is how a task's last run ended.
type TaskResult struct {
	StartTimeUtc Time
	EndTimeUtc   Time
	Status       tasks.Status
	Name         string
	Key          string
	Id           string
	ErrorMessage string `json:",omitempty"`
}

// TaskTriggerInfo is what runs a task: at startup, or every IntervalTicks.
type TaskTriggerInfo struct {
	Type          string
	IntervalTicks int64 `json:",omitempty"`
}

func newTaskInfo(info tasks.Info) TaskInfo {
	task := TaskInfo{Name: info.Name, State: info.State, Id: info.ID, Triggers: []TaskTriggerInfo{},
		Description: info.Description, Category: info.Category, Key: info.Key}
	if info.AtStart {
		task.Triggers = append(task.Triggers, TaskTriggerInfo{Type: "StartupTrigger"})
	}
	if info.Interval > 0 {
		task.Triggers = append(task.Triggers, TaskTriggerInfo{Type: "IntervalTrigger", IntervalTicks: int64(info.Interval / 100)})
	}
	if last := info.Last; last != nil {
		task.LastExecutionResult = &TaskResult{StartTimeUtc: Time(last.Start), EndTimeUtc: Time(last.End), Status: last.Status,
			Name: info.Name, Key: info.Key, Id: info.ID, ErrorMessage: last.Error}
	}
	return task
}

// scheduledTasks lists Polyfin's tasks by name. None is hidden and all are
// enabled, which isHidden and isEnabled filter on.
func (h *Handler) scheduledTasks(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	hidden, hiddenSet := b.bool(r, "isHidden")
	enabled, enabledSet := b.bool(r, "isEnabled")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	result := []TaskInfo{}
	if h.Tasks != nil && !(hiddenSet && hidden) && !(enabledSet && !enabled) {
		for _, info := range h.Tasks.Tasks(h.Accounts.Settings().Language) {
			result = append(result, newTaskInfo(info))
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// task finds the task the route names; ok is false once w was answered.
func (h *Handler) task(w http.ResponseWriter, r *http.Request) (tasks.Info, bool) {
	if h.Tasks != nil {
		if info, ok := h.Tasks.Task(r.PathValue("taskId"), h.Accounts.Settings().Language); ok {
			return info, true
		}
	}
	notFoundProblem(w)
	return tasks.Info{}, false
}

func (h *Handler) scheduledTask(w http.ResponseWriter, r *http.Request) {
	if info, ok := h.task(w, r); ok {
		writeJSON(w, http.StatusOK, newTaskInfo(info))
	}
}

// startTask runs a task now, unless it is running.
func (h *Handler) startTask(w http.ResponseWriter, r *http.Request) {
	if info, ok := h.task(w, r); ok {
		_ = h.Tasks.Run(info.ID)
		h.Logger.Info("A task was started by hand", "task", info.Key, "by", actor(callerFrom(r.Context())))
		w.WriteHeader(http.StatusNoContent)
	}
}

// stopTask asks a running task to stop.
func (h *Handler) stopTask(w http.ResponseWriter, r *http.Request) {
	if info, ok := h.task(w, r); ok {
		_ = h.Tasks.Cancel(info.ID)
		w.WriteHeader(http.StatusNoContent)
	}
}

// updateTaskTriggers refuses new triggers: Polyfin's tasks run on fixed
// schedules, which Jellyfin lets administrators change.
func (h *Handler) updateTaskTriggers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.task(w, r); ok {
		validationProblem(w, map[string][]string{"triggerInfos": {"Polyfin's tasks run on fixed schedules: their triggers cannot be changed."}})
	}
}

// logName is the name of the one log file Polyfin serves: the recent lines
// of its log, kept in memory.
const logName = "polyfin.log"

// LogFile describes a log file.
type LogFile struct {
	DateCreated  Time
	DateModified Time
	Size         int64
	Name         string
}

func (h *Handler) logFiles(w http.ResponseWriter, _ *http.Request) {
	files := []LogFile{}
	if h.Logs != nil {
		created, modified, size := h.Logs.Stat()
		files = append(files, LogFile{DateCreated: Time(created), DateModified: Time(modified), Size: int64(size), Name: logName})
	}
	writeJSON(w, http.StatusOK, files)
}

// logFile serves the recent log lines, with URLs and secrets redacted.
func (h *Handler) logFile(w http.ResponseWriter, r *http.Request) {
	name := query(r, "name")
	if strings.TrimSpace(name) == "" {
		validationProblem(w, map[string][]string{"name": {"The name field is required."}})
		return
	}
	if h.Logs == nil || !strings.EqualFold(name, logName) {
		writeJSON(w, http.StatusNotFound, "Log file not found.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.Logs.Contents())
}

// maxClientLog bounds the logs apps upload, as Jellyfin does.
const maxClientLog = 1_000_000

// ClientLogDocumentResponseDto names an uploaded app log.
type ClientLogDocumentResponseDto struct {
	FileName string
}

// clientLog takes the log an app uploads, as jellyfin-web's "send logs"
// does. Jellyfin writes it to a file; Polyfin writes it to its own log, at
// the Info level, redacted, under the file name it answers with.
func (h *Handler) clientLog(w http.ResponseWriter, r *http.Request) {
	tooLarge := func() {
		writeJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Payload must be less than %s bytes", "1,000,000"))
	}
	if r.ContentLength > maxClientLog {
		tooLarge()
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxClientLog))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		tooLarge()
		return
	}
	if err != nil {
		processingError(w, http.StatusBadRequest)
		return
	}
	c := callerFrom(r.Context())
	client, version := c.Device.Client, c.Device.ClientVersion
	if client == "" {
		client = "unknown-client"
	}
	if c.APIKey != nil {
		version = "apikey"
	} else if version == "" {
		version = "unknown-version"
	}
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	name := fmt.Sprintf("upload_%s_%s_%s_%s.log", client, version, h.now().UTC().Format("20060102150405"), hex.EncodeToString(raw[:]))
	h.Logger.Info("An app sent its log", "file", name, "user", c.User.Name, "document", logs.Redact(string(body)))
	writeJSON(w, http.StatusOK, ClientLogDocumentResponseDto{FileName: name})
}
