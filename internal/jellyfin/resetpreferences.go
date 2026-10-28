package jellyfin

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ResetPreferences forgets the display preferences and the configuration
// user saved from Jellyfin apps, with the configuration remembered for
// them: their apps read a new user's from their next request (My account ›
// Reset my preferences).
func (h *Handler) ResetPreferences(ctx context.Context, user accounts.ID) error {
	err := h.Preferences.Reset(ctx, user)
	h.configurations.Delete(user)
	return err
}

// LibraryOrders counts the users who keep an order of libraries of their
// own, saved from a Jellyfin app such as jellyfin-web's Settings › Home,
// which wins over the order of Content › Libraries (see arrangeViews).
func (h *Handler) LibraryOrders(ctx context.Context) (int, error) {
	return h.Preferences.LibraryOrders(ctx)
}

// ClearLibraryOrders forgets the order of libraries every user saved, with
// the configuration remembered for them, so that they all follow the
// order of Content › Libraries, and returns how many users it changed.
// The rest of their configuration stays.
func (h *Handler) ClearLibraryOrders(ctx context.Context) (int, error) {
	users, err := h.Preferences.ClearLibraryOrders(ctx)
	for _, user := range users {
		h.configurations.Delete(user)
	}
	return len(users), err
}
