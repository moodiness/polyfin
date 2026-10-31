package jellyfin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// channelChoice finds the key of a channel group, or of a channel, of the
// server's line-ups by its name.
func channelChoice(t *testing.T, s testServer, group bool, name string) string {
	t.Helper()
	groups, channels, err := s.library.ChannelChoices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if group {
		if i := slices.IndexFunc(groups, func(g library.ChannelGroupChoice) bool { return g.Name == name }); i >= 0 {
			return groups[i].Key
		}
	} else if i := slices.IndexFunc(channels, func(c library.ChannelChoice) bool { return c.Name == name }); i >= 0 {
		return channels[i].Key
	}
	t.Fatalf("no choice %q among %+v %+v", name, groups, channels)
	return ""
}

// A channel group a user's channel rule hides is hidden from them wherever
// Live TV off hides it: the channel lists, the channel and its programmes
// by their identifiers, the guide, what airs now, searches, playback,
// timers and recordings, those they scheduled themselves included. Other
// users still see it, and jellyfin-web's policy editor, which reads and
// sends EnableAllChannels, keeps the rule.
func TestChannelRuleHidesChannelsEverywhere(t *testing.T) {
	t.Parallel()
	provider := newIPTVProvider(t)
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	member := s.user("member", func(c *accounts.UserChanges) { c.LiveTvManagement = new(true) })
	token := s.signIn("member", "tv")
	s.user("other", nil)
	other := s.signIn("other", "tv")
	now := &atomic.Pointer[time.Time]{}
	current := time.Now()
	now.Store(&current)
	reminding(t, s, now)
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Guide: provider.url + "/epg.xml",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	zeb := channels.Items[0]
	tonight, ok := guidePrograms(t, s, token)["Zeb Tonight"]
	if !ok {
		t.Fatal("the guide has no programme")
	}
	// The member's recording of the channel, and their timer for tomorrow.
	var recording accounts.ID
	if err := s.pool.QueryRow(t.Context(), `INSERT INTO live_recordings (user_id, channel_id, name, start_at, end_at, started_at, ended_at, status)
		VALUES ($1, $2, 'Zeb Yesterday', now() - interval '1 day', now() - interval '23 hours', now() - interval '1 day',
		now() - interval '23 hours', 'Completed') RETURNING id`, member.ID, zeb.Id).Scan(&recording); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO live_timers (user_id, channel_id, name, start_at, end_at)
		VALUES ($1, $2, 'Zeb Tomorrow', now() + interval '1 day', now() + interval '25 hours')`, member.ID, zeb.Id); err != nil {
		t.Fatal(err)
	}
	counts := func(token string) (int, int) {
		t.Helper()
		var recordings, timers QueryResult
		s.get(t, "/LiveTv/Recordings", token, &recordings)
		s.get(t, "/LiveTv/Timers", token, &timers)
		return len(recordings.Items), len(timers.Items)
	}
	if r, tm := counts(token); r != 1 || tm != 1 {
		t.Fatalf("before the rule, %d recordings and %d timers", r, tm)
	}
	if status, _ := s.call(http.MethodGet, "/Polyfin/Reminders/Programs/"+tonight.Id, app("tv", token), nil); status != http.StatusOK {
		t.Fatalf("a programme's reminder state before the rule: %d", status)
	}

	news := channelChoice(t, s, true, "News")
	s.limit(t, member, func(c *accounts.UserChanges) {
		c.LiveTvChannels = &accounts.ChannelRule{Mode: accounts.ChannelsExcept, Groups: []string{news}}
	})

	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 || channels.Items[0].Name != "Orbe Junior" || channels.Items[0].Number != "2" {
		t.Errorf("channels without the news, numbered as for everyone: %+v", channels.Items)
	}
	for _, path := range []string{"/LiveTv/Channels/" + zeb.Id, "/Items/" + zeb.Id, "/LiveTv/Programs/" + tonight.Id, "/Items/" + tonight.Id,
		"/LiveTv/Timers/Defaults?programId=" + tonight.Id, fmt.Sprintf("/LiveTv/Recordings/%x", recording[:])} {
		if status, body := s.call(http.MethodGet, path, app("tv", token), nil); status != http.StatusNotFound {
			t.Errorf("%s of a hidden channel: %d %s", path, status, body)
		}
	}
	// The web client's script asks every page shown: a hidden programme is
	// no programme of the user's guide.
	if status, _ := s.call(http.MethodGet, "/Polyfin/Reminders/Programs/"+tonight.Id, app("tv", token), nil); status != http.StatusNoContent {
		t.Errorf("a hidden programme's reminder state: %d", status)
	}
	if _, ok := guidePrograms(t, s, token)["Zeb Tonight"]; ok {
		t.Error("the guide shows a hidden channel's programme")
	}
	var found QueryResult
	for _, path := range []string{"/LiveTv/Programs?channelIds=" + zeb.Id, "/LiveTv/Programs/Recommended?IsAiring=true",
		"/Items?recursive=true&searchTerm=zeb&includeItemTypes=TvChannel,LiveTvProgram"} {
		if s.get(t, path, token, &found); len(found.Items) != 0 {
			t.Errorf("%s lists a hidden channel: %+v", path, found.Items)
		}
	}
	if status, body := s.call(http.MethodPost, "/Items/"+zeb.Id+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a hidden channel: %d %s", status, body)
	}
	if r, tm := counts(token); r != 0 || tm != 0 {
		t.Errorf("with the rule, %d recordings and %d timers of a hidden channel", r, tm)
	}
	// Another user still has the channel, and the member's recording of it.
	if r, _ := counts(other); r != 1 {
		t.Errorf("another user's recordings: %d", r)
	}
	if s.get(t, "/LiveTv/Programs/Recommended?IsAiring=true", other, &found); len(found.Items) != 1 {
		t.Errorf("another user's guide: %+v", found.Items)
	}

	// The policy still enables every channel, as jellyfin-web reads it, and
	// saving it as jellyfin-web does keeps the rule.
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	root := s.signIn("root", "web")
	var me UserDto
	if s.get(t, "/Users/Me", token, &me); !me.Policy.EnableAllChannels || len(me.Policy.EnabledChannels) != 0 {
		t.Errorf("the policy's channels: %+v", me.Policy)
	}
	policy := map[string]any{"EnableLiveTvAccess": true, "EnableLiveTvManagement": true, "EnableAllChannels": true, "EnabledChannels": []string{},
		"EnableAllFolders": true}
	if status, body := s.call(http.MethodPost, "/Users/"+member.ID.String()+"/Policy", app("web", root), policy); status != http.StatusNoContent {
		t.Fatalf("policy: %d %s", status, body)
	}
	if user, err := s.store.User(t.Context(), member.ID); err != nil || user.LiveTvChannels.Mode != accounts.ChannelsExcept {
		t.Errorf("the rule after the policy was saved: %+v %v", user.LiveTvChannels, err)
	}
}

// A user's own sources are theirs: the administrator's rule, which names
// the server's channels, leaves them alone.
func TestChannelRuleLeavesOwnSourcesAlone(t *testing.T) {
	t.Parallel()
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	s.setting(t, func(settings *accounts.Settings) { settings.PersonalAddons = true })
	member := s.user("member", func(c *accounts.UserChanges) { c.UseSharedAddons = new(true) })
	token := s.signIn("member", "tv")
	account := iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Server", Account: account}, false); err != nil {
		t.Fatal(err)
	}
	s.limit(t, member, func(c *accounts.UserChanges) {
		c.LiveTvChannels = &accounts.ChannelRule{Mode: accounts.ChannelsOnly, Channels: []string{"none"}}
	})
	var channels QueryResult
	if s.get(t, "/LiveTv/Channels", token, &channels); len(channels.Items) != 0 {
		t.Fatalf("the server's channels outside the rule: %+v", channels.Items)
	}
	own := iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=own&password=secret"}
	if _, err := s.iptv.Add(t.Context(), addons.Personal(member.ID), iptv.NewSource{Name: "Mine", Account: own}, false); err != nil {
		t.Fatal(err)
	}
	if s.get(t, "/LiveTv/Channels", token, &channels); len(channels.Items) != 2 {
		t.Errorf("the user's own channels: %+v", channels.Items)
	}
}

// The channels Polyfin airs follow the server's channels in the numbering
// every user shares: a user whose rule hides some of the server's channels
// sees them under the same numbers as the others, never under the number
// of a channel listed before them.
func TestChannelRuleKeepsTheNumbersOfAiredChannels(t *testing.T) {
	t.Parallel()
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	s.user("other", nil)
	other := s.signIn("other", "tv")
	account := iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Account: account}, false); err != nil {
		t.Fatal(err)
	}
	store := virtualtv.New(s.pool)
	s.library.UseVirtual(ownChannels{store})
	if _, err := store.Create(t.Context(), virtualtv.Channel{Name: "Evenings", Enabled: true, Seed: 1, Epoch: time.Now(),
		Source:  virtualtv.Source{Kind: virtualtv.SourceTitles, Items: []virtualtv.SourceItem{{ID: accounts.ID{1}}}},
		Entries: []virtualtv.Entry{{Item: accounts.ID{1}, Title: accounts.ID{1}, Kind: virtualtv.EntryMovie, Name: "Feature", Runtime: time.Hour}}}); err != nil {
		t.Fatal(err)
	}
	numbers := func(token string) map[string]string {
		t.Helper()
		var channels QueryResult
		s.get(t, "/LiveTv/Channels", token, &channels)
		numbers := map[string]string{}
		for _, c := range channels.Items {
			numbers[c.Name] = c.Number
		}
		return numbers
	}
	everyone := numbers(other)
	if len(everyone) != 3 || everyone["Evenings"] == "" {
		t.Fatalf("the channels: %v", everyone)
	}
	s.limit(t, member, func(c *accounts.UserChanges) {
		c.LiveTvChannels = &accounts.ChannelRule{Mode: accounts.ChannelsExcept, Groups: []string{channelChoice(t, s, true, "News")}}
	})
	limited := numbers(token)
	if len(limited) != 2 || limited["Orbe Junior"] != everyone["Orbe Junior"] || limited["Evenings"] != everyone["Evenings"] {
		t.Errorf("the channels without the news, %v, numbered unlike everyone's %v", limited, everyone)
	}
}

// A rule names channels by the identity their line-up keeps: a refresh
// that renumbers the channels, and the administrator moving one to
// another category, keep it shown and the others hidden.
func TestChannelRuleSurvivesRenumberingAndRegrouping(t *testing.T) {
	t.Parallel()
	var renumbered atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zeb, orbe := "5", "6"
		if renumbered.Load() {
			zeb, orbe = "41", "1"
		}
		_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-chno=%q group-title=\"Kids\",Orbe Junior\n%s/live/2.ts\n"+
			"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-chno=%q group-title=\"News\",Zeb One\n%s/live/1.ts\n", orbe, server.URL, zeb, server.URL)
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Account: iptv.Account{Kind: addons.KindM3U, URL: server.URL + "/get.php"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	zeb := channelChoice(t, s, false, "Zeb One")
	s.limit(t, member, func(c *accounts.UserChanges) {
		c.LiveTvChannels = &accounts.ChannelRule{Mode: accounts.ChannelsOnly, Channels: []string{zeb}}
	})
	listed := func() []string {
		t.Helper()
		var channels QueryResult
		s.get(t, "/LiveTv/Channels", token, &channels)
		var names []string
		for _, c := range channels.Items {
			names = append(names, c.Name+" "+c.Number)
		}
		return names
	}
	if got := listed(); !slices.Equal(got, []string{"Zeb One 5"}) {
		t.Fatalf("before the refresh: %v", got)
	}
	renumbered.Store(true)
	if err := s.iptv.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"Zeb One 41"}) {
		t.Errorf("after the refresh: %v", got)
	}
	categories, err := s.iptv.Categories(t.Context(), addons.Shared(), addon.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	kids := categories[slices.IndexFunc(categories, func(c iptv.Category) bool { return c.Name == "Kids" })].ID
	_, found, err := s.iptv.ListChannels(t.Context(), addons.Shared(), addon.ID, iptv.ChannelFilter{Q: "zeb", Limit: 1})
	if err != nil || len(found) != 1 {
		t.Fatalf("the channel in the line-up: %v %v", found, err)
	}
	if _, err := s.iptv.UpdateChannel(t.Context(), addons.Shared(), addon.ID, found[0].ID,
		iptv.ChannelChanges{Category: &iptv.Nullable[accounts.ID]{Value: &kids}}); err != nil {
		t.Fatal(err)
	}
	if got := listed(); !slices.Equal(got, []string{"Zeb One 41"}) {
		t.Errorf("once moved to another category: %v", got)
	}
}

// Replay lists no folder of a channel the user's rule hides, and none of
// its programmes plays.
func TestChannelRuleHidesReplay(t *testing.T) {
	t.Parallel()
	provider := newReplayProvider(t, "")
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	addReplaySource(t, s, provider)
	folders, programmes := replayProgrammes(t, s, token)
	if len(folders) != 2 || folders[0].Name != "Zeb One" || len(programmes) == 0 {
		t.Fatalf("Replay before the rule: %+v %+v", folders, programmes)
	}
	zeb := channelChoice(t, s, false, "Zeb One")
	s.limit(t, member, func(c *accounts.UserChanges) {
		c.LiveTvChannels = &accounts.ChannelRule{Mode: accounts.ChannelsExcept, Channels: []string{zeb}}
	})
	if hidden, _ := replayProgrammes(t, s, token); len(hidden) != 1 || hidden[0].Name != "Quill" {
		t.Errorf("Replay folders with the rule: %+v", hidden)
	}
	for _, path := range []string{"/Items/" + folders[0].Id, "/Items/" + programmes[0].Id} {
		if status, body := s.call(http.MethodGet, path, app("tv", token), nil); status != http.StatusNotFound {
			t.Errorf("%s of a hidden channel: %d %s", path, status, body)
		}
	}
	if status, body := s.call(http.MethodPost, "/Items/"+programmes[0].Id+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a hidden channel's replay: %d %s", status, body)
	}
}
