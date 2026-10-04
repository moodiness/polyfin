package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// A user's own addons are left out while the server or the user's own
// permission turns them off, and come back once both allow them again.
func TestPersonalAddonsTurnedOffAreLeftOut(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	e.install(addons.Shared(), ratedAddon())
	// (Users' addons on the local network, as the test's, are an
	// administrator's.)
	e.install(addons.Personal(e.admin.ID), fakeRatings())
	admin := e.admin
	mine := e.library(admin, "Mine")
	expect := func(what string, own bool) {
		t.Helper()
		libraries, err := e.service.Libraries(ctx, admin)
		if got := names(libraries); err != nil || slices.Contains(got, "Mine") != own || !slices.Contains(got, "Top") {
			t.Fatalf("%s: libraries %v %v", what, got, err)
		}
		page, err := e.service.Children(ctx, admin, mine.ID, 0, 10, "")
		if own && (err != nil || !slices.Equal(names(page.Items), []string{"Crime"})) || !own && !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: listing the own library: %v %v", what, names(page.Items), err)
		}
	}
	expect("by default", true)
	// The server's addons stay, even when the user had turned them off.
	if err := e.addons.SetUsesSharedAddons(ctx, admin.ID, false); err != nil {
		t.Fatal(err)
	}
	var err error
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	expect("user's permission off", false)
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{PersonalAddons: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.addons.SetUsesSharedAddons(ctx, admin.ID, true); err != nil {
		t.Fatal(err)
	}
	expect("user's permission back on", true)

	settings := e.users.Settings()
	settings.PersonalAddons = false
	if _, err := e.users.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	expect("server's switch off", false)
	if kept, err := e.addons.Addons(ctx, addons.Personal(admin.ID)); err != nil || len(kept) != 1 {
		t.Errorf("the own addon is kept: %v %v", kept, err)
	}
	settings.PersonalAddons = true
	if _, err := e.users.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	expect("server's switch back on", true)
}
