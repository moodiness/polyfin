package library

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

func programKeys(programs []Item) []string {
	result := make([]string, 0, len(programs))
	for _, p := range programs {
		result = append(result, p.Channel.Name+" "+p.Channel.Number+": "+p.Name+" @"+p.StartDate.UTC().Format(time.TimeOnly))
	}
	return result
}

// What airs within a short time is read from the programmes that may
// still air in it only: it lists what reading the channels' whole guide
// lists, a programme longer than any other, overlapping ones and those
// ending or starting at the window's edges among them, and again once
// the guide is downloaded anew. A listing by channel number with a limit
// lists the first ones a listing of every channel lists.
func TestAiringListsWhatTheWholeGuideLists(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(),
		stremio.Meta{ID: "tv:a", Type: "tv", Name: "A"},
		stremio.Meta{ID: "tv:b", Type: "tv", Name: "B"},
		stremio.Meta{ID: "tv:c", Type: "tv", Name: "C"},
		stremio.Meta{ID: "tv:d", Type: "tv", Name: "D"},
		stremio.Meta{ID: "tv:e", Type: "tv", Name: "E"},
	)
	body := `<?xml version="1.0" encoding="UTF-8"?><tv>
		<channel id="a"><display-name>A</display-name></channel>
		<channel id="b"><display-name>B</display-name></channel>
		<channel id="c"><display-name>C</display-name></channel>
		<channel id="d"><display-name>D</display-name></channel>` +
		// A: a marathon of 30 hours over now, and shorter ones.
		programme("a", now.Add(-20*time.Hour), now.Add(10*time.Hour), "Marathon") +
		programme("a", now.Add(-time.Hour), now.Add(-30*time.Minute), "Earlier") +
		programme("a", now.Add(-30*time.Minute), now, "Ends now") +
		programme("a", now, now.Add(time.Hour), "Starts now") +
		// B: overlapping programmes.
		programme("b", now.Add(-2*time.Hour), now.Add(2*time.Hour), "Long news") +
		programme("b", now.Add(-10*time.Minute), now.Add(20*time.Minute), "Flash") +
		programme("b", now.Add(time.Second), now.Add(time.Hour), "Next second") +
		// C: one programme an hour.
		programme("c", now.Add(-90*time.Minute), now.Add(-30*time.Minute), "C past") +
		programme("c", now.Add(-30*time.Minute), now.Add(30*time.Minute), "C now") +
		programme("c", now.Add(30*time.Minute), now.Add(90*time.Minute), "C next") +
		// D: nothing now.
		programme("d", now.Add(3*time.Hour), now.Add(4*time.Hour), "D later") +
		`</tv>`
	guide := newGuideServer(t, body)
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	windows := [][2]time.Time{
		{now, now.Add(time.Second)},
		{now.Add(-time.Second), now},
		{now.Add(-10 * time.Minute), now.Add(50 * time.Minute)},
		{now.Add(2 * time.Hour), now.Add(2*time.Hour + time.Second)},
		{now.Add(3 * time.Hour), now.Add(4 * time.Hour)},
	}
	compare := func(when string) {
		t.Helper()
		v, err := e.service.view(t.Context(), e.member)
		if err != nil {
			t.Fatal(err)
		}
		all, _ := e.service.channels(t.Context(), v)
		channels := map[string]Item{}
		for _, c := range all {
			channels[c.StremioID] = c
		}
		keep := func(Item) bool { return true }
		for _, w := range windows {
			whole, err := e.service.guidePrograms(t.Context(), v, channels, nil, GuideQuery{From: w[0], To: w[1]}, keep)
			if err != nil {
				t.Fatal(err)
			}
			airing, err := e.service.airingGuidePrograms(t.Context(), v, channels, nil, w[0], w[1], keep)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := programKeys(airing), programKeys(whole); !slices.Equal(got, want) || len(want) == 0 && w[0].Before(now.Add(time.Hour)) {
				t.Errorf("%s, %s to %s: airing %q, whole guide %q", when, w[0].Sub(now), w[1].Sub(now), got, want)
			}
			// A listing by channel with a limit lists the first ones.
			byChannel, err := e.service.Guide(t.Context(), e.member, GuideQuery{From: w[0], To: w[1], ByChannel: true})
			if err != nil {
				t.Fatal(err)
			}
			for limit := 1; limit <= len(byChannel)+1; limit++ {
				first, err := e.service.Guide(t.Context(), e.member, GuideQuery{From: w[0], To: w[1], ByChannel: true, Limit: limit})
				if err != nil {
					t.Fatal(err)
				}
				if got, want := programKeys(first), programKeys(byChannel[:min(limit, len(byChannel))]); !slices.Equal(got, want) {
					t.Errorf("%s, %s: the first %d by channel %q, want %q", when, w[0].Sub(now), limit, got, want)
				}
			}
		}
	}
	compare("first download")
	// Downloaded anew, with a longer marathon: the span read for the last
	// download no longer bounds the read.
	guide.set(strings.Replace(body, xmltvTime(now.Add(-20*time.Hour)), xmltvTime(now.Add(-40*time.Hour)), 1), 0)
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	compare("second download")
}

// A listing by channel number with a limit reads the first channels a
// slice at a time, each slice with every channel of its last one's number,
// and stops once it has enough: it lists what sorting every channel's
// programmes lists first.
func TestFirstChannelsListWhatEveryChannelLists(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)
	numbers := []string{"1", "2", "2", "2", "3", "4.1", "4", "10", "10", "11", "12", "13"}
	channels := map[string]Item{}
	programmes := map[string][]Item{}
	for i, number := range numbers {
		id := fmt.Sprintf("tv:%d", i)
		channel := Item{ID: itemID("test|" + id), StremioID: id, Name: "Channel " + id, Number: number}
		channels[id] = channel
		// Every third channel has nothing on; the others one or two
		// programmes, at different starts.
		for j := range i % 3 {
			at := start.Add(time.Duration(j*10-i) * time.Minute)
			end := at.Add(time.Hour)
			programmes[id] = append(programmes[id], Item{ID: itemID(fmt.Sprintf("p|%d|%d", i, j)), Kind: KindProgram,
				Name: fmt.Sprintf("P%d.%d", i, j), StartDate: &at, EndDate: &end,
				Channel: &channel})
		}
	}
	var reads [][]string
	listed := func(part map[string]Item, _ GuideQuery) ([]Item, error) {
		var names []string
		var found []Item
		for id := range part {
			names = append(names, part[id].Number)
			found = append(found, programmes[id]...)
		}
		slices.Sort(names)
		reads = append(reads, names)
		return found, nil
	}
	every, _ := listed(channels, GuideQuery{})
	sortPrograms(every, true)
	for limit := 1; limit <= len(every)+1; limit++ {
		reads = nil
		first, err := firstChannelsPrograms(channels, GuideQuery{Limit: limit}, 2, listed)
		if err != nil {
			t.Fatal(err)
		}
		sortPrograms(first, true)
		first = first[:min(limit, len(first))]
		if got, want := programKeys(first), programKeys(every[:min(limit, len(every))]); !slices.Equal(got, want) {
			t.Errorf("first %d: %q, want %q (read %q)", limit, got, want, reads)
		}
		// No slice cuts a number in two.
		for _, read := range reads {
			if slices.Contains(read, "2") && slices.Index(read, "2")+3 > len(read) || slices.Contains(read, "10") && !slices.Equal(read[slices.Index(read, "10"):][:2], []string{"10", "10"}) {
				t.Errorf("first %d: a slice cuts a number: %q", limit, reads)
			}
		}
	}
	// One programme is read from the first slice only.
	reads = nil
	if _, err := firstChannelsPrograms(channels, GuideQuery{Limit: 1}, 2, listed); err != nil || len(reads) != 1 {
		t.Errorf("one programme read %q (%v)", reads, err)
	}
}

// An identifier searched among the guides' programmes in vain is not
// searched again while the guides keep their downloads: a new download
// may hold it, and it is searched again.
func TestProgrammeMissesHoldWhileTheGuidesDo(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	// A channel of its own: programme identifiers, remembered for the whole
	// process, are those of no other test.
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:misses", Type: "tv", Name: "Misses"})
	guide := newGuideServer(t, `<?xml version="1.0" encoding="UTF-8"?><tv><channel id="misses"><display-name>Misses</display-name></channel>`+
		programme("misses", now.Add(-time.Hour), now.Add(time.Hour), "Now")+`</tv>`)
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	airing, err := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second))
	if err != nil || len(airing) != 1 {
		t.Fatal(airing, err)
	}
	id := airing[0].ID
	// As after a restart: nothing listed lately.
	programRefs.Delete(id)
	if item, err := e.service.Item(t.Context(), e.member, id); err != nil || item.Name != "Now" {
		t.Fatalf("searched: %+v %v", item, err)
	}
	// A view's identifier is no programme: the miss is remembered.
	view := itemID("view|test-" + t.Name())
	if _, err := e.service.Item(t.Context(), e.member, view); err != ErrNotFound {
		t.Fatalf("a view: %v", err)
	}
	state, err := e.service.guideState(t.Context())
	if missed, ok := programMisses.Get(view); err != nil || !ok || missed != state {
		t.Fatalf("miss remembered %q %v, state %q (%v)", missed, ok, state, err)
	}
	// A miss holds while the guides do: one remembered for the programme
	// hides it.
	programRefs.Delete(id)
	programMisses.Put(id, state)
	if _, err := e.service.Item(t.Context(), e.member, id); err != ErrNotFound {
		t.Fatalf("a remembered miss: %v", err)
	}
	// Downloaded anew, the guides are searched again.
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	if item, err := e.service.Item(t.Context(), e.member, id); err != nil || item.Name != "Now" {
		t.Errorf("after a new download: %+v %v", item, err)
	}
	// So when a channel is mapped anew.
	programRefs.Delete(id)
	if state, err = e.service.guideState(t.Context()); err != nil {
		t.Fatal(err)
	}
	programMisses.Put(id, state)
	if _, err := e.service.db.Exec(t.Context(), "UPDATE live_guide_maps SET xmltv_id = xmltv_id"); err != nil {
		t.Fatal(err)
	}
	if item, err := e.service.Item(t.Context(), e.member, id); err != nil || item.Name != "Now" {
		t.Errorf("after a mapping: %+v %v", item, err)
	}
}
