package preferences

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Track choices are kept per user and title, with an audio track's
// variant, an empty one forgets the title's, a new one replaces the
// variant, and a subtitle turned off keeps no language. Forgetting a
// user's choices, variants included, or resetting their preferences,
// leaves other users'.
func TestTrackChoicesPerUserAndTitle(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	ctx := t.Context()
	sam, kim := createUser(t, users, "sam").ID, createUser(t, users, "kim").ID
	series, movie, dubbed := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	japanese := TrackChoice{AudioLanguage: "jpn", Subtitle: SubtitleChoice{Language: "eng", HearingImpaired: true}}
	france := TrackChoice{AudioLanguage: "fre", AudioVariant: "fr-FR"}
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
	put(sam, dubbed, france)
	put(kim, series, japanese)
	check("kept", sam, map[accounts.ID]TrackChoice{series: japanese, movie: {Subtitle: SubtitleChoice{Off: true}}, dubbed: france})

	put(sam, movie, TrackChoice{})
	put(sam, dubbed, TrackChoice{AudioLanguage: "fre"})
	check("an empty choice, a variant replaced", sam, map[accounts.ID]TrackChoice{series: japanese, dubbed: {AudioLanguage: "fre"}})
	put(sam, dubbed, france)

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
