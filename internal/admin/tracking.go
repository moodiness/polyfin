package admin

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/trackers"
)

// trackingServiceJSON is how one of the signed-in user's tracking services
// stands. Tokens and keys are never sent. Connection is "code" (a code to
// enter on the service's site), "signin" (signing in there to allow
// Polyfin, Code then holding the page, without a user code), "pin"
// (allowing Polyfin there, then pasting the code it shows: AniList),
// "redirect" (allowing Polyfin there, which sends the browser back with a
// code, or pasting the address it sent it to: MyAnimeList) or "key" (an
// API key or user token); Music tells a service told the songs the user
// plays, which has no history to import.
type trackingServiceJSON struct {
	Service     string            `json:"service"`
	Connection  string            `json:"connection"`
	Music       bool              `json:"music"`
	Available   bool              `json:"available"`
	Connected   bool              `json:"connected"`
	Account     *string           `json:"account"`
	ConnectedAt *time.Time        `json:"connectedAt"`
	LastSentAt  *time.Time        `json:"lastSentAt"`
	Problem     *string           `json:"problem"`
	Code        *trackingCodeJSON `json:"code"`
	// ExpiresAt is when a token that cannot be refreshed expires
	// (AniList's), which stops the connection; null otherwise.
	ExpiresAt *time.Time `json:"expiresAt"`
	// ImportHistory tells that the service's watch history is imported;
	// Importing, that an import runs now; LastImport, how the last went;
	// NextImportAt, when the next runs, null while none is planned.
	ImportHistory bool                `json:"importHistory"`
	Importing     bool                `json:"importing"`
	LastImport    *trackingImportJSON `json:"lastImport"`
	NextImport    *time.Time          `json:"nextImportAt"`
	// Watchlist tells that the service has a watchlist Polyfin imports
	// (Trakt, Simkl, AniList, MyAnimeList), ImportWatchlist that it does.
	Watchlist       bool `json:"watchlist"`
	ImportWatchlist bool `json:"importWatchlist"`
	// Ratings tells that the service keeps ratings (AniList,
	// MyAnimeList), SyncRatings that the user's are synced with it.
	Ratings     bool `json:"ratings"`
	SyncRatings bool `json:"syncRatings"`
}

// trackingImportJSON is how the last import of a watch history and a
// watchlist went; Rated and RatingsSkipped count the titles it rated and
// the ratings of series spread over several entries skipped.
type trackingImportJSON struct {
	At             time.Time `json:"at"`
	Played         int       `json:"played"`
	Resumed        int       `json:"resumed"`
	Watchlist      int       `json:"watchlist"`
	Unmapped       int       `json:"unmapped"`
	Rated          int       `json:"rated"`
	RatingsSkipped int       `json:"ratingsSkipped"`
	Problem        *string   `json:"problem"`
}

// trackingCodeJSON is a code waiting for the user to enter it on the
// service's site.
type trackingCodeJSON struct {
	UserCode        string    `json:"userCode"`
	VerificationURL string    `json:"verificationUrl"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func newTrackingServiceJSON(status trackers.Status) trackingServiceJSON {
	result := trackingServiceJSON{Service: status.Service, Connection: "key", Music: status.Music, Available: status.Available,
		Connected: status.Connected, ConnectedAt: utcSeconds(status.ConnectedAt), LastSentAt: utcSeconds(status.LastSentAt)}
	switch {
	case status.ByCode:
		result.Connection = "code"
	case status.BySignIn:
		result.Connection = "signin"
	case status.ByPaste && status.Service == trackers.AniList:
		result.Connection = "pin"
	case status.ByPaste:
		result.Connection = "redirect"
	}
	result.ExpiresAt = utcSeconds(status.ExpiresAt)
	if status.Account != "" {
		result.Account = &status.Account
	}
	if status.Problem != "" {
		result.Problem = &status.Problem
	}
	if status.Code != nil {
		result.Code = &trackingCodeJSON{UserCode: status.Code.UserCode, VerificationURL: status.Code.VerificationURL,
			ExpiresAt: status.Code.ExpiresAt.UTC().Truncate(time.Second)}
	}
	result.ImportHistory, result.Importing, result.NextImport = status.ImportHistory, status.Importing, utcSeconds(status.NextImport)
	result.Watchlist, result.ImportWatchlist = status.Watchlist, status.ImportWatchlist
	result.Ratings, result.SyncRatings = status.Ratings, status.SyncRatings
	if last := status.LastImport; last != nil {
		result.LastImport = &trackingImportJSON{At: last.At.UTC().Truncate(time.Second), Played: last.Played, Resumed: last.Resumed,
			Watchlist: last.Watchlist, Unmapped: last.Unmapped, Rated: last.Rated, RatingsSkipped: last.RatingsSkipped}
		if last.Problem != "" {
			result.LastImport.Problem = &last.Problem
		}
	}
	return result
}

// utcSeconds is t in UTC to the second, as the admin app shows dates.
func utcSeconds(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return new(t.UTC().Truncate(time.Second))
}

// ownTracking lists the signed-in user's tracking services, with how often,
// in minutes, their watch histories are imported.
func (h *handler) ownTracking(w http.ResponseWriter, r *http.Request) {
	if h.Trackers == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	statuses, err := h.Trackers.Statuses(r.Context(), sessionFrom(r.Context()).User.ID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := struct {
		Services           []trackingServiceJSON `json:"services"`
		ImportEveryMinutes int                   `json:"importEveryMinutes"`
	}{Services: make([]trackingServiceJSON, 0, len(statuses)), ImportEveryMinutes: h.Accounts.Settings().ImportEveryMinutes}
	for _, status := range statuses {
		result.Services = append(result.Services, newTrackingServiceJSON(status))
	}
	writeJSON(w, http.StatusOK, result)
}

// connectTracking connects the signed-in user to a service: with the API
// key or user token the body holds, or by starting a code the user enters,
// or a sign-in, on its site.
func (h *handler) connectTracking(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	user := sessionFrom(r.Context()).User.ID
	var (
		status trackers.Status
		err    error
	)
	if trackers.ByCode(service) || trackers.BySignIn(service) || trackers.ByPaste(service) {
		status, err = h.Trackers.StartCode(r.Context(), user, service)
	} else {
		var body struct {
			Key string `json:"key"`
		}
		if !decode(w, r, &body) {
			return
		}
		status, err = h.Trackers.ConnectKey(r.Context(), user, service, body.Key)
	}
	h.connectAnswer(w, r, status, err)
}

// connectAnswer answers an attempt to connect a service.
func (h *handler) connectAnswer(w http.ResponseWriter, r *http.Request, status trackers.Status, err error) {
	switch {
	case errors.Is(err, trackers.ErrInvalidKey):
		writeError(w, http.StatusBadRequest, "invalid_key")
	case errors.Is(err, trackers.ErrInvalidCode):
		writeError(w, http.StatusBadRequest, "invalid_code")
	case errors.Is(err, trackers.ErrNotAvailable):
		writeError(w, http.StatusConflict, "not_available")
	case errors.Is(err, trackers.ErrAppRefused):
		writeError(w, http.StatusConflict, "app_refused")
	case errors.Is(err, trackers.ErrUnreachable):
		writeError(w, http.StatusBadGateway, "service_unreachable")
	case errors.Is(err, trackers.ErrUnknownService):
		writeError(w, http.StatusNotFound, "not_found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, newTrackingServiceJSON(status))
	}
}

// exchangeTrackingCode connects the signed-in user to AniList with the
// code it showed them, or to MyAnimeList with the address it sent their
// browser back to, or its code, once they allowed Polyfin there. 400
// invalid_code tells a code the service refused, or one for no
// connection started (or started too long ago).
func (h *handler) exchangeTrackingCode(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !trackers.ByPaste(service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	status, err := h.Trackers.Exchange(r.Context(), sessionFrom(r.Context()).User.ID, service, body.Code)
	h.connectAnswer(w, r, status, err)
}

// disconnectTracking forgets the signed-in user's connection to a service.
func (h *handler) disconnectTracking(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := h.Trackers.Disconnect(r.Context(), sessionFrom(r.Context()).User.ID, service); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setTrackingImport turns the import of the signed-in user's watch history
// or watchlist from a connected service, or the sync of their ratings with
// it, on or off; turned on, it imports at once. A body may change several.
func (h *handler) setTrackingImport(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		ImportHistory   *bool `json:"importHistory"`
		ImportWatchlist *bool `json:"importWatchlist"`
		SyncRatings     *bool `json:"syncRatings"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ImportHistory == nil && body.ImportWatchlist == nil && body.SyncRatings == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	user := sessionFrom(r.Context()).User.ID
	var status trackers.Status
	var err error
	if body.ImportHistory != nil {
		status, err = h.Trackers.SetImport(r.Context(), user, service, *body.ImportHistory)
	}
	if err == nil && body.ImportWatchlist != nil {
		status, err = h.Trackers.SetWatchlistImport(r.Context(), user, service, *body.ImportWatchlist)
	}
	if err == nil && body.SyncRatings != nil {
		status, err = h.Trackers.SetRatingsSync(r.Context(), user, service, *body.SyncRatings)
	}
	h.importAnswer(w, r, http.StatusOK, status, err)
}

// importTracking imports the signed-in user's watch history from a
// service at once.
func (h *handler) importTracking(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if h.Trackers == nil || !slices.Contains(trackers.Services, service) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	status, err := h.Trackers.ImportNow(r.Context(), sessionFrom(r.Context()).User.ID, service)
	h.importAnswer(w, r, http.StatusAccepted, status, err)
}

func (h *handler) importAnswer(w http.ResponseWriter, r *http.Request, success int, status trackers.Status, err error) {
	switch {
	case errors.Is(err, trackers.ErrUnknownService):
		// A music service, which has no history to import.
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, trackers.ErrNotConnected):
		writeError(w, http.StatusConflict, "not_connected")
	case errors.Is(err, trackers.ErrImportOff):
		writeError(w, http.StatusConflict, "import_off")
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, success, newTrackingServiceJSON(status))
	}
}
