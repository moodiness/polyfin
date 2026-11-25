package jellyfin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/logs"
)

// credentialParameters are the query parameters of the playback routes
// that stand for a user (see readCredentials and streamAccess): the log
// tells they were sent, never their values.
var credentialParameters = []string{"ApiKey", "api_key", "playSessionId", grantParameter}

// logAppRequest tells on the detailed log a playback request as the app
// sent it, so that what an app asks can be read without a capture of its
// traffic: the app and its version, the route with its values, the query
// with credentials' values left out, and body, the PlaybackInfoDto posted,
// device profile included, when there is one. Items and versions are named
// by their identifiers; no address, token or play session is.
func (h *Handler) logAppRequest(r *http.Request, body []byte) {
	if !h.Logger.Enabled(r.Context(), slog.LevelDebug) {
		return
	}
	c := readCredentials(r, true)
	route := logs.Route(r)
	attrs := []any{"app", truncate(c.Client), "app_version", truncate(c.Version), "route", route}
	for part := range strings.SplitSeq(route, "/") {
		if name, ok := strings.CutPrefix(part, "{"); ok {
			name = strings.TrimSuffix(name, "}")
			attrs = append(attrs, name, truncate(r.PathValue(name)))
		}
	}
	values := r.URL.Query()
	for name := range values {
		if slices.ContainsFunc(credentialParameters, func(parameter string) bool { return strings.EqualFold(parameter, name) }) {
			values[name] = []string{"sent"}
		}
	}
	attrs = append(attrs, "query", values.Encode())
	var compact bytes.Buffer
	if json.Compact(&compact, body) == nil {
		attrs = append(attrs, "body", compact.String())
	}
	h.Logger.Debug("An app sent a playback request", attrs...)
}
