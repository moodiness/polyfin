package jellyfin

import (
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
)

// kids makes a group with member in it, following it, and a sibling who
// keeps their own settings.
func (s contentServer) kids(t *testing.T) (group accounts.Group, sibling accounts.User, siblingToken string) {
	t.Helper()
	group, err := s.store.CreateGroup(t.Context(), "Kids", nil)
	if err != nil {
		t.Fatal(err)
	}
	sibling = s.user("sibling", nil)
	if _, err := s.store.AddGroupMembers(t.Context(), group.ID, []accounts.ID{s.member.ID, sibling.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.UpdateUser(t.Context(), sibling.ID, accounts.UserChanges{FollowGroup: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	return group, sibling, s.signIn("sibling", "phone")
}

func (s contentServer) changeGroup(t *testing.T, group accounts.Group, changes accounts.UserChanges) {
	t.Helper()
	if _, err := s.store.UpdateGroup(t.Context(), group.ID, accounts.GroupChanges{Limits: changes}); err != nil {
		t.Fatal(err)
	}
}

// TestGroupLimitsApplyToTheMembersApps changes a group, section by section,
// and checks each change on the apps its member is signed in to, while the
// member who keeps their own settings sees everything.
func TestGroupLimitsApplyToTheMembersApps(t *testing.T) {
	t.Parallel()
	s := newContentServer(t)
	var clock atomic.Pointer[time.Time]
	now := time.Date(2026, time.October, 7, 12, 0, 0, 0, time.Local)
	clock.Store(&now)
	s.handler.now = func() time.Time { return *clock.Load() }
	group, _, siblingToken := s.kids(t)
	movies := "/Items?parentId=" + s.ids["Top"]
	playable := func(token, title string) bool {
		status, _ := s.call(http.MethodPost, "/Items/"+s.ids[title]+"/PlaybackInfo", app("tablet", token), map[string]any{})
		return status == http.StatusOK
	}
	download := func(token, title string) int {
		response, _ := fetchURL(t, s.url+"/Items/"+s.ids[title]+"/Download?ApiKey="+token, nil)
		return response.StatusCode
	}
	if got := s.names(t, s.memberToken, movies); !slices.Equal(got, []string{"Allowed", "Restricted", "Unrated"}) {
		t.Fatalf("member's movies before: %v", got)
	}

	// Parental control.
	s.changeGroup(t, group, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)}})
	if got := s.policy(t, s.member)["MaxParentalRating"]; got != float64(13) {
		t.Errorf("policy's rating: %v", got)
	}
	if got := s.names(t, s.memberToken, movies); !slices.Equal(got, []string{"Allowed", "Unrated"}) {
		t.Errorf("member's movies under the group's rating: %v", got)
	}
	if playable(s.memberToken, "Restricted") || download(s.memberToken, "Restricted") != http.StatusNotFound {
		t.Error("member plays or downloads Restricted")
	}
	if !playable(siblingToken, "Restricted") {
		t.Error("the sibling, on their own settings, cannot play Restricted")
	}

	// Blocked genres, and parental control taken off.
	s.changeGroup(t, group, accounts.UserChanges{Parental: &accounts.ParentalControl{}, BlockedGenres: &[]string{"drama"}})
	if got := s.names(t, s.memberToken, movies); !slices.Equal(got, []string{"Unrated"}) {
		t.Errorf("member's movies with dramas blocked: %v", got)
	}
	if playable(s.memberToken, "Allowed") {
		t.Error("member plays a drama")
	}

	// Visible libraries.
	top, _ := accounts.ParseID(s.ids["Top"])
	s.changeGroup(t, group, accounts.UserChanges{BlockedGenres: &[]string{}, HiddenLibraries: &[]accounts.ID{top}})
	if got := s.names(t, s.memberToken, "/UserViews"); slices.Contains(got, "Top") || !slices.Contains(got, "Shows") {
		t.Errorf("member's views: %v", got)
	}
	if policy := s.policy(t, s.member); policy["EnableAllFolders"] != false {
		t.Errorf("policy's folders: %v", policy["EnableAllFolders"])
	}
	if got := s.names(t, siblingToken, "/UserViews"); !slices.Contains(got, "Top") {
		t.Errorf("sibling's views: %v", got)
	}

	// Downloads.
	s.changeGroup(t, group, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{}, ContentDownloading: new(false)})
	if got := s.policy(t, s.member)["EnableContentDownloading"]; got != false {
		t.Errorf("policy's downloads: %v", got)
	}
	if status := download(s.memberToken, "Allowed"); status == http.StatusOK {
		t.Error("member downloads with the group's downloads off")
	}
	if status := download(siblingToken, "Allowed"); status != http.StatusOK {
		t.Errorf("sibling's download: %d", status)
	}

	// Allowed hours: noon is outside them, for the app already signed in.
	s.changeGroup(t, group, accounts.UserChanges{ContentDownloading: new(true),
		AccessSchedules: &[]accounts.AccessSchedule{{Day: "Everyday", StartHour: 17, EndHour: 20}}})
	if status, _ := s.call(http.MethodGet, "/UserViews", app("tablet", s.memberToken), nil); status != http.StatusForbidden {
		t.Errorf("member's views outside the group's hours: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/UserViews", app("phone", siblingToken), nil); status != http.StatusOK {
		t.Errorf("sibling's views at noon: %d", status)
	}
	evening := now.Add(6 * time.Hour)
	clock.Store(&evening)
	if status, _ := s.call(http.MethodGet, "/UserViews", app("tablet", s.memberToken), nil); status != http.StatusOK {
		t.Errorf("member's views in the group's hours: %d", status)
	}
}

func TestPoliciesFromAppsDetachMembersOnlyWhenAGroupSettingChanges(t *testing.T) {
	t.Parallel()
	s := newContentServer(t)
	group, _, _ := s.kids(t)
	s.changeGroup(t, group, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(13)},
		HiddenLibraries: &[]accounts.ID{{9}}, AccessSchedules: &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 8, EndHour: 20}}})
	member := func() accounts.User {
		user, err := s.store.User(t.Context(), s.member.ID)
		if err != nil {
			t.Fatal(err)
		}
		return user
	}

	// Disabling the member, the rest as the app read it: they still follow,
	// though the group hides a library the policy cannot name.
	if status, body := s.setPolicy(t, s.member, func(p map[string]any) { p["IsDisabled"] = true }); status != http.StatusNoContent {
		t.Fatalf("disabling: %d %s", status, body)
	}
	if got := member(); !got.IsDisabled || !got.FollowGroup || len(got.HiddenLibraries) != 1 {
		t.Errorf("disabled: %+v", got)
	}
	if status, body := s.setPolicy(t, s.member, func(p map[string]any) { p["IsDisabled"] = false }); status != http.StatusNoContent {
		t.Fatalf("enabling: %d %s", status, body)
	}

	// A group setting changed detaches the member, and the whole policy is
	// kept.
	status, body := s.setPolicy(t, s.member, func(p map[string]any) {
		p["MaxParentalRating"], p["IsHidden"], p["EnableContentDownloading"] = nil, true, false
	})
	if status != http.StatusNoContent {
		t.Fatalf("changing the rating: %d %s", status, body)
	}
	got := member()
	if got.FollowGroup || got.GroupID == nil || *got.GroupID != group.ID || got.Parental.MaxRating != nil || !got.IsHidden ||
		got.ContentDownloading || len(got.AccessSchedules) != 1 {
		t.Errorf("after the change: %+v", got)
	}
	page, err := s.activity.Entries(t.Context(), activity.Query{Types: []string{"GroupMemberDetached"}})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Name != "member no longer follows the group Kids: changed from Test App" {
		t.Errorf("activity: %+v %v", page.Entries, err)
	}
	// The group's next change passes the member by.
	s.changeGroup(t, group, accounts.UserChanges{Parental: &accounts.ParentalControl{MaxRating: new(7)}})
	if got := member(); got.Parental.MaxRating != nil {
		t.Errorf("detached member's rating: %v", *got.Parental.MaxRating)
	}

	// Made an administrator from an app, a member leaves the group.
	sibling, _ := s.store.User(t.Context(), s.member.ID)
	if _, err := s.store.UpdateUser(t.Context(), sibling.ID, accounts.UserChanges{FollowGroup: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	if status, body := s.setPolicy(t, s.member, func(p map[string]any) { p["IsAdministrator"] = true }); status != http.StatusNoContent {
		t.Fatalf("making an administrator: %d %s", status, body)
	}
	if got := member(); !got.IsAdministrator || got.GroupID != nil {
		t.Errorf("administrator: %+v", got)
	}
}
