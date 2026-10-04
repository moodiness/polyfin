package admin

import (
	"net/http"
	"strconv"

	"github.com/moodiness/polyfin/internal/config"
)

// The log lines one request returns, by default and at most.
const (
	defaultLogLines = 500
	maxLogLines     = 5000
)

// logLines returns the log lines written after the first `after`, redacted
// as they were kept, the newest when there are more than `limit`, with the
// `after` of the next request.
func (h *handler) logLines(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var after uint64
	if raw := query.Get("after"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		after = value
	}
	limit := defaultLogLines
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxLogLines {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = value
	}
	lines, next := []string{}, uint64(0)
	if h.Logs != nil {
		lines, next = h.Logs.Since(after, limit)
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "next": next})
}

// downloadLog answers every log line kept, as a text file.
func (h *handler) downloadLog(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="polyfin.log"`)
	w.Header().Set("Cache-Control", "no-store")
	if h.Logs != nil {
		_, _ = w.Write(h.Logs.Contents())
	}
}

type variableJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Set tells whether the environment sets it; Value is the default
	// otherwise. Hidden marks a secret, whose value is not shown.
	Set    bool `json:"set"`
	Hidden bool `json:"hidden"`
	Known  bool `json:"known"`
}

// variables lists the POLYFIN_ environment variables in effect, without
// secrets (see config.Variables).
func (h *handler) variables(w http.ResponseWriter, _ *http.Request) {
	result := make([]variableJSON, 0, len(h.Variables))
	for _, v := range h.Variables {
		result = append(result, newVariableJSON(v))
	}
	writeJSON(w, http.StatusOK, result)
}

func newVariableJSON(v config.Variable) variableJSON {
	return variableJSON{Name: v.Name, Value: v.Value, Set: v.Set, Hidden: v.Hidden, Known: v.Known}
}
