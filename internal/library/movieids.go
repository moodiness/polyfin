package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// movieIDsPage is how many titles each read of MovieIDs asks.
const movieIDsPage = 100

// MovieIDs lists the identifiers, under Jellyfin's names (Tmdb, Imdb), of
// the first most movies of each of the server's libraries of movies, as
// the server's addons list them to a request without a user, read as
// background work (see Background): what box sets learn ahead (see the
// boxset package). A library that cannot be read is left out.
func (s *Service) MovieIDs(ctx context.Context, most int) ([]map[string]string, error) {
	libraries, err := s.ServerLibraries(ctx)
	if err != nil {
		return nil, err
	}
	ctx = WithKinds(Background(ctx), func(kind Kind) bool { return kind == KindMovie })
	var ids []map[string]string
	for _, l := range libraries {
		if l.CollectionType != "movies" {
			continue
		}
		for start := 0; start < most; {
			page, err := s.Children(ctx, accounts.User{}, l.ID, start, min(movieIDsPage, most-start), "")
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				s.logger.Debug("A library's movies could not be listed for box sets", "library", l.Name, "error", err)
				break
			}
			for _, item := range page.Items {
				if item.Kind == KindMovie && len(item.ProviderIDs) > 0 {
					ids = append(ids, item.ProviderIDs)
				}
			}
			start += len(page.Items)
			if len(page.Items) == 0 || start >= page.Total && !page.More {
				break
			}
		}
	}
	return ids, nil
}
