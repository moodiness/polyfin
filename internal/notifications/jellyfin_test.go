package notifications

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/plays"
	"github.com/moodiness/polyfin/internal/userdata"
)

// jellyfinTarget is a Jellyfin target at path of the fake targets.
func (h harness) jellyfinTarget(path string, events ...string) Draft {
	return Draft{Kind: Jellyfin, Name: new("Jellyfin " + path), Address: new(h.targets.url + path), Events: events}
}

// The titles the Jellyfin tests play: a movie, and an episode of a series,
// as the library describes them.
func jellyfinTitles() (movie, series, episode library.Item) {
	released, aired, first := time.Date(2008, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 3, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2019, 9, 1, 0, 0, 0, 0, time.UTC)
	movie = library.Item{ID: accounts.ID{0x31}, Kind: library.KindMovie, Name: "Night Train", Overview: "A train at night.",
		Taglines: []string{"All aboard."}, Genres: []string{"Drama", "Mystery"}, ProductionYear: 2008, PremiereDate: &released,
		Runtime: 100 * time.Minute, ProviderIDs: map[string]string{"Imdb": "tt0000031", "Tmdb": "31"}}
	series = library.Item{ID: accounts.ID{0x40}, Kind: library.KindSeries, Name: "Harbour", ProductionYear: 2019, PremiereDate: &first}
	episode = library.Item{ID: accounts.ID{0x41}, Kind: library.KindEpisode, Name: "Pilot", SeriesID: series.ID, SeriesName: "Harbour",
		SeasonID: accounts.ID{0x42}, ParentIndexNumber: 1, IndexNumber: 2, ProductionYear: 2020, PremiereDate: &aired, Runtime: 45 * time.Minute,
		ProviderIDs: map[string]string{"Tvdb": "4102", "Imdb": "tt0000041"}}
	return movie, series, episode
}

// playedTitle is item as the playbacks Polyfin follows know it.
func playedTitle(item library.Item) plays.Title {
	t := plays.Title{Item: item.ID, Kind: plays.Movie, Name: item.Name}
	if item.Kind == library.KindEpisode {
		t.Kind, t.SeriesID, t.SeriesName, t.Season, t.Episode = plays.Episode, item.SeriesID, item.SeriesName, item.ParentIndexNumber, item.IndexNumber
	}
	return t
}

// player reports the playbacks of user on a device to a tracker telling
// h's service.
type player struct {
	tracker      *plays.Tracker
	user         accounts.User
	device       accounts.ID
	deviceID     string
	app, browser string
}

func (h harness) player(tracker *plays.Tracker, user accounts.User, device byte) player {
	return player{tracker: tracker, user: user, device: accounts.ID{0xd0, device}, deviceID: "device-" + string('a'+rune(device)),
		app: "Jellyfin Web", browser: "Chrome"}
}

func (p player) report(kind int, item library.Item, position time.Duration, paused bool) {
	p.tracker.Report(plays.Report{Kind: kind, Device: p.device, DeviceID: p.deviceID, User: p.user.ID, UserName: p.user.Name, App: p.app,
		DeviceName: p.browser, Title: playedTitle(item), Position: position, PositionKnown: true, Paused: paused, Method: plays.Conversion})
}

// fields checks that body holds want, field by field, numbers as JSON
// decodes them, and lacks the fields want maps to nil.
func fields(t *testing.T, what string, body map[string]any, want map[string]any) {
	t.Helper()
	for name, value := range want {
		got, ok := body[name]
		switch {
		case value == nil && ok:
			t.Errorf("%s: %s is %v, want none", what, name, got)
		case value != nil && got != value:
			t.Errorf("%s: %s is %#v, want %#v", what, name, got, value)
		}
	}
}

// minutes is a number of minutes in Jellyfin's ticks, as JSON decodes it.
func minutes(n int) float64 {
	return float64(time.Duration(n) * time.Minute / 100)
}

// A Jellyfin target receives each event as Jellyfin's webhook plugin sends
// it with all its properties: the server, the user, the item with its
// series and provider IDs, the playback or the saved data, by the plugin's
// field names, with the target's own headers.
func TestJellyfinTargetsReceiveThePluginsFields(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	movie, series, episode := jellyfinTitles()
	h.library.describe(movie, series, episode)
	draft := h.jellyfinTarget("/jellyfin", JellyfinEvents...)
	draft.Headers = &[]Header{{Name: "x-api-key", Value: "s3cret-api-key"}}
	added := h.add(t, nil, draft)
	if !slices.Equal(added.Headers, []string{"X-Api-Key"}) || added.Address != h.targets.url || !slices.Equal(added.Events, JellyfinEvents) {
		t.Fatalf("the target shows headers %v, address %q, events %v", added.Headers, added.Address, added.Events)
	}

	tracker := plays.NewTracker(h.Playback)
	phone := h.player(tracker, h.member, 1)
	phone.report(plays.ReportStart, movie, 0, false)
	phone.report(plays.ReportStop, movie, 99*time.Minute, false)
	phone.report(plays.ReportStart, episode, 0, false)
	phone.report(plays.ReportProgress, episode, time.Minute, true)
	phone.report(plays.ReportStop, episode, 10*time.Minute, false)
	lastPlayed := time.Date(2026, 11, 20, 21, 0, 0, 0, time.UTC)
	h.UserDataSaved(h.member, movie, userdata.Data{Played: true, PlayCount: 2, Favorite: true, Rating: new(8.0), LastPlayed: &lastPlayed},
		userdata.SavedTogglePlayed)
	h.dispatch(h.episodeEvent(nil, h.member.ID, episode), func(t target) bool { return t.owner == nil })
	blocked := h.member
	blocked.BlockedUntil, blocked.InvalidLoginAttempts = new(time.Now().Add(time.Hour)), 5
	h.AccountBlocked(blocked, accounts.DeviceInfo{Client: "Jellyfin Web", DeviceName: "Chrome", RemoteAddress: "203.0.113.5"})

	got := h.targets.wait(t, "/jellyfin", 8)
	var kinds []string
	for _, r := range got {
		kinds = append(kinds, r.body["NotificationType"].(string))
		if r.header.Get("X-Api-Key") != "s3cret-api-key" || r.header.Get("Content-Type") != "application/json" {
			t.Errorf("%s came with X-Api-Key %q, Content-Type %q", kinds[len(kinds)-1], r.header.Get("X-Api-Key"), r.header.Get("Content-Type"))
		}
		fields(t, kinds[len(kinds)-1], r.body, map[string]any{"ServerId": "fedcba9876543210fedcba9876543210",
			"ServerName": h.store.Settings().ServerName, "ServerVersion": "12.4.0", "ServerUrl": "", "type": nil, "version": nil})
		if at, err := time.Parse(time.RFC3339Nano, r.body["UtcTimestamp"].(string)); err != nil || time.Since(at) > time.Minute {
			t.Errorf("%s at %v (%v)", kinds[len(kinds)-1], r.body["UtcTimestamp"], err)
		}
		if _, err := time.Parse(time.RFC3339Nano, r.body["Timestamp"].(string)); err != nil {
			t.Errorf("%s: Timestamp %v: %v", kinds[len(kinds)-1], r.body["Timestamp"], err)
		}
	}
	want := []string{JellyfinPlaybackStart, JellyfinPlaybackStop, JellyfinPlaybackStart, JellyfinPlaybackProgress, JellyfinPlaybackStop,
		JellyfinUserDataSaved, JellyfinItemAdded, JellyfinUserLockedOut}
	if !slices.Equal(kinds, want) {
		t.Fatalf("received %v, want %v", kinds, want)
	}

	member, session := map[string]any{"NotificationUsername": "member", "Username": "member", "UserId": h.member.ID.String()},
		map[string]any{"DeviceId": "device-b", "DeviceName": "Chrome", "ClientName": "Jellyfin Web", "Client": "Jellyfin Web",
			"PlayMethod": "Transcode", "IsAutomated": false}
	movieFields := map[string]any{"ItemId": movie.ID.String(), "ItemType": "Movie", "Name": "Night Train", "Overview": "A train at night.",
		"Tagline": "All aboard.", "Genres": "Drama, Mystery", "Year": float64(2008), "PremiereDate": "2008-05-01",
		"RunTimeTicks": minutes(100), "RunTime": "01:40:00", "Provider_imdb": "tt0000031", "Provider_tmdb": "31",
		"SeriesName": nil, "SeasonNumber": nil}
	episodeFields := map[string]any{"ItemId": episode.ID.String(), "ItemType": "Episode", "Name": "Pilot", "SeriesName": "Harbour",
		"SeriesId": series.ID.String(), "SeasonId": episode.SeasonID.String(), "SeasonNumber": float64(1), "SeasonNumber00": "01",
		"SeasonNumber000": "001", "EpisodeNumber": float64(2), "EpisodeNumber00": "02", "EpisodeNumber000": "002",
		// An episode's year is its series'.
		"Year": float64(2019), "SeriesPremiereDate": "2019-09-01", "PremiereDate": "2020-03-04", "RunTimeTicks": minutes(45),
		"RunTime": "00:45:00", "Provider_tvdb": "4102", "Provider_imdb": "tt0000041"}
	for _, check := range []struct {
		what string
		body map[string]any
		want []map[string]any
	}{
		{"the movie's start", got[0].body, []map[string]any{member, session, movieFields,
			{"PlaybackPositionTicks": float64(0), "PlaybackPosition": "00:00:00", "IsPaused": false, "PlayedToCompletion": nil}}},
		{"the movie's stop", got[1].body, []map[string]any{member, session, movieFields,
			{"PlaybackPositionTicks": minutes(99), "PlaybackPosition": "01:39:00", "PlayedToCompletion": true}}},
		{"the episode's start", got[2].body, []map[string]any{member, session, episodeFields}},
		{"the episode's pause", got[3].body, []map[string]any{member, session, episodeFields,
			{"PlaybackPositionTicks": minutes(1), "PlaybackPosition": "00:01:00", "IsPaused": true}}},
		{"the episode's stop", got[4].body, []map[string]any{member, session, episodeFields,
			{"PlaybackPositionTicks": minutes(10), "PlayedToCompletion": false}}},
		{"the movie's data", got[5].body, []map[string]any{member, movieFields,
			{"Played": true, "Favorite": true, "PlayCount": float64(2), "Likes": true, "Rating": float64(8),
				"LastPlayedDate": "2026-11-20T21:00:00Z", "SaveReason": "TogglePlayed", "PlaybackPositionTicks": float64(0), "DeviceId": nil}}},
		{"the new episode", got[6].body, []map[string]any{episodeFields, {"UserId": nil, "NotificationUsername": nil, "DeviceId": nil}}},
		{"the blocked account", got[7].body, []map[string]any{member, {"ItemId": nil, "DeviceId": nil}}},
	} {
		for _, want := range check.want {
			fields(t, check.what, check.body, want)
		}
	}
}

// A server's Jellyfin target with a user filter receives the events of
// those users, and those of no user; each target receives the
// notification types it chose; a user's own receives their events only;
// and no other kind of target receives positions or saved data.
func TestJellyfinUserFiltersAndEventChoicesHold(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	movie, series, episode := jellyfinTitles()
	h.library.describe(movie, series, episode)
	filtered := h.jellyfinTarget("/admins", JellyfinEvents...)
	filtered.Users = &[]accounts.ID{h.admin.ID}
	if target := h.add(t, nil, filtered); !slices.Equal(target.Users, []accounts.ID{h.admin.ID}) {
		t.Fatalf("the filter is %v", target.Users)
	}
	h.add(t, nil, h.jellyfinTarget("/stops", JellyfinPlaybackStop))
	h.add(t, &h.member, h.jellyfinTarget("/mine", JellyfinEvents...))
	h.add(t, nil, h.webhook("/polyfin", EventsFor(nil)...))

	tracker := plays.NewTracker(h.Playback)
	phone, laptop := h.player(tracker, h.member, 1), h.player(tracker, h.admin, 2)
	phone.report(plays.ReportStart, movie, 0, false)
	laptop.report(plays.ReportStart, episode, 0, false)
	phone.report(plays.ReportStop, movie, time.Minute, false)
	laptop.report(plays.ReportStop, episode, time.Minute, false)
	h.UserDataSaved(h.member, movie, userdata.Data{Favorite: true}, userdata.SavedUpdateUserRating)
	h.UserDataSaved(h.admin, episode, userdata.Data{Played: true}, userdata.SavedTogglePlayed)
	h.dispatch(h.episodeEvent(nil, h.member.ID, episode), func(t target) bool { return t.owner == nil })
	h.targets.wait(t, "/admins", 4)
	h.targets.wait(t, "/mine", 3)
	h.targets.wait(t, "/stops", 2)
	h.idle(t)

	of := func(path string) []string {
		var found []string
		for _, r := range h.targets.at(path) {
			user, _ := r.body["NotificationUsername"].(string)
			found = append(found, strings.TrimSpace(r.body["NotificationType"].(string)+" "+user))
		}
		return found
	}
	for path, want := range map[string][]string{
		"/admins": {"PlaybackStart admin", "PlaybackStop admin", "UserDataSaved admin", "ItemAdded"},
		"/mine":   {"PlaybackStart member", "PlaybackStop member", "UserDataSaved member"},
		"/stops":  {"PlaybackStop member", "PlaybackStop admin"},
	} {
		if got := of(path); !slices.Equal(got, want) {
			t.Errorf("%s received %v, want %v", path, got, want)
		}
	}
	for _, r := range h.targets.at("/polyfin") {
		if kind := r.body["type"]; kind == PlaybackProgressed || kind == DataSaved {
			t.Errorf("a webhook received %v", kind)
		}
	}

	// What may not be saved.
	ghost := accounts.ID{0xee}
	for _, bad := range []struct {
		owner *accounts.User
		draft func(*Draft)
		want  error
	}{
		{&h.member, func(d *Draft) { d.Users = &[]accounts.ID{h.member.ID} }, ErrInvalidUserFilter},
		{nil, func(d *Draft) { d.Users = &[]accounts.ID{ghost} }, ErrInvalidUserFilter},
		{nil, func(d *Draft) { d.Kind, d.Events, d.Users = Webhook, nil, &[]accounts.ID{h.admin.ID} }, ErrInvalidUserFilter},
		{nil, func(d *Draft) { d.GroupMinutes = new(5) }, ErrInvalidGroupMinutes},
		{nil, func(d *Draft) { d.Events = []string{PlaybackStarted} }, ErrInvalidEvents},
		{nil, func(d *Draft) { d.Kind, d.Events = Webhook, []string{JellyfinPlaybackStart} }, ErrInvalidEvents},
		{nil, func(d *Draft) { d.Headers = &[]Header{{Name: "Bad Name", Value: "x"}} }, ErrInvalidHeaders},
		{nil, func(d *Draft) { d.Headers = &[]Header{{Name: "Host", Value: "example.org"}} }, ErrInvalidHeaders},
		{nil, func(d *Draft) { d.Headers = &[]Header{{Name: "X-Key", Value: "a"}, {Name: "x-key", Value: "b"}} }, ErrInvalidHeaders},
		{nil, func(d *Draft) { d.Headers = &[]Header{{Name: "X-Key", Value: "a\r\nX-Other: b"}} }, ErrInvalidHeaders},
		{nil, func(d *Draft) { d.Headers = &[]Header{{Name: "X-Key", Value: ""}} }, ErrInvalidHeaders},
		{nil, func(d *Draft) { d.Kind, d.Events, d.Headers = Webhook, nil, &[]Header{{Name: "X-Key", Value: "a"}} }, ErrInvalidHeaders},
	} {
		d := h.jellyfinTarget("/refused", JellyfinItemAdded)
		bad.draft(&d)
		if _, err := h.Create(t.Context(), bad.owner, d); !errors.Is(err, bad.want) {
			t.Errorf("%+v: %v, want %v", d, err, bad.want)
		}
	}
}

// A header saved without a value keeps its value, which is never shown
// nor logged; "Send a test" sends the plugin's Generic notification with
// the headers.
func TestJellyfinHeadersKeepTheirValuesUnseen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	draft := h.jellyfinTarget("/hooked", JellyfinItemAdded)
	draft.Headers = &[]Header{{Name: "Authorization", Value: "Bearer kept-very-secret"}}
	target := h.add(t, nil, draft)
	changed, err := h.Update(t.Context(), nil, target.ID, Draft{Headers: &[]Header{{Name: "authorization"}, {Name: "X-Extra", Value: "two"}}})
	if err != nil || !slices.Equal(changed.Headers, []string{"Authorization", "X-Extra"}) {
		t.Fatalf("headers %v: %v", changed.Headers, err)
	}
	h.targets.then("/hooked", reply(http.StatusBadRequest, `{"error":"Bearer kept-very-secret is wrong"}`))
	result, err := h.Test(t.Context(), nil, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := h.targets.wait(t, "/hooked", 1)[0]
	if got.header.Get("Authorization") != "Bearer kept-very-secret" || got.header.Get("X-Extra") != "two" {
		t.Errorf("the test came with %v", got.header)
	}
	fields(t, "the test", got.body, map[string]any{"NotificationType": "Generic", "Name": "Test message", "ServerVersion": "12.4.0"})
	if result.Delivered || strings.Contains(result.Answer, "kept-very-secret") {
		t.Errorf("the refused test answers %q", result.Answer)
	}
	listed, err := h.Targets(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if shown := strings.Join(listed[0].Headers, " ") + listed[0].Address; strings.Contains(shown, "kept") || strings.Contains(h.log.String(), "kept-very") {
		t.Errorf("a header's value shows: %q, or is logged", shown)
	}
}

// Positions are told at most every timing.progressEvery per playback,
// counted from its start, two playbacks apart; a pause is told at once.
// Positions saved for a user's item are paced the same, other saves sent
// at once.
func TestJellyfinProgressIsPacedPerPlayback(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.timing.progressEvery = 300 * time.Millisecond
	movie, series, episode := jellyfinTitles()
	h.library.describe(movie, series, episode)
	h.add(t, nil, h.jellyfinTarget("/progress", JellyfinPlaybackProgress))
	h.add(t, nil, h.jellyfinTarget("/saved", JellyfinUserDataSaved))

	tracker := plays.NewTracker(h.Playback)
	phone, laptop := h.player(tracker, h.member, 1), h.player(tracker, h.admin, 2)
	phone.report(plays.ReportStart, movie, 0, false)
	laptop.report(plays.ReportStart, episode, 0, false)
	for i := range 5 {
		phone.report(plays.ReportProgress, movie, time.Duration(i)*time.Second, false)
	}
	saved := func(reason string) { h.UserDataSaved(h.member, movie, userdata.Data{Position: time.Minute}, reason) }
	saved(userdata.SavedPlaybackStart)
	saved(userdata.SavedPlaybackProgress)
	saved(userdata.SavedPlaybackProgress)
	time.Sleep(350 * time.Millisecond)
	phone.report(plays.ReportProgress, movie, 10*time.Second, false)
	phone.report(plays.ReportProgress, movie, 11*time.Second, false)
	laptop.report(plays.ReportProgress, episode, 10*time.Second, false)
	phone.report(plays.ReportProgress, movie, 12*time.Second, true)
	saved(userdata.SavedPlaybackProgress)
	saved(userdata.SavedPlaybackProgress)
	saved(userdata.SavedTogglePlayed)
	saved(userdata.SavedPlaybackFinished)
	h.targets.wait(t, "/progress", 3)
	h.targets.wait(t, "/saved", 4)
	h.idle(t)

	var progress, reasons []string
	for _, r := range h.targets.at("/progress") {
		progress = append(progress, r.body["DeviceId"].(string)+" "+r.body["PlaybackPosition"].(string))
	}
	for _, r := range h.targets.at("/saved") {
		reasons = append(reasons, r.body["SaveReason"].(string))
	}
	if want := []string{"device-b 00:00:10", "device-c 00:00:10", "device-b 00:00:12"}; !slices.Equal(progress, want) {
		t.Errorf("positions told %v, want %v", progress, want)
	}
	want := []string{userdata.SavedPlaybackStart, userdata.SavedPlaybackProgress, userdata.SavedTogglePlayed, userdata.SavedPlaybackFinished}
	if !slices.Equal(reasons, want) {
		t.Errorf("saves told %v, want %v", reasons, want)
	}
}

// A Jellyfin target's message is tried again after a server error, as
// the same message, and keeps to the target's cap of messages a minute.
func TestJellyfinTargetsRetryAndKeepTheirCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.timing.minute, h.timing.giveUp = 500*time.Millisecond, 5*time.Second
	movie, series, episode := jellyfinTitles()
	h.library.describe(movie, series, episode)
	draft := h.jellyfinTarget("/capped", JellyfinPlaybackStart)
	draft.PerMinute = new(1)
	h.add(t, nil, draft)
	h.targets.then("/capped", status(http.StatusServiceUnavailable))

	tracker := plays.NewTracker(h.Playback)
	h.player(tracker, h.member, 1).report(plays.ReportStart, movie, 0, false)
	h.player(tracker, h.admin, 2).report(plays.ReportStart, episode, 0, false)
	got := h.targets.wait(t, "/capped", 3)
	h.idle(t)
	if len(got) != 3 {
		t.Fatalf("%d requests, want 3", len(got))
	}
	if got[0].body["UtcTimestamp"] != got[1].body["UtcTimestamp"] || got[1].body["ItemId"] != movie.ID.String() ||
		got[2].body["ItemId"] != episode.ID.String() {
		t.Errorf("sent %v, %v, %v; want the movie twice, then the episode", got[0].body["ItemId"], got[1].body["ItemId"], got[2].body["ItemId"])
	}
	for i := 1; i < len(got); i++ {
		if gap := got[i].at.Sub(got[i-1].at); gap < 450*time.Millisecond {
			t.Errorf("request %d came %v after the one before, within the cap", i, gap)
		}
	}
	targets, _ := h.Targets(t.Context(), nil)
	if targets[0].Problem != "" || targets[0].LastSentAt == nil {
		t.Errorf("after delivering: problem %q, last sent %v", targets[0].Problem, targets[0].LastSentAt)
	}
}
