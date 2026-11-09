package jellyfin

import (
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/library"
)

// ratingFilterOf reads what a listing of /Items asks of its items'
// ratings: minOfficialRating, maxOfficialRating, hasParentalRating,
// hasOfficialRating and officialRatings, separated by "|" (see
// library.RatingFilter). A flag that is not a boolean is refused, as
// Jellyfin refuses it.
func ratingFilterOf(b bindErrors, r *http.Request) library.RatingFilter {
	f := library.RatingFilter{MinOfficialRating: query(r, "minOfficialRating"), MaxOfficialRating: query(r, "maxOfficialRating")}
	if value, set := b.bool(r, "hasParentalRating"); set {
		f.HasParentalRating = &value
	}
	if value, set := b.bool(r, "hasOfficialRating"); set {
		f.HasOfficialRating = &value
	}
	for rating := range strings.SplitSeq(query(r, "officialRatings"), "|") {
		if rating = strings.TrimSpace(rating); rating != "" {
			f.OfficialRatings = append(f.OfficialRatings, rating)
		}
	}
	return f
}
