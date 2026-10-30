package jellyfin

import (
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
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

// maxPosterRatings is how many titles one /Polyfin/Items/Ratings request
// may name: the cards of a screen, a row's or a page's.
const maxPosterRatings = 100

// PosterRatings is what /Polyfin/Items/Ratings answers: whether the web
// player shows ratings on posters (Settings.PosterRatings, while
// Settings.PosterBadges draws none over them), and the ratings of the
// movies and series named, those with none left out.
type PosterRatings struct {
	Enabled bool
	Items   []PosterRating
}

// PosterRating is a title's ratings on its poster: its CommunityRating,
// shown with a star, whatever gave it, and, while the server has an
// MDBList key, its critics' and audience scores, as
// /Polyfin/Items/{id}/Ratings gives them. Each is null when the title has
// none. imdb tells the star rating is the IMDb rating MDBList gave.
type PosterRating struct {
	Id              string
	CommunityRating *float64
	CriticsScore    *float64
	AudienceScore   *float64
	imdb            bool
}

// posterRatings answers /Polyfin/Items/Ratings?Ids=…, for the cards the
// web player's script shows ratings on: one request for the cards of a
// screen. It reads what listings read, the scores kept included, and has
// MDBList asked about nothing a listing of the same titles would not;
// switched off, or while badges are drawn over posters, it reads nothing.
// Identifiers past the first 100, of items the user cannot reach, or of
// other items than movies and series, are left out.
func (h *Handler) posterRatings(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	settings := h.Accounts.Settings()
	answer := PosterRatings{Enabled: settings.PosterRatings && !settings.PosterBadges, Items: []PosterRating{}}
	ids := slices.Compact(slices.SortedFunc(slices.Values(queryIDs(r, "ids")), compareIDs))
	if !answer.Enabled || len(ids) == 0 {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	if len(ids) > maxPosterRatings {
		ids = ids[:maxPosterRatings]
	}
	items, err := h.Library.ItemsOf(r.Context(), user, ids, []library.Kind{library.KindMovie, library.KindSeries})
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	audience := settings.CriticRatingShows == "audience"
	for _, item := range items {
		rating := h.posterRating(item, audience)
		if rating.CommunityRating != nil || rating.CriticsScore != nil || rating.AudienceScore != nil {
			answer.Items = append(answer.Items, rating)
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// posterRating returns the ratings a movie's or a series' poster shows;
// audience tells CriticRating carries the audience score.
func (h *Handler) posterRating(item library.Item, audience bool) PosterRating {
	rating := PosterRating{Id: item.ID.String()}
	if item.CommunityRating > 0 {
		rating.CommunityRating = new(item.CommunityRating)
	}
	if scores, _, known := h.Library.Scores(item); known {
		rating.CriticsScore, rating.AudienceScore = scores.Critics, scores.Audience
		if audience {
			rating.AudienceScore = item.CriticRating
		} else {
			rating.CriticsScore = item.CriticRating
		}
		rating.imdb = scores.IMDb != nil && *scores.IMDb == item.CommunityRating
	}
	return rating
}

func compareIDs(a, b accounts.ID) int {
	return slices.Compare(a[:], b[:])
}
