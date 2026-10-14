package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/ratings"
)

// Ratings from MDBList.
//
// With the server's MDBList key (see package ratings), movies and series
// show the ratings MDBList gives them: CommunityRating carries the IMDb
// rating in place of the addon's, and CriticRating the critics' score or
// the audience score, as Settings › Content › Critic rating shows chooses.
// They apply wherever titles show, before administrators' edits, which
// still win (see overridden); Original carries them too, so that an edit
// leaving them as they are keeps following MDBList. Titles whose ratings
// are not kept are asked about in the background as they show: nothing
// waits for MDBList.

// UseRatings sets what looks titles' ratings up; it is called before the
// service is used. Without it, titles show their addons' ratings.
func (s *Service) UseRatings(r *ratings.Service) { s.ratings = r }

// ratingsTitle returns what MDBList is asked about for an item: movies
// and series only, by their IMDb or TMDB identifier.
func ratingsTitle(item Item) (ratings.Title, bool) {
	if item.Kind != KindMovie && item.Kind != KindSeries {
		return ratings.Title{}, false
	}
	imdb, tmdb := item.ProviderIDs["Imdb"], item.ProviderIDs["Tmdb"]
	if imdb == "" && tmdb == "" {
		return ratings.Title{}, false
	}
	return ratings.Title{Series: item.Kind == KindSeries, IMDb: imdb, TMDB: tmdb, Released: item.PremiereDate}, true
}

// keptScores returns the scores kept of items, by their place in items,
// nil when none are, and has those not kept asked about.
func (s *Service) keptScores(items []Item) []ratings.Scores {
	if !s.ratings.Enabled() {
		return nil
	}
	var titles []ratings.Title
	for i, item := range items {
		if t, ok := ratingsTitle(item); ok {
			if titles == nil {
				titles = make([]ratings.Title, len(items))
			}
			titles[i] = t
		}
	}
	if titles == nil {
		return nil
	}
	return s.ratings.Kept(context.Background(), titles)
}

// rate sets on item the scores MDBList gives it: the IMDb rating as its
// community rating, and the score the settings choose as its critic
// rating. A score MDBList does not have leaves the field as it was.
func rate(item *Item, scores ratings.Scores, audience bool) {
	if scores.IMDb != nil {
		item.CommunityRating = *scores.IMDb
	}
	critic := scores.Critics
	if audience {
		critic = scores.Audience
	}
	if critic != nil {
		item.CriticRating = new(*critic)
	}
}

// audienceShown reports whether CriticRating carries the audience score.
func (s *Service) audienceShown() bool {
	return s.settings().CriticRatingShows == "audience"
}

// rated returns item with the scores kept of it.
func (s *Service) rated(item Item) Item {
	if scores := s.keptScores([]Item{item}); scores != nil {
		rate(&item, scores[0], s.audienceShown())
	}
	return item
}

// Scores returns the scores kept of a movie or series, and whether it is
// waiting to be asked about or being asked about, for the web player's
// title page; ok is false for other items, and for every item while no
// key is saved.
func (s *Service) Scores(item Item) (scores ratings.Scores, pending, ok bool) {
	t, ok := ratingsTitle(item)
	if !ok || !s.ratings.Enabled() {
		return ratings.Scores{}, false, false
	}
	if kept := s.ratings.Kept(context.Background(), []ratings.Title{t}); kept != nil {
		scores = kept[0]
	}
	return scores, s.ratings.Pending(t), true
}
