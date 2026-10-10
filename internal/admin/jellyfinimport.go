package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
)

// The import of users and watch data from a Jellyfin server lives under
// the users: it creates accounts and fills each account's watch data. The
// API key and passwords are never stored nor sent back: the admin app sends
// them with each request.

// jellyfinError answers the errors of reading a Jellyfin server, and of
// asking for an import while one runs. It reports whether err was one.
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

// jellyfinConnectionJSON is the server an administrator connects to, with
// an API key or a user's name and password: one of the two.
type jellyfinConnectionJSON struct {
	Address string `json:"address"`
	APIKey  string `json:"apiKey"`
	Account *struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	} `json:"account"`
}

// credentials reads the key or the account given, and reports false
// unless exactly one is. A password may be empty: an account may have none.
func (c jellyfinConnectionJSON) credentials() (jellyfinimport.Credentials, bool) {
	key := strings.TrimSpace(c.APIKey)
	if c.Account == nil {
		return jellyfinimport.Credentials{Key: key}, key != ""
	}
	name := strings.TrimSpace(c.Account.Name)
	return jellyfinimport.Credentials{Name: name, Password: c.Account.Password}, key == "" && name != ""
}

type jellyfinServerJSON struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Address string `json:"address"`
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

// jellyfinImportUsers lists the users of a Jellyfin server.
func (h *handler) jellyfinImportUsers(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body jellyfinConnectionJSON
	if !decode(w, r, &body) {
		return
	}
	credentials, ok := body.credentials()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	server, users, err := h.JellyfinImport.Users(r.Context(), body.Address, credentials)
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
	// KeyOwner is the Jellyfin user the key belongs to, null for an API key
	// of the server: a user's key imports only its owner's watch data.
	result := struct {
		Server   jellyfinServerJSON `json:"server"`
		Users    []jellyfinUserJSON `json:"users"`
		KeyOwner *string            `json:"keyOwner"`
	}{Server: jellyfinServerJSON{Name: server.Name, Version: server.Version, Address: server.Address}, Users: make([]jellyfinUserJSON, 0, len(users)),
		KeyOwner: optional(server.KeyOwner)}
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
// account cannot read of another user.
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
// imports the watch data of the Jellyfin users asked in the background.
func (h *handler) startJellyfinImport(w http.ResponseWriter, r *http.Request) {
	if h.JellyfinImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		jellyfinConnectionJSON
		Users []jellyfinImportEntryJSON `json:"users"`
	}
	if !decode(w, r, &body) {
		return
	}
	seen := map[string]bool{}
	for _, entry := range body.Users {
		if entry.JellyfinID == "" || seen[entry.JellyfinID] || (entry.UserID == nil) == (entry.Create == nil) ||
			entry.JellyfinPassword != nil && !entry.WatchData {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		seen[entry.JellyfinID] = true
	}
	credentials, ok := body.credentials()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var created []accounts.User
	status, err := h.JellyfinImport.Start(r.Context(), body.Address, credentials,
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
			// a wrong password creates no one.
			for _, entry := range body.Users {
				if entry.JellyfinPassword == nil {
					continue
				}
				err := signIn(entry.JellyfinID, *entry.JellyfinPassword)
				for _, known := range []struct {
					err  error
					code string
				}{
					{jellyfinimport.ErrSignInRefused, "jellyfin_password_refused"},
					{jellyfinimport.ErrSignInForbidden, "jellyfin_sign_in_forbidden"},
					{jellyfinimport.ErrOtherUser, "jellyfin_other_user"},
				} {
					if errors.Is(err, known.err) {
						return nil, &jellyfinEntryError{http.StatusBadRequest, known.code, entry.JellyfinID}
					}
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
	if jellyfinError(w, err) {
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

// jellyfinImportJSON is how an import goes, or went.
type jellyfinImportJSON struct {
	ID     string `json:"id"`
	Server struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"server"`
	State     string                   `json:"state"`
	Problem   *string                  `json:"problem"`
	StartedAt time.Time                `json:"startedAt"`
	EndedAt   *time.Time               `json:"endedAt"`
	Users     []jellyfinUserImportJSON `json:"users"`
}

type jellyfinUserImportJSON struct {
	JellyfinID     string                  `json:"jellyfinId"`
	JellyfinName   string                  `json:"jellyfinName"`
	UserID         string                  `json:"userId"`
	UserName       string                  `json:"userName"`
	State          string                  `json:"state"`
	Read           int                     `json:"read"`
	Played         int                     `json:"played"`
	Resumed        int                     `json:"resumed"`
	Favorites      int                     `json:"favorites"`
	UnmatchedCount int                     `json:"unmatchedCount"`
	Unmatched      []jellyfinUnmatchedJSON `json:"unmatched"`
	Problem        *string                 `json:"problem"`
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
	result := &jellyfinImportJSON{ID: status.ID, State: status.State, Problem: optional(status.Problem),
		StartedAt: status.StartedAt.UTC().Truncate(time.Second), EndedAt: utcSeconds(status.EndedAt),
		Users: make([]jellyfinUserImportJSON, 0, len(status.Users))}
	result.Server.Name, result.Server.Address = status.ServerName, status.Address
	for _, u := range status.Users {
		user := jellyfinUserImportJSON{JellyfinID: u.JellyfinID, JellyfinName: u.JellyfinName, UserID: u.User.String(), UserName: u.UserName,
			State: u.State, Read: u.Read, Played: u.Played, Resumed: u.Resumed, Favorites: u.Favorites, UnmatchedCount: u.UnmatchedCount,
			Unmatched: make([]jellyfinUnmatchedJSON, 0, len(u.Unmatched)), Problem: optional(u.Problem)}
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
