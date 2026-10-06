package library

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A guide from a user's own addon, which may only reach public addresses,
// can describe a programme of a channel the server's addon lists: its
// artwork stays confined.
func TestProgrammeArtworkIsConfinedByItsGuide(t *testing.T) {
	e := newEnv(t)
	addon := accounts.ID{1}
	channel := record{ID: itemID(channelKey("tv:one")), Key: channelKey("tv:one"), Kind: KindChannel, Addon: &addon,
		Meta: &stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"}}
	video := stremio.Video{ID: "p", Thumbnail: "http://10.0.0.1/thumb.jpg", StartTime: "2026-01-01T10:00:00Z", EndTime: "2026-01-01T11:00:00Z"}
	for _, tc := range []struct {
		name            string
		guide, channels bool
		save            bool
		want            bool
	}{
		{"confined guide, open channel", true, false, true, true},
		{"open guide, confined channel", false, true, true, true},
		{"both open", false, false, true, false},
		{"channel unknown", false, false, false, true},
	} {
		key := programKey("tv:one", tc.name)
		program := record{ID: itemID(key), Key: key, Kind: KindProgram, Channel: "tv:one", Video: &video, Confined: tc.guide}
		if !tc.save {
			program.Channel = "tv:unknown"
		}
		channel.Confined = tc.channels
		if err := e.service.save(t.Context(), []record{channel, program}); err != nil {
			t.Fatal(err)
		}
		_, confined, err := e.service.Artwork(t.Context(), program.ID, "Primary")
		if err != nil || confined != tc.want {
			t.Errorf("%s: confined %v (%v), want %v", tc.name, confined, err, tc.want)
		}
	}
}

// guideChannels installs a catalog of n channels whose guide gives each
// three programmes: one airing now and the next two, and fetches it.
func (e env) guideChannels(t *testing.T, n int, now time.Time) []Item {
	t.Helper()
	var metas []stremio.Meta
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv>`)
	for i := range n {
		id := fmt.Sprintf("ch%03d", i)
		metas = append(metas, stremio.Meta{ID: "tv:" + id, Type: "tv", Name: "Channel " + id, ChannelNumber: i + 1})
		fmt.Fprintf(&body, `<channel id="%s"><display-name>Channel %s</display-name></channel>`, "tv:"+id, id)
		for k := range 3 {
			start := now.Add(time.Duration(k-1)*time.Hour + 30*time.Minute)
			body.WriteString(programme("tv:"+id, start, start.Add(time.Hour), fmt.Sprintf("Show %d of %s", k, id)))
		}
	}
	body.WriteString(`</tv>`)
	key := e.tvCatalog(addons.Shared(), metas...)
	guide := newGuideServer(t, body.String())
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	channels, err := e.service.Channels(t.Context(), e.member)
	if err != nil || len(channels) != n {
		t.Fatalf("channels: %d %v", len(channels), err)
	}
	return channels
}

// A guide page reads the programmes of its channels only, and a listing by
// start time no further than its limit; programmes are kept as no item,
// yet each opens by its identifier, even once forgotten from memory.
func TestGuidePagesReadOnlyWhatTheyShow(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	channels := e.guideChannels(t, 30, now)
	page := []accounts.ID{channels[3].ID, channels[7].ID}
	programs, err := e.service.Guide(t.Context(), e.member, GuideQuery{From: now, To: now.Add(24 * time.Hour), Channels: page})
	if err != nil {
		t.Fatal(err)
	}
	if len(programs) != 6 {
		t.Fatalf("programmes of two channels: %q", programTitles(programs))
	}
	for _, p := range programs {
		if p.Channel.ID != page[0] && p.Channel.ID != page[1] {
			t.Errorf("a programme of another channel: %s", p.Channel.Name)
		}
	}
	var kept int
	if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM items WHERE kind = 'program'").Scan(&kept); err != nil || kept != 0 {
		t.Errorf("%d programmes kept as items (%v)", kept, err)
	}
	// Upcoming, by start time, five of them.
	upcoming, err := e.service.Guide(t.Context(), e.member, GuideQuery{From: now, To: now.Add(7 * 24 * time.Hour), Limit: 5,
		Keep: func(p Item) bool { return p.StartDate.After(now) }})
	if err != nil {
		t.Fatal(err)
	}
	if got := programTitles(upcoming); len(got) != 5 || !strings.HasSuffix(got[0], "Show 1 of ch000") || !strings.HasSuffix(got[4], "Show 1 of ch004") {
		t.Errorf("five upcoming: %q", got)
	}
	// Opened by its identifier: from the programmes listed, then, once
	// they are forgotten (a restart), from the guide.
	wanted := programs[1]
	for _, forget := range []bool{false, true} {
		if forget {
			programRefs.Delete(wanted.ID)
		}
		item, err := e.service.Item(t.Context(), e.member, wanted.ID)
		if err != nil || item.Name != wanted.Name || item.Kind != KindProgram || item.Channel == nil || item.Channel.ID != wanted.Channel.ID {
			t.Errorf("programme by id (forgotten %v): %+v %v", forget, item, err)
		}
	}
	if _, err := e.service.Item(t.Context(), e.member, accounts.ID{1, 2, 3}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an identifier of nothing: %v", err)
	}
}

// Programmes kept as items, of Native EPG guides, are deleted two days
// after they ended, unless a timer or a recording names them.
func TestPastProgrammesAreSwept(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC()
	e.service.now = func() time.Time { return now }
	save := func(name string, end time.Time) accounts.ID {
		key := programKey("tv:one", name)
		video := stremio.Video{ID: name, StartTime: end.Add(-time.Hour).Format(time.RFC3339), EndTime: end.Format(time.RFC3339)}
		if err := e.service.save(t.Context(), []record{{ID: itemID(key), Key: key, Kind: KindProgram, Channel: "tv:one", Video: &video}}); err != nil {
			t.Fatal(err)
		}
		return itemID(key)
	}
	old := save("old", now.Add(-72*time.Hour))
	recorded := save("recorded", now.Add(-72*time.Hour))
	recent := save("recent", now.Add(-time.Hour))
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO live_recordings (channel_id, program_id, name, start_at, end_at, status, ended_at)
		VALUES ($1, $2, 'Recorded', $3, $4, 'Completed', $4)`, accounts.ID{9}, recorded, now.Add(-73*time.Hour), now.Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	deleted, err := e.service.SweepPrograms(t.Context())
	if err != nil || deleted != 1 {
		t.Fatalf("swept %d: %v", deleted, err)
	}
	for id, want := range map[accounts.ID]bool{old: false, recorded: true, recent: true} {
		if _, err := e.service.load(t.Context(), id); (err == nil) != want {
			t.Errorf("%v kept: %v, want %v", id, err == nil, want)
		}
	}
}
