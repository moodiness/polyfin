package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
)

// folderJSON describes a local folder: what it holds, its path or share
// address, the kind of share it is ("smb", "webdav", empty for a path in
// the container) with its user and whether a password is stored, never
// the password itself, how its last scan went and whether one is under
// way, its files, those matched to a title and the others, and the links
// made by hand.
type folderJSON struct {
	Kind        string     `json:"kind"`
	Path        string     `json:"path"`
	Share       string     `json:"share"`
	User        string     `json:"user"`
	PasswordSet bool       `json:"passwordSet"`
	CheckedAt   *time.Time `json:"checkedAt"`
	ScannedAt   *time.Time `json:"scannedAt"`
	Error       string     `json:"error"`
	Scanning    bool       `json:"scanning"`
	Files       int        `json:"files"`
	Matched     int        `json:"matched"`
	Unmatched   int        `json:"unmatched"`
	Links       []linkJSON `json:"links"`
}

type linkJSON struct {
	Unit   string `json:"unit"`
	ImdbID string `json:"imdbId"`
}

func newFolderJSON(f localfiles.Folder) *folderJSON {
	links := make([]linkJSON, 0, len(f.Links))
	for _, l := range f.Links {
		links = append(links, linkJSON{Unit: l.Unit, ImdbID: l.IMDb})
	}
	return &folderJSON{Kind: f.Kind, Path: f.Addon.ManifestURL, Share: f.Share, User: f.User, PasswordSet: f.PasswordSet,
		CheckedAt: f.CheckedAt, ScannedAt: f.ScannedAt, Error: f.Error, Scanning: f.Scanning, Files: f.Files, Matched: f.Matched,
		Unmatched: f.Unmatched, Links: links}
}

// folderScope answers the folder routes' scope: local folders are the
// server's alone, which administrators manage.
func (h *handler) folderScope(w http.ResponseWriter, r *http.Request) (addons.Scope, bool) {
	scope, ok := h.scope(w, r)
	if !ok {
		return scope, false
	}
	if scope.Owner != nil || h.Folders == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return scope, false
	}
	return scope, true
}

// folderRequest adds or changes a local folder: its name, its path in the
// container or a share's address, and, on adding, what it holds, "movies"
// or "shows"; a share's user and password. On a change, a password left
// out keeps the one stored, and an empty one removes it.
type folderRequest struct {
	Name     *string `json:"name"`
	Path     *string `json:"path"`
	Kind     string  `json:"kind"`
	User     *string `json:"user"`
	Password *string `json:"password"`
}

// addFolder adds a local folder to the server's addons and scans it in
// the background.
func (h *handler) addFolder(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	var body folderRequest
	if !decode(w, r, &body) {
		return
	}
	folder := localfiles.NewFolder{Kind: body.Kind}
	for _, field := range []struct {
		from *string
		to   *string
	}{{body.Name, &folder.Name}, {body.Path, &folder.Path}, {body.User, &folder.User}, {body.Password, &folder.Password}} {
		if field.from != nil {
			*field.to = *field.from
		}
	}
	ctx := context.WithoutCancel(r.Context())
	addon, err := h.Folders.Add(ctx, folder)
	if err == nil {
		h.Activity.AddonInstalled(ctx, sessionFrom(ctx).User, addon.Manifest.Name, addon.Manifest.Version, true)
	}
	h.answerAddon(w, r, scope, http.StatusCreated, addon, err)
}

// updateFolder changes a local folder's name, path or address, or a
// share's user or password; a new path or address, a new user or
// password, is scanned at once.
func (h *handler) updateFolder(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body folderRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Name == nil && body.Path == nil && body.User == nil && body.Password == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	addon, err := h.Folders.Update(context.WithoutCancel(r.Context()), id,
		localfiles.Changes{Name: body.Name, Path: body.Path, User: body.User, Password: body.Password})
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}

// startFolderScan scans a local folder in the background; the answer tells
// the scan under way.
func (h *handler) startFolderScan(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	h.scanFolder(w, r, scope, id)
}

func (h *handler) scanFolder(w http.ResponseWriter, r *http.Request, scope addons.Scope, id accounts.ID) {
	if _, err := h.Folders.Folder(r.Context(), id); err != nil {
		h.answerFailure(w, r, err)
		return
	}
	h.Folders.StartScan(id)
	addon, err := h.Addons.Find(r.Context(), id)
	h.answerAddon(w, r, scope, http.StatusAccepted, addon, err)
}

type unmatchedJSON struct {
	Path   string `json:"path"`
	Unit   string `json:"unit"`
	Title  string `json:"title"`
	Year   *int   `json:"year"`
	Size   int64  `json:"size"`
	Reason string `json:"reason"`
}

// folderUnmatched lists the files of a local folder no title was matched
// to, with why, and how many there are.
func (h *handler) folderUnmatched(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.folderScope(w, r); !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, total, err := h.Folders.Unmatched(r.Context(), id)
	if h.answerFailure(w, r, err) {
		return
	}
	result := struct {
		Total int             `json:"total"`
		Files []unmatchedJSON `json:"files"`
	}{Total: total, Files: make([]unmatchedJSON, 0, len(list))}
	for _, u := range list {
		file := unmatchedJSON{Path: u.Path, Unit: u.Unit, Title: u.Title, Size: u.Size, Reason: u.Reason}
		if u.Year > 0 {
			file.Year = new(u.Year)
		}
		result.Files = append(result.Files, file)
	}
	writeJSON(w, http.StatusOK, result)
}

// linkFolderFile links a file of a local folder, or a show's folder, to an
// IMDb identifier, for good.
func (h *handler) linkFolderFile(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Path   string `json:"path"`
		ImdbID string `json:"imdbId"`
	}
	if !decode(w, r, &body) {
		return
	}
	err := h.Folders.Link(context.WithoutCancel(r.Context()), id, body.Path, body.ImdbID)
	h.answerFolder(w, r, scope, id, err)
}

// unlinkFolderFile removes a link made by hand: the files are matched
// again by their name.
func (h *handler) unlinkFolderFile(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Folders.Unlink(context.WithoutCancel(r.Context()), id, r.URL.Query().Get("path"))
	h.answerFolder(w, r, scope, id, err)
}

// answerFolder answers a change of a folder with the folder.
func (h *handler) answerFolder(w http.ResponseWriter, r *http.Request, scope addons.Scope, id accounts.ID, err error) {
	if h.answerFailure(w, r, err) {
		return
	}
	addon, err := h.Addons.Find(r.Context(), id)
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}
