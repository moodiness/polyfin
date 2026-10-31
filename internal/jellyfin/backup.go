package jellyfin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/backup"
)

// backupEngineVersion is the version of Polyfin's backups, the database
// as pg_dump writes it, which apps only show.
const backupEngineVersion = "1.0.0"

// BackupManifestDto describes a backup of the folder: Path is its file
// name, which /Backup/Manifest and /Backup/Restore take. Polyfin backs up
// its database alone.
type BackupManifestDto struct {
	ServerVersion       string
	BackupEngineVersion string
	DateCreated         Time
	Path                string
	Options             BackupOptionsDto
}

// BackupOptionsDto tells what a backup holds.
type BackupOptionsDto struct {
	Metadata  bool
	Trickplay bool
	Subtitles bool
	Database  bool
}

func backupManifest(file backup.File) BackupManifestDto {
	return BackupManifestDto{ServerVersion: Version, BackupEngineVersion: backupEngineVersion, DateCreated: Time(file.MadeAt),
		Path: file.Name, Options: BackupOptionsDto{Database: true}}
}

// backupRoutes registers Jellyfin's backups, for administrators, as the
// database backups in the folder: listed, made with Back up the database,
// and restored as the admin app restores them. Deliberate differences:
// Jellyfin's backups also hold its metadata, trickplay images and
// subtitles, whichever options ask; Polyfin's hold the database alone,
// whatever is asked. A restore of a backup holding stored keys the current
// POLYFIN_SECRET_KEY cannot decrypt is refused, as apps cannot show that
// warning: the admin app restores it.
func (h *Handler) backupRoutes(rt *router) {
	administrator := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(func(w http.ResponseWriter, r *http.Request) {
			if !callerFrom(r.Context()).User.IsAdministrator {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		}))
	}
	administrator(http.MethodGet, "/Backup", h.listBackups)
	administrator(http.MethodPost, "/Backup/Create", h.createBackup)
	administrator(http.MethodGet, "/Backup/Manifest", h.backupManifest)
	administrator(http.MethodPost, "/Backup/Restore", h.restoreBackup)
}

// folderBackups lists the backups of the folder, newest first: none while
// backups are off.
func (h *Handler) folderBackups(r *http.Request) ([]backup.File, error) {
	if !h.Backups.Available() {
		return nil, nil
	}
	listing, err := h.Backups.Backups(r.Context())
	if errors.Is(err, backup.ErrOff) {
		return nil, nil
	}
	return listing.Folder, err
}

func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	files, err := h.folderBackups(r)
	if err != nil {
		h.Logger.Warn("The backups could not be listed", "error", err)
		processingError(w, http.StatusInternalServerError)
		return
	}
	manifests := []BackupManifestDto{}
	for _, file := range files {
		manifests = append(manifests, backupManifest(file))
	}
	writeJSON(w, http.StatusOK, manifests)
}

// createBackup backs the database up now, as Back up the database does,
// and answers the backup's manifest. While backups are off, it is refused,
// as before.
func (h *Handler) createBackup(w http.ResponseWriter, r *http.Request) {
	if !h.Backups.Available() {
		refused(w, r)
		return
	}
	if err := h.Backups.Run(r.Context()); err != nil {
		h.Logger.Warn("A backup asked for by an app failed", "error", err)
		processingError(w, http.StatusInternalServerError)
		return
	}
	status, err := h.Backups.Status(r.Context())
	if err != nil || status.MadeAt == nil {
		processingError(w, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, backupManifest(backup.File{Name: status.File, Size: status.Size, MadeAt: *status.MadeAt}))
}

func (h *Handler) backupManifest(w http.ResponseWriter, r *http.Request) {
	path, ok := queryParam(r, "path")
	if !ok {
		validationProblem(w, map[string][]string{"path": {"A value for the 'path' parameter or property was not provided."}})
		return
	}
	files, err := h.folderBackups(r)
	if err != nil {
		processingError(w, http.StatusInternalServerError)
		return
	}
	for _, file := range files {
		if file.Name == path {
			writeJSON(w, http.StatusOK, backupManifest(file))
			return
		}
	}
	notFoundProblem(w)
}

// restoreBackup restores the backup of the folder ArchiveFileName names:
// 204, then Polyfin stops, restores it, and starts again, as Jellyfin
// does. A backup the server cannot restore is refused with 400, one it
// does not have with 404. An encrypted backup is refused with 400 and
// errEncrypted's words: apps cannot ask for its passphrase.
func (h *Handler) restoreBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ArchiveFileName string
	}
	present, ok := readJSONBody(w, r, &body, "backupRestoreRequestDto")
	if !ok {
		return
	}
	if !present || body.ArchiveFileName == "" {
		requireBody(w, "backupRestoreRequestDto")
		return
	}
	if !h.Backups.Available() {
		notFoundProblem(w)
		return
	}
	plan, err := h.Backups.Prepare(r.Context(), nil, body.ArchiveFileName)
	if err == nil && plan.Locked {
		h.Backups.Discard(plan.ID)
		h.refuseEncrypted(w, body.ArchiveFileName)
		return
	}
	if err == nil {
		err = h.Backups.Start(r.Context(), plan.ID, false, callerFrom(r.Context()).User.Name)
	}
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, backup.ErrBackupNotFound):
		notFoundProblem(w)
	case errors.Is(err, backup.ErrUnreadableSecrets), errors.Is(err, backup.ErrNotABackup), errors.Is(err, backup.ErrNewerBackup),
		errors.Is(err, backup.ErrChangedBackup), errors.Is(err, backup.ErrNoPgRestore), errors.Is(err, backup.ErrRestoring):
		h.Logger.Warn("A restore asked for by an app was refused", "file", body.ArchiveFileName, "error", err)
		processingError(w, http.StatusBadRequest)
	default:
		h.Logger.Warn("A restore asked for by an app failed", "file", body.ArchiveFileName, "error", err)
		processingError(w, http.StatusInternalServerError)
	}
}

// errEncrypted is what an app is answered when it asks to restore an
// encrypted backup.
const errEncrypted = "This backup is encrypted: restore it from Polyfin's admin app, which asks for its passphrase."

// refuseEncrypted answers 400 to the restore of the encrypted backup
// file, saying where to restore it.
func (h *Handler) refuseEncrypted(w http.ResponseWriter, file string) {
	h.Logger.Warn("A restore asked for by an app was refused: the backup is encrypted", "file", file)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(errEncrypted))
}
