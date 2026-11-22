package notifications

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
)

// LocalTitlesAdded tells the server's Jellyfin targets that a scan of a
// local folder or a network share found the movies and episodes items new
// (see localfiles.Service.UseAdded): one ItemAdded each, described as an
// administrator sees it. A title an administrator cannot see, or none
// exists, is not told. It returns at once: the titles are described and
// the messages sent in the background.
func (s *Service) LocalTitlesAdded(items []accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var admins []accounts.ID
	for id := range s.admins {
		admins = append(admins, id)
	}
	if len(admins) == 0 || len(items) == 0 {
		return
	}
	// The same administrator each time, whose view the titles take.
	slices.SortFunc(admins, func(a, b accounts.ID) int { return slices.Compare(a[:], b[:]) })
	viewer := admins[0]
	items = slices.Clone(items)
	s.spawn(func() {
		user, err := s.accounts.User(s.ctx, viewer)
		if err != nil {
			return
		}
		for _, id := range items {
			item, err := s.library.Item(s.ctx, user, id)
			if err != nil {
				if s.ctx.Err() != nil {
					return
				}
				s.logger.Debug("A title a local folder added could not be described", "item_id", id.String(), "error", err)
				continue
			}
			s.dispatch(s.localTitleEvent(viewer, item), func(t target) bool { return t.owner == nil })
		}
	})
}

// localTitleEvent tells that item was added to a local folder, as viewer
// sees it.
func (s *Service) localTitleEvent(viewer accounts.ID, item library.Item) Event {
	ev := s.newEvent(LocalTitleAdded)
	ev.jellyfin = &jellyfinSource{viewer: viewer, item: &item}
	name := item.Name
	switch {
	case item.Kind == library.KindEpisode && item.SeriesName != "":
		name = fmt.Sprintf("%s S%02dE%02d", item.SeriesName, item.ParentIndexNumber, item.IndexNumber)
	case item.ProductionYear > 0:
		name += " (" + strconv.Itoa(item.ProductionYear) + ")"
	}
	ev.Title = s.phrase(i18n.Text{"Added: %s", "Ajouté\u202f: %s", "Hinzugefügt: %s", "Añadido: %s", "Aggiunto: %s", "Adicionado: %s"}, name)
	ev.URL = s.webLink(item.ID)
	return ev
}
