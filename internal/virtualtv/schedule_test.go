package virtualtv

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func entries(runtimes ...time.Duration) []Entry {
	var list []Entry
	for i, runtime := range runtimes {
		var id accounts.ID
		id[0], id[1] = byte(i>>8), byte(i)
		list = append(list, Entry{Item: id, Title: id, Kind: EntryMovie, Name: fmt.Sprintf("Title %d", i), Runtime: runtime})
	}
	return list
}

func minutes(values ...int) []time.Duration {
	var list []time.Duration
	for _, v := range values {
		list = append(list, time.Duration(v)*time.Minute)
	}
	return list
}

// contiguous fails unless the airings follow each other without a gap
// nor an overlap, from first to last, each lasting.
func contiguous(t *testing.T, airings []Airing, first, last time.Time) {
	t.Helper()
	if len(airings) == 0 {
		t.Fatal("no airing")
	}
	if airings[0].Start.After(first) {
		t.Fatalf("the first airing starts at %v, after %v", airings[0].Start, first)
	}
	for i, a := range airings {
		if !a.End.After(a.Start) {
			t.Fatalf("airing %d lasts nothing: %v to %v", i, a.Start, a.End)
		}
		if i > 0 && !a.Start.Equal(airings[i-1].End) {
			t.Fatalf("airing %d starts at %v, the one before ended at %v", i, a.Start, airings[i-1].End)
		}
	}
	if end := airings[len(airings)-1].End; end.Before(last) {
		t.Fatalf("the last airing ends at %v, before %v", end, last)
	}
}

func TestScheduleIsDeterministicAndWithoutGaps(t *testing.T) {
	t.Parallel()
	epoch := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, shuffle := range []bool{false, true} {
		c := Channel{Entries: entries(minutes(42, 23, 95, 61, 7, 130, 44)...), Shuffle: shuffle, Seed: 7, Epoch: epoch}
		from, to := epoch.AddDate(0, 0, 40).Add(5*time.Hour+17*time.Minute), epoch.AddDate(0, 0, 47)
		week := c.Airings(from, to, time.UTC)
		contiguous(t, week, from, to)
		// Computed again, or in two parts, the schedule is the same.
		if again := c.Airings(from, to, time.UTC); !reflect.DeepEqual(week, again) {
			t.Fatalf("shuffle %v: the schedule changed when computed again", shuffle)
		}
		middle := from.Add(50 * time.Hour)
		parts := c.Airings(from, middle, time.UTC)
		second := c.Airings(middle, to, time.UTC)
		if parts[len(parts)-1].Start.Equal(second[0].Start) {
			second = second[1:]
		}
		if parts = append(parts, second...); !reflect.DeepEqual(week, parts) {
			t.Fatalf("shuffle %v: the schedule computed in two parts differs", shuffle)
		}
		// Nothing airs before the epoch.
		if before := c.Airings(epoch.Add(-time.Hour), epoch.Add(time.Minute), time.UTC); !before[0].Start.Equal(epoch) {
			t.Fatalf("shuffle %v: the first airing starts at %v, not at the epoch", shuffle, before[0].Start)
		}
	}
}

func TestScheduleShufflesEachRoundBySeed(t *testing.T) {
	t.Parallel()
	epoch := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	list := entries(minutes(10, 10, 10, 10, 10, 10, 10, 10)...)
	rounds := func(seed int64) [][]accounts.ID {
		c := Channel{Entries: list, Shuffle: true, Seed: seed, Epoch: epoch}
		var result [][]accounts.ID
		airings := c.Airings(epoch, epoch.Add(4*80*time.Minute), time.UTC)
		for i := 0; i < len(airings); i += len(list) {
			var round []accounts.ID
			for _, a := range airings[i : i+len(list)] {
				round = append(round, a.Entry.Item)
			}
			result = append(result, round)
		}
		return result
	}
	first := rounds(1)
	if len(first) != 4 {
		t.Fatalf("got %d rounds, want 4", len(first))
	}
	for i, round := range first {
		// Each round airs each entry once.
		sorted := slices.Clone(round)
		slices.SortFunc(sorted, func(a, b accounts.ID) int { return int(a[1]) - int(b[1]) })
		for j, id := range sorted {
			if id != list[j].Item {
				t.Fatalf("round %d does not air each entry once: %v", i, round)
			}
		}
	}
	if reflect.DeepEqual(first[0], first[1]) && reflect.DeepEqual(first[1], first[2]) {
		t.Fatal("every round has the same order")
	}
	if reflect.DeepEqual(first, rounds(2)) {
		t.Fatal("another seed gives the same order")
	}
	if !reflect.DeepEqual(first, rounds(1)) {
		t.Fatal("the same seed gives another order")
	}
}

func TestScheduleClipsBlocks(t *testing.T) {
	t.Parallel()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	// The block airs 50-minute episodes from 18:00 to 20:00: two whole,
	// then one cut at 20:00, which airs in full the next evening.
	block := Block{Start: 18 * 60, End: 20 * 60, Entries: entries(minutes(50, 50, 50, 50, 50, 50, 50)...)}
	for i := range block.Entries {
		block.Entries[i].Item[0] = 9
	}
	epoch := time.Date(2026, 3, 27, 0, 0, 0, 0, paris)
	c := Channel{Entries: entries(minutes(95, 37, 120)...), Shuffle: true, Seed: 3, Epoch: epoch, Blocks: []Block{block}}
	// Over the change to summer time, on the 29th.
	from, to := epoch, epoch.AddDate(0, 0, 5)
	airings := c.Airings(from, to, paris)
	contiguous(t, airings, from, to)
	var evenings [][]Airing
	for _, a := range airings {
		inBlock := a.Entry.Item[0] == 9
		local := a.Start.In(paris)
		minute := local.Hour()*60 + local.Minute()
		if inBlock != (minute >= 18*60 && minute < 20*60) {
			t.Fatalf("%v airs %q, in the block %v", local, a.Entry.Name, inBlock)
		}
		if !inBlock {
			continue
		}
		if minute == 18*60 {
			evenings = append(evenings, nil)
		}
		evenings[len(evenings)-1] = append(evenings[len(evenings)-1], a)
	}
	if len(evenings) != 5 {
		t.Fatalf("got %d evenings, want 5", len(evenings))
	}
	for day, evening := range evenings {
		if len(evening) != 3 {
			t.Fatalf("evening %d airs %d entries, want 3", day, len(evening))
		}
		last := evening[2]
		if end := last.End.In(paris); end.Hour() != 20 || end.Minute() != 0 || last.End.Sub(last.Start) != 20*time.Minute {
			t.Fatalf("evening %d's last entry airs %v to %v, want 19:40 to 20:00", day, last.Start.In(paris), end)
		}
		// What the end cut opens the next evening, in full.
		if day+1 < len(evenings) {
			next := evenings[day+1][0]
			if next.Entry.Item != last.Entry.Item || next.End.Sub(next.Start) != 50*time.Minute {
				t.Fatalf("evening %d opens with %q for %v, want %q in full", day+1, next.Entry.Name, next.End.Sub(next.Start), last.Entry.Name)
			}
		}
	}
	// The day of the change has 23 hours, aired whole.
	day := c.Airings(time.Date(2026, 3, 29, 0, 0, 0, 0, paris), time.Date(2026, 3, 30, 0, 0, 0, 0, paris), paris)
	contiguous(t, day, time.Date(2026, 3, 29, 0, 0, 0, 0, paris), time.Date(2026, 3, 30, 0, 0, 0, 0, paris))
}

func TestScheduleAirsAnEntryShorterTimeOnce(t *testing.T) {
	t.Parallel()
	// A 30-minute block of 2-hour movies airs a new one each day.
	block := Block{Start: 60, End: 90, Entries: entries(minutes(120, 120, 120)...)}
	epoch := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	c := Channel{Entries: entries(minutes(30)...), Epoch: epoch, Blocks: []Block{block}}
	var aired []accounts.ID
	for day := range 3 {
		at := epoch.AddDate(0, 0, day).Add(70 * time.Minute)
		a, ok := c.AiringAt(at, time.UTC)
		if !ok {
			t.Fatalf("nothing airs at %v", at)
		}
		aired = append(aired, a.Entry.Item)
	}
	if aired[0] == aired[1] || aired[1] == aired[2] {
		t.Fatalf("the block airs the same movie two days in a row: %v", aired)
	}
}

func TestAiringAtNinetySecondsIn(t *testing.T) {
	t.Parallel()
	epoch := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c := Channel{Entries: entries(minutes(25, 40)...), Epoch: epoch}
	// The second entry airs from 00:25 on.
	a, ok := c.AiringAt(epoch.Add(25*time.Minute+90*time.Second), time.UTC)
	if !ok || a.Entry.Item != c.Entries[1].Item || a.Start != epoch.Add(25*time.Minute) {
		t.Fatalf("got %+v, want the second entry from 00:25", a)
	}
	if _, ok := c.AiringAt(epoch.Add(-time.Second), time.UTC); ok {
		t.Fatal("something airs before the epoch")
	}
}

func TestValidateBlocks(t *testing.T) {
	t.Parallel()
	valid := Channel{Name: "Evenings", Source: Source{Kind: SourceSeries, Items: []SourceItem{{ID: accounts.ID{1}}}}, Entries: entries(minutes(30)...),
		Blocks: []Block{{Start: 0, End: 60, Source: Source{Kind: SourceTitles, Items: []SourceItem{{ID: accounts.ID{2}}}}, Entries: entries(minutes(20)...)}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	overlapping := valid
	overlapping.Blocks = append(slices.Clone(valid.Blocks), Block{Start: 30, End: 90, Source: valid.Blocks[0].Source, Entries: valid.Blocks[0].Entries})
	if err := overlapping.Validate(); err != ErrInvalidBlocks {
		t.Fatalf("overlapping blocks: got %v", err)
	}
	empty := valid
	empty.Entries = nil
	if err := empty.Validate(); err != ErrNoEntries {
		t.Fatalf("no entries: got %v", err)
	}
}
