package preferences

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Track choices are kept per user and title, an empty one forgets the
// title's, and a subtitle turned off keeps no language. Forgetting a
// user's choices, or resetting their preferences, leaves other users'.
func TestTrackChoicesPerUserAndTitle(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	ctx := t.Context()
	sam, kim := createUser(t, users, "sam").ID, createUser(t, users, "kim").ID
	series, movie := accounts.ID{1}, accounts.ID{2}
	japanese := TrackChoice{AudioLanguage: "jpn", Subtitle: SubtitleChoice{Language: "eng", HearingImpaired: true}}
	put := func(user, title accounts.ID, choice TrackChoice) {
		t.Helper()
		if err := store.PutTrackChoice(ctx, user, title, choice); err != nil {
			t.Fatal(err)
		}
	}
	check := func(when string, user accounts.ID, want map[accounts.ID]TrackChoice) {
		t.Helper()
		got, err := store.TrackChoices(ctx, user)
		if err != nil || !maps.Equal(got, want) {
			t.Errorf("%s: %v %v, want %v", when, got, err, want)
		}
	}
	put(sam, series, japanese)
	put(sam, movie, TrackChoice{Subtitle: SubtitleChoice{Off: true, Language: "fre"}})
	put(kim, series, japanese)
	check("kept", sam, map[accounts.ID]TrackChoice{series: japanese, movie: {Subtitle: SubtitleChoice{Off: true}}})

	put(sam, movie, TrackChoice{})
	check("an empty choice", sam, map[accounts.ID]TrackChoice{series: japanese})

	if err := store.ForgetTrackChoices(ctx, sam); err != nil {
		t.Fatal(err)
	}
	check("forgotten", sam, map[accounts.ID]TrackChoice{})
	check("another user's", kim, map[accounts.ID]TrackChoice{series: japanese})

	if err := store.Reset(ctx, kim); err != nil {
		t.Fatal(err)
	}
	check("after a reset", kim, map[accounts.ID]TrackChoice{})
}
