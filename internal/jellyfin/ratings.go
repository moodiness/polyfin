package jellyfin

import (
	"net/http"
)

// TitleRatings is what /Polyfin/Items/{itemId}/Ratings answers of a movie
// or series, for the web player's script to show its three scores on the
// title's page: its CommunityRating, the IMDb rating, and its critics' and
// audience scores, from 0 to 100, the one its CriticRating carries as that
// field shows it, an administrator's edit winning, each null when it has
// none. Pending tells that MDBList is being asked about the title: the
// script asks again. Without the server's MDBList key, and for other
// items, all are null.
type TitleRatings struct {
	CommunityRating *float64
	CriticsScore    *float64
	AudienceScore   *float64
	Pending         bool
}

// titleRatings answers /Polyfin/Items/{itemId}/Ratings.
func (h *Handler) titleRatings(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	var answer TitleRatings
	scores, pending, known := h.Library.Scores(item)
	if known {
		answer.Pending = pending
		if item.CommunityRating > 0 {
			answer.CommunityRating = new(item.CommunityRating)
		}
		answer.CriticsScore, answer.AudienceScore = scores.Critics, scores.Audience
		if h.Accounts.Settings().CriticRatingShows == "audience" {
			answer.AudienceScore = item.CriticRating
		} else {
			answer.CriticsScore = item.CriticRating
		}
	}
	writeJSON(w, http.StatusOK, answer)
}
