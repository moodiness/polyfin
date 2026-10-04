package localization

import "testing"

func TestRatingsOfEverySystemAddonsSend(t *testing.T) {
	for _, tc := range []struct {
		rating string
		score  int
		sub    int // -1 for none
	}{
		// United States, films and television, as AIOMetadata sends them.
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
		score, ok := RatingScore(tc.rating)
		sub := -1
		if score.SubScore != nil {
			sub = *score.SubScore
		}
		if !ok || score.Score != tc.score || sub != tc.sub {
			t.Errorf("%q: %+v (sub %d) %v, want %d/%d", tc.rating, score, sub, ok, tc.score, tc.sub)
		}
	}
	for _, unrated := range []string{"", "NR", "Unrated", "Not Rated", "N/A", "UR", "X-rated?", "+12", "us:", "TV"} {
		if score, ok := RatingScore(unrated); ok {
			t.Errorf("%q has a score: %+v", unrated, score)
		}
	}
}
