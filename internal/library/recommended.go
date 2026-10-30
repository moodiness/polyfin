package library

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
)

// recommendedKinds are what a list of recommended titles holds.
var recommendedKinds = []Kind{KindMovie, KindSeries}

// Recommended lists from start the movies and series of ids, in order, as
// a tracking service recommends them to the user: those the user may not
// see, by parental control or blocked genres, those they hid, and those
// no addon describes, are left out, as the Watchlist view leaves them
// out. first describes only as many titles as the window shows, as a home
// row does; a listing describes them all to count them.
func (s *Service) Recommended(ctx context.Context, user accounts.User, ids []accounts.ID, start, count int, first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	ids = slices.DeleteFunc(slices.Clone(ids), func(id accounts.ID) bool { return v.hidden[id] })
	return s.describedPage(ctx, v, ids, recommendedKinds, start, count, first)
}
