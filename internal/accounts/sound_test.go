package accounts

import (
	"errors"
	"testing"
)

// Boost the volume and Convert the sound are off for a new user and a new
// group, are the user's to change, are held by a group for the members
// who follow it, are copied into a group made from a user, and go back off
// with Reset to defaults.
func TestSoundOptionsAreGroupSettings(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user, err := store.CreateUser(ctx, NewUser{Name: "sam", Password: "correct horse"})
	if err != nil || user.BoostVolume || user.ConvertSound {
		t.Fatalf("a new user: %+v %v", user, err)
	}
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{BoostVolume: new(true)}, nil); err != nil || !user.BoostVolume || user.ConvertSound {
		t.Fatalf("boosting: %+v %v", user, err)
	}
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{ConvertSound: new(true)}, nil); err != nil || !user.BoostVolume || !user.ConvertSound {
		t.Fatalf("converting: %+v %v", user, err)
	}
	if copied, err := store.CreateGroup(ctx, "Copied", &user.ID); err != nil || !copied.BoostVolume || !copied.ConvertSound {
		t.Errorf("a group made from the user: %+v %v", copied, err)
	}

	group, err := store.CreateGroup(ctx, "Home", nil)
	if err != nil || group.BoostVolume || group.ConvertSound {
		t.Fatalf("a new group: %+v %v", group, err)
	}
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{ConvertSound: new(true)}}); err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser(ctx, NewUser{Name: "kim", Password: "correct horse", Group: &group.ID})
	if err != nil || !member.ConvertSound || member.BoostVolume || !member.FollowGroup {
		t.Fatalf("a member: %+v %v", member, err)
	}
	// A following member's own change is refused; one keeping the group's
	// value is not.
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{BoostVolume: new(true)}, nil); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("a following member's change: %v", err)
	}
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{ConvertSound: new(true)}, nil); err != nil {
		t.Errorf("a following member keeping the group's value: %v", err)
	}
	if _, err := store.ResetGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if member, err = store.User(ctx, member.ID); err != nil || member.ConvertSound {
		t.Errorf("the member once the group is reset: %+v %v", member, err)
	}

	if user, err = store.ResetLimits(ctx, user.ID); err != nil || user.BoostVolume || user.ConvertSound {
		t.Errorf("reset to defaults: %+v %v", user, err)
	}
}
