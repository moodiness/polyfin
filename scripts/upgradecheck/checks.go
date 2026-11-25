package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// facts are what one check reads from a server: a value for each key, as
// text. observations are the facts of every check, by check.
type (
	facts        map[string]string
	observations map[string]facts
)

// check reads facts from a server about what the seed made. The checks
// run on the old release once it is seeded, then on the candidate, whose
// facts must be those the old release answered: a key the old release
// answered that the candidate answers otherwise, or not at all, fails the
// check. Keys only the candidate answers, such as the fields a newer
// release adds, are not compared. A list is one key, so that an item
// gained or lost changes its value.
type check struct {
	name    string
	observe func(ctx context.Context, a *api, d *seeded, f facts) error
}

// checks are run in this order: the devices' tokens are read before any
// new sign-in.
var checks = []check{
	{"sign-in", observeSignIn},
	{"server", observeServer},
	{"settings", observeSettings},
	{"users", observeUsers},
	{"groups", observeGroups},
	{"libraries", observeLibraries},
	{"playlists", observePlaylists},
	{"collections", observeCollection},
	{"home rows", observeHomeRows},
	{"user data", observeUserData},
	{"display preferences", observeDisplayPreferences},
	{"routes", observeRoutes},
}

// observe runs every check on a server. A check that fails to read its
// facts fails with the end of the server's log, which tells why.
func (u *upgrade) observe(ctx context.Context, server *container) (observations, error) {
	all := observations{}
	for _, c := range checks {
		f := facts{}
		if err := c.observe(ctx, server.api, &u.data, f); err != nil {
			return nil, &stepError{check: c.name, err: fmt.Errorf("%w\n%s", err, u.logTail(ctx, server.name))}
		}
		all[c.name] = f
	}
	return all, nil
}

// compareObservations fails with every check whose facts differ, each
// difference on a line of its own: the key, what the old release
// answered, and what the candidate does.
func compareObservations(before, after observations) error {
	var failed []string
	var lines []string
	for _, c := range checks {
		var differences []string
		old, now := before[c.name], after[c.name]
		for _, key := range sortedKeys(old) {
			value, found := now[key]
			switch {
			case !found:
				differences = append(differences, fmt.Sprintf("%s: was %q, now absent", key, old[key]))
			case value != old[key]:
				differences = append(differences, fmt.Sprintf("%s: was %q, now %q", key, old[key], value))
			}
		}
		if len(differences) > 0 {
			failed = append(failed, c.name)
			lines = append(lines, fmt.Sprintf("check %q, %d difference(s):", c.name, len(differences)))
			for _, difference := range differences {
				lines = append(lines, "  "+difference)
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return &stepError{check: strings.Join(failed, ", "), err: errors.New(strings.Join(lines, "\n"))}
}

// expected are facts the old release must answer once seeded, so that a
// seed that made nothing, or a check that reads nothing, cannot pass.
var expected = map[string]facts{
	"sign-in": {
		"admin: the token from the seed": "valid", "alice: the token from the seed": "valid", "bob: the token from the seed": "valid",
		"alice: signs in": "yes", "carol: signs in": "no (403)", "admin: signs in to the admin app": "yes",
	},
	"server":   {"StartupWizardCompleted": "true", "ServerName": serverName, "setupRequired": "false"},
	"settings": {"serverName": serverName, "language": "fr", "ratingCountry": "FR", "playedPercent": "85", "updateCheck": "false"},
	"users": {
		"alice: downloads": "false", "alice: maxPlaybacks": "2", "carol: isDisabled": "true", "admin: isAdministrator": "true",
		"bob: groupId": "group " + groupName,
	},
	"groups":    {groupName + ": downloads": "false", groupName + ": maxBitrate": "8000000", groupName + ": members": "bob"},
	"libraries": {"alice sees": videos, videos + ": items": "Beach Day, Garden Party, School Play", music + ": items": "First Light, Second Wind"},
	"playlists": {
		evening + ": items": "School Play, Beach Day", selection + ": items": "Garden Party, Beach Day",
		quiet + ": items": "Second Wind, First Light", "bob sees " + evening: "404",
	},
	"collections": {summer + ": items": "Beach Day, Garden Party"},
	"home rows":   {summerRow + ": kind": "collection", selectedRow + ": users": "alice, bob", "alice: " + summerRow: "off"},
	"user data": {
		"alice: Beach Day": "played, 1 play, not favorite, at 0", "alice: School Play": "not played, 0 plays, favorite, at 0",
		"alice: Garden Party": "not played, 0 plays, not favorite, at 10000000", "alice: resume": "Garden Party",
		"bob: Garden Party": "not played, 0 plays, favorite, at 0", "admin: First Light": "not played, 0 plays, favorite, at 0",
	},
	"display preferences": {"SortOrder": "Descending", "homesection1": "latestmedia", "landing of " + videos: "favorites"},
	"routes":              {"GET /web/index.html": "200", "GET /System/Info/Public": "200"},
}

// seededAsAsked fails with the expected facts the old release did not
// answer as expected.
func seededAsAsked(before observations) error {
	var lines []string
	var failed []string
	for _, c := range checks {
		var wrong []string
		for _, key := range sortedKeys(expected[c.name]) {
			want := expected[c.name][key]
			if got, found := before[c.name][key]; !found {
				wrong = append(wrong, fmt.Sprintf("%s: absent, want %q", key, want))
			} else if got != want {
				wrong = append(wrong, fmt.Sprintf("%s: %q, want %q", key, got, want))
			}
		}
		if len(wrong) > 0 {
			failed = append(failed, c.name)
			lines = append(lines, fmt.Sprintf("check %q, the old release answers what the seed did not make:", c.name))
			for _, line := range wrong {
				lines = append(lines, "  "+line)
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return &stepError{check: strings.Join(failed, ", "), err: errors.New(strings.Join(lines, "\n"))}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// pick sets, under prefix, the fields of object it has: a string as it
// is, any other value as compact JSON.
func pick(f facts, prefix string, object map[string]any, fields ...string) {
	for _, field := range fields {
		value, ok := object[field]
		if text, isText := value.(string); isText {
			f[prefix+field] = text
		} else if ok {
			encoded, _ := json.Marshal(value)
			f[prefix+field] = string(encoded)
		}
	}
}

// names lists items' names, in order.
func names(items []item) string {
	list := make([]string, len(items))
	for i, item := range items {
		list[i] = item.Name
	}
	return strings.Join(list, ", ")
}

// adminSession signs user in to the admin app.
func adminSession(ctx context.Context, a *api, user *account) (caller, error) {
	return a.signIn(ctx, "/admin/api/session", map[string]string{"name": user.name, "password": user.password})
}

// observeSignIn reads whether the devices the seed signed in, and the
// administrator's admin session, are still signed in, then whether each
// user signs in, carol being disabled.
func observeSignIn(ctx context.Context, a *api, d *seeded, f facts) error {
	for _, user := range []*account{d.admin, d.alice, d.bob} {
		var me struct{ Id string }
		status, err := a.call(ctx, user.device, "GET", "/Users/Me", nil, &me)
		switch {
		case err != nil:
			return err
		case status == http.StatusOK && me.Id == user.id:
			f[user.name+": the token from the seed"] = "valid"
		default:
			f[user.name+": the token from the seed"] = fmt.Sprintf("refused (%d)", status)
		}
	}
	status, err := a.call(ctx, d.adminSession, "GET", "/admin/api/session", nil, nil)
	if err != nil {
		return err
	}
	f["admin: the admin app's session from the seed"] = fmt.Sprint(status)
	for _, user := range []*account{d.admin, d.alice, d.bob, d.carol} {
		status, err := a.call(ctx, caller{device: "upgradecheck-" + user.name + "-check"}, "POST", "/Users/AuthenticateByName",
			map[string]string{"Username": user.name, "Pw": user.password}, nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK {
			f[user.name+": signs in"] = "yes"
		} else {
			f[user.name+": signs in"] = fmt.Sprintf("no (%d)", status)
		}
	}
	if _, err := adminSession(ctx, a, d.admin); err != nil {
		f["admin: signs in to the admin app"] = err.Error()
	} else {
		f["admin: signs in to the admin app"] = "yes"
	}
	return nil
}

// observeServer reads the server's identity, as apps see it, and whether
// it asks for its setup again.
func observeServer(ctx context.Context, a *api, _ *seeded, f facts) error {
	var info map[string]any
	if _, err := a.call(ctx, nobody, "GET", "/System/Info/Public", nil, &info, http.StatusOK); err != nil {
		return err
	}
	for _, field := range []string{"Id", "ServerName", "StartupWizardCompleted"} {
		f[field] = fmt.Sprint(info[field])
	}
	var status map[string]any
	if _, err := a.call(ctx, nobody, "GET", "/admin/api/status", nil, &status, http.StatusOK); err != nil {
		return err
	}
	for _, field := range []string{"serverId", "setupRequired", "database"} {
		f[field] = fmt.Sprint(status[field])
	}
	return nil
}

// observeSettings reads the settings the seed changed, and the language
// the setup chose.
func observeSettings(ctx context.Context, a *api, d *seeded, f facts) error {
	admin, err := adminSession(ctx, a, d.admin)
	if err != nil {
		return err
	}
	var settings map[string]any
	if _, err := a.call(ctx, admin, "GET", "/admin/api/settings", nil, &settings, http.StatusOK); err != nil {
		return err
	}
	pick(f, "", settings, append(sortedKeys(changedSettings), "language")...)
	return nil
}

// observeUsers reads each user's account and the settings the seed
// changed, their group by name.
func observeUsers(ctx context.Context, a *api, d *seeded, f facts) error {
	admin, err := adminSession(ctx, a, d.admin)
	if err != nil {
		return err
	}
	var users []map[string]any
	if _, err := a.call(ctx, admin, "GET", "/admin/api/users", nil, &users, http.StatusOK); err != nil {
		return err
	}
	var listed []string
	for _, user := range users {
		name := fmt.Sprint(user["name"])
		listed = append(listed, name)
		pick(f, name+": ", user, "id", "isAdministrator", "isHidden", "isDisabled", "downloads", "maxPlaybacks", "maxBitrate",
			"remoteAccess", "collectionManagement")
		hidden, _ := user["hiddenLibraries"].([]any)
		var views []string
		for _, id := range hidden {
			views = append(views, viewName(d, fmt.Sprint(id)))
		}
		f[name+": hiddenLibraries"] = strings.Join(views, ", ")
		if group, ok := user["groupId"].(string); ok && group == d.group {
			f[name+": groupId"] = "group " + groupName
		} else if ok {
			f[name+": groupId"] = group
		}
	}
	slices.Sort(listed)
	f["users"] = strings.Join(listed, ", ")
	return nil
}

func viewName(d *seeded, id string) string {
	for name, view := range d.views {
		if view == id {
			return name
		}
	}
	return id
}

// observeGroups reads the group, its settings the seed changed, and its
// members.
func observeGroups(ctx context.Context, a *api, d *seeded, f facts) error {
	admin, err := adminSession(ctx, a, d.admin)
	if err != nil {
		return err
	}
	var groups []struct{ Name string }
	if _, err := a.call(ctx, admin, "GET", "/admin/api/groups", nil, &groups, http.StatusOK); err != nil {
		return err
	}
	listed := make([]string, len(groups))
	for i, group := range groups {
		listed[i] = group.Name
	}
	slices.Sort(listed)
	f["groups"] = strings.Join(listed, ", ")
	var group map[string]any
	if _, err := a.call(ctx, admin, "GET", "/admin/api/groups/"+d.group, nil, &group, http.StatusOK); err != nil {
		return err
	}
	pick(f, groupName+": ", group, "name", "downloads", "maxBitrate", "memberCount")
	var members []struct{ Name string }
	if _, err := a.call(ctx, admin, "GET", "/admin/api/groups/"+d.group+"/members", nil, &members, http.StatusOK); err != nil {
		return err
	}
	list := make([]string, len(members))
	for i, member := range members {
		list[i] = member.Name
	}
	f[groupName+": members"] = strings.Join(list, ", ")
	return nil
}

// observeLibraries reads the local folders' views and the items under
// them, with their identifiers, on which what users did and their
// playlists depend, and the views alice sees, music being hidden from
// her.
func observeLibraries(ctx context.Context, a *api, d *seeded, f facts) error {
	views, err := listViews(ctx, a, d.admin.device, d.admin.id)
	if err != nil {
		return err
	}
	for _, view := range views {
		if view.Name != videos && view.Name != music {
			continue
		}
		f[view.Name+": id"] = view.Id
		items, err := listItems(ctx, a, d.admin.device, d.admin.id, view.Id)
		if err != nil {
			return err
		}
		f[view.Name+": items"] = names(items)
		for _, item := range items {
			f[view.Name+": "+item.Name] = item.Type + " " + item.Id
		}
	}
	if views, err = listViews(ctx, a, d.alice.device, d.alice.id); err != nil {
		return err
	}
	var seen []string
	for _, view := range views {
		if view.Name == videos || view.Name == music {
			seen = append(seen, view.Name)
		}
	}
	f["alice sees"] = strings.Join(seen, ", ")
	return nil
}

// observePlaylists reads each playlist's name and items in order, as its
// owner sees it, and whether bob sees alice's, which is not shared.
func observePlaylists(ctx context.Context, a *api, d *seeded, f facts) error {
	for _, playlist := range []struct {
		name  string
		owner *account
	}{{evening, d.alice}, {selection, d.admin}, {quiet, d.admin}} {
		id := d.playlists[playlist.name]
		var named struct{ Name string }
		if _, err := a.call(ctx, playlist.owner.device, "GET", "/Users/"+playlist.owner.id+"/Items/"+id, nil, &named, http.StatusOK); err != nil {
			return err
		}
		f[playlist.name+": name"] = named.Name
		var items struct{ Items []item }
		if _, err := a.call(ctx, playlist.owner.device, "GET", "/Playlists/"+id+"/Items?userId="+playlist.owner.id, nil, &items,
			http.StatusOK); err != nil {
			return err
		}
		f[playlist.name+": items"] = names(items.Items)
	}
	status, err := a.call(ctx, d.bob.device, "GET", "/Users/"+d.bob.id+"/Items/"+d.playlists[evening], nil, nil)
	if err != nil {
		return err
	}
	f["bob sees "+evening] = fmt.Sprint(status)
	return nil
}

// observeCollection reads the collection's name and items.
func observeCollection(ctx context.Context, a *api, d *seeded, f facts) error {
	var named struct{ Name, Type string }
	if _, err := a.call(ctx, d.admin.device, "GET", "/Users/"+d.admin.id+"/Items/"+d.collection, nil, &named, http.StatusOK); err != nil {
		return err
	}
	f[summer+": name"] = named.Name
	f[summer+": type"] = named.Type
	items, err := listItems(ctx, a, d.admin.device, d.admin.id, d.collection)
	if err != nil {
		return err
	}
	f[summer+": items"] = names(items)
	return nil
}

// observeHomeRows reads the home rows the administrator made, and the row
// alice turned off for herself.
func observeHomeRows(ctx context.Context, a *api, d *seeded, f facts) error {
	admin, err := adminSession(ctx, a, d.admin)
	if err != nil {
		return err
	}
	var rows struct{ Rows []map[string]any }
	if _, err := a.call(ctx, admin, "GET", "/admin/api/home-rows", nil, &rows, http.StatusOK); err != nil {
		return err
	}
	var order []string
	for _, row := range rows.Rows {
		name := fmt.Sprint(row["name"])
		order = append(order, name)
		pick(f, name+": ", row, "id", "kind", "place", "count", "everyone")
		f[name+": collection"] = idName(row["collectionId"], map[string]string{summer: d.collection})
		f[name+": playlist"] = idName(row["playlistId"], d.playlists)
		users, _ := row["users"].([]any)
		var names []string
		for _, user := range users {
			names = append(names, idName(user, map[string]string{"alice": d.alice.id, "bob": d.bob.id}))
		}
		slices.Sort(names)
		f[name+": users"] = strings.Join(names, ", ")
	}
	f["rows"] = strings.Join(order, ", ")
	alice, err := adminSession(ctx, a, d.alice)
	if err != nil {
		return err
	}
	var own struct {
		Rows []struct {
			Name string
			Off  bool
		}
	}
	if _, err := a.call(ctx, alice, "GET", "/admin/api/account/home-rows", nil, &own, http.StatusOK); err != nil {
		return err
	}
	for _, row := range own.Rows {
		state := "on"
		if row.Off {
			state = "off"
		}
		f["alice: "+row.Name] = state
	}
	return nil
}

// idName names an identifier among those of named, else gives it as is.
func idName(id any, named map[string]string) string {
	text, _ := id.(string)
	for name, known := range named {
		if known == text && text != "" {
			return name
		}
	}
	return text
}

// observeUserData reads what each user did with the videos and songs:
// played, how many times, favorite, and where they stopped, and alice's
// Continue Watching.
func observeUserData(ctx context.Context, a *api, d *seeded, f facts) error {
	for _, user := range []*account{d.admin, d.alice, d.bob} {
		for _, view := range []string{videos, music} {
			if user == d.alice && view == music {
				continue
			}
			items, err := listItems(ctx, a, user.device, user.id, d.views[view])
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.UserData == nil {
					f[user.name+": "+item.Name] = "no user data"
					continue
				}
				played, favorite := "not played", "not favorite"
				if item.UserData.Played {
					played = "played"
				}
				if item.UserData.IsFavorite {
					favorite = "favorite"
				}
				plays := fmt.Sprintf("%d plays", item.UserData.PlayCount)
				if item.UserData.PlayCount == 1 {
					plays = "1 play"
				}
				f[user.name+": "+item.Name] = fmt.Sprintf("%s, %s, %s, at %d", played, plays, favorite, item.UserData.PlaybackPositionTicks)
			}
		}
	}
	var resume struct{ Items []item }
	if _, err := a.call(ctx, d.alice.device, "GET", "/UserItems/Resume?userId="+d.alice.id, nil, &resume, http.StatusOK); err != nil {
		return err
	}
	f["alice: resume"] = names(resume.Items)
	return nil
}

// observeDisplayPreferences reads the display preferences alice's web
// client saved.
func observeDisplayPreferences(ctx context.Context, a *api, d *seeded, f facts) error {
	var preferences struct {
		SortBy      string
		SortOrder   string
		CustomPrefs map[string]string
	}
	if _, err := a.call(ctx, d.alice.device, "GET", "/DisplayPreferences/usersettings?userId="+d.alice.id+"&client=emby", nil,
		&preferences, http.StatusOK); err != nil {
		return err
	}
	f["SortBy"] = preferences.SortBy
	f["SortOrder"] = preferences.SortOrder
	f["homesection0"] = preferences.CustomPrefs["homesection0"]
	f["homesection1"] = preferences.CustomPrefs["homesection1"]
	f["landing of "+videos] = preferences.CustomPrefs["landing-"+d.views[videos]]
	return nil
}

// observeRoutes reads the status a few routes answer: the web player's
// and the admin app's pages, the health check, and the Jellyfin routes
// apps call first.
func observeRoutes(ctx context.Context, a *api, d *seeded, f facts) error {
	for _, route := range []struct {
		who  caller
		path string
	}{
		{nobody, "/web/index.html"}, {nobody, "/admin/"}, {nobody, "/healthz"}, {nobody, "/System/Ping"},
		{nobody, "/System/Info/Public"}, {nobody, "/Users/Public"}, {nobody, "/"},
		{d.alice.device, "/System/Info"}, {d.alice.device, "/Users/" + d.alice.id}, {d.alice.device, "/UserViews?userId=" + d.alice.id},
		{d.admin.device, "/System/Info"},
	} {
		status, err := a.call(ctx, route.who, "GET", route.path, nil, nil)
		if err != nil {
			return err
		}
		key := "GET " + route.path
		if route.who != nobody {
			key = strings.ReplaceAll(key, d.alice.id, "{alice}") + ", " + strings.TrimPrefix(route.who.device, "upgradecheck-")
		}
		f[key] = fmt.Sprint(status)
	}
	return nil
}
