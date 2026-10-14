package library

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotShareable reports a title a share link cannot open: neither a
// movie nor an episode.
var ErrNotShareable = errors.New("only a movie or an episode can be shared")

// Shareable returns the title id names when user may give it through a
// share link: a movie or an episode they may see, parental control,
// blocked genres and the titles they hid applying. ErrNotFound reports a
// title they may not see, ErrNotShareable one of another kind.
func (s *Service) Shareable(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	item, err := s.Item(ctx, user, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindMovie && item.Kind != KindEpisode {
		return Item{}, ErrNotShareable
	}
	hidden, err := s.HiddenTitles(ctx, user)
	if err != nil {
		return Item{}, err
	}
	if hidden.Hides(item) {
		return Item{}, ErrNotFound
	}
	return item, nil
}

// ShareTitle names a title as share links list it: a movie with its year,
// an episode with its series, season and number.
func ShareTitle(item Item) string {
	switch {
	case item.Kind == KindEpisode:
		name := fmt.Sprintf("%s S%02dE%02d", item.SeriesName, item.ParentIndexNumber, item.IndexNumber)
		if item.Name != "" {
			name += " · " + item.Name
		}
		return name
	case item.ProductionYear > 0:
		return item.Name + " (" + strconv.Itoa(item.ProductionYear) + ")"
	}
	return item.Name
}
