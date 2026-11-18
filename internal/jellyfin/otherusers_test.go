package jellyfin

import (
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// severalUsers is a server whose accounts differ in everything apps may
// read of a user.
type severalUsers struct {
	testServer
	// root is an administrator and kid a member with limits, both shown on
	// the sign-in screen; kid has a picture. ghost is hidden from it. gone
	// is disabled and over has ended, both shown. member is hidden.
	root, kid, ghost, gone, over, member accounts.User
	rootToken, memberToken               string
	// newPolicy and newConfiguration are those kid's apps read of kid
	// before any limit and any saved setting: a new account's.
	newPolicy, newConfiguration map[string]any
}

func newSeveralUsers(t *testing.T) severalUsers {
	t.Helper()
	s := severalUsers{testServer: newTestServer(t, 10)}
	shown := func(c *accounts.UserChanges) { c.IsHidden = new(false) }
	s.root = s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator, c.IsHidden = new(true), new(false) })
	s.kid = s.user("kid", shown)
	s.ghost = s.user("ghost", nil)
	s.gone = s.user("gone", func(c *accounts.UserChanges) { c.IsHidden, c.IsDisabled = new(false), new(true) })
	yesterday := accounts.Day(time.Now()).AddDate(0, 0, -1)
	s.over = s.user("over", func(c *accounts.UserChanges) { c.IsHidden, c.EndsOn = new(false), &yesterday })
	s.member = s.user("member", nil)

	// Everyone used the server: a sign-in each, a wrong password for the
	// administrator, settings saved by kid and by member.
	s.rootToken = s.signIn("root", "dashboard")
	kidToken := s.signIn("kid", "tablet")
	s.signIn("ghost", "phone")
	s.memberToken = s.signIn("member", "laptop")
	var fresh struct{ Policy, Configuration map[string]any }
	if status := s.get(t, "/Users/Me", kidToken, &fresh); status != http.StatusOK || len(fresh.Policy) == 0 || len(fresh.Configuration) == 0 {
		t.Fatalf("kid as a new account: %d %+v", status, fresh)
	}
	s.newPolicy, s.newConfiguration = fresh.Policy, fresh.Configuration
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = 5 })
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("x", ""), map[string]string{"Username": "root", "Pw": "wrong horse"}); status != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", status)
	}
	for _, token := range []string{kidToken, s.memberToken} {
		if status := s.configure(t, "/Users/Configuration", token, map[string]any{"SubtitleLanguagePreference": "fre", "SubtitleMode": "Always"}); status != http.StatusNoContent {
			t.Fatalf("configuration: %d", status)
		}
	}
	// The limits the administrator then gives kid, and kid's picture.
	if _, err := s.store.UpdateUser(t.Context(), s.kid.ID, accounts.UserChanges{
		Parental:        &accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}},
		AccessSchedules: &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 17, EndHour: 20}},
		MaxBitrate:      new(4_000_000), MaxPlaybacks: new(1), RemoteAccess: new(false), ContentDownloading: new(false),
		SyncPlay: new(accounts.SyncPlayNone), LiveTv: new(false),
	}, nil); err != nil {
		t.Fatal(err)
	}
	var err error
	if s.kid, err = s.store.SetImage(t.Context(), s.kid.ID, picture(t, 64, 64, 255)); err != nil || s.kid.ImageTag == "" {
		t.Fatalf("kid's picture: %q %v", s.kid.ImageTag, err)
	}
	return s
}

// usersAt reads a list of users as authorization gets it, by name.
func (s severalUsers) usersAt(t *testing.T, path, authorization string) map[string]map[string]any {
	t.Helper()
	status, body := s.call(http.MethodGet, path, authorization, nil)
	var users []map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &users) != nil {
		t.Fatalf("%s: %d %s", path, status, body)
	}
	byName := map[string]map[string]any{}
	for _, user := range users {
		byName[user["Name"].(string)] = user
	}
	if len(byName) != len(users) {
		t.Fatalf("%s names a user twice: %s", path, body)
	}
	return byName
}

// one reads a user by identifier as authorization gets it.
func (s severalUsers) one(t *testing.T, id accounts.ID, authorization string) map[string]any {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Users/"+id.String(), authorization, nil)
	var user map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &user) != nil {
		t.Fatalf("/Users/%s: %d %s", id, status, body)
	}
	return user
}

// appKeys are what apps read of a user who is not theirs: jellyfin-web's
// sign-in screen (Name, Id, PrimaryImageTag, HasPassword), its SyncPlay
// menu (Name, Id, PrimaryImageTag) and its Quick Connect page (Id, Name,
// Policy.IsDisabled); and what apps that check a user's whole shape must
// find, the objects and the values that are one of a list.
var appKeys = []string{"Name", "Id", "ServerId", "PrimaryImageTag", "HasPassword", "HasConfiguredPassword",
	"HasConfiguredEasyPassword", "EnableAutoLogin", "Policy.IsAdministrator", "Policy.IsHidden", "Policy.IsDisabled",
	"Policy.SyncPlayAccess", "Policy.AuthenticationProviderId", "Policy.PasswordResetProviderId", "Policy.AccessSchedules",
	"Policy.BlockUnratedItems", "Policy.EnabledFolders", "Policy.EnableAllFolders", "Policy.LoginAttemptsBeforeLockout",
	"Configuration.SubtitleMode", "Configuration.PlayDefaultAudioTrack", "Configuration.OrderedViews",
	"Configuration.CastReceiverId"}

// tellsOnlyNameAndPicture checks that entry describes user as someone else
// reads them: the name, the identifier and the picture's tag, and a new
// account's for all the rest, with every key apps read.
func (s severalUsers) tellsOnlyNameAndPicture(t *testing.T, where string, entry map[string]any, user accounts.User) {
	t.Helper()
	want := map[string]any{"Name": user.Name, "Id": user.ID.String(), "ServerId": testServerID,
		"HasPassword": true, "HasConfiguredPassword": true, "HasConfiguredEasyPassword": false, "EnableAutoLogin": false,
		"Configuration": s.newConfiguration, "Policy": s.newPolicy}
	if user.ImageTag != "" {
		want["PrimaryImageTag"] = user.ImageTag
	}
	if !reflect.DeepEqual(entry, want) {
		got, _ := json.Marshal(entry)
		t.Errorf("%s, %s: %s", where, user.Name, got)
	}
	for _, key := range appKeys {
		object, name := entry, key
		if part, rest, nested := strings.Cut(key, "."); nested {
			object, _ = entry[part].(map[string]any)
			name = rest
		}
		if _, present := object[name]; !present && (key != "PrimaryImageTag" || user.ImageTag != "") {
			t.Errorf("%s, %s: no %s", where, user.Name, key)
		}
	}
}

func TestAMemberReadsOnlyTheNameAndPictureOfOtherUsers(t *testing.T) {
	t.Parallel()
	s := newSeveralUsers(t)
	member, root := app("laptop", s.memberToken), app("dashboard", s.rootToken)

	// The list: the users shown on the sign-in screen and the member.
	listed := s.usersAt(t, "/Users", member)
	if names := slices.Sorted(maps.Keys(listed)); !slices.Equal(names, []string{"kid", "member", "root"}) {
		t.Fatalf("a member lists %v", names)
	}
	s.tellsOnlyNameAndPicture(t, "GET /Users", listed["kid"], s.kid)
	s.tellsOnlyNameAndPicture(t, "GET /Users", listed["root"], s.root)
	s.tellsOnlyNameAndPicture(t, "GET /Users/{userId}", s.one(t, s.kid.ID, member), s.kid)
	s.tellsOnlyNameAndPicture(t, "GET /Users/{userId}", s.one(t, s.root.ID, member), s.root)

	// A user hidden from the sign-in screen, a disabled one and one whose
	// account ended are answered as an identifier nobody has.
	nobody, unknown := s.call(http.MethodGet, "/Users/0123456789abcdef0123456789abcdef", member, nil)
	if nobody != http.StatusNotFound {
		t.Fatalf("an identifier nobody has: %d %s", nobody, unknown)
	}
	for _, user := range []accounts.User{s.ghost, s.gone, s.over} {
		if status, body := s.call(http.MethodGet, "/Users/"+user.ID.String(), member, nil); status != nobody || string(body) != string(unknown) {
			t.Errorf("%s as a member reads them: %d %s, want %d %s", user.Name, status, body, nobody, unknown)
		}
	}

	// The filters apply to what a member lists.
	for query, want := range map[string][]string{"?isHidden=true": {"member"}, "?isHidden=false": {"kid", "root"},
		"?isDisabled=true": {}, "?isDisabled=false": {"kid", "member", "root"}} {
		if names := slices.Sorted(maps.Keys(s.usersAt(t, "/Users"+query, member))); !slices.Equal(names, want) {
			t.Errorf("a member lists %v with %s, want %v", names, query, want)
		}
	}

	// The member's own entry is whole wherever it is read: the one an
	// administrator reads of them, but for the time of the last request.
	whole := s.one(t, s.member.ID, root)
	var me map[string]any
	if status := s.get(t, "/Users/Me", s.memberToken, &me); status != http.StatusOK {
		t.Fatalf("/Users/Me: %d", status)
	}
	for where, own := range map[string]map[string]any{"GET /Users": listed["member"], "GET /Users/{userId}": s.one(t, s.member.ID, member), "GET /Users/Me": me} {
		if own["LastActivityDate"] == nil || own["LastLoginDate"] == nil {
			t.Errorf("%s, the member's own dates: %v, %v", where, own["LastLoginDate"], own["LastActivityDate"])
		}
		own, want := maps.Clone(own), maps.Clone(whole)
		delete(own, "LastActivityDate")
		delete(want, "LastActivityDate")
		if !reflect.DeepEqual(own, want) {
			got, _ := json.Marshal(own)
			t.Errorf("%s, the member's own entry: %s", where, got)
		}
	}
	policy, _ := whole["Policy"].(map[string]any)
	configuration, _ := whole["Configuration"].(map[string]any)
	if policy["IsHidden"] != true || policy["LoginAttemptsBeforeLockout"] != float64(5) || configuration["SubtitleMode"] != "Always" ||
		configuration["SubtitleLanguagePreference"] != "fre" {
		t.Errorf("the member's own policy and configuration: %v %v", policy, configuration)
	}

	// An administrator reads every user, whole, as does an API key.
	_, key, err := s.store.CreateAPIKey(t.Context(), "Monitor")
	if err != nil {
		t.Fatal(err)
	}
	for who, read := range map[string]func(path string) (int, []byte){
		"an administrator": func(path string) (int, []byte) { return s.call(http.MethodGet, path, root, nil) },
		"an API key":       func(path string) (int, []byte) { return s.call(http.MethodGet, path+"?ApiKey="+key, "", nil) },
	} {
		status, body := read("/Users")
		var users []UserDto
		if status != http.StatusOK || json.Unmarshal(body, &users) != nil || len(users) != 6 {
			t.Fatalf("%s lists: %d %s", who, status, body)
		}
		byName := map[string]UserDto{}
		for _, user := range users {
			byName[user.Name] = user
		}
		if admin := byName["root"]; !admin.Policy.IsAdministrator || admin.LastLoginDate == nil ||
			admin.Policy.InvalidLoginAttemptCount != 1 || admin.Policy.LoginAttemptsBeforeLockout != 5 {
			t.Errorf("%s reads the administrator: %+v", who, admin)
		}
		if kid := byName["kid"]; kid.Policy.MaxParentalRating == nil || *kid.Policy.MaxParentalRating != 13 ||
			!slices.Equal(kid.Policy.BlockUnratedItems, []string{"Movie"}) || len(kid.Policy.AccessSchedules) != 1 ||
			kid.Policy.AccessSchedules[0].StartHour != 17 || kid.Policy.RemoteClientBitrateLimit != 4_000_000 ||
			kid.Policy.MaxActiveSessions != 1 || kid.Policy.EnableRemoteAccess || kid.Policy.EnableContentDownloading ||
			kid.Policy.EnableLiveTvAccess || kid.Policy.SyncPlayAccess != "None" || kid.LastActivityDate == nil ||
			kid.Configuration.SubtitleLanguagePreference != "fre" || kid.Configuration.SubtitleMode != "Always" ||
			kid.PrimaryImageTag != s.kid.ImageTag {
			t.Errorf("%s reads the member with limits: %+v", who, kid)
		}
		if !byName["ghost"].Policy.IsHidden || byName["ghost"].LastLoginDate == nil || !byName["gone"].Policy.IsDisabled ||
			!byName["over"].Policy.IsDisabled {
			t.Errorf("%s reads the hidden, disabled and ended users: %+v %+v %+v", who, byName["ghost"], byName["gone"], byName["over"])
		}
		for _, user := range []accounts.User{s.ghost, s.gone, s.over, s.kid} {
			status, body := read("/Users/" + user.ID.String())
			var one UserDto
			if status != http.StatusOK || json.Unmarshal(body, &one) != nil || !reflect.DeepEqual(one, byName[user.Name]) {
				t.Errorf("%s reads %s: %d %s", who, user.Name, status, body)
			}
		}
	}
}

func TestTheSignInScreenTellsOnlyNamesAndPictures(t *testing.T) {
	t.Parallel()
	s := newSeveralUsers(t)
	shown := s.usersAt(t, "/Users/Public", "")
	if names := slices.Sorted(maps.Keys(shown)); !slices.Equal(names, []string{"kid", "root"}) {
		t.Fatalf("the sign-in screen lists %v", names)
	}
	s.tellsOnlyNameAndPicture(t, "GET /Users/Public", shown["kid"], s.kid)
	s.tellsOnlyNameAndPicture(t, "GET /Users/Public", shown["root"], s.root)
}
