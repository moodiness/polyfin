package accounts

import (
	"errors"
	"slices"
	"testing"
)

func TestParentalControlAllowsAsJellyfinDoes(t *testing.T) {
	upToPG13 := ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}}
	upToR := ParentalControl{MaxRating: new(17), MaxSubRating: new(0)}
	for _, tc := range []struct {
		control       ParentalControl
		kind, rating  string
		allowed       bool
		justification string
	}{
		{ParentalControl{}, "Movie", "NC-17", true, "no limit"},
		{upToPG13, "Movie", "PG-13", true, "at the limit"},
		{upToPG13, "Movie", "pg", true, "below the limit, any case"},
		{upToPG13, "Movie", "R", false, "above the limit"},
		{upToPG13, "Series", "TV-14", false, "a TV rating above the limit"},
		{upToPG13, "Movie", "12", true, "a plain age below the limit"},
		{upToPG13, "Movie", "FSK 16", false, "a German rating above the limit"},
		{upToPG13, "Movie", "FR-12", true, "a French rating with its country"},
		{upToPG13, "Movie", "", false, "unrated movies are blocked"},
		{upToPG13, "Movie", "NR", false, "a rating saying the movie is not rated"},
		{upToPG13, "Movie", "Approved by nobody", false, "an unknown rating counts as none"},
		{upToPG13, "Series", "", true, "unrated series are not blocked"},
		{upToR, "Movie", "R", true, "the limit's subscore"},
		{upToR, "Movie", "NC-17", false, "above the limit's subscore"},
		{upToR, "Series", "TV-MA", false, "TV-MA warns of more than R"},
		{upToR, "Movie", "R18", false, "adult only"},
		{ParentalControl{BlockUnrated: []string{"Movie"}}, "Movie", "XXX", true, "rated, without a limit"},
	} {
		if got := tc.control.Allows(tc.kind, tc.rating); got != tc.allowed {
			t.Errorf("%s (%s %q): allowed=%v", tc.justification, tc.kind, tc.rating, got)
		}
	}
}

func TestParentalControlIsStoredNormalized(t *testing.T) {
	store := newStore(t)
	user := mustCreate(t, store, NewUser{Name: "child", Password: "correct horse"})
	if user.Parental.Restricted() || user.Parental.BlockUnrated == nil {
		t.Fatalf("new user's parental control: %+v", user.Parental)
	}
	updated, err := store.UpdateUser(t.Context(), user.ID, UserChanges{Parental: &ParentalControl{
		MaxRating: new(13), MaxSubRating: new(0), BlockUnrated: []string{"series", "MOVIE", "Series"},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(updated.Parental.BlockUnrated, []string{"Movie", "Series"}) || *updated.Parental.MaxRating != 13 {
		t.Errorf("stored control: %+v", updated.Parental)
	}
	// Devices load the user with the control.
	token, _, err := store.SignInDevice(t.Context(), user.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, signed, err := store.DeviceByToken(t.Context(), token, "192.0.2.1"); err != nil || !signed.Parental.Restricted() {
		t.Errorf("signed-in user's control: %+v %v", signed.Parental, err)
	}
	// A subscore without a limit limits nothing.
	updated, _ = store.UpdateUser(t.Context(), user.ID, UserChanges{Parental: &ParentalControl{MaxSubRating: new(1)}}, nil)
	if updated.Parental.Restricted() || updated.Parental.MaxSubRating != nil {
		t.Errorf("subscore alone: %+v", updated.Parental)
	}
	if _, err := store.UpdateUser(t.Context(), user.ID, UserChanges{Parental: &ParentalControl{BlockUnrated: []string{"Film"}}}, nil); !errors.Is(err, ErrInvalidParentalControl) {
		t.Errorf("unknown kind: %v", err)
	}
}
