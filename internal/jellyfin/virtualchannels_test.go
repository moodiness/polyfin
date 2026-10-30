package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// ownChannels lists the channels of a store, each played from a version
// standing for its playout.
type ownChannels struct{ store *virtualtv.Store }

func (o ownChannels) Channels(ctx context.Context) ([]virtualtv.Channel, error) {
	return o.store.Channels(ctx)
}

func (o ownChannels) Version(item accounts.ID, c virtualtv.Channel) library.Version {
	return library.Version{ID: accounts.ID{0xfe}, Item: item, Name: c.Name, URL: "http://127.0.0.1:1/playout/" + c.ID.String() + "/live.m3u8"}
}

// A channel built from titles lists its schedule as an ordinary channel's
// guide: the movie airing since 90 seconds ago, then the series' episode,
// one after the other, its programmes opened by their identifiers, and its
// one version its playout. A user whose parental control hides one of its
// titles does not see it, and a title a user hid leaves the guide.
func TestChannelsOfTitlesListTheirSchedule(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	member := s.user("member", nil)
	token := s.signIn("member", "web")
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	childToken := s.signIn("child", "web")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	allowed, show := parseTestID(t, itemIDOf(t, s, token, ids["Top"], "Allowed")), parseTestID(t, itemIDOf(t, s, token, ids["Shows"], "Show"))
	entries, err := s.handler.ChannelEntries(t.Context(), member, virtualtv.Source{Kind: virtualtv.SourceTitles,
		Items: []virtualtv.SourceItem{{ID: allowed}, {ID: show}}})
	if err != nil {
		t.Fatal(err)
	}
	// A movie without a runtime is given 90 minutes, an episode 30.
	if len(entries) != 2 || entries[0].Name != "Allowed" || entries[0].Runtime != 90*time.Minute || entries[1].Name != "Show" ||
		entries[1].Episode != "Pilot" || entries[1].Season != 1 || entries[1].Number != 1 || entries[1].Runtime != 30*time.Minute ||
		entries[1].Rating != "TV-MA" {
		t.Fatalf("entries: %+v", entries)
	}
	store := virtualtv.New(s.pool)
	s.library.UseVirtual(ownChannels{store})
	epoch := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	channel, err := store.Create(t.Context(), virtualtv.Channel{Name: "Evenings", Enabled: true, Source: virtualtv.Source{Kind: virtualtv.SourceTitles,
		Items: []virtualtv.SourceItem{{ID: allowed}, {ID: show}}}, Entries: entries, Seed: 1, Epoch: epoch})
	if err != nil {
		t.Fatal(err)
	}

	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 || channels.Items[0].Name != "Evenings" || channels.Items[0].Type != "TvChannel" || channels.Items[0].Number != "1" ||
		channels.Items[0].CurrentProgram == nil || channels.Items[0].CurrentProgram.Name != "Allowed" {
		t.Fatalf("channels: %+v", channels.Items)
	}
	id := channels.Items[0].Id
	now := time.Now().UTC()
	query := "/LiveTv/Programs?channelIds=" + id + "&MinEndDate=" + url.QueryEscape(now.Format(time.RFC3339)) +
		"&MaxStartDate=" + url.QueryEscape(now.Add(3*time.Hour).Format(time.RFC3339)) + "&SortBy=StartDate&Fields=ChannelInfo"
	var programs QueryResult
	s.get(t, query, token, &programs)
	var names []string
	for i, p := range programs.Items {
		names = append(names, p.Name)
		if *p.ChannelId != id || p.ChannelName != "Evenings" {
			t.Errorf("programme %d is not on the channel: %+v", i, p)
		}
		if i > 0 && !time.Time(*p.StartDate).Equal(time.Time(*programs.Items[i-1].EndDate)) {
			t.Errorf("programme %d starts at %v, the one before ended at %v", i, time.Time(*p.StartDate), time.Time(*programs.Items[i-1].EndDate))
		}
	}
	// 90 minutes of movie, 30 of episode, and the movie again.
	if !slices.Equal(names, []string{"Allowed", "Show", "Allowed"}) {
		t.Fatalf("programmes: %q", names)
	}
	// The movie airing now started 90 seconds ago, at the epoch.
	if first := programs.Items[0]; !time.Time(*first.StartDate).Equal(epoch) || first.IsMovie == nil {
		t.Errorf("the programme airing now starts at %v, want %v", time.Time(*first.StartDate), epoch)
	}
	episode := programs.Items[1]
	if episode.EpisodeTitle != "Pilot" || episode.IsSeries == nil || episode.ParentIndexNumber == nil || *episode.ParentIndexNumber != 1 {
		t.Errorf("the episode's programme: %+v", episode)
	}
	var opened BaseItemDto
	if status := s.get(t, "/LiveTv/Programs/"+episode.Id, token, &opened); status != http.StatusOK || opened.Name != "Show" ||
		opened.EpisodeTitle != "Pilot" {
		t.Errorf("the episode's programme by its identifier: %d %+v", status, opened)
	}
	versions, err := s.library.Versions(t.Context(), member, parseTestID(t, id))
	if err != nil || len(versions) != 1 || versions[0].URL != "http://127.0.0.1:1/playout/"+channel.ID.String()+"/live.m3u8" {
		t.Errorf("the channel's versions: %+v %v", versions, err)
	}

	// The child's parental control hides the series: the channel is not
	// theirs, nor are its programmes.
	var hidden QueryResult
	s.get(t, "/LiveTv/Channels", childToken, &hidden)
	if len(hidden.Items) != 0 {
		t.Errorf("the child sees a channel airing a TV-MA series: %+v", hidden.Items)
	}
	if status, _ := s.call(http.MethodGet, "/LiveTv/Programs/"+episode.Id, app("web", childToken), nil); status != http.StatusNotFound {
		t.Errorf("the child opens one of its programmes: %d", status)
	}
	// A series the member hid leaves their guide.
	if err := userdata.New(s.pool).Hide(t.Context(), member.ID, show, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.get(t, query, token, &programs)
	names = nil
	for _, p := range programs.Items {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"Allowed", "Allowed"}) {
		t.Errorf("programmes once the series is hidden: %q", names)
	}
}

func parseTestID(t *testing.T, raw string) accounts.ID {
	t.Helper()
	id, err := accounts.ParseID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The shared stream of a channel built from titles plays as any channel's:
// a user whose quality group is below it has it converted down by the live
// pipeline, and one who may not convert is refused it.
func TestQualityGroupsConvertChannelsOfTitlesDown(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	token := s.signIn("member", "web")
	store := virtualtv.New(s.pool)
	s.library.UseVirtual(ownChannels{store})
	entries := []virtualtv.Entry{{Item: accounts.ID{1}, Title: accounts.ID{1}, Kind: virtualtv.EntryMovie, Name: "Movie", Runtime: time.Hour}}
	if _, err := store.Create(t.Context(), virtualtv.Channel{Name: "Movies", Enabled: true, Source: virtualtv.Source{Kind: virtualtv.SourceTitles,
		Items: []virtualtv.SourceItem{{ID: accounts.ID{1}}}}, Entries: entries, Seed: 1, Epoch: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	channel := channels.Items[0].Id
	// The playout's stream, analyzed, is 1080p.
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Format, analysis.Duration, analysis.Remote = "hls", 0, true
	for i := range analysis.Streams {
		if analysis.Streams[i].Type == "video" {
			analysis.Streams[i].Width, analysis.Streams[i].Height = 1920, 1080
		}
	}
	data, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", accounts.ID{0xfe}, data); err != nil {
		t.Fatal(err)
	}
	chrome, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.group(t, user, 1080)
	if answer := s.ask(t, token, channel, chrome, nil); len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("1080p: %+v", answer)
	}
	s.group(t, user, 720)
	s.permit(t, user, false, true, true)
	if answer := s.ask(t, token, channel, chrome, nil); !answer.refused() {
		t.Errorf("720p without conversion: %+v", answer)
	}
	needsEncoders(t)
	s.permit(t, user, true, true, true)
	answer := s.ask(t, token, channel, chrome, nil)
	if len(answer.MediaSources) != 1 {
		t.Fatalf("720p: %+v", answer)
	}
	if source := answer.MediaSources[0]; source.SupportsDirectPlay || source.TranscodingUrl == "" || videoHeightOf(source) != 720 {
		t.Errorf("720p: direct play %v, height %d, %s", source.SupportsDirectPlay, videoHeightOf(source), source.TranscodingUrl)
	}
}
