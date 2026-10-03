package jellyfin

import (
	"net/http"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
)

// Localization lists: the choices apps offer for languages (audio and
// subtitle preferences), countries, parental ratings and the server's
// interface language.

type CultureDto struct {
	Name                        string
	DisplayName                 string
	TwoLetterISOLanguageName    string
	ThreeLetterISOLanguageName  string
	ThreeLetterISOLanguageNames []string
}

type CountryInfo struct {
	Name                     string
	DisplayName              string
	TwoLetterISORegionName   string
	ThreeLetterISORegionName string
}

// ParentalRating leaves out the score of the ratings that say a title was
// not rated, as Jellyfin omits null values.
type ParentalRating struct {
	Name        string
	Value       *int                 `json:",omitempty"`
	RatingScore *ParentalRatingScore `json:",omitempty"`
}

// ParentalRatingScore is named in camel case in Jellyfin's API too.
type ParentalRatingScore struct {
	Score    int  `json:"score"`
	SubScore *int `json:"subScore,omitempty"`
}

type LocalizationOption struct {
	Name  string
	Value string
}

// interfaceLanguageNames name the server languages in themselves.
var interfaceLanguageNames = map[string]string{"en": "English", "fr": "Français"}

func (h *Handler) localizationRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	signedIn(http.MethodGet, "/Localization/Cultures", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cultures())
	})
	signedIn(http.MethodGet, "/Localization/Countries", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, countries())
	})
	signedIn(http.MethodGet, "/Localization/ParentalRatings", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, parentalRatings())
	})
	signedIn(http.MethodGet, "/Localization/Options", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, localizationOptions())
	})
}

// cultures lists the ISO 639-2 languages. Their three-letter code is the
// bibliographic one ("fre"), which apps save as the user's language
// preferences; ThreeLetterISOLanguageNames adds the terminology one
// ("fra"). A language without an ISO 639-1 code has an empty two-letter
// one.
var cultures = sync.OnceValue(func() []CultureDto {
	languages := localization.Languages()
	result := make([]CultureDto, len(languages))
	for i, language := range languages {
		codes := []string{language.Bibliographic}
		if language.Terminology != "" {
			codes = append(codes, language.Terminology)
		}
		result[i] = CultureDto{
			Name:                        language.Name,
			DisplayName:                 language.Name,
			TwoLetterISOLanguageName:    language.Alpha2,
			ThreeLetterISOLanguageName:  language.Bibliographic,
			ThreeLetterISOLanguageNames: codes,
		}
	}
	return result
})

// countries lists the ISO 3166-1 countries, named by their two-letter code
// as Jellyfin names them.
var countries = sync.OnceValue(func() []CountryInfo {
	list := localization.Countries()
	result := make([]CountryInfo, len(list))
	for i, country := range list {
		result[i] = CountryInfo{
			Name:                     country.Alpha2,
			DisplayName:              country.Name,
			TwoLetterISORegionName:   country.Alpha2,
			ThreeLetterISORegionName: country.Alpha3,
		}
	}
	return result
})

// parentalRatings lists the US ratings, Jellyfin's default metadata
// country being the United States. Value is the score, the integer older
// apps compare.
var parentalRatings = sync.OnceValue(func() []ParentalRating {
	ratings := localization.Ratings()
	result := make([]ParentalRating, len(ratings))
	for i, rating := range ratings {
		result[i] = ParentalRating{Name: rating.Name}
		if rating.Score != nil {
			result[i].Value = rating.Score
			result[i].RatingScore = &ParentalRatingScore{Score: *rating.Score}
		}
	}
	return result
})

// localizationOptions lists the languages Polyfin speaks, the server
// languages.
var localizationOptions = sync.OnceValue(func() []LocalizationOption {
	result := make([]LocalizationOption, len(accounts.Languages))
	for i, language := range accounts.Languages {
		result[i] = LocalizationOption{Name: interfaceLanguageNames[language], Value: language}
	}
	return result
})
