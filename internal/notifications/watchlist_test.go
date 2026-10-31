package notifications

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// titleLibrary describes one series, and no other title.
type titleLibrary struct {
	fakeLibrary
	series library.Item
}

func (l *titleLibrary) Item(_ context.Context, _ accounts.User, id accounts.ID) (library.Item, error) {
	if id == l.series.ID {
		return l.series, nil
	}
	return library.Item{}, library.ErrNotFound
}

// A title of a user's watchlist found available is told to the user's own
// targets that chose it, never to the server's, which cannot, nor to
// administrators'; a title the user may no longer see is not told of.
func TestWatchlistTitlesFoundAvailableReachTheUsersOwnTargetsOnly(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	series := library.Item{ID: accounts.ID{0xa7}, Kind: library.KindSeries, Name: "Example Show", ProductionYear: 2024,
		ProviderIDs: map[string]string{"Imdb": "tt0000007"}}
	h.Service.library = &titleLibrary{series: series}
	if _, err := h.Create(ctx, nil, h.webhook("/server", TitleAvailable)); !errors.Is(err, ErrInvalidEvents) {
		t.Errorf("a server target choosing titles found available: %v", err)
	}
	if slices.Contains(EventsFor(nil), TitleAvailable) || !slices.Contains(EventsFor(&h.member), TitleAvailable) {
		t.Errorf("events the server's and members' targets choose from: %v, %v", EventsFor(nil), EventsFor(&h.member))
	}
	h.add(t, nil, h.webhook("/server", EventsFor(nil)...))
	h.add(t, &h.admin, h.webhook("/admin", EventsFor(&h.admin)...))
	h.add(t, &h.member, h.webhook("/member", TitleAvailable))

	h.WatchlistTitleAvailable(ctx, h.member, accounts.ID{0xb8})
	h.WatchlistTitleAvailable(ctx, h.member, series.ID)
	member := h.targets.wait(t, "/member", 1)[0].body
	h.idle(t)
	if got := len(h.targets.at("/member")); got != 1 {
		t.Errorf("the member was told %d times", got)
	}
	if got := len(h.targets.at("/server")) + len(h.targets.at("/admin")); got != 0 {
		t.Errorf("%d messages to targets that are not told", got)
	}
	title, _ := member["watchlistTitle"].(map[string]any)
	who, _ := member["user"].(map[string]any)
	if member["type"] != TitleAvailable || member["title"] == "" || member["message"] == "" || who["name"] != "member" ||
		member["url"] != "https://media.example.org/web/#/details?id="+series.ID.String()+"&serverId=fedcba9876543210fedcba9876543210" {
		t.Errorf("the member's message: %v", member)
	}
	ids, _ := title["providerIds"].(map[string]any)
	if title["id"] != series.ID.String() || title["kind"] != "series" || title["name"] != "Example Show" || title["year"] != 2024.0 ||
		ids["Imdb"] != "tt0000007" {
		t.Errorf("the title told: %v", title)
	}
}
