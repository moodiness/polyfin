package accounts

import (
	"errors"
	"testing"
)

// Play local files first is off for a new user and a new group, is held
// by a group for the members who follow it, is copied into a group made
// from a user, and goes back off with Reset to defaults, for a user and
// for a group.
func TestLocalFirstIsAGroupSetting(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user, err := store.CreateUser(ctx, NewUser{Name: "sam", Password: "correct horse"})
	if err != nil || user.LocalFirst {
		t.Fatalf("a new user: %+v %v", user, err)
	}
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{LocalFirst: new(true)}, nil); err != nil || !user.LocalFirst {
		t.Fatalf("giving it: %+v %v", user, err)
	}
	if copied, err := store.CreateGroup(ctx, "Copied", &user.ID); err != nil || !copied.LocalFirst {
		t.Errorf("a group made from the user: %+v %v", copied, err)
	}

	group, err := store.CreateGroup(ctx, "Home", nil)
	if err != nil || group.LocalFirst {
		t.Fatalf("a new group: %+v %v", group, err)
	}
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{LocalFirst: new(true)}}); err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser(ctx, NewUser{Name: "kim", Password: "correct horse", Group: &group.ID})
	if err != nil || !member.LocalFirst || !member.FollowGroup {
		t.Fatalf("a member: %+v %v", member, err)
	}
	// A following member's own change is refused, unless it detaches them.
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{LocalFirst: new(false)}, nil); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("a following member's change: %v", err)
	}
	if _, err := store.ResetGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if member, err = store.User(ctx, member.ID); err != nil || member.LocalFirst {
		t.Errorf("the member once the group is reset: %+v %v", member, err)
	}

	if user, err = store.ResetLimits(ctx, user.ID); err != nil || user.LocalFirst {
		t.Errorf("reset to defaults: %+v %v", user, err)
	}
}
