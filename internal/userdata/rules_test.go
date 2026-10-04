package userdata

import (
	"testing"
	"time"
)

// jellyfin are the thresholds of a Jellyfin server's default configuration,
// which the server settings default to.
var jellyfin = Thresholds{Resume: 5, Played: 90}

// These cases are what a Jellyfin 12.1 server did with the same reports,
// on 6-minute and 1-minute items.
func TestPlaybackReportsFollowJellyfin(t *testing.T) {
	const long, short = 6 * time.Minute, time.Minute
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	played := Data{Played: true, PlayCount: 1}
	for _, tc := range []struct {
		name   string
		before Data
		report func(*Data)
		want   Data
	}{
		{"starting counts a play", Data{}, func(d *Data) { d.Start(now) },
			Data{PlayCount: 1, LastPlayed: &now}},
		{"halfway is a resume point", Data{}, func(d *Data) { d.Reach(long/2, long, jellyfin) },
			Data{Position: long / 2, Runtime: long}},
		{"just after the start is kept", Data{}, func(d *Data) { d.Reach(long*6/100, long, jellyfin) },
			Data{Position: long * 6 / 100, Runtime: long}},
		{"just before the end is kept", Data{}, func(d *Data) { d.Reach(long*89/100, long, jellyfin) },
			Data{Position: long * 89 / 100, Runtime: long}},
		{"the very start leaves nothing to resume", Data{Position: long / 2, Runtime: long}, func(d *Data) { d.Reach(long*3/100, long, jellyfin) },
			Data{Runtime: long}},
		{"the end plays the item without another play", Data{PlayCount: 1}, func(d *Data) { d.Reach(long*91/100, long, jellyfin) },
			Data{Played: true, PlayCount: 1, Runtime: long}},
		{"the start of a played item keeps it played", played, func(d *Data) { d.Reach(long*2/100, long, jellyfin) },
			Data{Played: true, PlayCount: 1, Runtime: long}},
		{"replaying a played item resumes it", played, func(d *Data) { d.Start(now); d.Reach(long/2, long, jellyfin) },
			Data{Played: true, PlayCount: 2, LastPlayed: &now, Position: long / 2, Runtime: long}},
		{"a short item is played past its start", Data{}, func(d *Data) { d.Reach(short/2, short, jellyfin) },
			Data{Played: true, Runtime: short}},
		{"a short item is not played at its very start", Data{}, func(d *Data) { d.Reach(short*3/100, short, jellyfin) },
			Data{Runtime: short}},
		{"a stop without a position plays the item once more", Data{PlayCount: 1}, func(d *Data) { d.Finish() },
			Data{Played: true, PlayCount: 2}},
		{"without a runtime the position is kept", Data{}, func(d *Data) { d.Reach(long/2, 0, jellyfin) },
			Data{Position: long / 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.before
			tc.report(&data)
			if data.Played != tc.want.Played || data.PlayCount != tc.want.PlayCount || data.Position != tc.want.Position ||
				data.Runtime != tc.want.Runtime || (data.LastPlayed == nil) != (tc.want.LastPlayed == nil) {
				t.Errorf("got %+v, want %+v", data, tc.want)
			}
		})
	}
}

func TestThresholdsChooseWhatIsKeptAndPlayed(t *testing.T) {
	const long, short = time.Hour, time.Minute
	for _, tc := range []struct {
		name       string
		thresholds Thresholds
		position   time.Duration
		runtime    time.Duration
		want       Data
	}{
		{"a higher played threshold keeps 95%", Thresholds{Resume: 5, Played: 98}, long * 95 / 100, long,
			Data{Position: long * 95 / 100, Runtime: long}},
		{"past it, the item is played", Thresholds{Resume: 5, Played: 98}, long * 99 / 100, long,
			Data{Played: true, Runtime: long}},
		{"a lower played threshold plays 75%", Thresholds{Resume: 5, Played: 70}, long * 75 / 100, long,
			Data{Played: true, Runtime: long}},
		{"at 100, only the very end is not resumed", Thresholds{Resume: 5, Played: 100}, long * 999 / 1000, long,
			Data{Position: long * 999 / 1000, Runtime: long}},
		{"a higher resume threshold drops 15%", Thresholds{Resume: 20, Played: 90}, long * 15 / 100, long,
			Data{Runtime: long}},
		{"past it, the resume point is kept", Thresholds{Resume: 20, Played: 90}, long * 25 / 100, long,
			Data{Position: long * 25 / 100, Runtime: long}},
		{"at 0, the first minute is kept", Thresholds{Resume: 0, Played: 90}, time.Minute, long,
			Data{Position: time.Minute, Runtime: long}},
		{"a short item is still played past its start", Thresholds{Resume: 20, Played: 100}, short / 2, short,
			Data{Played: true, Runtime: short}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data Data
			data.Reach(tc.position, tc.runtime, tc.thresholds)
			if data != tc.want {
				t.Errorf("got %+v, want %+v", data, tc.want)
			}
		})
	}
}

func TestPlayedPercentage(t *testing.T) {
	if percent, ok := (Data{Position: 90 * time.Second, Runtime: 6 * time.Minute}).PlayedPercentage(); !ok || percent != 25 {
		t.Errorf("a quarter in: %v %v", percent, ok)
	}
	for name, data := range map[string]Data{
		"no resume point": {Runtime: time.Hour},
		"unknown runtime": {Position: time.Minute},
	} {
		if _, ok := data.PlayedPercentage(); ok {
			t.Errorf("%s has a percentage", name)
		}
	}
}

// These cases are what a Jellyfin 12.1 server did when users marked items
// played or unplayed from an app.
func TestPlayedMarksFollowJellyfin(t *testing.T) {
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	earlier := time.Date(2018, 3, 3, 0, 0, 0, 0, time.UTC)
	older := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		before Data
		mark   func(*Data)
		want   Data
	}{
		{"a first mark counts a play now", Data{}, func(d *Data) { d.MarkPlayed(nil, now) },
			Data{Played: true, PlayCount: 1, LastPlayed: &now}},
		{"marking a played item again changes nothing", Data{Played: true, PlayCount: 1, LastPlayed: &earlier},
			func(d *Data) { d.MarkPlayed(nil, now) }, Data{Played: true, PlayCount: 1, LastPlayed: &earlier}},
		{"an undated mark keeps the plays counted", Data{PlayCount: 5}, func(d *Data) { d.MarkPlayed(nil, now) },
			Data{Played: true, PlayCount: 5, LastPlayed: &now}},
		{"an undated mark keeps the date", Data{LastPlayed: &earlier}, func(d *Data) { d.MarkPlayed(nil, now) },
			Data{Played: true, PlayCount: 1, LastPlayed: &earlier}},
		{"marking played drops the resume point", Data{PlayCount: 1, LastPlayed: &earlier, Position: time.Hour, Runtime: 2 * time.Hour},
			func(d *Data) { d.MarkPlayed(nil, now) }, Data{Played: true, PlayCount: 1, LastPlayed: &earlier, Runtime: 2 * time.Hour}},
		{"a dated mark counts one more play on its date", Data{Played: true, PlayCount: 2, LastPlayed: &now},
			func(d *Data) { d.MarkPlayed(&older, now) }, Data{Played: true, PlayCount: 3, LastPlayed: &older}},
		{"unmarking forgets plays, date and resume point", Data{Played: true, PlayCount: 3, LastPlayed: &now, Position: time.Hour, Favorite: true, Rating: new(10.0)},
			func(d *Data) { d.MarkUnplayed() }, Data{Favorite: true, Rating: new(10.0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.before
			tc.mark(&data)
			if data.Played != tc.want.Played || data.PlayCount != tc.want.PlayCount || data.Position != tc.want.Position ||
				data.Favorite != tc.want.Favorite || (data.Rating == nil) != (tc.want.Rating == nil) ||
				(data.LastPlayed == nil) != (tc.want.LastPlayed == nil) || data.LastPlayed != nil && !data.LastPlayed.Equal(*tc.want.LastPlayed) {
				t.Errorf("got %+v, want %+v", data, tc.want)
			}
		})
	}
}

func TestRatingsReadAsLikes(t *testing.T) {
	var d Data
	if d.Likes() != nil {
		t.Error("an unrated item is liked or disliked")
	}
	d.Like(true)
	if *d.Rating != 10 || !*d.Likes() {
		t.Errorf("a like: %v", *d.Rating)
	}
	d.Like(false)
	if *d.Rating != 1 || *d.Likes() {
		t.Errorf("a dislike: %v", *d.Rating)
	}
	for rating, likes := range map[float64]bool{4.5: false, 7: true} {
		if got := *(Data{Rating: &rating}).Likes(); got != likes {
			t.Errorf("rated %v: likes %v", rating, got)
		}
	}
}
