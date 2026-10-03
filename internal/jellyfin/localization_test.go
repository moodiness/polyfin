package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLocalizationListsMatchJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	for fixture, path := range map[string]string{
		"localization-cultures":         "/Localization/Cultures",
		"localization-countries":        "/Localization/Countries",
		"localization-parental-ratings": "/Localization/ParentalRatings",
		"localization-options":          "/Localization/Options",
	} {
		if status, _ := s.call(http.MethodGet, path, "", nil); status != http.StatusUnauthorized {
			t.Errorf("%s without credentials: %d", path, status)
		}
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		_ = json.Unmarshal(raw, &want)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: %v in %s", path, err, body)
		}
		for _, difference := range compareShapes(fixture, want, got, shapeRules{}) {
			t.Error(difference)
		}
	}
}

func TestLocalizationListsHoldTheStandardsEntries(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")

	var cultures []CultureDto
	s.get(t, "/Localization/Cultures", token, &cultures)
	find := func(code string) (CultureDto, bool) {
		i := slices.IndexFunc(cultures, func(c CultureDto) bool { return c.ThreeLetterISOLanguageName == code })
		if i < 0 {
			return CultureDto{}, false
		}
		return cultures[i], true
	}
	if french, ok := find("fre"); !ok || french.DisplayName != "French" || french.TwoLetterISOLanguageName != "fr" ||
		!slices.Equal(french.ThreeLetterISOLanguageNames, []string{"fre", "fra"}) {
		t.Errorf("French: %+v", french)
	}
	// A language without an ISO 639-1 code is listed too.
	if filipino, ok := find("fil"); !ok || filipino.TwoLetterISOLanguageName != "" || !slices.Equal(filipino.ThreeLetterISOLanguageNames, []string{"fil"}) {
		t.Errorf("Filipino: %+v", filipino)
	}
	if _, ok := find("qaa-qtz"); ok {
		t.Error("the range reserved for local use is listed as a language")
	}
	if !slices.IsSortedFunc(cultures, func(a, b CultureDto) int { return strings.Compare(a.Name, b.Name) }) {
		t.Error("languages are not sorted by name")
	}

	var countries []CountryInfo
	s.get(t, "/Localization/Countries", token, &countries)
	if len(countries) != 249 || !slices.Contains(countries, CountryInfo{Name: "FR", DisplayName: "France", TwoLetterISORegionName: "FR", ThreeLetterISORegionName: "FRA"}) ||
		!slices.Contains(countries, CountryInfo{Name: "BO", DisplayName: "Bolivia", TwoLetterISORegionName: "BO", ThreeLetterISORegionName: "BOL"}) {
		t.Errorf("countries: %d, France or Bolivia missing", len(countries))
	}

	var ratings []ParentalRating
	s.get(t, "/Localization/ParentalRatings", token, &ratings)
	score := map[string]int{}
	for i, rating := range ratings {
		if rating.RatingScore == nil {
			if rating.Value != nil || i > 0 && ratings[i-1].RatingScore != nil {
				t.Errorf("unrated %+v is not listed first, without a value", rating)
			}
			continue
		}
		if rating.Value == nil || *rating.Value != rating.RatingScore.Score || i > 0 && ratings[i-1].RatingScore != nil && ratings[i-1].RatingScore.Score > rating.RatingScore.Score {
			t.Errorf("rating %s out of order or with a value apart from its score", rating.Name)
		}
		score[rating.Name] = rating.RatingScore.Score
	}
	if score["G"] != 0 || score["PG-13"] != 13 || score["TV-14"] != 14 || score["R"] != 17 || score["NC-17"] != 18 || ratings[0].Name != "NR" {
		t.Errorf("ratings: %+v", score)
	}

	var options []LocalizationOption
	s.get(t, "/Localization/Options", token, &options)
	if !slices.Equal(options, []LocalizationOption{{"English", "en"}, {"Français", "fr"}}) {
		t.Errorf("options: %+v", options)
	}
}
