package admin

import (
	"net/http"

	"github.com/moodiness/polyfin/internal/addons"
)

// musicJSON describes what an Eclipse addon offers: the content its tracks
// are (music, audiobook or podcast), the settings it declares, and the
// values chosen for them, a setting left out being sent with its default.
type musicJSON struct {
	ContentType string            `json:"contentType"`
	Settings    []settingJSON     `json:"settings"`
	Values      map[string]string `json:"values"`
}

type settingJSON struct {
	Key         string       `json:"key"`
	Type        string       `json:"type"`
	Label       string       `json:"label"`
	Help        string       `json:"help"`
	Default     string       `json:"default"`
	Options     []optionJSON `json:"options"`
	MaxLength   int          `json:"maxLength"`
	Placeholder string       `json:"placeholder"`
	Min         *float64     `json:"min"`
	Max         *float64     `json:"max"`
	Step        *float64     `json:"step"`
}

type optionJSON struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func newMusicJSON(addon addons.Addon) *musicJSON {
	if !addon.Eclipse() {
		return nil
	}
	m := addon.Music
	result := &musicJSON{ContentType: m.ContentType, Settings: make([]settingJSON, 0, len(m.Settings)), Values: map[string]string{}}
	for _, s := range m.Settings {
		options := make([]optionJSON, 0, len(s.Options))
		for _, o := range s.Options {
			options = append(options, optionJSON(o))
		}
		result.Settings = append(result.Settings, settingJSON{Key: s.Key, Type: s.Type, Label: s.Label, Help: s.Help, Default: s.Default,
			Options: options, MaxLength: s.MaxLength, Placeholder: s.Placeholder, Min: s.Min, Max: s.Max, Step: s.Step})
		if value, ok := addon.Settings[s.Key]; ok {
			result.Values[s.Key] = value
		}
	}
	return result
}

// saveAddonSettings sets the values of an Eclipse addon's settings, which
// every request to the addon carries from then on. Like the addon's address,
// they may hold the user's credentials: they are only shown to who can
// change them, the administrators for the server's addons and the user for
// their own.
func (h *handler) saveAddonSettings(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok || h.personalAddonsRefused(w, r, scope) {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Values map[string]string `json:"values"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Values == nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	addon, err := h.Addons.SetSettings(r.Context(), scope, id, body.Values)
	h.answerAddon(w, r, scope, http.StatusOK, addon, err)
}
