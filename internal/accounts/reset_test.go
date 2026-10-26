package accounts

import (
	"errors"
	"testing"
	"time"
)

// everyLimit changes a user's limits in every section a group holds, away
// from a new user's.
var everyLimit = UserChanges{
	Parental:             &ParentalControl{MaxRating: new(13), BlockUnrated: []string{"movie"}},
	VideoTranscoding:     new(false),
	AudioTranscoding:     new(false),
	ContentDownloading:   new(false),
	PersonalAddons:       new(false),
	MaxPlaybacks:         new(2),
	MaxBitrate:           new(4_000_000),
	LiveTv:               new(false),
	SyncPlay:             new(SyncPlayNone),
	HiddenLibraries:      &[]ID{{1}},
	BlockedGenres:        &[]string{"Horror"},
	AccessSchedules:      &[]AccessSchedule{{Day: "Weekday", StartHour: 8, EndHour: 20}},
	SubtitleManagement:   new(true),
	CollectionManagement: new(true),
	QualityGroup:         new(720),
	TorrentStreams:       new(true),
}

// Reset to defaults gives a user's limits the values Create a user gives,
// for an administrator as for a user, and keeps the rest of the account.
// Their apps already signed in have the new limits at once, and a user who
// may no longer share loses their links.
func TestResetLimitsGivesANewUsersAndKeepsTheRest(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	fresh := mustCreate(t, store, NewUser{Name: "fresh", Password: "correct horse"})
	freshAdministrator := mustCreate(t, store, NewUser{Name: "boss", Password: "correct horse", IsAdministrator: true})

	alex := sharer(t, store)
	ends := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	alex, err := store.UpdateUser(ctx, alex.ID, UserChanges{IsHidden: new(true), EndsOn: &ends, MetadataLanguage: new("fr"),
		UseSharedAddons: new(false)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if alex, err = store.UpdateUser(ctx, alex.ID, everyLimit, nil); err != nil {
		t.Fatal(err)
	}
	_, link, err := store.CreateShareLink(ctx, newShareLink(alex))
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.SignInDevice(ctx, alex.ID, device("alex-tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, signed, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || signed.MaxPlaybacks != 2 {
		t.Fatalf("signed in before: %+v %v", signed, err)
	}

	// The preview changes nothing.
	preview, err := store.LimitsAfterReset(ctx, alex.ID)
	if err != nil || !preview.equal(fresh.Limits()) {
		t.Errorf("preview: %+v %v, want %+v", preview, err, fresh.Limits())
	}
	if got := mustUser(t, store, alex.ID); !got.Limits().equal(alex.Limits()) {
		t.Errorf("the preview changed alex: %+v", got.Limits())
	}

	reset, err := store.ResetLimits(ctx, alex.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reset.Limits().equal(fresh.Limits()) {
		t.Errorf("after the reset: %+v, want a new user's %+v", reset.Limits(), fresh.Limits())
	}
	if reset.Name != "alex" || !reset.IsHidden || reset.IsAdministrator || reset.EndsOn == nil || !reset.EndsOn.Equal(ends) ||
		reset.MetadataLanguage != "fr" || reset.UseSharedAddons {
		t.Errorf("the reset changed what it keeps: %+v", reset)
	}
	if _, err := store.Authenticate(ctx, "alex", "correct horse"); err != nil {
		t.Errorf("the password after the reset: %v", err)
	}
	if _, signed, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || !signed.Limits().equal(fresh.Limits()) {
		t.Errorf("the signed-in app after the reset: %+v %v", signed.Limits(), err)
	}
	if _, _, err := store.ShareLinkByToken(ctx, link); err == nil {
		t.Error("a share link still works once its user may no longer share")
	}

	// An administrator gets an administrator's.
	boss, err := store.UpdateUser(ctx, freshAdministrator.ID, UserChanges{RemoteControl: new(false), LiveTvManagement: new(false),
		CollectionManagement: new(false), SubtitleManagement: new(false), MaxPlaybacks: new(3)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if boss, err = store.ResetLimits(ctx, boss.ID); err != nil || !boss.IsAdministrator || !boss.Limits().equal(freshAdministrator.Limits()) {
		t.Errorf("the administrator after the reset: %+v %v, want %+v", boss.Limits(), err, freshAdministrator.Limits())
	}

	if _, err := store.ResetLimits(ctx, ID{9}); !errors.Is(err, ErrNotFound) {
		t.Errorf("resetting no one: %v", err)
	}
}

// A member following their group has the group's values: they are not
// reset. One with their own settings gets a new user's, and stays in the
// group without following it. A group's reset reaches its followers at
// once, apps already signed in included, and keeps its name.
func TestResetsFollowTheGroupRules(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	fresh := mustCreate(t, store, NewUser{Name: "fresh", Password: "correct horse"})
	group := kidsGroup(t, store)
	ann := mustCreate(t, store, NewUser{Name: "ann", Password: "correct horse", Group: &group.ID})
	cat := mustCreate(t, store, NewUser{Name: "cat", Password: "correct horse", Group: &group.ID})
	cat, err := store.UpdateUser(ctx, cat.ID, UserChanges{FollowGroup: new(false), MaxBitrate: new(2_000_000)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.LimitsAfterReset(ctx, ann.ID); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("previewing a follower: %v", err)
	}
	if _, err := store.ResetLimits(ctx, ann.ID); !errors.Is(err, ErrFollowsGroup) {
		t.Errorf("resetting a follower: %v", err)
	}
	if got := mustUser(t, store, ann.ID); !got.FollowGroup || !got.Limits().equal(group.Limits) {
		t.Errorf("ann after a refused reset: %+v", got)
	}

	reset, err := store.ResetLimits(ctx, cat.ID)
	if err != nil || reset.GroupID == nil || *reset.GroupID != group.ID || reset.FollowGroup || !reset.Limits().equal(fresh.Limits()) {
		t.Errorf("cat after the reset: %+v %v", reset, err)
	}
	cat = reset
	// Cat no longer follows the group: its later changes pass her by.
	if _, err := store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{MaxPlaybacks: new(4)}}); err != nil {
		t.Fatal(err)
	}
	if got := mustUser(t, store, cat.ID); got.MaxPlaybacks != 0 {
		t.Errorf("cat followed the group after her reset: %+v", got.Limits())
	}

	token, _, err := store.SignInDevice(ctx, ann.ID, device("ann-tablet"))
	if err != nil {
		t.Fatal(err)
	}
	if _, signed, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || signed.MaxPlaybacks != 4 {
		t.Fatalf("ann's app before the group's reset: %+v %v", signed, err)
	}
	empty, err := store.CreateGroup(ctx, "Empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview, err := store.GroupLimitsAfterReset(ctx, group.ID); err != nil || !preview.equal(empty.Limits) {
		t.Errorf("the group's preview: %+v %v, want %+v", preview, err, empty.Limits)
	}
	if got, err := store.Group(ctx, group.ID); err != nil || got.MaxPlaybacks != 4 {
		t.Errorf("the preview changed the group: %+v %v", got, err)
	}
	group, err = store.ResetGroup(ctx, group.ID)
	if err != nil || group.Name != "Kids" || group.Members != 2 || !group.Limits.equal(empty.Limits) {
		t.Fatalf("the group after its reset: %+v %v", group, err)
	}
	if got := mustUser(t, store, ann.ID); !got.FollowGroup || !got.Limits().equal(fresh.Limits()) {
		t.Errorf("ann after the group's reset: %+v", got.Limits())
	}
	if _, signed, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || !signed.Limits().equal(fresh.Limits()) {
		t.Errorf("ann's app after the group's reset: %+v %v", signed.Limits(), err)
	}
	if got := mustUser(t, store, cat.ID); !got.Limits().equal(cat.Limits()) {
		t.Errorf("cat, with her own settings, changed with the group: %+v", got.Limits())
	}
	if _, err := store.ResetGroup(ctx, ID{9}); !errors.Is(err, ErrNotFound) {
		t.Errorf("resetting no group: %v", err)
	}
	if _, err := store.GroupLimitsAfterReset(ctx, ID{9}); !errors.Is(err, ErrNotFound) {
		t.Errorf("previewing no group: %v", err)
	}
}
