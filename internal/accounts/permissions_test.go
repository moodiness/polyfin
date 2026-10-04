package accounts

import "testing"

func TestPermissionsCombineTheServersSwitchesAndTheUsers(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	// Granted by default, as Jellyfin grants them.
	if settings := store.Settings(); !settings.Transcoding || !settings.Downloads {
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
	settings.Transcoding, settings.Downloads = false, false
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if got := store.Conversions(user); got != (Conversions{}) || store.MayDownload(user) {
		t.Errorf("server's switches off: %+v, download %v", got, store.MayDownload(user))
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got.Transcoding || got.Downloads {
		t.Errorf("settings after reopening: %+v", got)
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
