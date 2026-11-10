package accounts

import (
	"errors"
	"slices"
	"testing"
)

// A member keeps some sections as their own and follows the group in the
// others: the group's changes reach only those, a change to a followed
// section is refused unless the same request makes it own, following a
// section again takes the group's values back in it alone, and a reset
// gives a new user's values to the own sections only.
func TestMembersFollowTheirGroupSectionBySection(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	fresh := mustCreate(t, store, NewUser{Name: "fresh", Password: "correct horse"})
	group := kidsGroup(t, store)
	sam := mustCreate(t, store, NewUser{Name: "sam", Password: "correct horse", Group: &group.ID})

	if _, err := store.UpdateUser(ctx, sam.ID, UserChanges{OwnSections: &[]string{"name"}}, nil); !errors.Is(err, ErrInvalidOwnSections) {
		t.Errorf("an unknown section: %v", err)
	}
	sam, err := store.UpdateUser(ctx, sam.ID, UserChanges{OwnSections: &[]string{SectionGenres, SectionPlayback, SectionPlayback},
		LiveTv: new(true), MaxPlaybacks: new(5)}, nil)
	if err != nil || !slices.Equal(sam.OwnSections, []string{SectionPlayback, SectionGenres}) || !sam.LiveTv || sam.MaxPlaybacks != 5 ||
		sam.QualityGroup != 720 || !slices.Equal(sam.BlockedGenres, []string{"Horror"}) {
		t.Fatalf("making two sections own: %v %+v %v", sam.OwnSections, sam.Limits(), err)
	}
	if !sam.FollowsGroup(SectionAccess) || sam.FollowsGroup(SectionPlayback) {
		t.Errorf("FollowsGroup: %v", sam.OwnSections)
	}
	if _, err := store.UpdateUser(ctx, sam.ID, UserChanges{ContentDownloading: new(true)}, nil); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("changing a followed section: %v", err)
	}
	if got, err := store.UpdateUser(ctx, sam.ID, UserChanges{ContentDownloading: new(true), DetachFromGroup: true}, nil); err != nil ||
		!slices.Equal(got.OwnSections, []string{SectionAccess, SectionPlayback, SectionGenres}) || !got.ContentDownloading {
		t.Errorf("detaching from a changed section: %v %v", got.OwnSections, err)
	}
	if sam, err = store.UpdateUser(ctx, sam.ID, UserChanges{OwnSections: &[]string{SectionPlayback, SectionGenres}}, nil); err != nil ||
		sam.ContentDownloading || sam.MaxPlaybacks != 5 {
		t.Fatalf("following Access again: %v %+v %v", sam.OwnSections, sam.Limits(), err)
	}

	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{QualityGroup: new(1080), LiveTv: new(false),
		BlockedGenres: &[]string{"War"}, AccessSchedules: &[]AccessSchedule{}, ContentDownloading: new(true)}}); err != nil {
		t.Fatal(err)
	}
	got := mustUser(t, store, sam.ID)
	if got.QualityGroup != 720 || !got.LiveTv || !slices.Equal(got.BlockedGenres, []string{"Horror"}) ||
		len(got.AccessSchedules) != 0 || !got.ContentDownloading {
		t.Errorf("after a group change: %+v", got.Limits())
	}

	reset, err := store.ResetLimits(ctx, sam.ID)
	if err != nil || !slices.Equal(reset.OwnSections, []string{SectionPlayback, SectionGenres}) ||
		!reset.Limits().sameIn(fresh.Limits(), SectionPlayback) || !reset.Limits().sameIn(fresh.Limits(), SectionGenres) ||
		!reset.ContentDownloading || reset.Parental.MaxRating == nil || *reset.Parental.MaxRating != 13 {
		t.Errorf("a reset of the own sections: %+v %v", reset.Limits(), err)
	}

	group, _ = store.Group(ctx, group.ID)
	sam, err = store.UpdateUser(ctx, sam.ID, UserChanges{OwnSections: &[]string{SectionGenres}}, nil)
	if err != nil || sam.QualityGroup != 1080 || sam.LiveTv || !sam.Limits().sameIn(group.Limits, SectionPlayback) ||
		!slices.Equal(sam.BlockedGenres, fresh.BlockedGenres) {
		t.Errorf("following Playback again: %+v %v", sam.Limits(), err)
	}

	// Deleting the group keeps the values and clears the own sections.
	if _, err := store.DeleteGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := mustUser(t, store, sam.ID); got.GroupID != nil || len(got.OwnSections) != 0 || !got.Limits().equal(sam.Limits()) {
		t.Errorf("after the group's deletion: %v %+v", got.OwnSections, got.Limits())
	}
}
