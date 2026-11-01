package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Now playing names where a play reads from among the sources its user
// uses (see addons.UsedBy): a file another user's listing remembered from
// an addon the player does not use is read from theirs.
func TestThePlaysOriginIsAmongItsUsersSources(t *testing.T) {
	t.Parallel()
	s := usedByOn(t)
	movie, err := accounts.ParseID(s.titles(t, s.bobToken, "Friends")["Movie"])
	if err != nil {
		t.Fatal(err)
	}
	// boss lists the movie last: the file One is remembered from Family
	// Picks, with Friends Stream's copy as its mirror.
	if _, err := s.library.Versions(t.Context(), s.bob, movie); err != nil {
		t.Fatal(err)
	}
	versions, err := s.library.Versions(t.Context(), s.boss, movie)
	if err != nil || len(versions) == 0 || versions[0].Filename != "One.mkv" {
		t.Fatalf("boss's versions: %+v %v", versions, err)
	}
	one := versions[0].ID
	if got := s.handler.origin(t.Context(), &LiveSession{User: s.bob}, one); got.Name != "Friends Stream" {
		t.Errorf("bob's play of One reads from %q", got.Name)
	}
	if got := s.handler.origin(t.Context(), &LiveSession{User: s.alice}, one); got.Name != "Family Picks" {
		t.Errorf("alice's play of One reads from %q", got.Name)
	}
}
