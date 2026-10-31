package jellyfin

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Play local files first offers a user the folder's version before the
// addons' streams, in item details and PlaybackInfo, whatever the order
// of sources says, the others keeping the criteria's order; Play takes
// it. A user without it keeps the server's order.
func TestLocalFilesFirst(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	shelf := p.versions[4]
	local, localToken := p.another(t, "local", "fre")
	local, err := p.store.UpdateUser(t.Context(), local.ID, accounts.UserChanges{LocalFirst: new(true)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, user accounts.User, token string, want ...string) []MediaSourceInfo {
		t.Helper()
		sources := p.detailed(t, user, token)
		if got := firstWords(sources); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
		return sources
	}
	check("without rules", p.user, p.token, "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish", "Shelf")
	check("without rules, local files first", local, localToken, "Shelf", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish")

	// The criteria order each side: Relay Two before Relay One, and the
	// folder after them for others.
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"source", "language"}
		s.VersionSourceOrder = []accounts.ID{p.two.ID, p.one.ID}
	})
	check("the server's order", p.user, p.token, "TwoFrench", "TwoEnglish", "OneEnglish", "OneFrench", "Shelf")
	sources := check("local files first", local, localToken, "Shelf", "TwoFrench", "TwoEnglish", "OneFrench", "OneEnglish")
	// The first source carries the title's identifier, as without rules.
	if sources[0].Id != p.movie || sources[0].ETag != shelf.ID.String() {
		t.Errorf("identifiers: %s %s", sources[0].Id, sources[0].ETag)
	}

	// Play, with no version picked, takes the first that plays: the
	// folder's file for the user who plays local files first.
	p.analyzed(t, shelf, "h264-aac-mp4")
	p.analyzed(t, p.versions[2], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, localToken, p.movie, chrome, nil); len(answer.MediaSources) == 0 || answer.MediaSources[0].ETag != shelf.ID.String() {
		t.Errorf("PlaybackInfo, local files first: %v", firstWords(answer.MediaSources))
	}
	if answer := p.ask(t, p.token, p.movie, chrome, nil); len(answer.MediaSources) == 0 || answer.MediaSources[0].ETag != p.versions[2].ID.String() {
		t.Errorf("PlaybackInfo, the server's order: %v", firstWords(answer.MediaSources))
	}
}

// A group's Play local files first reaches the members who follow it, at
// once, and leaves those with their own settings as they are.
func TestLocalFilesFirstFromTheGroup(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	ctx := t.Context()
	group, err := p.store.CreateGroup(ctx, "Home", nil)
	if err != nil {
		t.Fatal(err)
	}
	follower, err := p.store.CreateUser(ctx, accounts.NewUser{Name: "kim", Password: "correct horse", Group: &group.ID})
	if err != nil {
		t.Fatal(err)
	}
	own, err := p.store.CreateUser(ctx, accounts.NewUser{Name: "lee", Password: "correct horse", Group: &group.ID})
	if err != nil {
		t.Fatal(err)
	}
	if own, err = p.store.UpdateUser(ctx, own.ID, accounts.UserChanges{FollowGroup: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	followerToken, ownToken := p.signIn("kim", "tv"), p.signIn("lee", "tv")
	addonOrder := []string{"OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish", "Shelf"}
	if got := firstWords(p.detailed(t, follower, followerToken)); !slices.Equal(got, addonOrder) {
		t.Errorf("before the group's change: %v", got)
	}
	if _, err := p.store.UpdateGroup(ctx, group.ID, accounts.GroupChanges{Limits: accounts.UserChanges{LocalFirst: new(true)}}); err != nil {
		t.Fatal(err)
	}
	if got := firstWords(p.detailed(t, follower, followerToken)); !slices.Equal(got, []string{"Shelf", "OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish"}) {
		t.Errorf("a member following the group: %v", got)
	}
	if got := firstWords(p.detailed(t, own, ownToken)); !slices.Equal(got, addonOrder) {
		t.Errorf("a member with their own settings: %v", got)
	}
}
