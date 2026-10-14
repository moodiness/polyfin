package notifications

import (
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ShareJSON is the share link a guest played a title through: its ID, its
// title as Jellyfin apps know it, of Kind "movie" or "episode", named as
// when the link was made, and when the link expires.
type ShareJSON struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"itemId"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// shareEvent tells that a guest played the title of link.
func (s *Service) shareEvent(link accounts.ShareLink) Event {
	ev := s.newEvent(SharePlayed)
	ev.User = &UserJSON{ID: link.User.ID.String(), Name: link.User.Name}
	ev.Share = &ShareJSON{ID: link.ID.String(), ItemID: link.Item.String(), Kind: link.Kind, Name: link.Title, ExpiresAt: link.ExpiresAt.UTC()}
	ev.Title = s.phrase("Shared link played: %s", "Lien partagé lu : %s", link.Title)
	ev.Message = s.phrase("A guest played %[2]s through the link shared by %[1]s.",
		"Un invité a lu %[2]s grâce au lien partagé par %[1]s.", link.User.Name, link.Title)
	ev.URL = s.adminLink("/me/account#shared-links")
	return ev
}

// SharePlayed tells the targets of the user who made link, and the
// server's, that a guest played its title. Callers tell it at most once an
// hour per link (see accounts.Store.ShareLinkToNotify). It returns at
// once: the message is sent in the background.
func (s *Service) SharePlayed(link accounts.ShareLink) {
	ev := s.shareEvent(link)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		s.dispatch(ev, func(t target) bool { return t.owner == nil || *t.owner == link.User.ID })
	})
}
