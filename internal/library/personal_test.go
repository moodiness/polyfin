package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A user's own addons are left out while the server or the user's own
// permission turns them off, and come back once both allow them again.
func TestPersonalAddonsTurnedOffAreLeftOut(t *testing.T) {
	t.Parallel()
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
	var err error
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{UseSharedAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{PersonalAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	expect("user's permission off", false)
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{PersonalAddons: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{UseSharedAddons: new(true)}, nil); err != nil {
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

// Under parental control, a user's own addons are not used: their
// libraries leave the user's apps, and their versions leave the titles
// the server's addons list, while the server's stay.
func TestParentalControlLeavesOwnAddonsOut(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	e.install(addons.Shared(), ratedAddon())
	family := stremio.Meta{ID: "tt1", Type: "movie", Name: "Family"}
	// (Users' addons on the local network, as the test's, are an
	// administrator's.)
	e.install(addons.Personal(e.admin.ID), &fakeAddon{
		manifest: stremio.Manifest{ID: "own", Name: "Mine", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}},
		catalogs: map[string][]stremio.Meta{"movie/mine": {family}},
		metas:    map[string]stremio.Meta{"movie/tt1": family},
		streams:  map[string][]stremio.Stream{"movie/tt1": {{Name: "Own copy", URL: "https://own.example/tt1"}}},
	})
	admin := e.admin
	mine := e.library(admin, "Mine")
	item := itemID(titleKey(KindMovie, "tt1"))
	expect := func(what string, own bool) {
		t.Helper()
		libraries, err := e.service.Libraries(ctx, admin)
		if got := names(libraries); err != nil || slices.Contains(got, "Mine") != own || !slices.Contains(got, "Top") {
			t.Errorf("%s: libraries %v %v", what, got, err)
		}
		if _, err := e.service.Children(ctx, admin, mine.ID, 0, 10, ""); (err == nil) != own {
			t.Errorf("%s: listing the own library: %v", what, err)
		}
		versions, err := e.service.Versions(ctx, admin, item)
		var got []string
		for _, v := range versions {
			got = append(got, v.Name)
		}
		if err != nil || slices.Contains(got, "Own copy") != own || !slices.Contains(got, "1080p") {
			t.Errorf("%s: versions %v %v", what, got, err)
		}
	}
	expect("without parental control", true)
	var err error
	if admin, err = e.users.UpdateUser(ctx, admin.ID, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}}, nil); err != nil {
		t.Fatal(err)
	}
	if !admin.Restricted() || !e.users.Settings().PersonalAddonsAllowed(admin) {
		t.Fatalf("restricted %v, own addons allowed %v", admin.Restricted(), e.users.Settings().PersonalAddonsAllowed(admin))
	}
	expect("under parental control", false)
	if kept, err := e.addons.Addons(ctx, addons.Personal(admin.ID)); err != nil || len(kept) != 1 {
		t.Errorf("the own addon is kept: %v %v", kept, err)
	}
}
