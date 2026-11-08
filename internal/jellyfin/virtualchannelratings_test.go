package jellyfin

import (
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// ratedChannel is a server whose child is limited to 13, under a rating
// country, with a movie of the server's addon that a channel of titles
// airs once saved.
type ratedChannel struct {
	s          testServer
	admin      accounts.User
	child      accounts.User
	adminToken string
	childToken string
	movie      string
	entries    []virtualtv.Entry
	channel    string
	now, next  string
}

// newRatedChannel installs addon, sets the rating country and finds the
// movie named name in the library view named view.
func newRatedChannel(t *testing.T, addon, country, view, name string) *ratedChannel {
	t.Helper()
	s := newTestServer(t, 10)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon, false); err != nil {
		t.Fatal(err)
	}
	if country != "US" {
		settings := s.store.Settings()
		settings.RatingCountry = country
		if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
	}
	c := &ratedChannel{s: s}
	c.admin = s.user("admin", func(u *accounts.UserChanges) { u.IsAdministrator = new(true) })
	c.child = s.user("child", func(u *accounts.UserChanges) { u.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	c.adminToken, c.childToken = s.signIn("admin", "dashboard"), s.signIn("child", "web")
	var views QueryResult
	s.get(t, "/UserViews", c.adminToken, &views)
	for _, v := range views.Items {
		if v.Name == view {
			c.movie = itemIDOf(t, s, c.adminToken, v.Id, name)
		}
	}
	if c.movie == "" {
		t.Fatalf("no %s in %s: %+v", name, view, views.Items)
	}
	now := &atomic.Pointer[time.Time]{}
	current := time.Now()
	now.Store(&current)
	reminding(t, s, now)
	return c
}

// edit saves the metadata editor's form for the movie with fields, the
// others left empty: they follow the addon.
func (c *ratedChannel) edit(t *testing.T, fields map[string]any) {
	t.Helper()
	body := editorBody(c.movie, fields)
	body["ProviderIds"] = map[string]string{}
	if status, data := c.s.call(http.MethodPost, "/Items/"+c.movie, app("dashboard", c.adminToken), body); status != http.StatusNoContent {
		t.Fatalf("edit %v: %d %s", fields, status, data)
	}
}

// air saves a channel airing the movie, which started 90 seconds ago, and
// sets the child a reminder of its next airing.
func (c *ratedChannel) air(t *testing.T) {
	t.Helper()
	source := virtualtv.Source{Kind: virtualtv.SourceTitles, Items: []virtualtv.SourceItem{{ID: parseTestID(t, c.movie)}}}
	entries, err := c.s.handler.ChannelEntries(t.Context(), c.admin, source)
	if err != nil {
		t.Fatal(err)
	}
	c.entries = entries
	store := virtualtv.New(c.s.pool)
	c.s.library.UseVirtual(ownChannels{store})
	epoch := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	if _, err := store.Create(t.Context(), virtualtv.Channel{Name: "Evenings", Enabled: true, Source: source, Entries: entries, Seed: 1,
		Epoch: epoch}); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	c.s.get(t, "/LiveTv/Channels", c.adminToken, &channels)
	if len(channels.Items) != 1 || channels.Items[0].CurrentProgram == nil {
		t.Fatalf("channels: %+v", channels.Items)
	}
	c.channel, c.now = channels.Items[0].Id, channels.Items[0].CurrentProgram.Id
	programs := c.programs(t, c.adminToken)
	if len(programs) < 2 || programs[0].Id != c.now {
		t.Fatalf("programmes: %+v", programs)
	}
	c.next = programs[1].Id
}

// programs lists the channel's programmes of the next three hours that
// the user of token sees.
func (c *ratedChannel) programs(t *testing.T, token string) []BaseItemDto {
	t.Helper()
	now := time.Now().UTC()
	var programs QueryResult
	c.s.get(t, "/LiveTv/Programs?channelIds="+c.channel+"&MinEndDate="+url.QueryEscape(now.Format(time.RFC3339))+
		"&MaxStartDate="+url.QueryEscape(now.Add(3*time.Hour).Format(time.RFC3339))+"&SortBy=StartDate", token, &programs)
	return programs.Items
}

// remind sets the child a reminder of the next airing.
func (c *ratedChannel) remind(t *testing.T) {
	t.Helper()
	if status, body := c.s.call(http.MethodPost, "/Polyfin/Reminders", app("web", c.childToken), map[string]any{"ProgramId": c.next}); status != http.StatusOK {
		t.Fatalf("the child's reminder: %d %s", status, body)
	}
}

// expect checks that the child sees the channel, its guide, the programme
// airing now, its stream and their reminder of the next airing, or none of
// them, and that the programme shows the official and custom ratings
// given.
func (c *ratedChannel) expect(t *testing.T, step string, shown bool, official, custom string) {
	t.Helper()
	var program BaseItemDto
	if status := c.s.get(t, "/LiveTv/Programs/"+c.now, c.adminToken, &program); status != http.StatusOK ||
		program.OfficialRating != official || program.CustomRating != custom {
		t.Errorf("%s: the programme's ratings: %d %q %q, want %q %q", step, status, program.OfficialRating, program.CustomRating, official, custom)
	}
	var channels QueryResult
	c.s.get(t, "/LiveTv/Channels", c.childToken, &channels)
	if shown != (len(channels.Items) == 1) || shown && (channels.Items[0].CurrentProgram == nil || channels.Items[0].CurrentProgram.Id != c.now) {
		t.Errorf("%s: the child's channels: %+v", step, channels.Items)
	}
	if got := len(c.programs(t, c.childToken)); shown != (got > 0) {
		t.Errorf("%s: the child's guide holds %d programmes", step, got)
	}
	want := http.StatusNotFound
	if shown {
		want = http.StatusOK
	}
	if status, _ := c.s.call(http.MethodGet, "/LiveTv/Programs/"+c.now, app("web", c.childToken), nil); status != want {
		t.Errorf("%s: the child opens the programme airing now: %d, want %d", step, status, want)
	}
	if versions, err := c.s.library.Versions(t.Context(), c.child, parseTestID(t, c.channel)); shown != (err == nil && len(versions) == 1) {
		t.Errorf("%s: the child's versions of the channel: %+v %v", step, versions, err)
	}
	var list struct{ Items []ReminderDto }
	if status := c.s.get(t, "/Polyfin/Reminders", c.childToken, &list); status != http.StatusOK || shown != (len(list.Items) == 1) {
		t.Errorf("%s: the child's reminders: %d %+v", step, status, list.Items)
	}
}

// A channel of titles judges a title by the rating an administrator set,
// as soon as it is set: a PG movie rated R hides the channel, its guide,
// what airs now and the reminders of its programmes from a child limited
// to 13, and clearing the custom rating shows them again.
func TestChannelsOfTitlesFollowACustomRating(t *testing.T) {
	t.Parallel()
	c := newRatedChannel(t, ratingsAddon(t), "US", "Top", "Allowed")
	c.air(t)
	c.remind(t)
	c.expect(t, "as the addon rates it", true, "PG", "")
	c.edit(t, map[string]any{"CustomRating": "R"})
	c.expect(t, "rated R", false, "PG", "R")
	c.edit(t, map[string]any{})
	c.expect(t, "the custom rating cleared", true, "PG", "")
}

// The same holds under Italy: a movie rated T there, given VM18.
func TestChannelsOfTitlesFollowACustomRatingUnderTheRatingCountry(t *testing.T) {
	t.Parallel()
	c := newRatedChannel(t, italianAddon(t), "IT", "Top", "Day")
	c.air(t)
	c.remind(t)
	c.expect(t, "as the addon rates it", true, "T", "")
	c.edit(t, map[string]any{"CustomRating": "VM18"})
	c.expect(t, "rated VM18", false, "T", "VM18")
	c.edit(t, map[string]any{})
	c.expect(t, "the custom rating cleared", true, "T", "")
}

// An edited official rating counts too, also when it was set before the
// channel was saved: the channel keeps the addon's rating, so clearing the
// edit shows the channel again.
func TestChannelsOfTitlesFollowAnEditedOfficialRating(t *testing.T) {
	t.Parallel()
	c := newRatedChannel(t, ratingsAddon(t), "US", "Top", "Allowed")
	c.edit(t, map[string]any{"OfficialRating": "R"})
	c.air(t)
	if len(c.entries) != 1 || c.entries[0].Rating != "PG" {
		t.Errorf("the entries keep the addon's rating: %+v", c.entries)
	}
	c.expect(t, "rated R", false, "R", "")
	c.edit(t, map[string]any{})
	c.remind(t)
	c.expect(t, "the official rating cleared", true, "PG", "")
	c.edit(t, map[string]any{"OfficialRating": "R"})
	c.expect(t, "rated R again", false, "R", "")
}
