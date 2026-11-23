package silotest

import (
	"fmt"
	"slices"
)

// Not interested. The Apple apps offer it on a movie's or series' card and
// page whenever the card's user_state carries not_interested, whatever the
// server's discovery document says: MediaCardPersonalState
// .offersNotInterested, the More menu's NotInterestedMenuButton
// (Components/NotInterestedMenuButton.swift), which draws nothing while it
// is absent, and NotInterestedSync.canMark
// (Components/PersonalListActions.swift), movies and series only. The mark
// goes with PUT /api/v2/not-interested/{item_id} and is cleared with
// DELETE, either taking any answer but 204 as a failure
// (APIv2Client.setNotInterested and setMembership,
// Networking/APIv2/APIv2Client.swift).

// markableTypes are the card types NotInterestedSync.canMark takes.
var markableTypes = []string{"movie", "series"}

// CheckNotInterested checks the user_state of cards, decoded from a page
// or a detail, against the marks expected, by content_id: every card's
// user_state carries not_interested, the contract requiring it, true for a
// movie or series marked, false for the others, so the apps offer the mark
// where it can be set and show it where it is.
func CheckNotInterested(cards []map[string]any, marked []string) error {
	c := &checker{}
	for _, card := range cards {
		id, _ := card["content_id"].(string)
		kind, _ := card["type"].(string)
		state, ok := card["user_state"].(object)
		if !ok {
			continue
		}
		value, sent := state["not_interested"].(bool)
		if !sent {
			c.fail("%s: user_state carries no not_interested, and the apps offer no mark", id)
			continue
		}
		if want := slices.Contains(marked, id) && slices.Contains(markableTypes, kind); value != want {
			c.fail("%s (%s): not_interested %v, want %v", id, kind, value, want)
		}
	}
	return c.err()
}

// CheckMarkAnswer checks the answer to a mark or its clearing: the apps
// take only 204.
func CheckMarkAnswer(status int) error {
	if status != 204 {
		return fmt.Errorf("the apps take %d as a failure", status)
	}
	return nil
}
