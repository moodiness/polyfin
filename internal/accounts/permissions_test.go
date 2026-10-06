package accounts

import (
	"slices"
	"testing"
)

func TestConversionsCombineTheServersSwitchAndTheUsers(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	// Granted by default, as Jellyfin grants them.
	if settings := store.Settings(); !settings.Transcoding {
		t.Errorf("default settings: %+v", settings)
	}
	if !user.VideoTranscoding || !user.AudioTranscoding || !user.ContentDownloading {
		t.Errorf("new user: %+v", user)
	}
	if got := store.Conversions(user); got != (Conversions{Video: true, Audio: true}) || !store.MayDownload(user) {
		t.Errorf("by default: %+v, download %v", got, store.MayDownload(user))
	}

	off := false
	user, err := store.UpdateUser(ctx, user.ID, UserChanges{VideoTranscoding: &off, ContentDownloading: &off}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Conversions(user); got != (Conversions{Audio: true}) || store.MayDownload(user) {
		t.Errorf("user's video and downloads off: %+v, download %v", got, store.MayDownload(user))
	}
	// Other changes keep them.
	hidden := true
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{IsHidden: &hidden}, nil); err != nil || user.VideoTranscoding || user.ContentDownloading {
		t.Errorf("after another change: %+v %v", user, err)
	}

	on := true
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{VideoTranscoding: &on, ContentDownloading: &on}, nil); err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.Transcoding = false
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	// The server's switch stops conversions; downloads are the user's
	// alone.
	if got := store.Conversions(user); got != (Conversions{}) || !store.MayDownload(user) {
		t.Errorf("server's conversion off: %+v, download %v", got, store.MayDownload(user))
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got.Transcoding {
		t.Errorf("settings after reopening: %+v", got)
	}
}

func TestTurningDownloadsOffForEveryone(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	carol := mustCreate(t, store, NewUser{Name: "carol", Password: "correct horse"})
	if _, err := store.UpdateUser(ctx, carol.ID, UserChanges{ContentDownloading: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.SignInDevice(ctx, alice.ID, device("alice-tv"))
	if err != nil {
		t.Fatal(err)
	}
	// Alice's app is resolved once, with her permission on.
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || !user.ContentDownloading {
		t.Fatalf("before: %+v %v", user, err)
	}

	changed, err := store.TurnOffDownloads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, user := range changed {
		if user.ContentDownloading {
			t.Errorf("%s answered with the permission on", user.Name)
		}
		names = append(names, user.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{alice.Name, bob.Name}) {
		t.Errorf("changed %v, want the users who had the permission", names)
	}
	for _, id := range []ID{alice.ID, bob.ID, carol.ID} {
		if user, err := store.User(ctx, id); err != nil || user.ContentDownloading || store.MayDownload(user) {
			t.Errorf("%s: %+v %v", user.Name, user.ContentDownloading, err)
		}
	}
	// Her app already signed in loses the permission at once.
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.ContentDownloading {
		t.Errorf("signed-in app: %+v %v", user.ContentDownloading, err)
	}
	// Once nobody has it, nothing changes.
	if again, err := store.TurnOffDownloads(ctx); err != nil || len(again) != 0 {
		t.Errorf("again: %d users, %v", len(again), err)
	}
}

func TestPersonalAddonsNeedTheServersSwitchAndTheUsers(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	if !user.PersonalAddons || !store.Settings().PersonalAddons || !store.Settings().PersonalAddonsAllowed(user) {
		t.Fatalf("by default: user %v, server %v", user.PersonalAddons, store.Settings().PersonalAddons)
	}
	user, err := store.UpdateUser(ctx, user.ID, UserChanges{PersonalAddons: new(false)}, nil)
	if err != nil || user.PersonalAddons || store.Settings().PersonalAddonsAllowed(user) {
		t.Fatalf("user's permission off: %+v %v", user, err)
	}
	// Other changes keep it.
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{IsHidden: new(true)}, nil); err != nil || user.PersonalAddons {
		t.Errorf("after another change: %+v %v", user, err)
	}
	if user, err = store.UpdateUser(ctx, user.ID, UserChanges{PersonalAddons: new(true)}, nil); err != nil || !user.PersonalAddons {
		t.Fatalf("user's permission back on: %+v %v", user, err)
	}
	settings := store.Settings()
	settings.PersonalAddons = false
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if store.Settings().PersonalAddonsAllowed(user) {
		t.Error("allowed while the server's switch is off")
	}
}
