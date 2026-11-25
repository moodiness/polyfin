package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// upgrade is one release upgraded to the candidate, on one kind of
// database.
type upgrade struct {
	options
	from     string
	oldImage string
	database string
	media    string
	// name prefixes the upgrade's containers, volume and network.
	name    string
	shipped map[int]migration

	containers []string
	networks   []string
	data       seeded

	err  error
	took time.Duration
}

func (u *upgrade) title() string {
	return fmt.Sprintf("from %s, %s database", u.from, u.database)
}

func (u *upgrade) logf(format string, args ...any) {
	fmt.Printf("[%s] %s\n", u.title(), fmt.Sprintf(format, args...))
}

// stepError is a failed step: the step, and the check that failed in it.
type stepError struct {
	step  string
	check string
	err   error
}

func (e *stepError) Error() string {
	where := fmt.Sprintf("step %q", e.step)
	if e.check != "" {
		where += fmt.Sprintf(", check %q", e.check)
	}
	return where + ":\n" + indent(e.err.Error())
}

// run takes the steps of the upgrade, and keeps the first failure.
func (u *upgrade) run(ctx context.Context) {
	began := time.Now()
	defer func() { u.took = time.Since(began) }()
	defer u.cleanUp()
	step := func(name string, do func() error) bool {
		if u.err != nil {
			return false
		}
		u.logf("%s", name)
		started := time.Now()
		if err := do(); err != nil {
			var failed *stepError
			if !errors.As(err, &failed) {
				failed = &stepError{err: err}
			}
			failed.step = name
			u.err = failed
			u.logf("%s: FAILED", name)
			return false
		}
		u.logf("%s: done in %s", name, time.Since(started).Round(100*time.Millisecond))
		return true
	}
	var old, candidate *container
	var before observations
	step("prepare the "+u.database+" database", func() (err error) {
		if _, err = docker(ctx, "volume", "create", u.name+"-data"); err == nil && u.database == external {
			err = u.startPostgres(ctx)
		}
		return err
	})
	step("start "+u.from, func() (err error) {
		old, err = u.startServer(ctx, u.oldImage, u.name+"-old")
		return err
	})
	step("seed through the APIs of "+u.from, func() error {
		if err := u.seed(ctx, old); err != nil {
			return fmt.Errorf("%w\n%s", err, u.logTail(ctx, old.name))
		}
		return nil
	})
	step("record the data on "+u.from, func() (err error) {
		if before, err = u.observe(ctx, old); err == nil {
			err = seededAsAsked(before)
		}
		return err
	})
	step("stop "+u.from, func() error { return u.stopServer(ctx, old.name) })
	step("start the candidate", func() (err error) {
		candidate, err = u.startServer(ctx, u.candidate, u.name+"-candidate")
		return err
	})
	step("check the migrations", func() error {
		applied, err := u.query(ctx, candidate.name, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
		if err != nil {
			return err
		}
		return compareMigrations(parseApplied(applied), u.shipped)
	})
	step("check the data after the upgrade", func() error {
		after, err := u.observe(ctx, candidate)
		if err != nil {
			return err
		}
		return compareObservations(before, after)
	})
	step("stop the candidate", func() error { return u.stopServer(ctx, candidate.name) })
}

// account is a user the seed makes, with the Jellyfin device it signed
// in as.
type account struct {
	name     string
	password string
	id       string
	device   caller
}

// seeded is what the seed made, which the checks look up.
type seeded struct {
	admin, alice, bob, carol *account
	adminSession             caller
	group                    string
	// views are the libraries' views by name, items the videos and songs
	// by name, playlists by name, rows the home rows by name.
	views      map[string]string
	items      map[string]string
	playlists  map[string]string
	collection string
	rows       map[string]string
}

// The data the seed makes, which the checks expect back. Names are in
// French, the server's language, with accents.
const (
	serverName  = "Maison des tests"
	groupName   = "Famille"
	videos      = "Vidéos de famille"
	music       = "Musique"
	evening     = "Soirée"
	selection   = "Sélection"
	quiet       = "Musique douce"
	summer      = "Été"
	summerRow   = "Notre été"
	selectedRow = "À regarder"
)

// changedSettings are the server settings the seed changes from their
// defaults, as the admin app's Settings page saves them.
var changedSettings = map[string]any{
	"serverName":          serverName,
	"ratingCountry":       "FR",
	"playedPercent":       float64(85),
	"resumePercent":       float64(3),
	"updateCheck":         false,
	"quickConnectEnabled": false,
}

// seed has the old release make representative data through its own APIs,
// as an administrator and users of the admin app and of Jellyfin's apps
// would: the setup, settings, a group, users, two local folders, playlists,
// a collection, home rows, display preferences and what users did.
func (u *upgrade) seed(ctx context.Context, server *container) error {
	a := server.api
	d := &u.data
	d.admin = &account{name: "admin", password: "Upgrade-Admin-1"}
	d.alice = &account{name: "alice", password: "Upgrade-Alice-1"}
	d.bob = &account{name: "bob", password: "Upgrade-Bob-1"}
	d.carol = &account{name: "carol", password: "Upgrade-Carol-1"}
	failed := func(what string, err error) error { return fmt.Errorf("%s: %w", what, err) }

	code, err := setupCode(ctx, server.name)
	if err != nil {
		return failed("read the setup code", err)
	}
	if d.adminSession, err = a.signIn(ctx, "/admin/api/setup",
		map[string]string{"setupCode": code, "name": d.admin.name, "password": d.admin.password, "language": "fr"}); err != nil {
		return failed("create the first administrator in French", err)
	}
	admin := d.adminSession

	var settings map[string]any
	if _, err := a.call(ctx, admin, "GET", "/admin/api/settings", nil, &settings, http.StatusOK); err != nil {
		return failed("read the settings", err)
	}
	for key, value := range changedSettings {
		if _, ok := settings[key]; !ok {
			return failed("change the settings", fmt.Errorf("they have no %s", key))
		}
		settings[key] = value
	}
	if _, err := a.call(ctx, admin, "PUT", "/admin/api/settings", settings, nil, http.StatusOK); err != nil {
		return failed("change the settings", err)
	}

	var created struct{ ID string }
	if _, err := a.call(ctx, admin, "POST", "/admin/api/groups", map[string]string{"name": groupName}, &created, http.StatusCreated); err != nil {
		return failed("create the group", err)
	}
	d.group = created.ID
	if _, err := a.call(ctx, admin, "PATCH", "/admin/api/groups/"+d.group, map[string]any{"downloads": false, "maxBitrate": 8000000},
		nil, http.StatusOK); err != nil {
		return failed("change the group's settings", err)
	}
	for _, user := range []*account{d.alice, d.bob, d.carol} {
		body := map[string]any{"name": user.name, "password": user.password}
		if user == d.bob {
			body["groupId"] = d.group
		}
		if _, err := a.call(ctx, admin, "POST", "/admin/api/users", body, &created, http.StatusCreated); err != nil {
			return failed("create the user "+user.name, err)
		}
		user.id = created.ID
	}
	if _, err := a.call(ctx, admin, "PATCH", "/admin/api/users/"+d.carol.id, map[string]any{"isDisabled": true}, nil, http.StatusOK); err != nil {
		return failed("disable carol", err)
	}

	for _, folder := range []struct{ name, path, kind string }{
		{videos, "/media/videos", "homevideos"}, {music, "/media/music", "music"},
	} {
		if _, err := a.call(ctx, admin, "POST", "/admin/api/scopes/shared/folders",
			map[string]string{"name": folder.name, "path": folder.path, "kind": folder.kind}, nil, http.StatusCreated); err != nil {
			return failed("add the local folder "+folder.path, err)
		}
	}
	for _, user := range []*account{d.admin, d.alice, d.bob} {
		if user.device, user.id, err = a.authenticate(ctx, "upgradecheck-"+user.name, user.name, user.password); err != nil {
			return failed("sign "+user.name+" in to the Jellyfin API", err)
		}
	}
	if err := u.waitForScans(ctx, a); err != nil {
		return failed("scan the local folders", err)
	}
	if _, err := a.call(ctx, admin, "PATCH", "/admin/api/users/"+d.alice.id,
		map[string]any{"downloads": false, "maxPlaybacks": 2, "hiddenLibraries": []string{d.views[music]}}, nil, http.StatusOK); err != nil {
		return failed("change alice's settings", err)
	}

	d.playlists = map[string]string{}
	for _, playlist := range []struct {
		owner     *account
		name      string
		mediaType string
		items     []string
	}{
		{d.alice, evening, "Video", []string{"School Play", "Beach Day"}},
		{d.admin, selection, "Video", []string{"Garden Party", "Beach Day"}},
		{d.admin, quiet, "Audio", []string{"Second Wind", "First Light"}},
	} {
		var made struct{ Id string }
		if _, err := a.call(ctx, playlist.owner.device, "POST", "/Playlists", map[string]any{
			"Name": playlist.name, "Ids": d.ids(playlist.items...), "UserId": playlist.owner.id, "MediaType": playlist.mediaType,
		}, &made, http.StatusOK); err != nil {
			return failed("create the playlist "+playlist.name, err)
		}
		d.playlists[playlist.name] = made.Id
	}
	var collection struct{ Id string }
	if _, err := a.call(ctx, d.admin.device, "POST", "/Collections?name="+url.QueryEscape(summer)+"&ids="+
		strings.Join(d.ids("Beach Day", "Garden Party"), ","), nil, &collection, http.StatusOK); err != nil {
		return failed("create the collection", err)
	}
	d.collection = collection.Id

	d.rows = map[string]string{}
	for _, row := range []map[string]any{
		{"name": summerRow, "kind": "collection", "place": "top", "collectionId": d.collection, "everyone": true},
		{"name": selectedRow, "kind": "playlist", "place": "bottom", "playlistId": d.playlists[selection], "count": 10,
			"users": []string{d.alice.id, d.bob.id}},
	} {
		if _, err := a.call(ctx, admin, "POST", "/admin/api/home-rows", row, &created, http.StatusCreated); err != nil {
			return failed(fmt.Sprintf("create the home row %s", row["name"]), err)
		}
		d.rows[row["name"].(string)] = created.ID
	}
	alice, err := a.signIn(ctx, "/admin/api/session", map[string]string{"name": d.alice.name, "password": d.alice.password})
	if err != nil {
		return failed("sign alice in to the admin app", err)
	}
	if _, err := a.call(ctx, alice, "PUT", "/admin/api/account/home-rows/"+d.rows[summerRow], map[string]bool{"off": true},
		nil, http.StatusNoContent); err != nil {
		return failed("turn a home row off for alice", err)
	}

	for _, mark := range []struct {
		user *account
		path string
		item string
	}{
		{d.alice, "/UserPlayedItems/", "Beach Day"},
		{d.alice, "/UserFavoriteItems/", "School Play"},
		{d.bob, "/UserPlayedItems/", "School Play"},
		{d.bob, "/UserFavoriteItems/", "Garden Party"},
		{d.admin, "/UserFavoriteItems/", "First Light"},
	} {
		if _, err := a.call(ctx, mark.user.device, "POST", mark.path+d.items[mark.item], nil, nil, http.StatusOK); err != nil {
			return failed(fmt.Sprintf("mark %s for %s", mark.item, mark.user.name), err)
		}
	}
	if _, err := a.call(ctx, d.alice.device, "POST", "/UserItems/"+d.items["Garden Party"]+"/UserData",
		map[string]int64{"PlaybackPositionTicks": int64(10_000_000)}, nil, http.StatusOK); err != nil {
		return failed("save alice's resume point", err)
	}
	if _, err := a.call(ctx, d.alice.device, "POST", "/DisplayPreferences/usersettings?userId="+d.alice.id+"&client=emby",
		map[string]any{"Id": "usersettings", "Client": "emby", "SortBy": "SortName", "SortOrder": "Descending",
			"CustomPrefs": map[string]string{"homesection0": "resume", "homesection1": "latestmedia", "landing-" + d.views[videos]: "favorites"}},
		nil, http.StatusNoContent); err != nil {
		return failed("save alice's display preferences", err)
	}
	return nil
}

// ids are the identifiers of the items named.
func (d *seeded) ids(names ...string) []string {
	ids := make([]string, len(names))
	for i, name := range names {
		ids[i] = d.items[name]
	}
	return ids
}

// scanWait bounds the scans of the local folders.
const scanWait = 2 * time.Minute

// waitForScans waits for the views of both local folders, and for their
// three videos and two songs, then keeps their identifiers.
func (u *upgrade) waitForScans(ctx context.Context, a *api) error {
	d := &u.data
	admin := d.admin.device
	deadline := time.Now().Add(scanWait)
	for {
		views, err := listViews(ctx, a, admin, d.admin.id)
		if err != nil {
			return err
		}
		d.views = map[string]string{}
		for _, view := range views {
			d.views[view.Name] = view.Id
		}
		d.items = map[string]string{}
		if d.views[videos] != "" && d.views[music] != "" {
			for _, view := range []string{videos, music} {
				items, err := listItems(ctx, a, admin, d.admin.id, d.views[view])
				if err != nil {
					return err
				}
				for _, item := range items {
					d.items[item.Name] = item.Id
				}
			}
		}
		want := []string{"Beach Day", "Garden Party", "School Play", "First Light", "Second Wind"}
		if !slices.ContainsFunc(want, func(name string) bool { return d.items[name] == "" }) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("after %s, the libraries list %d of the 5 media files", scanWait, len(d.items))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

type item struct {
	Name     string
	Id       string
	Type     string
	UserData *struct {
		Played                bool
		PlayCount             int
		IsFavorite            bool
		PlaybackPositionTicks int64
	}
}

func listViews(ctx context.Context, a *api, who caller, user string) ([]item, error) {
	var views struct{ Items []item }
	_, err := a.call(ctx, who, "GET", "/UserViews?userId="+user, nil, &views, http.StatusOK)
	return views.Items, err
}

// listItems lists the videos and songs under a view or a collection, with
// what the user did with them, sorted by name here: the order a server
// sorts them in is not what the checks compare.
func listItems(ctx context.Context, a *api, who caller, user, parent string) ([]item, error) {
	var items struct{ Items []item }
	_, err := a.call(ctx, who, "GET", "/Items?userId="+user+"&parentId="+parent+
		"&recursive=true&includeItemTypes=Video,Audio&fields=UserData", nil, &items, http.StatusOK)
	slices.SortFunc(items.Items, func(a, b item) int { return strings.Compare(a.Name, b.Name) })
	return items.Items, err
}
