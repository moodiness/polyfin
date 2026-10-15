package accounts

import (
	"errors"
	"slices"
	"testing"
)

// kidsGroup is a group with limits set in every section, and the limits.
func kidsGroup(t *testing.T, store *Store) Group {
	t.Helper()
	group, err := store.CreateGroup(t.Context(), "Kids", nil)
	if err != nil {
		t.Fatal(err)
	}
	library := ID{1}
	group, err = store.UpdateGroup(t.Context(), group.ID, GroupChanges{Limits: UserChanges{
		Parental:           &ParentalControl{MaxRating: new(13), BlockUnrated: []string{"movie"}},
		ContentDownloading: new(false),
		MaxPlaybacks:       new(2),
		QualityGroup:       new(720),
		HiddenLibraries:    &[]ID{library},
		BlockedGenres:      &[]string{"Horror"},
		AccessSchedules:    &[]AccessSchedule{{Day: "weekday", StartHour: 8, EndHour: 20}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return group
}

func mustUser(t *testing.T, store *Store, id ID) User {
	t.Helper()
	user, err := store.User(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestNewGroupsStartAsANewUserOrCopyOne(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	plain := mustCreate(t, store, NewUser{Name: "plain", Password: "correct horse"})
	group, err := store.CreateGroup(ctx, "  Family ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "Family" || group.Members != 0 || !group.Limits.equal(plain.Limits()) {
		t.Errorf("new group: %+v, want the limits of a new user %+v", group, plain.Limits())
	}

	model := mustCreate(t, store, NewUser{Name: "model", Password: "correct horse"})
	model, err = store.UpdateUser(ctx, model.ID, UserChanges{MaxBitrate: new(4_000_000), CanShare: new(true),
		BlockedGenres: &[]string{"Crime"}, VideoTranscoding: new(false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := store.CreateGroup(ctx, "Copied", &model.ID)
	if err != nil || !copied.Limits.equal(model.Limits()) {
		t.Errorf("copied group: %+v %v, want %+v", copied.Limits, err, model.Limits())
	}

	for name, want := range map[string]error{"family": ErrGroupNameTaken, " ": ErrInvalidGroupName, "a\nb": ErrInvalidGroupName} {
		if _, err := store.CreateGroup(ctx, name, nil); !errors.Is(err, want) {
			t.Errorf("group named %q: %v, want %v", name, err, want)
		}
	}
	if _, err := store.CreateGroup(ctx, "Nobody's", &ID{9}); !errors.Is(err, ErrInvalidGroupCopy) {
		t.Errorf("copying no user: %v", err)
	}
	if _, err := store.UpdateGroup(ctx, copied.ID, GroupChanges{Name: new("FAMILY")}); !errors.Is(err, ErrGroupNameTaken) {
		t.Errorf("renaming to a name taken: %v", err)
	}
	if _, err := store.UpdateGroup(ctx, copied.ID, GroupChanges{Limits: UserChanges{MaxPlaybacks: new(99)}}); !errors.Is(err, ErrInvalidMaxPlaybacks) {
		t.Errorf("a limit out of range: %v", err)
	}
	groups, err := store.Groups(ctx)
	if err != nil || len(groups) != 2 || groups[0].Name != "Copied" || groups[1].Name != "Family" {
		t.Errorf("groups: %+v %v", groups, err)
	}
}

func TestGroupChangesReachEveryFollowingMemberAtOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	var members []User
	for _, name := range []string{"ann", "ben", "cat"} {
		members = append(members, mustCreate(t, store, NewUser{Name: name, Password: "correct horse", Group: &group.ID}))
	}
	for _, member := range members {
		if member.GroupID == nil || *member.GroupID != group.ID || !member.FollowGroup || !member.Limits().equal(group.Limits) {
			t.Fatalf("%s joining: %+v", member.Name, member)
		}
	}
	// Cat keeps her own settings.
	cat, err := store.UpdateUser(ctx, members[2].ID, UserChanges{FollowGroup: new(false)}, nil)
	if err != nil || cat.FollowGroup || !cat.Limits().equal(group.Limits) {
		t.Fatalf("detaching: %+v %v", cat, err)
	}
	// Ann's app is signed in, and resolved once.
	token, _, err := store.SignInDevice(ctx, members[0].ID, device("ann-tablet"))
	if err != nil {
		t.Fatal(err)
	}
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.ContentDownloading {
		t.Fatalf("before: %+v %v", user, err)
	}

	changed, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{
		ContentDownloading: new(true),
		AccessSchedules:    &[]AccessSchedule{{Day: "Everyday", StartHour: 7, EndHour: 21}},
		Parental:           &ParentalControl{MaxRating: new(10)},
	}})
	if err != nil || changed.Members != 3 {
		t.Fatalf("changing the group: %+v %v", changed, err)
	}
	for _, member := range members[:2] {
		if got := mustUser(t, store, member.ID); !got.FollowGroup || !got.Limits().equal(changed.Limits) {
			t.Errorf("%s after the change: %+v, want %+v", member.Name, got.Limits(), changed.Limits)
		}
	}
	if got := mustUser(t, store, cat.ID); !got.Limits().equal(group.Limits) {
		t.Errorf("cat, detached, changed: %+v", got.Limits())
	}
	// Ann's app has the group's values at once.
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || !user.ContentDownloading ||
		!slices.Equal(user.AccessSchedules, []AccessSchedule{{Day: "Everyday", StartHour: 7, EndHour: 21}}) {
		t.Errorf("signed-in app after the change: %+v %v", user, err)
	}
}

func TestDetachedMembersKeepTheirValuesAndFollowingAgainRestoresTheGroups(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	sam := mustCreate(t, store, NewUser{Name: "sam", Password: "correct horse", Group: &group.ID})

	// While Sam follows the group, a setting it holds is the group's.
	if _, err := store.UpdateUser(ctx, sam.ID, UserChanges{MaxPlaybacks: new(5)}, nil); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("changing a group setting while following: %v", err)
	}
	// The group's own value, or a setting no group holds, are fine.
	if got, err := store.UpdateUser(ctx, sam.ID, UserChanges{MaxPlaybacks: new(2), IsHidden: new(true)}, nil); err != nil || !got.FollowGroup {
		t.Errorf("same value: %+v %v", got, err)
	}

	sam, err := store.UpdateUser(ctx, sam.ID, UserChanges{FollowGroup: new(false), MaxPlaybacks: new(5), BlockedGenres: &[]string{}}, nil)
	if err != nil || sam.FollowGroup || sam.MaxPlaybacks != 5 || len(sam.BlockedGenres) != 0 || sam.QualityGroup != 720 {
		t.Fatalf("detaching with changes: %+v %v", sam, err)
	}
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{QualityGroup: new(1080)}}); err != nil {
		t.Fatal(err)
	}
	if got := mustUser(t, store, sam.ID); got.QualityGroup != 720 || got.MaxPlaybacks != 5 {
		t.Errorf("detached after a group change: %+v", got.Limits())
	}
	// Following again takes the group's values back.
	sam, err = store.UpdateUser(ctx, sam.ID, UserChanges{FollowGroup: new(true)}, nil)
	group, _ = store.Group(ctx, group.ID)
	if err != nil || !sam.FollowGroup || !sam.Limits().equal(group.Limits) || sam.QualityGroup != 1080 {
		t.Errorf("following again: %+v %v", sam.Limits(), err)
	}
	// Leaving keeps the values as Sam's own.
	sam, err = store.UpdateUser(ctx, sam.ID, UserChanges{Group: &ID{}}, nil)
	if err != nil || sam.GroupID != nil || !sam.FollowGroup || !sam.Limits().equal(group.Limits) {
		t.Errorf("leaving: %+v %v", sam, err)
	}
	if _, err := store.UpdateUser(ctx, sam.ID, UserChanges{Group: &ID{7}}, nil); !errors.Is(err, ErrInvalidGroup) {
		t.Errorf("joining no group: %v", err)
	}
}

func TestChangesFromAppsDetachOnlyWhenAGroupSettingDiffers(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	sam := mustCreate(t, store, NewUser{Name: "sam", Password: "correct horse", Group: &group.ID})
	// The whole policy, as read, with only IsDisabled changed.
	whole := func(user User) UserChanges {
		l := user.Limits()
		return UserChanges{IsHidden: &user.IsHidden, IsDisabled: new(true), Parental: &l.Parental, VideoTranscoding: &l.VideoTranscoding,
			AudioTranscoding: &l.AudioTranscoding, ContentDownloading: &l.ContentDownloading, MaxPlaybacks: &l.MaxPlaybacks,
			MaxBitrate: &l.MaxBitrate, LiveTv: &l.LiveTv, SyncPlay: &l.SyncPlay, RemoteControl: &l.RemoteControl,
			HiddenLibraries: &l.HiddenLibraries, AccessSchedules: &l.AccessSchedules, CollectionManagement: &l.CollectionManagement,
			SubtitleManagement: &l.SubtitleManagement, LiveTvManagement: &l.LiveTvManagement, DetachFromGroup: true}
	}
	got, err := store.UpdateUser(ctx, sam.ID, whole(sam), nil)
	if err != nil || !got.IsDisabled || !got.FollowGroup {
		t.Fatalf("disabling with the policy as read: %+v %v", got, err)
	}
	changes := whole(got)
	changes.IsDisabled = new(false)
	changes.ContentDownloading = new(true)
	got, err = store.UpdateUser(ctx, sam.ID, changes, nil)
	if err != nil || got.FollowGroup || got.GroupID == nil || !got.ContentDownloading || got.IsDisabled || got.MaxPlaybacks != 2 {
		t.Errorf("a group setting changed: %+v %v", got, err)
	}
}

func TestAdministratorsNeverJoinAGroup(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	admin := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	member := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})

	if _, err := store.UpdateUser(ctx, admin.ID, UserChanges{Group: &group.ID}, nil); !errors.Is(err, ErrAdministratorGroup) {
		t.Errorf("administrator joining: %v", err)
	}
	if _, err := store.CreateUser(ctx, NewUser{Name: "boss", Password: "correct horse", IsAdministrator: true, Group: &group.ID}); !errors.Is(err, ErrAdministratorGroup) {
		t.Errorf("administrator created in a group: %v", err)
	}
	if _, err := store.AddGroupMembers(ctx, group.ID, []ID{member.ID, admin.ID}); !errors.Is(err, ErrAdministratorGroup) {
		t.Errorf("adding an administrator: %v", err)
	}
	if got := mustUser(t, store, member.ID); got.GroupID != nil {
		t.Errorf("added with an administrator: %+v", got.GroupID)
	}
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{IsAdministrator: new(true), Group: &group.ID}, nil); !errors.Is(err, ErrAdministratorGroup) {
		t.Errorf("made an administrator while joining: %v", err)
	}
	if _, err := store.db.Exec(ctx, "UPDATE users SET group_id = $2 WHERE id = $1", admin.ID, group.ID); err == nil {
		t.Error("the database took an administrator in a group")
	}

	joined, err := store.AddGroupMembers(ctx, group.ID, []ID{member.ID, member.ID})
	if err != nil || len(joined) != 1 || !joined[0].Limits().equal(group.Limits) {
		t.Fatalf("adding the member: %+v %v", joined, err)
	}
	// Made an administrator, the member leaves the group, keeping its
	// values.
	promoted, err := store.UpdateUser(ctx, member.ID, UserChanges{IsAdministrator: new(true)}, nil)
	if err != nil || promoted.GroupID != nil || promoted.MaxPlaybacks != 2 {
		t.Errorf("made an administrator: %+v %v", promoted, err)
	}
}

func TestDeletingAGroupLeavesItsMembersLimitsUnchanged(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	ann := mustCreate(t, store, NewUser{Name: "ann", Password: "correct horse", Group: &group.ID})
	ben := mustCreate(t, store, NewUser{Name: "ben", Password: "correct horse", Group: &group.ID})
	ben, err := store.UpdateUser(ctx, ben.ID, UserChanges{FollowGroup: new(false), MaxPlaybacks: new(4)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	invite, _, err := store.CreateInvite(ctx, NewInvite{MaxUses: 1, Group: &group.ID, CreatedBy: ann.ID})
	if err != nil || invite.Group == nil || invite.Group.Name != "Kids" {
		t.Fatalf("invite: %+v %v", invite, err)
	}

	deleted, err := store.DeleteGroup(ctx, group.ID)
	if err != nil || deleted.Name != "Kids" || deleted.Members != 2 {
		t.Fatalf("deleting: %+v %v", deleted, err)
	}
	for _, before := range []User{ann, ben} {
		after := mustUser(t, store, before.ID)
		if after.GroupID != nil || !after.FollowGroup || !after.Limits().equal(before.Limits()) {
			t.Errorf("%s after the deletion: %+v, want %+v", before.Name, after.Limits(), before.Limits())
		}
	}
	if invites, err := store.Invites(ctx); err != nil || len(invites) != 1 || invites[0].Group != nil {
		t.Errorf("invite after the deletion: %+v %v", invites, err)
	}
	if _, err := store.DeleteGroup(ctx, group.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting again: %v", err)
	}
}

func TestInvitesNamingAGroupPutTheirAccountsInIt(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group := kidsGroup(t, store)
	admin := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	model := mustCreate(t, store, NewUser{Name: "model", Password: "correct horse"})
	model, err := store.UpdateUser(ctx, model.ID, UserChanges{IsHidden: new(false), MaxBitrate: new(3_000_000)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateInvite(ctx, NewInvite{MaxUses: 1, Group: &ID{5}, CreatedBy: admin.ID}); !errors.Is(err, ErrInvalidGroup) {
		t.Errorf("an invite naming no group: %v", err)
	}
	_, token, err := store.CreateInvite(ctx, NewInvite{MaxUses: 2, Group: &group.ID, Model: &model.ID, CreatedBy: admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	guest, invite, err := store.AcceptInvite(ctx, token, "guest", "correct horse")
	if err != nil || invite.Group == nil || invite.Group.ID != group.ID {
		t.Fatalf("accepting: %+v %v", invite, err)
	}
	// The model's other settings stay; the group's are the group's.
	if guest.GroupID == nil || *guest.GroupID != group.ID || !guest.FollowGroup || !guest.Limits().equal(group.Limits) || guest.IsHidden {
		t.Errorf("guest: %+v", guest)
	}
	if members, err := store.GroupMembers(ctx, group.ID); err != nil || len(members) != 1 || members[0].ID != guest.ID {
		t.Errorf("members: %+v %v", members, err)
	}
}

func TestTurningDownloadsOffKeepsMembersFollowing(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	group, err := store.CreateGroup(ctx, "Kids", nil)
	if err != nil {
		t.Fatal(err)
	}
	sam := mustCreate(t, store, NewUser{Name: "sam", Password: "correct horse", Group: &group.ID})
	users, groups, err := store.TurnOffDownloads(ctx)
	if err != nil || len(users) != 1 || len(groups) != 1 || groups[0].ContentDownloading {
		t.Fatalf("turning off: %d users, %+v, %v", len(users), groups, err)
	}
	if got := mustUser(t, store, sam.ID); got.ContentDownloading || !got.FollowGroup {
		t.Errorf("sam: %+v", got)
	}
}
