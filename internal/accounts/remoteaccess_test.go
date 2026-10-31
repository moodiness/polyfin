package accounts

import (
	"errors"
	"testing"
)

// A user may connect from the local network always, from outside it only
// with remote access.
func TestAllowedFrom(t *testing.T) {
	for _, tc := range []struct {
		remote, local, allowed bool
	}{
		{true, true, true},
		{true, false, true},
		{false, true, true},
		{false, false, false},
	} {
		if got := (User{RemoteAccess: tc.remote}).AllowedFrom(tc.local); got != tc.allowed {
			t.Errorf("remote access %v, local %v: allowed=%v", tc.remote, tc.local, got)
		}
	}
}

// Remote access is on for a new user and a new group, is stored with the
// user and read by their devices, is held by a group for the members who
// follow it, is copied into a group made from a user, and comes back with
// Reset to defaults.
func TestRemoteAccessIsAGroupSetting(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user, err := store.CreateUser(ctx, NewUser{Name: "kid", Password: "correct horse"})
	if err != nil || !user.RemoteAccess {
		t.Fatalf("a new user: %+v %v", user, err)
	}
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{RemoteAccess: new(false)}, nil); err != nil || user.RemoteAccess {
		t.Fatalf("taking it away: %+v %v", user, err)
	}
	token, _, err := store.SignInDevice(ctx, user.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, signed, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || signed.RemoteAccess {
		t.Errorf("the signed-in user: %+v %v", signed, err)
	}

	copied, err := store.CreateGroup(ctx, "Copied", &user.ID)
	if err != nil || copied.RemoteAccess {
		t.Errorf("a group made from the user: %+v %v", copied, err)
	}
	group, err := store.CreateGroup(ctx, "Home only", nil)
	if err != nil || !group.RemoteAccess {
		t.Fatalf("a new group: %+v %v", group, err)
	}
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{RemoteAccess: new(false)}}); err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser(ctx, NewUser{Name: "cat", Password: "correct horse", Group: &group.ID})
	if err != nil || member.RemoteAccess || !member.FollowGroup {
		t.Fatalf("a member: %+v %v", member, err)
	}
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{RemoteAccess: new(true)}}); err != nil {
		t.Fatal(err)
	}
	if member, err = store.User(ctx, member.ID); err != nil || !member.RemoteAccess {
		t.Errorf("the member once the group allows it: %+v %v", member, err)
	}
	// A following member's own change is refused, unless it detaches them.
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{RemoteAccess: new(false)}, nil); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("a following member's change: %v", err)
	}

	if user, err = store.ResetLimits(ctx, user.ID); err != nil || !user.RemoteAccess {
		t.Errorf("reset to defaults: %+v %v", user, err)
	}
}
