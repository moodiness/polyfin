package admin

import (
	"cmp"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
)

// The import of users and watch data from a Jellyfin, Emby or Plex server
// lives under the users: it creates accounts and fills each account's watch
// data. The API key, token and passwords are never stored nor sent back:
// the admin app sends them with each request.

// jellyfinError answers the errors of reading a server, and of asking for
// an import while one runs. It reports whether err was one.
func jellyfinError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{jellyfinimport.ErrInvalidAddress, http.StatusBadRequest, "invalid_jellyfin_address"},
		// Not 401, which the admin app reads as its own session ending.
		{jellyfinimport.ErrKeyRefused, http.StatusBadRequest, "jellyfin_key_refused"},
		// The key or account is known but may not list the server's users.
		{jellyfinimport.ErrForbidden, http.StatusBadRequest, "jellyfin_key_limited"},
		{jellyfinimport.ErrNotKeyOwner, http.StatusBadRequest, "jellyfin_key_owner_only"},
		{jellyfinimport.ErrUserKey, http.StatusBadRequest, "emby_user_key"},
		{jellyfinimport.ErrSignInRefused, http.StatusBadRequest, "jellyfin_sign_in_refused"},
		{jellyfinimport.ErrSignInForbidden, http.StatusBadRequest, "jellyfin_sign_in_forbidden"},
		{jellyfinimport.ErrUnreachable, http.StatusBadGateway, "jellyfin_unreachable"},
		{jellyfinimport.ErrNotJellyfin, http.StatusBadGateway, "not_jellyfin"},
		{jellyfinimport.ErrRunning, http.StatusConflict, "jellyfin_import_running"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// importKind reads the kind of server a request names, Jellyfin when it
// names none, and reports false for an unknown one.
func importKind(kind string) (jellyfinimport.Kind, bool) {
	switch k := jellyfinimport.Kind(kind); k {
	case "":
		return jellyfinimport.Jellyfin, true
	case jellyfinimport.Jellyfin, jellyfinimport.Emby, jellyfinimport.Plex:
		return k, true
	}
	return "", false
}

// jellyfinConnectionJSON is the server an administrator connects to, of
// Kind, with an API key or a user's name and password: one of the two.
// Plex takes its owner's token as the API key, and no account.
type jellyfinConnectionJSON struct {
	Kind    string `json:"kind"`
	Address string `json:"address"`
	APIKey  string `json:"apiKey"`
	Account *struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	} `json:"account"`
}

// connection reads the server, and the key or the account given, and
// reports false unless exactly one is, or for an unknown kind. A password
// may be empty: an account may have none.
func (c jellyfinConnectionJSON) connection() (jellyfinimport.Connection, bool) {
	kind, ok := importKind(c.Kind)
	key := strings.TrimSpace(c.APIKey)
	connection := jellyfinimport.Connection{Kind: kind, Address: c.Address, Credentials: jellyfinimport.Credentials{Key: key}}
	if c.Account == nil {
		return connection, ok && key != ""
	}
	name := strings.TrimSpace(c.Account.Name)
	connection.Credentials = jellyfinimport.Credentials{Name: name, Password: c.Account.Password}
	return connection, ok && kind != jellyfinimport.Plex && key == "" && name != ""
}

type jellyfinServerJSON struct {
	Kind    jellyfinimport.Kind `json:"kind"`
	Name    string              `json:"name"`
	Version string              `json:"version,omitempty"`
	Address string              `json:"address"`
}

// jellyfinUserJSON is a user of the Jellyfin server; UserID is the Polyfin
// user of the same name, whatever its case, which the admin app suggests
// importing them as, null without one.
type jellyfinUserJSON struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	IsAdministrator bool       `json:"isAdministrator"`
	IsDisabled      bool       `json:"isDisabled"`
	IsHidden        bool       `json:"isHidden"`
	LastActivityAt  *time.Time `json:"lastActivityAt"`
	UserID          *string    `json:"userId"`
}

// jellyfinImportUsers lists the users of a server.
func (h *handler) jellyfinImportUsers(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body jellyfinConnectionJSON
	if !decode(w, r, &body) {
		return
	}
	connection, ok := body.connection()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	server, users, err := h.JellyfinImport.Users(r.Context(), connection)
	if jellyfinError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	own, err := h.Accounts.Users(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	byName := make(map[string]string, len(own))
	for _, user := range own {
		byName[strings.ToLower(user.Name)] = user.ID.String()
	}
	// KeyOwner is the user the key belongs to, null for an API key of the
	// server or Plex's owner's token: a user's key imports only its owner's
	// watch data.
	result := struct {
		Server   jellyfinServerJSON `json:"server"`
		Users    []jellyfinUserJSON `json:"users"`
		KeyOwner *string            `json:"keyOwner"`
	}{Server: jellyfinServerJSON{Kind: server.Kind, Name: server.Name, Version: server.Version, Address: server.Address},
		Users: make([]jellyfinUserJSON, 0, len(users)), KeyOwner: optional(server.KeyOwner)}
	for _, user := range users {
		listed := jellyfinUserJSON{ID: user.ID, Name: user.Name, IsAdministrator: user.Administrator, IsDisabled: user.Disabled,
			IsHidden: user.Hidden, LastActivityAt: utcSeconds(user.LastActivity)}
		if id, ok := byName[strings.ToLower(strings.TrimSpace(user.Name))]; ok {
			listed.UserID = &id
		}
		result.Users = append(result.Users, listed)
	}
	writeJSON(w, http.StatusOK, result)
}

// jellyfinImportEntryJSON maps a Jellyfin user to an existing Polyfin user
// (UserID) or a new one (Create), and tells whether their watch data is
// imported. JellyfinPassword, the user's password on the server, has the
// import sign in as them to read their watch data, which a user's key or
// account cannot read of another user. PlexPIN is a Plex Home profile's
// PIN, which an import the owner signed in to Plex for switches to it
// with.
type jellyfinImportEntryJSON struct {
	JellyfinID string  `json:"jellyfinId"`
	UserID     *string `json:"userId"`
	Create     *struct {
		Name            string `json:"name"`
		Password        string `json:"password"`
		IsAdministrator bool   `json:"isAdministrator"`
		// IsHidden hides the account from the sign-in screen; left out, it
		// is hidden, as created accounts are.
		IsHidden *bool `json:"isHidden"`
	} `json:"create"`
	WatchData        bool    `json:"watchData"`
	JellyfinPassword *string `json:"jellyfinPassword"`
	PlexPIN          *string `json:"plexPin"`
}

// jellyfinEntryError is an entry of an import that cannot be done: the
// answer names its Jellyfin user.
type jellyfinEntryError struct {
	status     int
	code       string
	jellyfinID string
}

func (e *jellyfinEntryError) Error() string {
	return e.code
}

// startJellyfinImport creates the Polyfin users chosen, all or none, then
// imports the watch data of the server's users asked in the background.
// With plexOwner, the administrator signed in to Plex as the owner of
// the Plex server, and each account is read with the token Plex gives
// the owner for it.
func (h *handler) startJellyfinImport(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		jellyfinConnectionJSON
		extrasJSON
		PlexOwner bool                      `json:"plexOwner"`
		Users     []jellyfinImportEntryJSON `json:"users"`
	}
	if !decode(w, r, &body) {
		return
	}
	connection, ok := body.connection()
	plex := connection.Kind == jellyfinimport.Plex
	if !ok || body.PlexOwner && !plex {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	connection.PlexOwner, connection.Extras = body.PlexOwner, body.extras()
	seen := map[string]bool{}
	for _, entry := range body.Users {
		// Plex has no sign-in: its owner's token reads every account, and
		// the owner's sign-in to Plex the tokens of the others.
		if entry.JellyfinID == "" || seen[entry.JellyfinID] || (entry.UserID == nil) == (entry.Create == nil) ||
			entry.JellyfinPassword != nil && (!entry.WatchData || plex) ||
			entry.PlexPIN != nil && (!entry.WatchData || !body.PlexOwner || len(*entry.PlexPIN) > 16) {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		seen[entry.JellyfinID] = true
	}
	var created []accounts.User
	status, err := h.JellyfinImport.Start(r.Context(), connection, sessionFrom(r.Context()).User,
		func(server jellyfinimport.Server, users []jellyfinimport.User, signIn jellyfinimport.SignIn) ([]jellyfinimport.Target, error) {
			listed := make(map[string]bool, len(users))
			for _, user := range users {
				listed[user.ID] = true
			}
			// chosen are the targets of the entries, in their order, which
			// the import follows.
			chosen := make([]*jellyfinimport.Target, len(body.Users))
			var fresh []accounts.NewUser
			// freshAt are the entries of the users created, in order.
			var freshAt []int
			for i, entry := range body.Users {
				if !listed[entry.JellyfinID] {
					return nil, &jellyfinEntryError{http.StatusBadRequest, "unknown_jellyfin_user", entry.JellyfinID}
				}
				if entry.WatchData && server.KeyOwner != "" && entry.JellyfinID != server.KeyOwner && entry.JellyfinPassword == nil {
					// A user's key reads that user's watch data only.
					return nil, &jellyfinEntryError{http.StatusBadRequest, "jellyfin_key_owner_only", entry.JellyfinID}
				}
				if entry.Create != nil {
					hidden := entry.Create.IsHidden == nil || *entry.Create.IsHidden
					fresh = append(fresh, accounts.NewUser{Name: entry.Create.Name, Password: entry.Create.Password,
						IsAdministrator: entry.Create.IsAdministrator, IsHidden: hidden})
					freshAt = append(freshAt, i)
					continue
				}
				if !entry.WatchData {
					continue
				}
				id, err := accounts.ParseID(*entry.UserID)
				if err != nil {
					return nil, &jellyfinEntryError{http.StatusNotFound, "not_found", entry.JellyfinID}
				}
				user, err := h.Accounts.User(r.Context(), id)
				if errors.Is(err, accounts.ErrNotFound) {
					return nil, &jellyfinEntryError{http.StatusNotFound, "not_found", entry.JellyfinID}
				}
				if err != nil {
					return nil, err
				}
				chosen[i] = &jellyfinimport.Target{JellyfinID: entry.JellyfinID, User: user.ID, UserName: user.Name}
			}
			// The users read as themselves sign in before anyone is created:
			// a wrong password creates no one. With the owner signed in to
			// Plex, each account read takes its own token, a Plex Home
			// profile's with its PIN.
			for _, entry := range body.Users {
				password := entry.JellyfinPassword
				if body.PlexOwner && entry.WatchData {
					password = cmp.Or(entry.PlexPIN, new(""))
				}
				if password == nil {
					continue
				}
				err := signIn(entry.JellyfinID, *password)
				for _, known := range []struct {
					err  error
					code string
				}{
					{jellyfinimport.ErrSignInRefused, "jellyfin_password_refused"},
					{jellyfinimport.ErrSignInForbidden, "jellyfin_sign_in_forbidden"},
					{jellyfinimport.ErrOtherUser, "jellyfin_other_user"},
					{jellyfinimport.ErrPlexProfilePIN, "plex_profile_pin_refused"},
				} {
					if errors.Is(err, known.err) {
						return nil, &jellyfinEntryError{http.StatusBadRequest, known.code, entry.JellyfinID}
					}
				}
				if plex && errors.Is(err, jellyfinimport.ErrForbidden) {
					// Plex does not let the owner switch to the profile.
					return nil, &jellyfinEntryError{http.StatusBadRequest, "plex_profile_forbidden", entry.JellyfinID}
				}
				if err != nil {
					return nil, err
				}
			}
			var err error
			created, err = h.Accounts.CreateUsers(r.Context(), fresh)
			if failed, ok := errors.AsType[*accounts.UserError](err); ok {
				for _, known := range []struct {
					err    error
					status int
					code   string
				}{
					{accounts.ErrInvalidName, http.StatusBadRequest, "invalid_name"},
					{accounts.ErrInvalidPassword, http.StatusBadRequest, "invalid_password"},
					{accounts.ErrNameTaken, http.StatusConflict, "name_taken"},
				} {
					if errors.Is(failed.Err, known.err) {
						return nil, &jellyfinEntryError{known.status, known.code, body.Users[freshAt[failed.Index]].JellyfinID}
					}
				}
			}
			if err != nil {
				return nil, err
			}
			for j, user := range created {
				if entry := body.Users[freshAt[j]]; entry.WatchData {
					chosen[freshAt[j]] = &jellyfinimport.Target{JellyfinID: entry.JellyfinID, User: user.ID, UserName: user.Name}
				}
			}
			var targets []jellyfinimport.Target
			for _, target := range chosen {
				if target != nil {
					targets = append(targets, *target)
				}
			}
			return targets, nil
		})
	if failed, ok := errors.AsType[*jellyfinEntryError](err); ok {
		writeJSON(w, failed.status, map[string]string{"error": failed.code, "jellyfinId": failed.jellyfinID})
		return
	}
	// The users were created even when the import could not start.
	for _, user := range created {
		h.Activity.UserCreated(r.Context(), user)
	}
	if plexOwnerError(w, err) || jellyfinError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Created []userJSON          `json:"created"`
		Import  *jellyfinImportJSON `json:"import"`
	}{Created: make([]userJSON, 0, len(created)), Import: newJellyfinImportJSON(status)}
	for _, user := range created {
		result.Created = append(result.Created, newUserJSON(user))
	}
	writeJSON(w, http.StatusOK, result)
}

// jellyfinImport answers the import running, else the last one.
func (h *handler) jellyfinImport(w http.ResponseWriter, _ *http.Request) {
	if h.JellyfinImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Import *jellyfinImportJSON `json:"import"`
	}{newJellyfinImportJSON(h.JellyfinImport.Current())})
}

// stopJellyfinImport stops the import running; what it imported stays.
func (h *handler) stopJellyfinImport(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport != nil {
		h.JellyfinImport.Stop()
	}
	h.jellyfinImport(w, r)
}

// jellyfinImportJSON is how an import goes, or went. StartedBy is the
// Polyfin user who started it, and Own tells an import of their own watch
// data, made under My account. Extras are what it brings besides watch
// data: a user's own import brings no collections for a user who may not
// manage them.
type jellyfinImportJSON struct {
	ID     string `json:"id"`
	Server struct {
		Kind    jellyfinimport.Kind `json:"kind"`
		Name    string              `json:"name"`
		Address string              `json:"address"`
	} `json:"server"`
	StartedBy struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"startedBy"`
	Own    bool `json:"own"`
	Extras struct {
		Ratings     bool `json:"ratings"`
		Collections bool `json:"collections"`
		Playlists   bool `json:"playlists"`
	} `json:"extras"`
	State     string                   `json:"state"`
	Problem   *string                  `json:"problem"`
	StartedAt time.Time                `json:"startedAt"`
	EndedAt   *time.Time               `json:"endedAt"`
	Users     []jellyfinUserImportJSON `json:"users"`
}

// extrasJSON are what an import asked brings besides watch data, each by
// default.
type extrasJSON struct {
	Ratings     *bool `json:"ratings"`
	Collections *bool `json:"collections"`
	Playlists   *bool `json:"playlists"`
}

func (e extrasJSON) extras() jellyfinimport.Extras {
	on := func(asked *bool) bool { return asked == nil || *asked }
	return jellyfinimport.Extras{Ratings: on(e.Ratings), Collections: on(e.Collections), Playlists: on(e.Playlists)}
}

type jellyfinUserImportJSON struct {
	JellyfinID   string `json:"jellyfinId"`
	JellyfinName string `json:"jellyfinName"`
	UserID       string `json:"userId"`
	UserName     string `json:"userName"`
	State        string `json:"state"`
	Read         int    `json:"read"`
	// Sections, Listed and History tell what was read of a Plex server,
	// null for the other kinds.
	Sections       *int                    `json:"sections"`
	Listed         *int                    `json:"listed"`
	History        *int                    `json:"history"`
	Matched        int                     `json:"matched"`
	Played         int                     `json:"played"`
	Resumed        int                     `json:"resumed"`
	Favorites      int                     `json:"favorites"`
	Ratings        int                     `json:"ratings"`
	Watchlist      int                     `json:"watchlist"`
	Collections    int                     `json:"collections"`
	Playlists      int                     `json:"playlists"`
	ListTitles     int                     `json:"listTitles"`
	UpToDate       int                     `json:"upToDate"`
	Undated        int                     `json:"undated"`
	UnmatchedCount int                     `json:"unmatchedCount"`
	Unmatched      []jellyfinUnmatchedJSON `json:"unmatched"`
	Problem        *string                 `json:"problem"`
	NothingAdded   *string                 `json:"nothingAdded"`
	// Skipped are the collections and playlists no title of matched,
	// which were not created.
	Skipped []jellyfinSkippedJSON `json:"skipped"`
}

// jellyfinSkippedJSON is a collection or a playlist of the server that an
// import did not create; Kind is collection or playlist.
type jellyfinSkippedJSON struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// jellyfinUnmatchedJSON is a title no Polyfin title matched.
type jellyfinUnmatchedJSON struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Year    *int    `json:"year"`
	Series  *string `json:"series"`
	Season  *int    `json:"season"`
	Episode *int    `json:"episode"`
	Reason  string  `json:"reason"`
}

func newJellyfinImportJSON(status *jellyfinimport.Status) *jellyfinImportJSON {
	if status == nil {
		return nil
	}
	result := &jellyfinImportJSON{ID: status.ID, Own: status.Own, State: status.State, Problem: optional(status.Problem),
		StartedAt: status.StartedAt.UTC().Truncate(time.Second), EndedAt: utcSeconds(status.EndedAt),
		Users: make([]jellyfinUserImportJSON, 0, len(status.Users))}
	result.Server.Kind, result.Server.Name, result.Server.Address = status.Kind, status.ServerName, status.Address
	result.StartedBy.ID, result.StartedBy.Name = status.StartedBy.String(), status.StartedByName
	result.Extras.Ratings, result.Extras.Collections, result.Extras.Playlists = status.Extras.Ratings, status.Extras.Collections,
		status.Extras.Playlists
	for _, u := range status.Users {
		user := jellyfinUserImportJSON{JellyfinID: u.JellyfinID, JellyfinName: u.JellyfinName, UserID: u.User.String(), UserName: u.UserName,
			State: u.State, Read: u.Read, Matched: u.Matched, Played: u.Played, Resumed: u.Resumed, Favorites: u.Favorites,
			Ratings: u.Ratings, Watchlist: u.Watchlist, Collections: u.Collections, Playlists: u.Playlists, ListTitles: u.ListTitles,
			UpToDate: u.UpToDate, Undated: u.Undated, UnmatchedCount: u.UnmatchedCount, Unmatched: make([]jellyfinUnmatchedJSON, 0, len(u.Unmatched)),
			Problem: optional(u.Problem), NothingAdded: optional(u.NothingAdded), Skipped: make([]jellyfinSkippedJSON, 0, len(u.Skipped))}
		for _, list := range u.Skipped {
			user.Skipped = append(user.Skipped, jellyfinSkippedJSON{Kind: list.Kind, Name: list.Name})
		}
		if status.Kind == jellyfinimport.Plex {
			user.Sections, user.Listed, user.History = &u.Sections, &u.Listed, &u.History
		}
		for _, title := range u.Unmatched {
			entry := jellyfinUnmatchedJSON{Name: title.Name, Type: title.Kind, Series: optional(title.Series), Season: title.Season,
				Episode: title.Episode, Reason: title.Reason}
			if title.Year > 0 {
				entry.Year = &title.Year
			}
			user.Unmatched = append(user.Unmatched, entry)
		}
		result.Users = append(result.Users, user)
	}
	return result
}

// optional is text, nil when empty.
func optional(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}
