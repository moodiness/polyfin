package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// movieIDsPage is how many titles each read of libraryTitles asks.
const movieIDsPage = 100

// MovieIDs lists the identifiers, under Jellyfin's names (Tmdb, Imdb), of
// the first most movies of each of the server's libraries of movies, as
// the server's addons list them to a request without a user, read as
// background work (see libraryTitles): what box sets learn ahead (see the
// boxset package). It also tells how many libraries of movies the server
// has, and how many of them were read; one that cannot be read is left
// out, and logged.
func (s *Service) MovieIDs(ctx context.Context, most int) (ids []map[string]string, libraries, read int, err error) {
	titles, libraries, read, err := s.libraryTitles(ctx, most, func(collection string) bool { return collection == "movies" },
		func(kind Kind) bool { return kind == KindMovie })
	if err != nil {
		return nil, 0, 0, err
	}
	ids = make([]map[string]string, 0, len(titles))
	for _, title := range titles {
		ids = append(ids, title.ProviderIDs)
	}
	return ids, libraries, read, nil
}

// libraryTitles lists the first most titles of the kinds keeps keeps of
// each of the server's libraries whose collection type collections keeps,
// those with an identifier of another source, as the server's addons
// list them to a request without a user, read as background work (see
// Background). It also tells how many such libraries the server has, and
// how many of them were read: one that cannot be read is left out, and
// logged with the reason, at Info, so that a pass that learns nothing
// says why.
func (s *Service) libraryTitles(ctx context.Context, most int, collections func(string) bool, keeps func(Kind) bool) (titles []Item, libraries, read int, err error) {
	all, err := s.ServerLibraries(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	ctx = WithKinds(Background(ctx), keeps)
	for _, l := range all {
		if !collections(l.CollectionType) {
			continue
		}
		libraries++
		listed := true
		for start := 0; start < most; {
			page, err := s.Children(ctx, accounts.User{}, l.ID, start, min(movieIDsPage, most-start), "")
			if err != nil {
				if ctx.Err() != nil {
					return nil, 0, 0, ctx.Err()
				}
				s.logger.Info("A library's titles could not be listed for a background pass", "library", l.Name, "listed", start, "error", err)
				listed = start > 0
				break
			}
			for _, item := range page.Items {
				if keeps(item.Kind) && len(item.ProviderIDs) > 0 {
					titles = append(titles, item)
				}
			}
			start += len(page.Items)
			if len(page.Items) == 0 || start >= page.Total && !page.More {
				break
			}
		}
		if listed {
			read++
		}
	}
	return titles, libraries, read, nil
}
