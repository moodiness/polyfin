package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/nuvio"
	stremioimport "github.com/moodiness/polyfin/internal/appimport/stremio"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A user imports the watch data of their account on an app built on
// addons, under My account, when the setting that lets users import their
// watch history allows it; an administrator, into a user's account, from
// that user's page. Each app signs in its own way and answers the account
// read, which the server holds a few minutes while the user chooses the
// profile to import and its addons; the start is the same for every app.
// Passwords and the auth keys apps give are used for the sign-in only:
// never stored, never answered back. The addresses of the addons are
// answered redacted.

// appImportTarget is the account an app import goes into: the caller's, or
// the user the path names for an administrator's routes.
type appImportTarget func(w http.ResponseWriter, r *http.Request) (accounts.User, bool)

// ownAppImportTarget is the caller's account, while the setting lets users
// import their watch history.
func (h *handler) ownAppImportTarget(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	if h.AppImport == nil || !h.Accounts.Settings().ServerImports {
		writeError(w, http.StatusForbidden, "server_imports_disabled")
		return accounts.User{}, false
	}
	return sessionFrom(r.Context()).User, true
}

// userAppImportTarget is the account of the user the path names.
func (h *handler) userAppImportTarget(w http.ResponseWriter, r *http.Request) (accounts.User, bool) {
	if h.AppImport == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return accounts.User{}, false
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return accounts.User{}, false
	}
	user, err := h.Accounts.User(r.Context(), id)
	if accountError(w, err) {
		return accounts.User{}, false
	}
	if err != nil {
		h.internalError(w, r, err)
		return accounts.User{}, false
	}
	return user, true
}

// ownAppImport answers whether the user may import from an app, whether
// they may add their own addons, and the import into their account
// running, else the last one, null when none.
func (h *handler) ownAppImport(w http.ResponseWriter, r *http.Request) {
	user := sessionFrom(r.Context()).User
	enabled := h.AppImport != nil && h.Accounts.Settings().ServerImports
	var current *appImportJSON
	if enabled {
		current = newAppImportJSON(h.AppImport.Current(user.ID))
	}
	writeJSON(w, http.StatusOK, struct {
		Enabled       bool           `json:"enabled"`
		AddonsAllowed bool           `json:"addonsAllowed"`
		Import        *appImportJSON `json:"import"`
	}{enabled, h.Accounts.Settings().PersonalAddonsAllowed(user), current})
}

// userAppImport answers, to an administrator, whether the user the path
// names may add their own addons, and the import into their account
// running, else the last one.
func (h *handler) userAppImport(w http.ResponseWriter, r *http.Request) {
	user, ok := h.userAppImportTarget(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		AddonsAllowed bool           `json:"addonsAllowed"`
		Import        *appImportJSON `json:"import"`
	}{h.Accounts.Settings().PersonalAddonsAllowed(user), newAppImportJSON(h.AppImport.Current(user.ID))})
}

// readNuvio signs in to the Nuvio backend given with the email and
// password given, reads the account, signs out, and answers its profiles,
// which the server holds for the caller under the key it answers.
func (h *handler) readNuvio(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := target(w, r); !ok {
			return
		}
		if h.Nuvio == nil {
			writeError(w, http.StatusNotFound, "not_found")
			return
		}
		var body struct {
			Backend  string `json:"backend"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		account, err := h.Nuvio.Read(r.Context(), body.Backend, body.Email, body.Password)
		var unsupported *nuvio.BackupError
		if errors.As(err, &unsupported) {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "nuvio_backup_unsupported", "version": unsupported.Version})
			return
		}
		for _, known := range []struct {
			err    error
			status int
			code   string
		}{
			{nuvio.ErrInvalidAddress, http.StatusBadRequest, "invalid_nuvio_address"},
			// Not 401, which the admin app reads as its own session ending.
			{nuvio.ErrSignInRefused, http.StatusBadRequest, "nuvio_sign_in_refused"},
			{nuvio.ErrUnreachable, http.StatusBadGateway, "nuvio_unreachable"},
			{nuvio.ErrNotNuvio, http.StatusBadGateway, "not_nuvio"},
			{nuvio.ErrRateLimited, http.StatusServiceUnavailable, "nuvio_rate_limited"},
		} {
			if errors.Is(err, known.err) {
				writeError(w, known.status, known.code)
				return
			}
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		key := h.AppImport.Hold(sessionFrom(r.Context()).User.ID, account)
		writeJSON(w, http.StatusOK, newAppAccountJSON(key, account))
	}
}

// stremioReader reports whether the target's account may be read from
// Stremio, and answers why not.
func (h *handler) stremioReader(target appImportTarget, w http.ResponseWriter, r *http.Request) bool {
	if _, ok := target(w, r); !ok {
		return false
	}
	if h.Stremio == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return false
	}
	return true
}

// startStremioLink asks Stremio for a link code, which the server waits
// for on behalf of the caller, and answers it with the address the user
// approves it at: never the auth key the approval gives.
func (h *handler) startStremioLink(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.stremioReader(target, w, r) {
			return
		}
		link, err := h.Stremio.StartLink(r.Context(), sessionFrom(r.Context()).User.ID)
		if h.stremioError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Code string `json:"code"`
			Link string `json:"link"`
		}{link.Code, link.URL})
	}
}

// pollStremioLink asks Stremio whether the caller's link code is
// approved: until it is, it answers 202 and pending; once it is, it reads
// the account, signs out, and answers its profile, which the server holds
// for the caller under the key it answers.
func (h *handler) pollStremioLink(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.stremioReader(target, w, r) {
			return
		}
		account, err := h.Stremio.PollLink(r.Context(), sessionFrom(r.Context()).User.ID)
		if errors.Is(err, stremioimport.ErrLinkPending) {
			writeJSON(w, http.StatusAccepted, struct {
				Pending bool `json:"pending"`
			}{true})
			return
		}
		h.holdStremio(w, r, account, err)
	}
}

// readStremioPassword signs in to Stremio with the email and password
// given, reads the account, signs out, and answers it held, as
// pollStremioLink does.
func (h *handler) readStremioPassword(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.stremioReader(target, w, r) {
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		account, err := h.Stremio.SignIn(r.Context(), body.Email, body.Password)
		h.holdStremio(w, r, account, err)
	}
}

// readStremioKey reads the account with the auth key the user pasted,
// without signing it out, and answers it held, as pollStremioLink does.
func (h *handler) readStremioKey(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.stremioReader(target, w, r) {
			return
		}
		var body struct {
			Key string `json:"key"`
		}
		if !decode(w, r, &body) {
			return
		}
		account, err := h.Stremio.ReadKey(r.Context(), body.Key)
		h.holdStremio(w, r, account, err)
	}
}

// holdStremio holds the Stremio account read for the caller, and answers
// it, or answers why reading it failed.
func (h *handler) holdStremio(w http.ResponseWriter, r *http.Request, account appimport.Account, err error) {
	if h.stremioError(w, r, err) {
		return
	}
	key := h.AppImport.Hold(sessionFrom(r.Context()).User.ID, account)
	writeJSON(w, http.StatusOK, newAppAccountJSON(key, account))
}

// stremioError answers err, a way reading a Stremio account failed, and
// reports whether there was one.
func (h *handler) stremioError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		// Not 401, which the admin app reads as its own session ending.
		{stremioimport.ErrSignInRefused, http.StatusBadRequest, "stremio_sign_in_refused"},
		{stremioimport.ErrKeyRefused, http.StatusBadRequest, "stremio_key_refused"},
		{stremioimport.ErrLinkExpired, http.StatusConflict, "stremio_link_expired"},
		{stremioimport.ErrUnreachable, http.StatusBadGateway, "stremio_unreachable"},
		{stremioimport.ErrUnexpected, http.StatusBadGateway, "stremio_unexpected"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	h.internalError(w, r, err)
	return true
}

// startAppImport imports, in the background, the profile chosen of the
// account the caller holds, with the addons chosen, into the target's
// account.
func (h *handler) startAppImport(target appImportTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := target(w, r)
		if !ok {
			return
		}
		var body struct {
			Key     string `json:"key"`
			Profile string `json:"profile"`
			Addons  []int  `json:"addons"`
		}
		if !decode(w, r, &body) {
			return
		}
		status, err := h.AppImport.Start(sessionFrom(r.Context()).User, user, body.Key,
			appimport.Choice{Profile: body.Profile, Addons: body.Addons})
		for _, known := range []struct {
			err    error
			status int
			code   string
		}{
			{appimport.ErrExpired, http.StatusConflict, "app_import_expired"},
			{appimport.ErrUnknownProfile, http.StatusBadRequest, "invalid_request"},
			{appimport.ErrUnknownAddon, http.StatusBadRequest, "invalid_request"},
			{appimport.ErrAddonsNotAllowed, http.StatusForbidden, "personal_addons_disabled"},
			{appimport.ErrRunning, http.StatusConflict, "app_import_running"},
		} {
			if errors.Is(err, known.err) {
				writeError(w, known.status, known.code)
				return
			}
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Import *appImportJSON `json:"import"`
		}{newAppImportJSON(status)})
	}
}

// appAccountJSON is an account read: the key it is held under, and its
// profiles, each with how many titles it played, resumed and keeps in its
// library, and its addons, by their index, with their addresses redacted.
type appAccountJSON struct {
	Key      string           `json:"key"`
	App      string           `json:"app"`
	Profiles []appProfileJSON `json:"profiles"`
}

type appProfileJSON struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Entries int            `json:"entries"`
	Played  int            `json:"played"`
	Resumes int            `json:"resumes"`
	Library int            `json:"library"`
	Addons  []appAddonJSON `json:"addons"`
}

type appAddonJSON struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func newAppAccountJSON(key string, account appimport.Account) appAccountJSON {
	result := appAccountJSON{Key: key, App: account.App, Profiles: make([]appProfileJSON, 0, len(account.Profiles))}
	for _, p := range account.Profiles {
		profile := appProfileJSON{ID: p.ID, Name: p.Name, Entries: len(p.Entries), Addons: make([]appAddonJSON, 0, len(p.Addons))}
		for _, e := range p.Entries {
			if e.Played {
				profile.Played++
			}
			if e.Resume != nil {
				profile.Resumes++
			}
			if e.Favorite {
				profile.Library++
			}
		}
		for i, a := range p.Addons {
			profile.Addons = append(profile.Addons, appAddonJSON{Index: i, Name: a.Name, Address: stremio.RedactManifestURL(a.ManifestURL)})
		}
		result.Profiles = append(result.Profiles, profile)
	}
	return result
}

// appImportJSON is how an import from an app goes, or went.
type appImportJSON struct {
	ID      string `json:"id"`
	App     string `json:"app"`
	Profile string `json:"profile"`
	User    struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	StartedBy struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"startedBy"`
	State          string                  `json:"state"`
	Problem        *string                 `json:"problem"`
	StartedAt      time.Time               `json:"startedAt"`
	EndedAt        *time.Time              `json:"endedAt"`
	Read           int                     `json:"read"`
	Played         int                     `json:"played"`
	Resumed        int                     `json:"resumed"`
	Favorites      int                     `json:"favorites"`
	UnmatchedCount int                     `json:"unmatchedCount"`
	Unmatched      []appUnmatchedJSON      `json:"unmatched"`
	Addons         []appAddonInstalledJSON `json:"addons"`
}

// appUnmatchedJSON is a title of the app no Polyfin title matched: its
// identifier in the app, its name when the app gives one, and an
// episode's numbers.
type appUnmatchedJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Season  *int   `json:"season"`
	Episode *int   `json:"episode"`
	Reason  string `json:"reason"`
}

// appAddonInstalledJSON is an addon an import installs, and how it went.
type appAddonInstalledJSON struct {
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Installed bool    `json:"installed"`
	Problem   *string `json:"problem"`
}

func newAppImportJSON(status *appimport.Status) *appImportJSON {
	if status == nil {
		return nil
	}
	result := &appImportJSON{ID: status.ID, App: status.App, Profile: status.Profile, State: status.State, Problem: optional(status.Problem),
		StartedAt: status.StartedAt.UTC().Truncate(time.Second), EndedAt: utcSeconds(status.EndedAt), Read: status.Read,
		Played: status.Played, Resumed: status.Resumed, Favorites: status.Favorites, UnmatchedCount: status.UnmatchedCount,
		Unmatched: make([]appUnmatchedJSON, 0, len(status.Unmatched)), Addons: make([]appAddonInstalledJSON, 0, len(status.Addons))}
	result.User.ID, result.User.Name = status.User.String(), status.UserName
	result.StartedBy.ID, result.StartedBy.Name = status.StartedBy.String(), status.StartedByName
	for _, u := range status.Unmatched {
		entry := appUnmatchedJSON{ID: u.ID, Name: u.Name, Type: u.Kind, Reason: u.Reason}
		if u.Kind == "episode" {
			entry.Season, entry.Episode = &u.Season, &u.Episode
		}
		result.Unmatched = append(result.Unmatched, entry)
	}
	for _, a := range status.Addons {
		result.Addons = append(result.Addons, appAddonInstalledJSON{Name: a.Name, Address: a.Address, Installed: a.Installed,
			Problem: optional(a.Problem)})
	}
	return result
}
