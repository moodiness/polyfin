package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/backup"
	"github.com/moodiness/polyfin/internal/tasks"
)

// backupJSON is how the database backups go. Folder is the folder they are
// written to, empty when backups are off, which leaves the rest empty.
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
	// Destinations are where the backups are sent besides the folder, with
	// their last upload.
	Destinations []destinationJSON `json:"destinations"`
	// Restore is why a backup cannot be restored: "pg_restore_missing", or
	// empty when it can.
	Restore string `json:"restore"`
	// LastRestore is how the last restore went, null before the first.
	LastRestore *restoreJSON `json:"lastRestore"`
}

// restoreJSON is how a restore went: when it ran, the backup restored,
// the administrator who asked, and its error, empty after a success.
type restoreJSON struct {
	At            time.Time `json:"at"`
	File          string    `json:"file"`
	Administrator string    `json:"administrator"`
	Error         string    `json:"error"`
}

// destinationJSON is a destination. Its password, secret key and private
// key are never answered: SecretSet and PrivateKeySet tell whether they
// are set. HostKey is the SHA-256 fingerprint of the SFTP server's key
// saved at the first connection, empty before it. RanAt and Error are its
// last upload, NewestFile and NewestAt the newest backup there; Problem
// is null, "failed" or "stale"; Unreadable tells that POLYFIN_SECRET_KEY
// cannot decrypt its password or keys.
type destinationJSON struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Port          int        `json:"port"`
	Bucket        string     `json:"bucket"`
	Folder        string     `json:"folder"`
	Region        string     `json:"region"`
	Username      string     `json:"username"`
	SecretSet     bool       `json:"secretSet"`
	PrivateKeySet bool       `json:"privateKeySet"`
	HostKey       string     `json:"hostKey"`
	Kept          int        `json:"kept"`
	CreatedAt     time.Time  `json:"createdAt"`
	RanAt         *time.Time `json:"ranAt"`
	Error         string     `json:"error"`
	NewestFile    string     `json:"newestFile"`
	NewestAt      *time.Time `json:"newestAt"`
	Problem       *string    `json:"problem"`
	Unreadable    bool       `json:"unreadable"`
}

func newDestinationJSON(d backup.Destination, now time.Time) destinationJSON {
	result := destinationJSON{ID: d.ID.String(), Kind: d.Kind, Name: d.Name(), Address: d.Address, Port: d.Port, Bucket: d.Bucket,
		Folder: d.Folder, Region: d.Region, Username: d.Username, SecretSet: d.SecretSet, PrivateKeySet: d.PrivateKeySet,
		HostKey: d.HostKeyFingerprint(), Kept: d.Kept, CreatedAt: d.CreatedAt.UTC().Truncate(time.Second), RanAt: utcSeconds(d.RanAt),
		Error: d.Error, NewestFile: d.NewestFile, NewestAt: d.NewestAt, Unreadable: d.Unreadable}
	if problem := d.Problem(now); problem != "" {
		result.Problem = &problem
	}
	return result
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
	destinations, err := h.Backups.Destinations(ctx)
	if err != nil {
		return nil, err
	}
	last, err := h.Backups.LastRestore(ctx)
	if err != nil {
		return nil, err
	}
	now := h.now()
	result := &backupJSON{Folder: h.Backups.Dir(), Next: new(tasks.NextDaily(now, h.Backups.Hour())), RanAt: status.RanAt,
		Error: status.Error, MadeAt: status.MadeAt, File: status.File, Size: status.Size, Problem: status.Problem(now),
		Destinations: []destinationJSON{}}
	for _, d := range destinations {
		result.Destinations = append(result.Destinations, newDestinationJSON(d, now))
	}
	if errors.Is(h.Backups.CanRestore(), backup.ErrNoPgRestore) {
		result.Restore = "pg_restore_missing"
	}
	if last != nil {
		result.LastRestore = &restoreJSON{At: last.At.UTC().Truncate(time.Second), File: last.File, Administrator: last.Administrator,
			Error: last.Error}
	}
	return result, nil
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
		status = &backupJSON{Destinations: []destinationJSON{}}
	}
	writeJSON(w, http.StatusOK, status)
}

// backupError answers the errors of the backup routes and reports whether
// err was one of them.
func backupError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{backup.ErrDestinationNotFound, http.StatusNotFound, "not_found"},
		{backup.ErrTooManyDestinations, http.StatusConflict, "too_many_destinations"},
		{backup.ErrInvalidDestinationKind, http.StatusBadRequest, "invalid_destination_kind"},
		{backup.ErrInvalidDestinationAddress, http.StatusBadRequest, "invalid_destination_address"},
		{backup.ErrInvalidBucket, http.StatusBadRequest, "invalid_bucket"},
		{backup.ErrInvalidDestinationFolder, http.StatusBadRequest, "invalid_destination_folder"},
		{backup.ErrInvalidRegion, http.StatusBadRequest, "invalid_region"},
		{backup.ErrInvalidPort, http.StatusBadRequest, "invalid_port"},
		{backup.ErrInvalidUsername, http.StatusBadRequest, "invalid_destination_user"},
		{backup.ErrInvalidSecret, http.StatusBadRequest, "invalid_destination_secret"},
		{backup.ErrInvalidPrivateKey, http.StatusBadRequest, "invalid_private_key"},
		{backup.ErrInvalidDestinationKept, http.StatusBadRequest, "invalid_backups_kept"},
		{backup.ErrOff, http.StatusConflict, "backups_off"},
		{backup.ErrNoPgRestore, http.StatusConflict, "pg_restore_missing"},
		{backup.ErrBackupNotFound, http.StatusNotFound, "backup_not_found"},
		{backup.ErrNotABackup, http.StatusBadRequest, "not_a_backup"},
		{backup.ErrNewerBackup, http.StatusBadRequest, "newer_backup"},
		{backup.ErrChangedBackup, http.StatusBadRequest, "changed_backup"},
		{backup.ErrNoPlan, http.StatusConflict, "restore_expired"},
		{backup.ErrUnreadableSecrets, http.StatusConflict, "unreadable_secrets"},
		{backup.ErrRestoring, http.StatusConflict, "restoring"},
		{backup.ErrPassphraseRequired, http.StatusConflict, "passphrase_required"},
		{backup.ErrWrongPassphrase, http.StatusBadRequest, "wrong_passphrase"},
		{backup.ErrDamagedBackup, http.StatusBadRequest, "damaged_backup"},
		{backup.ErrNotEncrypted, http.StatusBadRequest, "not_encrypted"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// destinationDraftJSON adds a destination, or changes one: on a change,
// the fields left out keep their values, and kind is ignored. secret is
// an S3 secret key or a password, privateKey an SFTP private key, both
// emptied by "" and kept when left out; hostKey may only be "", to accept
// the key the SFTP server offers at the next connection.
type destinationDraftJSON struct {
	Kind       string  `json:"kind"`
	Address    *string `json:"address"`
	Port       *int    `json:"port"`
	Bucket     *string `json:"bucket"`
	Folder     *string `json:"folder"`
	Region     *string `json:"region"`
	Username   *string `json:"username"`
	Secret     *string `json:"secret"`
	PrivateKey *string `json:"privateKey"`
	HostKey    *string `json:"hostKey"`
	Kept       *int    `json:"kept"`
}

func (d destinationDraftJSON) draft() backup.Draft {
	return backup.Draft{Kind: d.Kind, Address: d.Address, Port: d.Port, Bucket: d.Bucket, Folder: d.Folder, Region: d.Region,
		Username: d.Username, Secret: d.Secret, PrivateKey: d.PrivateKey, HostKey: d.HostKey, Kept: d.Kept}
}

// destinationsJSON lists the destinations, and the kinds they may be.
type destinationsJSON struct {
	Destinations []destinationJSON `json:"destinations"`
	Kinds        []string          `json:"kinds"`
}

func (h *handler) backupDestinations(w http.ResponseWriter, r *http.Request) {
	destinations, err := h.Backups.Destinations(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	result := destinationsJSON{Destinations: []destinationJSON{}, Kinds: backup.Kinds}
	for _, d := range destinations {
		result.Destinations = append(result.Destinations, newDestinationJSON(d, now))
	}
	writeJSON(w, http.StatusOK, result)
}

// createBackupDestination adds a destination: 201 with it.
func (h *handler) createBackupDestination(w http.ResponseWriter, r *http.Request) {
	var body destinationDraftJSON
	if !decode(w, r, &body) {
		return
	}
	d, err := h.Backups.CreateDestination(r.Context(), body.draft())
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newDestinationJSON(d, h.now()))
}

// updateBackupDestination changes a destination.
func (h *handler) updateBackupDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body destinationDraftJSON
	if !decode(w, r, &body) {
		return
	}
	d, err := h.Backups.UpdateDestination(r.Context(), id, body.draft())
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newDestinationJSON(d, h.now()))
}

// deleteBackupDestination removes a destination; the backups sent there
// stay: 204.
func (h *handler) deleteBackupDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Backups.DeleteDestination(r.Context(), id)
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// destinationTestJSON is what came of Test: whether it worked, why not,
// and the destination as it stands after, with the host key an SFTP
// server's first connection saved.
type destinationTestJSON struct {
	OK          bool            `json:"ok"`
	Error       string          `json:"error"`
	Destination destinationJSON `json:"destination"`
}

// testBackupDestination writes a small file to a destination, reads it
// back and deletes it.
func (h *handler) testBackupDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.Backups.TestDestination(r.Context(), id)
	if errors.Is(err, backup.ErrDestinationNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	result := destinationTestJSON{OK: err == nil, Destination: newDestinationJSON(d, h.now())}
	if err != nil {
		result.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, result)
}

// backupFileJSON is a backup that can be restored: Size is -1 when its
// destination, a WebDAV server, does not tell it. Encrypted tells one
// encrypted with a passphrase, from its name.
type backupFileJSON struct {
	Name          string    `json:"name"`
	Size          int64     `json:"size"`
	MadeAt        time.Time `json:"madeAt"`
	BeforeRestore bool      `json:"beforeRestore"`
	Encrypted     bool      `json:"encrypted"`
}

func newBackupFilesJSON(files []backup.File) []backupFileJSON {
	result := make([]backupFileJSON, len(files))
	for i, f := range files {
		result[i] = backupFileJSON{Name: f.Name, Size: f.Size, MadeAt: f.MadeAt.UTC(), BeforeRestore: f.BeforeRestore, Encrypted: f.Encrypted}
	}
	return result
}

// backupFilesJSON lists the backups of the folder and of each destination,
// newest first; a destination that could not be read has its error.
type backupFilesJSON struct {
	Folder       []backupFileJSON       `json:"folder"`
	Destinations []destinationFilesJSON `json:"destinations"`
}

type destinationFilesJSON struct {
	Destination destinationJSON  `json:"destination"`
	Files       []backupFileJSON `json:"files"`
	Error       string           `json:"error"`
}

// backupFiles lists the backups that can be restored.
func (h *handler) backupFiles(w http.ResponseWriter, r *http.Request) {
	listing, err := h.Backups.Backups(r.Context())
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	result := backupFilesJSON{Folder: newBackupFilesJSON(listing.Folder), Destinations: []destinationFilesJSON{}}
	for _, d := range listing.Destinations {
		files := destinationFilesJSON{Destination: newDestinationJSON(d.Destination, now), Files: newBackupFilesJSON(d.Files)}
		if d.Error != nil {
			files.Error = d.Error.Error()
		}
		result.Destinations = append(result.Destinations, files)
	}
	writeJSON(w, http.StatusOK, result)
}

// restorePlanJSON is a backup prepared to be restored: what the
// confirmation shows. Unreadable counts, by kind, the stored secrets of
// the backup that POLYFIN_SECRET_KEY cannot decrypt: "settings",
// "tracking", "apps", "notifications", "shares" and "destinations".
// Encrypted tells an encrypted backup, Locked one whose passphrase is
// still needed: it is checked once unlocked, and Unreadable is empty
// until then.
type restorePlanJSON struct {
	ID          string         `json:"id"`
	File        string         `json:"file"`
	Destination *string        `json:"destination"`
	Uploaded    bool           `json:"uploaded"`
	MadeAt      *time.Time     `json:"madeAt"`
	Size        int64          `json:"size"`
	Unreadable  map[string]int `json:"unreadable"`
	Encrypted   bool           `json:"encrypted"`
	Locked      bool           `json:"locked"`
	ServerName  string         `json:"serverName"`
}

func (h *handler) newRestorePlanJSON(plan backup.Plan) restorePlanJSON {
	result := restorePlanJSON{ID: plan.ID, File: plan.File, Uploaded: plan.Uploaded, Size: plan.Size, Unreadable: plan.Unreadable,
		Encrypted: plan.Encrypted, Locked: plan.Locked, ServerName: h.Accounts.Settings().ServerName}
	if plan.MadeAt != nil {
		result.MadeAt = new(plan.MadeAt.UTC())
	}
	if plan.Destination != nil {
		result.Destination = new(plan.Destination.Name())
	}
	return result
}

// prepareRestore reads a backup of the folder, or of a destination, to
// restore it: the confirmation then shows what it found.
func (h *handler) prepareRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Destination *string `json:"destination"`
		File        string  `json:"file"`
	}
	if !decode(w, r, &body) {
		return
	}
	var destination *accounts.ID
	if body.Destination != nil {
		id, err := accounts.ParseID(*body.Destination)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		destination = &id
	}
	plan, err := h.Backups.Prepare(r.Context(), destination, body.File)
	h.answerPlan(w, r, plan, err)
}

// maxUpload bounds a backup an administrator sends.
const maxUpload = 64 << 30

// uploadRestore takes a backup an administrator sends as the request's
// body, named by the name query parameter, to restore it.
func (h *handler) uploadRestore(w http.ResponseWriter, r *http.Request) {
	plan, err := h.Backups.PrepareUpload(r.Context(), r.URL.Query().Get("name"), http.MaxBytesReader(w, r.Body, maxUpload))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	h.answerPlan(w, r, plan, err)
}

func (h *handler) answerPlan(w http.ResponseWriter, r *http.Request, plan backup.Plan, err error) {
	if backupError(w, err) {
		return
	}
	if err != nil {
		// A destination that cannot be reached: its own words.
		if r.Context().Err() == nil {
			h.Logger.Warn("A backup could not be prepared for a restore", "error", err)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "destination_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, h.newRestorePlanJSON(plan))
}

// discardRestore drops the backup prepared, when the restore dialog is
// closed without restoring it, with its file when it was downloaded or
// sent: 204, whether it was still prepared or not.
func (h *handler) discardRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	h.Backups.Discard(body.ID)
	w.WriteHeader(http.StatusNoContent)
}

// unlockRestore opens the encrypted backup prepared with the passphrase
// the administrator typed, then checks it: the plan, unlocked. A wrong
// passphrase leaves it prepared, to try another.
func (h *handler) unlockRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		Passphrase string `json:"passphrase"`
	}
	if !decode(w, r, &body) {
		return
	}
	plan, err := h.Backups.Unlock(r.Context(), body.ID, body.Passphrase)
	h.answerUnlocked(w, r, plan, err)
}

// answerUnlocked answers the plan Unlock gave.
func (h *handler) answerUnlocked(w http.ResponseWriter, r *http.Request, plan backup.Plan, err error) {
	if backupError(w, err) {
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			h.Logger.Warn("An encrypted backup could not be opened for a restore", "error", err)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unlock_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, h.newRestorePlanJSON(plan))
}

// downloadBackup sends a backup of the folder as it is, encrypted or not.
func (h *handler) downloadBackup(w http.ResponseWriter, r *http.Request) {
	f, file, err := h.Backups.OpenBackup(r.PathValue("name"))
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+file.Name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	// A download cut short resumes where it stopped.
	http.ServeContent(w, r, "", time.Time{}, f)
}

// decryptedDownload is a download of a backup decrypted, which
// checkBackupPassphrase allows once, for a minute, to the administrator
// who typed its passphrase.
type decryptedDownload struct {
	name, passphrase string
	user             accounts.ID
}

// decryptedDownloadLifetime is how long a download decrypted may start.
const decryptedDownloadLifetime = time.Minute

// checkBackupPassphrase checks that a passphrase opens an encrypted backup
// of the folder, and answers the ticket that downloads it decrypted, once,
// within a minute (see downloadDecryptedBackup): the browser then follows
// a plain link to it, and saves the dump as it comes, never holding it
// whole, while the passphrase never appears in an address.
func (h *handler) checkBackupPassphrase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := r.PathValue("name")
	dump, err := h.Backups.OpenDecrypted(name, body.Passphrase)
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	_ = dump.Close()
	ticket, now := randomToken(), h.now()
	h.decryptedDownloads.add(ticket, decryptedDownload{name: name, passphrase: body.Passphrase, user: sessionFrom(r.Context()).User.ID},
		now.Add(decryptedDownloadLifetime), now)
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket})
}

// downloadDecryptedBackup sends the dump an encrypted backup of the folder
// holds, for the ticket checkBackupPassphrase gave the same administrator.
// The download stops at the first part of the backup changed or missing,
// short of its length: the browser tells it failed.
func (h *handler) downloadDecryptedBackup(w http.ResponseWriter, r *http.Request) {
	download, ok := h.decryptedDownloads.take(r.URL.Query().Get("ticket"), h.now())
	if !ok || download.name != r.PathValue("name") || download.user != sessionFrom(r.Context()).User.ID {
		writeError(w, http.StatusNotFound, "download_expired")
		return
	}
	dump, err := h.Backups.OpenDecrypted(download.name, download.passphrase)
	if backupError(w, err) {
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	defer dump.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+dump.Name+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(dump.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, dump); err != nil {
		if r.Context().Err() == nil {
			h.Logger.Warn("A backup downloaded decrypted stopped", "file", dump.Name, "error", err)
		}
		// The connection is closed without ending the answer.
		panic(http.ErrAbortHandler)
	}
}

// startRestore restores the backup prepared, once the administrator typed
// the server's name: 202, then Polyfin stops, restores it, and starts
// again. A backup holding secrets the key cannot decrypt needs anyway.
func (h *handler) startRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		ServerName string `json:"serverName"`
		Anyway     bool   `json:"anyway"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.ServerName) != h.Accounts.Settings().ServerName {
		writeError(w, http.StatusBadRequest, "wrong_server_name")
		return
	}
	err := h.Backups.Start(r.Context(), body.ID, body.Anyway, sessionFrom(r.Context()).User.Name)
	if backupError(w, err) {
		return
	} else if err != nil {
		h.Logger.Warn("A restore could not start", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "restore_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"restoring": true})
}
