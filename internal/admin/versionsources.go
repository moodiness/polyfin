package admin

import (
	"context"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// versionSourceJSON is one of the server's sources listing versions of
// movies and episodes, as Order of sources shows it.
type versionSourceJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// versionSources lists the server's sources that list versions of movies
// and episodes, addons and folders, in the order of Content › Sources.
func (h *handler) versionSources(ctx context.Context) ([]addons.Addon, error) {
	installed, err := h.Addons.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(installed, func(addon addons.Addon) bool { return !addon.ListsVersions() }), nil
}

// placedSources returns the sources of order still among sources, in the
// order's order: a source removed since leaves it.
func placedSources(order []accounts.ID, sources []addons.Addon) []accounts.ID {
	placed := []accounts.ID{}
	for _, id := range order {
		if slices.ContainsFunc(sources, func(addon addons.Addon) bool { return addon.ID == id }) {
			placed = append(placed, id)
		}
	}
	return placed
}

// versionSourceOrder reads Order of sources from a settings save: left
// out, it keeps current, without the sources removed since; else each of
// its sources must be one of sources. ok is false for an unknown source;
// a source listed twice is the settings' to refuse.
func versionSourceOrder(raw *[]string, current []accounts.ID, sources []addons.Addon) (order []accounts.ID, ok bool) {
	if raw == nil {
		return placedSources(current, sources), true
	}
	order = make([]accounts.ID, 0, len(*raw))
	for _, text := range *raw {
		id, err := accounts.ParseID(text)
		if err != nil || !slices.ContainsFunc(sources, func(addon addons.Addon) bool { return addon.ID == id }) {
			return nil, false
		}
		order = append(order, id)
	}
	return order, true
}

// sourceIDs returns sources' identifiers as the admin API writes them.
func sourceIDs(ids []accounts.ID) []string {
	texts := make([]string, len(ids))
	for i, id := range ids {
		texts[i] = id.String()
	}
	return texts
}
