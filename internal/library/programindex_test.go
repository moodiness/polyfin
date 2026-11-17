package library

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// indexedGuide is a live TV catalog of two channels mapped to an XMLTV
// guide, whose statements are counted.
type indexedGuide struct {
	env
	traced *statements
	key    addons.LibraryKey
	guide  *guideServer
	// channels are the channels' Stremio identifiers, of this test only:
	// programme identifiers are remembered for the whole process.
	channels [2]string
	now      time.Time
}

func newIndexedGuide(t *testing.T, programmes string) indexedGuide {
	t.Helper()
	g := indexedGuide{env: newEnv(t), traced: &statements{}, now: time.Now().UTC().Truncate(time.Minute)}
	g.service.now = func() time.Time { return g.now }
	config := g.service.db.Config()
	config.ConnConfig.Tracer = g.traced
	traced, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	g.service.db = traced
	g.channels = [2]string{"tv:index-one-" + t.Name(), "tv:index-two-" + t.Name()}
	g.key = g.tvCatalog(addons.Shared(), stremio.Meta{ID: g.channels[0], Type: "tv", Name: "Index One"},
		stremio.Meta{ID: g.channels[1], Type: "tv", Name: "Index Two"})
	g.guide = newGuideServer(t, g.xmltv(programmes))
	if _, err := g.addons.SetGuide(t.Context(), addons.Shared(), g.key, g.guide.url); err != nil {
		t.Fatal(err)
	}
	g.download(t, programmes)
	return g
}

// xmltv is the guide of the two channels with programmes.
func (g indexedGuide) xmltv(programmes string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><tv><channel id="one"><display-name>Index One</display-name></channel>` +
		`<channel id="two"><display-name>Index Two</display-name></channel>` + programmes + `</tv>`
}

// download has the guide downloaded anew with programmes, then lists
// them, which keeps the channels as items, as a server's are.
func (g indexedGuide) download(t *testing.T, programmes string) {
	t.Helper()
	g.guide.set(g.xmltv(programmes), 0)
	if err := g.service.RefreshGuide(t.Context(), addons.Shared(), g.key); err != nil {
		t.Fatal(err)
	}
	listed, err := g.service.Programs(t.Context(), g.member, g.now.Add(-24*time.Hour), g.now.Add(24*time.Hour))
	if err != nil || len(listed) != strings.Count(programmes, "<programme ") {
		t.Fatalf("listed %d programmes of %d (%v)", len(listed), strings.Count(programmes, "<programme "), err)
	}
}

// id is the identifier of the programme starting at start on channel, 0
// or 1, as listings give it; it is forgotten, as after a restart.
func (g indexedGuide) id(channel int, start time.Time) accounts.ID {
	id := itemID(programKey(g.channels[channel], start.UTC().Format(time.RFC3339)))
	programRefs.Delete(id)
	return id
}

// lookups counts, among statements, the builds of the index, the reads of
// the guides' programmes, and the hashing searches of the former lookup.
func lookups(seen []statement) (builds, programmes, scans int) {
	for _, st := range seen {
		if strings.Contains(st.sql, "array_agg") && strings.Contains(st.sql, "live_guide_programmes") {
			builds++
		}
		if strings.Contains(st.sql, "live_guide_programmes") {
			programmes++
		}
		if strings.Contains(st.sql, "sha256") {
			scans++
		}
	}
	return builds, programmes, scans
}

func (g indexedGuide) item(t *testing.T, id accounts.ID) (Item, error, []statement) {
	t.Helper()
	var item Item
	var err error
	seen := g.traced.run(func() { item, err = g.service.Item(t.Context(), g.member, id) })
	return item, err, seen
}

// After a restart, a programme opened by its identifier is found in the
// index, built once from the guides, without hashing them in SQL; then
// other programmes are found in it.
func TestProgrammesAreFoundInTheIndexAfterARestart(t *testing.T) {
	t.Parallel()
	g := newIndexedGuide(t, "")
	now := g.now
	g.download(t, programme("one", now.Add(-time.Hour), now.Add(time.Hour), "Now")+
		programme("one", now.Add(time.Hour), now.Add(2*time.Hour), "Next")+
		programme("two", now.Add(-30*time.Minute), now.Add(30*time.Minute), "Elsewhere"))
	item, err, seen := g.item(t, g.id(0, now.Add(time.Hour)))
	if builds, _, scans := lookups(seen); err != nil || item.Name != "Next" || item.Channel == nil || item.Channel.StremioID != g.channels[0] ||
		builds != 1 || scans != 0 {
		t.Fatalf("after a restart: %+v %v, %d builds, %d scans", item, err, builds, scans)
	}
	item, err, seen = g.item(t, g.id(1, now.Add(-30*time.Minute)))
	if builds, _, scans := lookups(seen); err != nil || item.Name != "Elsewhere" || builds != 0 || scans != 0 {
		t.Errorf("another programme: %+v %v, %d builds, %d scans", item, err, builds, scans)
	}
	// The same as the listing's.
	listed, err := g.service.Programs(t.Context(), g.member, now, now.Add(time.Second))
	if err != nil || len(listed) != 2 {
		t.Fatal(listed, err)
	}
	for _, p := range listed {
		programRefs.Delete(p.ID)
		if found, err, _ := g.item(t, p.ID); err != nil || found.ID != p.ID || found.Name != p.Name || !found.StartDate.Equal(*p.StartDate) {
			t.Errorf("listed %+v, found %+v %v", p, found, err)
		}
	}
}

// An identifier of nothing, another server's or an app's folder, reads no
// programme: the index answers, once built.
func TestMissesReadNoProgramme(t *testing.T) {
	t.Parallel()
	g := newIndexedGuide(t, "")
	now := g.now
	g.download(t, programme("one", now.Add(-time.Hour), now.Add(time.Hour), "Now"))
	nothing := itemID("view|nothing-" + t.Name())
	if _, err, seen := g.item(t, nothing); err != ErrNotFound {
		t.Fatalf("an identifier of nothing: %v", err)
	} else if builds, _, scans := lookups(seen); builds != 1 || scans != 0 {
		t.Errorf("the first miss: %d builds, %d scans", builds, scans)
	}
	for _, id := range []accounts.ID{nothing, itemID("view|other-" + t.Name()), g.id(0, now.Add(-2*time.Hour))} {
		if _, err, seen := g.item(t, id); err != ErrNotFound {
			t.Errorf("a miss: %v", err)
		} else if _, programmes, _ := lookups(seen); programmes != 0 {
			t.Errorf("a miss read the programmes %d times: %v", programmes, seen)
		}
	}
}

// A new download of the guide, or a mapping, builds the index again: a
// programme added is found, one removed is not.
func TestTheIndexFollowsTheGuides(t *testing.T) {
	t.Parallel()
	g := newIndexedGuide(t, "")
	now := g.now
	kept := programme("one", now.Add(-time.Hour), now.Add(time.Hour), "Kept")
	g.download(t, kept+programme("one", now.Add(time.Hour), now.Add(2*time.Hour), "Removed"))
	if item, err, _ := g.item(t, g.id(0, now.Add(time.Hour))); err != nil || item.Name != "Removed" {
		t.Fatalf("before the download: %+v %v", item, err)
	}
	g.download(t, kept+programme("two", now.Add(time.Hour), now.Add(2*time.Hour), "Added"))
	item, err, seen := g.item(t, g.id(1, now.Add(time.Hour)))
	if builds, _, _ := lookups(seen); err != nil || item.Name != "Added" || builds != 1 {
		t.Errorf("a programme added: %+v %v, %d builds", item, err, builds)
	}
	if _, err, seen := g.item(t, g.id(0, now.Add(time.Hour))); err != ErrNotFound {
		t.Errorf("a programme removed: %v", err)
	} else if builds, _, _ := lookups(seen); builds != 0 {
		t.Errorf("a programme removed: %d builds", builds)
	}
	// A channel mapped anew builds it again too.
	if _, err := g.service.db.Exec(t.Context(), "UPDATE live_guide_maps SET xmltv_id = xmltv_id"); err != nil {
		t.Fatal(err)
	}
	item, err, seen = g.item(t, g.id(0, now.Add(-time.Hour)))
	if builds, _, _ := lookups(seen); err != nil || item.Name != "Kept" || builds != 1 {
		t.Errorf("after a mapping: %+v %v, %d builds", item, err, builds)
	}
}

// Eight identifiers looked up at once, as an Ids= list of programmes and
// of nothing asks them after a restart, build the index once and hash
// nothing in SQL.
func TestLookupsAtOnceShareOneBuild(t *testing.T) {
	t.Parallel()
	g := newIndexedGuide(t, "")
	now := g.now
	var programmes string
	for i := range 4 {
		start := now.Add(time.Duration(i) * time.Hour)
		programmes += programme("one", start, start.Add(time.Hour), "One "+string(rune('A'+i))) +
			programme("two", start, start.Add(time.Hour), "Two "+string(rune('A'+i)))
	}
	g.download(t, programmes)
	var ids []accounts.ID
	for i := range 3 {
		start := now.Add(time.Duration(i) * time.Hour)
		ids = append(ids, g.id(0, start), g.id(1, start))
	}
	ids = append(ids, itemID("view|a-"+t.Name()), itemID("view|b-"+t.Name()))
	found := make([]error, len(ids))
	seen := g.traced.run(func() {
		var wait sync.WaitGroup
		for i, id := range ids {
			wait.Go(func() { _, found[i] = g.service.Item(t.Context(), g.member, id) })
		}
		wait.Wait()
	})
	for i, err := range found {
		if want := i >= 6; (err == ErrNotFound) != want || err != nil && err != ErrNotFound {
			t.Errorf("identifier %d: %v", i, err)
		}
	}
	if builds, _, scans := lookups(seen); builds != 1 || scans != 0 {
		t.Errorf("eight at once: %d builds, %d scans", builds, scans)
	}
}

// The index hashes a programme's identifier as programItem does.
func TestTheIndexHashesAsProgrammes(t *testing.T) {
	t.Parallel()
	g := newIndexedGuide(t, "")
	// The guide gives it in its own zone (see xmltvTime).
	start := g.now.Add(3*time.Hour + 17*time.Second)
	g.download(t, programme("two", start, start.Add(time.Hour), "Zoned"))
	item, ok := programItem(Item{StremioID: g.channels[1]}, stremio.Video{StartTime: start.UTC().Format(time.RFC3339),
		EndTime: start.Add(time.Hour).UTC().Format(time.RFC3339)}, "")
	if !ok {
		t.Fatal("no programme")
	}
	programRefs.Delete(item.ID)
	if found, err, _ := g.item(t, item.ID); err != nil || found.Name != "Zoned" || !found.StartDate.Equal(start) {
		t.Errorf("found %+v %v", found, err)
	}
}
