package admin

import (
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/taste"
)

// tasteLeanings is how many values of each facet My account shows the
// user likes, and as many they dislike.
const tasteLeanings = 8

// tasteJSON is the caller's taste profile, as My account › Taste profile
// shows it: when it was built (null while it never was), how many titles
// it weighed, how sure it is, from 0 to 1, and whether it passed the
// confidence gate, what the user likes and dislikes of each facet, and
// whether their titles' details tell why a title is for them.
type tasteJSON struct {
	Built     *time.Time       `json:"built"`
	Titles    int              `json:"titles"`
	Certainty float64          `json:"certainty"`
	Confident bool             `json:"confident"`
	Facets    []tasteFacetJSON `json:"facets"`
	Taglines  bool             `json:"taglines"`
}

// tasteFacetJSON is what the user likes and dislikes of a facet: genre,
// director, cast, decade or language.
type tasteFacetJSON struct {
	Facet    taste.Facet        `json:"facet"`
	Liked    []tasteLeaningJSON `json:"liked"`
	Disliked []tasteLeaningJSON `json:"disliked"`
}

// tasteLeaningJSON is a value of a facet and its weight; vetoed tells a
// genre whose titles leave Top picks for you.
type tasteLeaningJSON struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Vetoed bool    `json:"vetoed,omitempty"`
}

// setOwnTaste turns the caller's taglines on or off.
func (h *handler) setOwnTaste(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Taglines *bool `json:"taglines"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Taglines == nil {
		writeError(w, http.StatusBadRequest, "invalid_taglines")
		return
	}
	if h.Taste == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := h.Taste.SetTaglines(r.Context(), sessionFrom(r.Context()).User.ID, *body.Taglines); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.ownTaste(w, r)
}

// ownTaste answers the caller's taste profile.
func (h *handler) ownTaste(w http.ResponseWriter, r *http.Request) {
	result := tasteJSON{Facets: []tasteFacetJSON{}, Taglines: true}
	if h.Taste == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	user := sessionFrom(r.Context()).User.ID
	profile, err := h.Taste.Profile(r.Context(), user)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if result.Taglines, err = h.Taste.Taglines(r.Context(), user); err != nil {
		h.internalError(w, r, err)
		return
	}
	if !profile.Built.IsZero() {
		result.Built = &profile.Built
	}
	result.Titles, result.Certainty, result.Confident = profile.Titles, profile.Certainty(), profile.Confident()
	for _, facet := range taste.Facets {
		liked, disliked := profile.Leanings(facet, tasteLeanings)
		result.Facets = append(result.Facets, tasteFacetJSON{Facet: facet, Liked: leaningsJSON(liked), Disliked: leaningsJSON(disliked)})
	}
	writeJSON(w, http.StatusOK, result)
}

func leaningsJSON(leanings []taste.Leaning) []tasteLeaningJSON {
	result := make([]tasteLeaningJSON, 0, len(leanings))
	for _, l := range leanings {
		result = append(result, tasteLeaningJSON{Name: l.Name, Weight: l.Weight, Vetoed: l.Vetoed})
	}
	return result
}
