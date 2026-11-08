package localization

import (
	"slices"
	"strings"
	"testing"
)

func TestRatingsOfEverySystemAddonsSend(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rating string
		score  int
		sub    int // -1 for none
	}{
		// United States, movies and television, as metadata addons send them.
		{"G", 0, 0}, {"PG", 10, 0}, {"PG-13", 13, 0}, {"R", 17, 0}, {"NC-17", 17, 1},
		{"TV-Y", 0, 0}, {"TV-Y7", 7, 0}, {"TV-Y7-FV", 7, 1}, {"TV-PG", 10, 0}, {"TV-14", 14, 0},
		{"TV-14-LV", 14, 1}, {"TV-MA", 17, 1},
		{"Rated R", 17, 0}, {"us:pg-13", 13, 0},
		// France.
		{"U", 0, -1}, {"TP", 0, -1}, {"Tous publics", 0, -1}, {"-10", 10, -1}, {"12", 12, -1}, {"-16", 16, -1}, {"18", 18, -1},
		{"FR-12", 12, -1},
		// Germany.
		{"FSK 0", 0, -1}, {"FSK-6", 6, -1}, {"FSK12", 12, -1}, {"DE-FSK-16", 16, -1}, {"DE: 18", 18, -1},
		// United Kingdom and anime: 12A for 12 and over, R18 for adults.
		{"12A", 12, -1}, {"15", 15, -1}, {"R18", 1000, -1}, {"GB:15", 15, -1},
		// Plain ages.
		{"16+", 16, -1}, {"7", 7, -1},
	} {
		score, ok := RatingScore(tc.rating, "US")
		sub := -1
		if score.SubScore != nil {
			sub = *score.SubScore
		}
		if !ok || score.Score != tc.score || sub != tc.sub {
			t.Errorf("%q: %+v (sub %d) %v, want %d/%d", tc.rating, score, sub, ok, tc.score, tc.sub)
		}
	}
	for _, unrated := range []string{"", "NR", "Unrated", "Not Rated", "N/A", "UR", "X-rated?", "+12", "us:", "TV"} {
		if score, ok := RatingScore(unrated, "US"); ok {
			t.Errorf("%q has a score: %+v", unrated, score)
		}
	}
}

// TestEveryCountryScoresItsRatings checks, for every country, its lowest
// and highest rating and another writing of one, so that a slip in a table
// shows; then that every rating of every table scores as written.
func TestEveryCountryScoresItsRatings(t *testing.T) {
	t.Parallel()
	type rated struct {
		rating string
		score  int
	}
	cases := map[string][]rated{
		"US": {{"G", 0}, {"TV-AO", 18}, {"TV-MA-LSV", 17}},
		"AR": {{"ATP", 0}, {"C", 18}, {"SAM 13", 13}},
		"AT": {{"0", 0}, {"16", 16}, {"12", 12}},
		"AU": {{"G", 0}, {"RC", 1001}, {"AV15+", 15}, {"MA15+", 15}, {"M", 15}},
		"BE": {{"AL", 0}, {"18", 18}, {"KNT", 16}},
		"BR": {{"L", 0}, {"18", 18}, {"Livre", 0}, {"A14", 14}},
		"CA": {{"G", 0}, {"R", 18}, {"14A", 14}, {"C8", 8}},
		"CH": {{"0", 0}, {"18", 18}, {"14", 14}},
		"CZ": {{"U", 0}, {"18", 18}, {"15", 15}},
		"DE": {{"FSK 0", 0}, {"18", 18}, {"FSK 16", 16}},
		"DK": {{"A", 0}, {"15", 15}, {"11", 11}},
		"ES": {{"A", 0}, {"X", 1000}, {"7i", 7}},
		"FI": {{"S", 0}, {"K-18", 18}, {"T", 0}, {"K-12", 12}},
		"FR": {{"TP", 0}, {"X", 1000}, {"Tous publics", 0}},
		"GB": {{"U", 0}, {"R18", 1000}, {"Uc", 0}, {"PG", 8}},
		"IE": {{"G", 0}, {"18", 18}, {"15A", 15}},
		"IT": {{"T", 0}, {"VM18", 18}, {"VM14", 14}, {"BA", 10}, {"Per tutti", 0}},
		"JP": {{"G", 0}, {"R18+", 18}, {"R15+", 15}},
		"KR": {{"ALL", 0}, {"제한상영가", 1000}, {"청소년관람불가", 18}},
		"MX": {{"AA", 0}, {"D", 1000}, {"B-15", 15}, {"B15", 15}},
		"NL": {{"AL", 0}, {"18", 18}, {"9", 9}},
		"NO": {{"A", 0}, {"18", 18}, {"9", 9}},
		"NZ": {{"G", 0}, {"R", 18}, {"M", 16}, {"RP13", 13}},
		"PL": {{"BO", 0}, {"18", 18}, {"Bez ograniczeń", 0}},
		"PT": {{"M/3", 3}, {"P", 1000}, {"M/12", 12}},
		"SE": {{"Btl", 0}, {"Från 15 år", 15}, {"Barntillåten", 0}},
	}
	for _, country := range RatingCountries() {
		ratings, ok := cases[country]
		if !ok {
			t.Errorf("%s: no case", country)
			continue
		}
		for _, tc := range ratings {
			if score, ok := RatingScore(tc.rating, country); !ok || score.Score != tc.score {
				t.Errorf("%s %q: %+v %v, want %d", country, tc.rating, score, ok, tc.score)
			}
			if score, ok := CountryScore(tc.rating, country); !ok || score.Score != tc.score {
				t.Errorf("%s %q is not its own: %+v %v, want %d", country, tc.rating, score, ok, tc.score)
			}
		}
	}
	for country, ratings := range countrySystems {
		for _, rating := range ratings {
			if score, ok := RatingScore(rating.name, country); !ok || score.Score != rating.score {
				t.Errorf("%s %q: %+v %v, want %d", country, rating.name, score, ok, rating.score)
			}
		}
	}
}

func TestRatingsMeanWhatTheirCountrySays(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rating, country string
		score           int
	}{
		// T is for all in Italy, PG for 8 in the United Kingdom, M for 15 in
		// Australia but 16 in New Zealand, C for all in Canada.
		{"T", "IT", 0},
		{"PG", "GB", 8}, {"PG", "US", 10}, {"PG", "", 10}, {"PG", "ZZ", 10},
		{"M", "AU", 15}, {"M", "NZ", 16},
		{"C", "CA", 0},
		// Ratings only other systems know are judged by the strictest.
		{"C", "US", 18}, {"VM18", "US", 18}, {"VM14", "US", 14},
		// Preceded by their country, whatever the rating country.
		{"IT-VM14", "US", 14}, {"it:vm14", "US", 14}, {"de:FSK 16", "US", 16}, {"UK:15", "US", 15},
		{"IT-VM14", "GB", 14}, {"NZ:M", "AU", 16},
		// Lists separated by "/", the first known.
		{"PG-13 / 12", "US", 13}, {"M/12", "PT", 12},
	} {
		if score, ok := RatingScore(tc.rating, tc.country); !ok || score.Score != tc.score {
			t.Errorf("%q in %q: %+v %v, want %d", tc.rating, tc.country, score, ok, tc.score)
		}
	}
}

func TestCountryScoreKnowsOnlyItsCountry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rating, country string
		ok              bool
	}{
		{"VM14", "IT", true}, {"VM14", "US", false}, {"PG-13", "IT", false}, {"IT-VM14", "IT", true},
		{"IT-VM14", "US", false}, {"PG-13", "US", true}, {"PG-13", "", true}, {"us:pg-13", "US", true},
		{"FSK 16", "DE", true}, {"16", "US", false}, {"", "US", false},
		{"VM14 / 14+", "IT", true}, {"VM14 / 14+", "US", false}, {"XXX", "IT", false},
	} {
		if score, ok := CountryScore(tc.rating, tc.country); ok != tc.ok {
			t.Errorf("%q in %q: %+v %v, want %v", tc.rating, tc.country, score, ok, tc.ok)
		}
	}
}

func TestRatingsListTheCountrysLimits(t *testing.T) {
	t.Parallel()
	// The US list is the one Polyfin offered before ratings had a country.
	want := []Rating{{Name: "Unrated"}}
	for _, group := range usRatings {
		for _, name := range group.names {
			want = append(want, Rating{Name: name, Score: &Score{Score: group.score, SubScore: new(group.subScore)}})
		}
	}
	for _, entry := range beyondUS {
		want = append(want, Rating{Name: entry.name, Score: &Score{Score: entry.score}})
	}
	us := Ratings("US")
	if !slices.EqualFunc(us, want, sameRating) {
		t.Errorf("US: %v, want %v", ratingNames(us), ratingNames(want))
	}
	if got := ratingNames(us[:5]); !slices.Equal(got, []string{"Unrated", "Approved", "G", "TV-G", "TV-Y"}) {
		t.Errorf("US starts %v", got)
	}
	if got := ratingNames(us[len(us)-3:]); !slices.Equal(got, []string{"21", "XXX", "Banned"}) {
		t.Errorf("US ends %v", got)
	}
	for _, country := range []string{"", "ZZ", "it"} {
		if got := Ratings(country); !slices.EqualFunc(got, us, sameRating) {
			t.Errorf("%q: %v, want the US list", country, ratingNames(got))
		}
	}

	// Elsewhere, one entry per level of the country's systems, named by the
	// ratings titles show at it, without the US ages it lacks.
	type level struct {
		name  string
		score int
	}
	for country, want := range map[string][]level{
		"IT": {{"T", 0}, {"6+", 6}, {"BA", 10}, {"VM12", 12}, {"VM14 / 14+", 14}, {"VM18 / 18+", 18},
			{"XXX", 1000}, {"Banned", 1001}},
		"FR": {{"TP / U", 0}, {"10", 10}, {"12", 12}, {"16", 16}, {"18", 18}, {"X", 1000}, {"Banned", 1001}},
		"ES": {{"A / Ai / TP", 0}, {"7 / 7i", 7}, {"12", 12}, {"16", 16}, {"18", 18}, {"X", 1000}, {"Banned", 1001}},
		"CA": {{"G / C", 0}, {"PG / C8", 8}, {"13+", 13}, {"14A / 14+", 14}, {"16+", 16}, {"18A / 18+ / R", 18},
			{"XXX", 1000}, {"Banned", 1001}},
		"AU": {{"G", 0}, {"PG", 8}, {"M / MA15+", 15}, {"R18+", 18}, {"X18+", 1000}, {"RC", 1001}},
		"AT": {{"0", 0}, {"6", 6}, {"8", 8}, {"10", 10}, {"12", 12}, {"14", 14}, {"16", 16}, {"XXX", 1000},
			{"Banned", 1001}},
	} {
		got := Ratings(country)
		var gotLevels []level
		for _, rating := range got[1:] {
			gotLevels = append(gotLevels, level{rating.Name, rating.Score.Score})
		}
		if got[0].Name != "Unrated" || !slices.Equal(gotLevels, want) {
			t.Errorf("%s: %v, want Unrated then %v", country, gotLevels, want)
		}
	}

	for _, country := range RatingCountries() {
		list := Ratings(country)
		if len(list) == 0 || list[0].Name != "Unrated" || list[0].Score != nil {
			t.Errorf("%s does not start with Unrated: %v", country, ratingNames(list))
			continue
		}
		for i, rating := range list[1:] {
			if rating.Score == nil {
				t.Errorf("%s %q has no score", country, rating.Name)
				continue
			}
			if i > 0 && compareScores(*list[i].Score, *rating.Score) > 0 {
				t.Errorf("%s: %q after %q", country, rating.Name, list[i].Name)
			}
		}
		if country == DefaultRatingCountry {
			continue
		}
		// One entry per level, named only by the country's own ratings.
		own := map[string]bool{"XXX": true, "Banned": true}
		for _, rating := range countrySystems[country] {
			if !rating.alias {
				own[rating.name] = true
			}
		}
		for i, rating := range list[1:] {
			if i > 0 && list[i].Score.Score == rating.Score.Score {
				t.Errorf("%s: %q and %q share a level", country, list[i].Name, rating.Name)
			}
			for part := range strings.SplitSeq(rating.Name, " / ") {
				if !own[part] {
					t.Errorf("%s: %q names %q, not one of its ratings", country, rating.Name, part)
				}
			}
			// Smart lists keep these names as rule values.
			if score, ok := RatingScore(rating.Name, country); !ok || score.Score != rating.Score.Score {
				t.Errorf("%s %q: %+v %v, want %d", country, rating.Name, score, ok, rating.Score.Score)
			}
			if _, ok := CountryScore(rating.Name, country); ok != (rating.Name != "XXX" && rating.Name != "Banned") {
				t.Errorf("%s %q: CountryScore %v", country, rating.Name, ok)
			}
		}
	}

	// A name with several ratings scores as they do, whatever the country.
	for _, country := range []string{"IT", "US", "GB"} {
		if score, ok := RatingScore("VM14 / 14+", country); !ok || score.Score != 14 {
			t.Errorf("%q in %s: %+v %v, want 14", "VM14 / 14+", country, score, ok)
		}
	}
}

func TestRatingCountries(t *testing.T) {
	t.Parallel()
	want := []string{"AR", "AT", "AU", "BE", "BR", "CA", "CH", "CZ", "DE", "DK", "ES", "FI", "FR", "GB", "IE", "IT",
		"JP", "KR", "MX", "NL", "NO", "NZ", "PL", "PT", "SE", "US"}
	if got := RatingCountries(); !slices.Equal(got, want) {
		t.Errorf("RatingCountries: %v, want %v", got, want)
	}
	for _, tc := range []struct {
		code string
		ok   bool
	}{{"IT", true}, {"US", true}, {"it", false}, {"", false}, {"ZZ", false}, {"UK", false}} {
		if ok := ValidRatingCountry(tc.code); ok != tc.ok {
			t.Errorf("ValidRatingCountry(%q) = %v, want %v", tc.code, ok, tc.ok)
		}
	}
}

func sameRating(a, b Rating) bool {
	if a.Name != b.Name || (a.Score == nil) != (b.Score == nil) {
		return false
	}
	return a.Score == nil || compareScores(*a.Score, *b.Score) == 0 && (a.Score.SubScore == nil) == (b.Score.SubScore == nil)
}

func compareScores(a, b Score) int {
	if a.Score != b.Score {
		return a.Score - b.Score
	}
	if a.SubScore == nil || b.SubScore == nil {
		return 0
	}
	return *a.SubScore - *b.SubScore
}

func ratingNames(list []Rating) []string {
	result := make([]string, len(list))
	for i, rating := range list {
		result[i] = rating.Name
	}
	return result
}
