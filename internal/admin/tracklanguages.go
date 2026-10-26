package admin

import (
	"sync"

	"github.com/moodiness/polyfin/internal/localization"
)

// trackLanguageJSON is a language the server's default audio and subtitle
// languages offer: its ISO 639-2/B code and its English name, as
// jellyfin-web lists a user's own preferences from /Localization/Cultures.
type trackLanguageJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// trackLanguages lists the languages of accounts.TrackLanguages but "",
// sorted by English name.
var trackLanguages = sync.OnceValue(func() []trackLanguageJSON {
	languages := localization.Languages()
	result := make([]trackLanguageJSON, len(languages))
	for i, language := range languages {
		result[i] = trackLanguageJSON{Code: language.Bibliographic, Name: language.Name}
	}
	return result
})
