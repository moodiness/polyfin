package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
)

// maxTitleQuery bounds the text of a title search, in bytes.
const maxTitleQuery = 200

// folderJSON describes a local folder: what it holds, its path or share
// address, the kind of share it is ("smb", "webdav", empty for a path in
// the container) with its user and whether a password is stored, never
// the password itself, how its last scan went and whether one is under
// way, its files, those matched to a title, how, and the others, the
// links made by hand, the episodes given by hand to the titles of shows'
// discs, and whether its titles show the artwork beside their files. A
// music folder's files are its tracks, matched those with a title and an
// album, which its albums and artists count; a home videos folder's are
// its videos, each its own title, which datedBy counts by where their
// dates come from.
type folderJSON struct {
	Kind        string             `json:"kind"`
	Path        string             `json:"path"`
	Share       string             `json:"share"`
	User        string             `json:"user"`
	PasswordSet bool               `json:"passwordSet"`
	CheckedAt   *time.Time         `json:"checkedAt"`
	ScannedAt   *time.Time         `json:"scannedAt"`
	Error       string             `json:"error"`
	Scanning    bool               `json:"scanning"`
	Files       int                `json:"files"`
	Matched     int                `json:"matched"`
	MatchedBy   matchedByJSON      `json:"matchedBy"`
	Unmatched   int                `json:"unmatched"`
	Albums      int                `json:"albums"`
	Artists     int                `json:"artists"`
	DatedBy     datedByJSON        `json:"datedBy"`
	Links       []linkJSON         `json:"links"`
	Episodes    []givenEpisodeJSON `json:"episodes"`
	UseArtwork  bool               `json:"useArtwork"`
}

// matchedByJSON counts the matched files by how they were matched: by an
// identifier in their name, by their NFO file, by a search, or by a link.
type matchedByJSON struct {
	Name   int `json:"name"`
	NFO    int `json:"nfo"`
	Search int `json:"search"`
	Link   int `json:"link"`
}

// datedByJSON counts a home videos folder's videos by where their dates
// come from: their NFO file, their recording (the creation time their
// container tells) or their file's modification time.
type datedByJSON struct {
	NFO     int `json:"nfo"`
	Created int `json:"created"`
	File    int `json:"file"`
}

type linkJSON struct {
	Unit   string `json:"unit"`
	ImdbID string `json:"imdbId"`
}

// givenEpisodeJSON is the episodes given by hand to a title of a show's
// disc: its path, its disc's kind ("bluray", "dvd", "bluray_image" or
// "dvd_image", empty when the disc is gone), its disc's folder or image,
// its number, and its episodes, "S01E05" or "S01E05-E06".
type givenEpisodeJSON struct {
	Path     string `json:"path"`
	Disc     string `json:"disc"`
	DiscPath string `json:"discPath"`
	Number   int    `json:"number"`
	Episode  string `json:"episode"`
}

func newFolderJSON(f localfiles.Folder) *folderJSON {
	links := make([]linkJSON, 0, len(f.Links))
	for _, l := range f.Links {
		links = append(links, linkJSON{Unit: l.Unit, ImdbID: l.IMDb})
	}
	episodes := make([]givenEpisodeJSON, 0, len(f.Episodes))
	for _, e := range f.Episodes {
		episodes = append(episodes, givenEpisodeJSON{Path: e.Path, Disc: e.Kind, DiscPath: e.Disc, Number: e.Number, Episode: e.Episode})
	}
	return &folderJSON{Kind: f.Kind, Path: f.Addon.ManifestURL, Share: f.Share, User: f.User, PasswordSet: f.PasswordSet,
		CheckedAt: f.CheckedAt, ScannedAt: f.ScannedAt, Error: f.Error, Scanning: f.Scanning, Files: f.Files, Matched: f.Matched,
		MatchedBy: matchedByJSON{Name: f.MatchedBy.Name, NFO: f.MatchedBy.NFO, Search: f.MatchedBy.Search, Link: f.MatchedBy.Link},
		Unmatched: f.Unmatched, Albums: f.Albums, Artists: f.Artists, DatedBy: datedByJSON{NFO: f.Dated.NFO, Created: f.Dated.Created, File: f.Dated.File},
		Links: links, Episodes: episodes, UseArtwork: f.UseArtwork}
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
// container or a share's address, and, on adding, what it holds, "movies",
// "shows", "music" or "homevideos"; a share's user and password; whether
// its titles show the artwork beside their files, yes when left out on
// adding. On a change, a password left out keeps the one stored, and an
// empty one removes it.
type folderRequest struct {
	Name       *string `json:"name"`
	Path       *string `json:"path"`
	Kind       string  `json:"kind"`
	User       *string `json:"user"`
	Password   *string `json:"password"`
	UseArtwork *bool   `json:"useArtwork"`
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
	folder := localfiles.NewFolder{Kind: body.Kind, UseArtwork: body.UseArtwork}
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

// updateFolder changes a local folder's name, path or address, a share's
// user or password, or whether its titles show the artwork beside their
// files; a new path or address, a new user or password, is scanned at
// once.
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
	if body.Name == nil && body.Path == nil && body.User == nil && body.Password == nil && body.UseArtwork == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	addon, err := h.Folders.Update(context.WithoutCancel(r.Context()), id,
		localfiles.Changes{Name: body.Name, Path: body.Path, User: body.User, Password: body.Password, UseArtwork: body.UseArtwork})
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
	// Disc is the kind of the disc the file is read as, "bluray" or "dvd",
	// unit being the disc's folder, or a Blu-ray disc's title other than
	// its main one, "<folder>#<playlist>", or "bluray_image" or
	// "dvd_image" for a disc image, its own unit; null for another file.
	Disc *string `json:"disc"`
	// DiscTitle tells which title of a show's disc the file is, its unit
	// being the show's folder; null for another file.
	DiscTitle *discTitleJSON `json:"discTitle"`
}

// discTitleJSON is a title of a show's disc: its disc's folder or image,
// its number, how long it plays in seconds, null when unknown, its
// episodes and those Polyfin suggests, "S01E05" or "S01E05-E06", null for
// none.
type discTitleJSON struct {
	DiscPath  string  `json:"discPath"`
	Number    int     `json:"number"`
	Length    *int    `json:"length"`
	Episode   *string `json:"episode"`
	Suggested *string `json:"suggested"`
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
	text := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	for _, u := range list {
		file := unmatchedJSON{Path: u.Path, Unit: u.Unit, Title: u.Title, Size: u.Size, Reason: u.Reason, Disc: text(u.Disc)}
		if u.Year > 0 {
			file.Year = new(u.Year)
		}
		if t := u.DiscTitle; t != nil {
			file.DiscTitle = &discTitleJSON{DiscPath: t.Disc, Number: t.Number, Episode: text(t.Episode), Suggested: text(t.Suggested)}
			if t.Length > 0 {
				file.DiscTitle.Length = new(int(t.Length / time.Second))
			}
		}
		result.Files = append(result.Files, file)
	}
	writeJSON(w, http.StatusOK, result)
}

// titleJSON is a title a search found for a link: its IMDb identifier,
// name, year, kind ("movie" or "show"), and the item whose Primary image
// is its poster, null without one.
type titleJSON struct {
	ImdbID   string  `json:"imdbId"`
	Name     string  `json:"name"`
	Year     *int    `json:"year"`
	Kind     string  `json:"kind"`
	PosterID *string `json:"posterId"`
}

// searchFolderTitles searches the metadata addons for titles of a local
// folder's kind, for one of its units to be linked to: query is free text,
// year, optional, puts that year's titles first.
func (h *handler) searchFolderTitles(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.folderScope(w, r); !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	year := 0
	if text := r.URL.Query().Get("year"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 || parsed > 9999 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		year = parsed
	}
	if len(query) > maxTitleQuery {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	items, err := h.Folders.Search(r.Context(), id, query, year)
	if h.answerFailure(w, r, err) {
		return
	}
	result := struct {
		Titles []titleJSON `json:"titles"`
	}{Titles: make([]titleJSON, 0, len(items))}
	for _, item := range items {
		title := titleJSON{ImdbID: item.ProviderIDs["Imdb"], Name: item.Name, Kind: "movie"}
		if item.Kind == library.KindSeries {
			title.Kind = "show"
		}
		if item.ProductionYear > 0 {
			title.Year = new(item.ProductionYear)
		}
		if item.Images.Primary != "" {
			title.PosterID = new(item.ID.String())
		}
		result.Titles = append(result.Titles, title)
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

// setFolderEpisodes gives titles of shows' discs of a shows folder their
// episodes, {"episodes": [{"path", "episode"}]}, "S01E05" or
// "S01E05-E06", an empty episode removing those given.
func (h *handler) setFolderEpisodes(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.folderScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Episodes []struct {
			Path    string `json:"path"`
			Episode string `json:"episode"`
		} `json:"episodes"`
	}
	if !decode(w, r, &body) {
		return
	}
	changes := make([]localfiles.EpisodeChange, 0, len(body.Episodes))
	for _, e := range body.Episodes {
		changes = append(changes, localfiles.EpisodeChange{Path: e.Path, Episode: e.Episode})
	}
	err := h.Folders.SetEpisodes(context.WithoutCancel(r.Context()), id, changes)
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
