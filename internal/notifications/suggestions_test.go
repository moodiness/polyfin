package notifications

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
)

// A home row suggested to a user is told to the user's own targets that
// chose it, never to the server's, which cannot, nor to administrators'.
// It names the administrator who signs its note, and gives the note as its
// message; without one, it says the row is on the home page. It opens the
// web player's home page.
func TestSuggestedRowsReachTheUsersOwnTargetsOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := t.Context()
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(ctx, nil, h.webhook("/server", TitlesSuggested)); !errors.Is(err, ErrInvalidEvents) {
		t.Errorf("a server target choosing suggestions: %v", err)
	}
	if slices.Contains(EventsFor(nil), TitlesSuggested) || !slices.Contains(EventsFor(&h.member), TitlesSuggested) {
		t.Errorf("events the server's and members' targets choose from: %v, %v", EventsFor(nil), EventsFor(&h.member))
	}
	h.add(t, nil, h.webhook("/server", EventsFor(nil)...))
	h.add(t, &h.admin, h.webhook("/admin", EventsFor(&h.admin)...))
	h.add(t, &h.member, h.webhook("/member", TitlesSuggested))

	row := homerows.Row{ID: accounts.ID{0xc1}, Name: "For the weekend", Note: "Start with the first one."}
	title, message := h.TitlesSuggested(h.member, homerows.Suggestion{Row: row, From: h.admin})
	if title != "admin suggests: For the weekend" || message != "Start with the first one." {
		t.Errorf("the words for the apps: %q, %q", title, message)
	}
	member := h.targets.wait(t, "/member", 1)[0].body
	h.idle(t)
	if got := len(h.targets.at("/server")) + len(h.targets.at("/admin")); got != 0 {
		t.Errorf("%d messages to targets that are not told", got)
	}
	suggestion, _ := member["suggestion"].(map[string]any)
	from, _ := suggestion["from"].(map[string]any)
	who, _ := member["user"].(map[string]any)
	if member["type"] != TitlesSuggested || member["title"] != title || member["message"] != message || who["name"] != "member" ||
		member["url"] != "https://media.example.org/web/#/home" {
		t.Errorf("the member's message: %v", member)
	}
	if suggestion["rowId"] != row.ID.String() || suggestion["name"] != "For the weekend" || suggestion["note"] != "Start with the first one." ||
		from["name"] != "admin" || from["id"] != h.admin.ID.String() {
		t.Errorf("the suggestion told: %v", suggestion)
	}

	title, message = h.SuggestionWords(homerows.Suggestion{Row: homerows.Row{Name: "Picks"}})
	if title != "Suggested for you: Picks" || message != "Picks is on your home page." {
		t.Errorf("the words without a note: %q, %q", title, message)
	}
}
