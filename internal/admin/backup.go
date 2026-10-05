package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/tasks"
)

// backupJSON is how the database backups go. Folder is POLYFIN_BACKUP_DIR,
// empty when backups are off, which leaves the rest empty.
type backupJSON struct {
	Folder string `json:"folder"`
	// Next is when the next backup is made, at the settings' hour.
	Next *time.Time `json:"next"`
	// RanAt is when the last run started, null before the first; Error is
	// why it failed, empty after a success.
	RanAt *time.Time `json:"ranAt"`
	Error string     `json:"error"`
	// MadeAt is when the last backup made started, null before the first;
	// File is its name in the folder, Size its bytes.
	MadeAt *time.Time `json:"madeAt"`
	File   string     `json:"file"`
	Size   int64      `json:"size"`
	// Problem tells whether the last run failed or the last backup made is
	// older than two days.
	Problem bool `json:"problem"`
}

// backupStatus describes the backups, nil when they are off.
func (h *handler) backupStatus(ctx context.Context) (*backupJSON, error) {
	if !h.Backups.Available() {
		return nil, nil
	}
	status, err := h.Backups.Status(ctx)
	if err != nil {
		return nil, err
	}
	now := h.now()
	return &backupJSON{Folder: h.Backups.Dir(), Next: new(tasks.NextDaily(now, h.Backups.Hour())), RanAt: status.RanAt,
		Error: status.Error, MadeAt: status.MadeAt, File: status.File, Size: status.Size, Problem: status.Problem(now)}, nil
}

// backup tells how the database backups go: an empty folder when they are
// off.
func (h *handler) backup(w http.ResponseWriter, r *http.Request) {
	status, err := h.backupStatus(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if status == nil {
		status = &backupJSON{}
	}
	writeJSON(w, http.StatusOK, status)
}
