package notifications

import (
	"context"
	"errors"
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
)

// TitleJSON is the movie or series of a user's watchlist that versions
// were found for: its item as Jellyfin apps know it, of Kind "movie" or
// "series", its name, its year, 0 when unknown, and its provider IDs.
type TitleJSON struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Year        int               `json:"year"`
	ProviderIDs map[string]string `json:"providerIds"`
}

// titleAvailableEvent tells user that versions were found for title, of
// their watchlist.
func (s *Service) titleAvailableEvent(user accounts.User, title library.Item) Event {
	ev := s.newEvent(TitleAvailable)
	ev.User = &UserJSON{ID: user.ID.String(), Name: user.Name}
	kind := "movie"
	if title.Kind == library.KindSeries {
		kind = "series"
	}
	ev.WatchlistTitle = &TitleJSON{ID: title.ID.String(), Kind: kind, Name: title.Name, Year: title.ProductionYear, ProviderIDs: title.ProviderIDs}
	if ev.WatchlistTitle.ProviderIDs == nil {
		ev.WatchlistTitle.ProviderIDs = map[string]string{}
	}
	name := title.Name
	if title.ProductionYear > 0 {
		name += " (" + strconv.Itoa(title.ProductionYear) + ")"
	}
	ev.Title = s.phrase(i18n.Text{"Versions found: %s", "Versions trouvées : %s", "Versionen gefunden: %s", "Versiones encontradas: %s",
		"Versioni trovate: %s", "Versões encontradas: %s"}, name)
	ev.Message = s.phrase(i18n.Text{"Versions of %s, on your watchlist, were found. They have not been tried yet.",
		"Des versions de %s, sur votre liste « À regarder », ont été trouvées. Elles n’ont pas encore été essayées.",
		"Versionen von %s auf deiner Merkliste wurden gefunden. Sie wurden noch nicht ausprobiert.",
		"Se encontraron versiones de %s, de tu lista «Para ver». Todavía no se han probado.",
		"Sono state trovate versioni di %s, nella tua lista «Da guardare». Non sono ancora state provate.",
		"Foram encontradas versões de %s, da sua lista “Para assistir”. Elas ainda não foram testadas."}, name)
	ev.URL = s.webLink(title.ID)
	return ev
}

// WatchlistTitleAvailable tells user, on their own targets only, that
// versions were found for title, a movie or series of their watchlist (see
// watchlist.Checker): callers tell it once per title. A title the user may
// no longer see is not told of. The message is sent in the background.
func (s *Service) WatchlistTitleAvailable(ctx context.Context, user accounts.User, title accounts.ID) {
	item, err := s.library.Item(ctx, user, title)
	if err != nil {
		if !errors.Is(err, library.ErrNotFound) && ctx.Err() == nil {
			s.logger.Warn("A watchlist title found available could not be described", "item_id", title.String(), "error", err)
		}
		return
	}
	s.dispatch(s.titleAvailableEvent(user, item), func(t target) bool { return t.owner != nil && *t.owner == user.ID })
}
