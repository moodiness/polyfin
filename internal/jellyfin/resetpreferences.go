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
