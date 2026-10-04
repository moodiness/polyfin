package library

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// mappingsOf tells each channel of a catalog's mapping, as
// "Name=guide position:guide channel", "*" marking a manual one.
func (e env) mappingsOf(key addons.LibraryKey, guides []addons.Guide) []string {
	e.t.Helper()
	_, mappings, err := e.service.Mappings(e.t.Context(), addons.Shared(), key, MappingAll, "", 0, 100)
	if err != nil {
		e.t.Fatal(err)
	}
	var result []string
	for _, m := range mappings {
		described := m.Name + "="
		if m.Mapping != nil && m.Mapping.Guide != nil {
			position := slices.IndexFunc(guides, func(g addons.Guide) bool { return g.ID == *m.Mapping.Guide }) + 1
			described += string(rune('0'+position)) + ":" + *m.Mapping.GuideChannel
		}
		if m.Mapping != nil && m.Mapping.Manual {
			described += "*"
		}
		result = append(result, described)
	}
	return result
}

// A catalog's channels map to its guides' channels by the identifier and
// name rules, the first guide winning among equal candidates even over
// one with more titles; manual mappings, "no guide" included, survive
// downloads and "unmapped" automatic mapping, and only a remap replaces
// them. The programmes follow the mappings.
func TestGuidesMapByRankAndKeepManualMappings(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(),
		stremio.Meta{ID: "tv:one", Type: "tv", Name: "Une"},
		stremio.Meta{ID: "tv:two", Type: "tv", Name: "Deux"},
		stremio.Meta{ID: "tv:three", Type: "tv", Name: "Trois"},
	)
	first := newGuideServer(t, `<tv><channel id="une.a"><display-name>Une</display-name></channel>
		<channel id="deux.a"><display-name>Deux</display-name></channel>`+
		programme("une.a", now.Add(-time.Hour), now.Add(time.Hour), "First Une")+
		programme("deux.a", now.Add(-time.Hour), now.Add(time.Hour), "First Deux")+`</tv>`)
	second := newGuideServer(t, `<tv><channel id="une.b"><display-name>Une</display-name></channel>
		<channel id="deux.b"><display-name>Deux</display-name><icon src="https://icons.example/deux.png"/></channel>
		<channel id="trois.b"><display-name>Trois</display-name></channel>`+
		programme("une.b", now.Add(-time.Hour), now, "Second Une 1")+programme("une.b", now, now.Add(time.Hour), "Second Une 2")+
		programme("deux.b", now.Add(-time.Hour), now.Add(time.Hour), "Second Deux")+
		programme("trois.b", now.Add(-time.Hour), now.Add(time.Hour), "Second Trois")+`</tv>`)
	guides, err := e.service.SetCatalogGuides(t.Context(), addons.Shared(), key, []addons.GuideAddress{{URL: first.url}, {URL: second.url}})
	if err != nil {
		t.Fatal(err)
	}
	if len(guides.Guides) != 2 || guides.Channels != 3 || guides.Mapped != 3 || guides.Manual != 0 || guides.Guides[1].Channels != 3 {
		t.Fatalf("catalog guides: %+v", guides)
	}
	list := guides.Guides
	expect := func(when string, want ...string) {
		t.Helper()
		if got := e.mappingsOf(key, list); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", when, got, want)
		}
	}
	airing := func() []string {
		t.Helper()
		programs, err := e.service.Programs(t.Context(), e.member, now, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		titles := programTitles(programs)
		slices.Sort(titles)
		return titles
	}
	expect("automatic", "Une=1:une.a", "Deux=1:deux.a", "Trois=2:trois.b")
	if got := airing(); !slices.Equal(got, []string{"Deux: First Deux", "Trois: Second Trois", "Une: First Une"}) {
		t.Errorf("programmes: %q", got)
	}

	// By hand: Deux to the second guide, Trois to no guide.
	ids := map[string]accounts.ID{"Une": itemID(channelKey("tv:one")), "Deux": itemID(channelKey("tv:two")), "Trois": itemID(channelKey("tv:three"))}
	if _, err := e.service.SetMapping(t.Context(), addons.Shared(), key, ids["Deux"], &list[1].ID, new("deux.b")); err != nil {
		t.Fatal(err)
	}
	if m, err := e.service.SetMapping(t.Context(), addons.Shared(), key, ids["Trois"], nil, nil); err != nil || m.Mapping == nil || !m.Mapping.Manual || m.Mapping.Guide != nil {
		t.Fatalf("no guide: %+v %v", m, err)
	}
	expect("by hand", "Une=1:une.a", "Deux=2:deux.b*", "Trois=*")
	if got := airing(); !slices.Equal(got, []string{"Deux: Second Deux", "Une: First Une"}) {
		t.Errorf("programmes after mapping by hand: %q", got)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	expect("after a download", "Une=1:une.a", "Deux=2:deux.b*", "Trois=*")
	result, err := e.service.Automap(t.Context(), addons.Shared(), key, AutomapUnmapped)
	if err != nil || result != (AutomapResult{Channels: 3, Mapped: 2, Changed: 0}) {
		t.Errorf("unmapped automatic mapping: %+v %v", result, err)
	}
	expect("after mapping the unmapped", "Une=1:une.a", "Deux=2:deux.b*", "Trois=*")
	if total, manual, _ := e.service.Mappings(t.Context(), addons.Shared(), key, MappingManual, "", 0, 100); total != 2 || len(manual) != 2 {
		t.Errorf("manual mappings: %d", total)
	}
	if total, unmapped, _ := e.service.Mappings(t.Context(), addons.Shared(), key, MappingUnmapped, "", 0, 100); total != 1 || unmapped[0].Name != "Trois" {
		t.Errorf("unmapped channels: %d %+v", total, unmapped)
	}

	// Dropping a manual mapping maps the channel automatically at once.
	if m, err := e.service.ClearMapping(t.Context(), addons.Shared(), key, ids["Trois"]); err != nil || m.Mapping == nil || m.Mapping.Manual || *m.Mapping.GuideChannel != "trois.b" {
		t.Errorf("cleared: %+v %v", m, err)
	}
	result, err = e.service.Automap(t.Context(), addons.Shared(), key, AutomapRemap)
	if err != nil || result != (AutomapResult{Channels: 3, Mapped: 3, Changed: 1}) {
		t.Errorf("remap: %+v %v", result, err)
	}
	expect("remapped", "Une=1:une.a", "Deux=1:deux.a", "Trois=2:trois.b")

	// Guide channels are searched across guides, with what airs now.
	total, found, err := e.service.GuideChannels(t.Context(), addons.Shared(), key, "DEUX", nil, 0, 100)
	if err != nil || total != 2 || found[0].Guide != list[0].ID || found[1].Icon != "https://icons.example/deux.png" ||
		found[1].Now == nil || found[1].Now.Title != "Second Deux" {
		t.Errorf("guide channels: %d %+v %v", total, found, err)
	}
	if total, found, _ := e.service.GuideChannels(t.Context(), addons.Shared(), key, "", &list[1].ID, 1, 1); total != 3 || len(found) != 1 || found[0].ID != "trois.b" {
		t.Errorf("a page of the second guide: %d %+v", total, found)
	}

	// Mistakes.
	if _, err := e.service.SetMapping(t.Context(), addons.Shared(), key, ids["Une"], &list[0].ID, new("trois.b")); !errors.Is(err, ErrInvalidMapping) {
		t.Errorf("a guide channel of another guide: %v", err)
	}
	if _, err := e.service.SetMapping(t.Context(), addons.Shared(), key, itemID(channelKey("tv:nope")), nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("a channel the catalog does not list: %v", err)
	}
	if _, err := e.service.Automap(t.Context(), addons.Shared(), key, "everything"); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("an unknown mode: %v", err)
	}

	// Removing the first guide maps its channels to the second one.
	if guides, err = e.service.SetCatalogGuides(t.Context(), addons.Shared(), key, []addons.GuideAddress{{ID: &list[1].ID}}); err != nil {
		t.Fatal(err)
	}
	list = guides.Guides
	expect("without the first guide", "Une=1:une.b", "Deux=1:deux.b", "Trois=1:trois.b")
	if second.downloads.Load() != 2 {
		t.Errorf("a guide kept by its id was downloaded again: %d downloads", second.downloads.Load())
	}
	if _, err := e.service.SetCatalogGuides(t.Context(), addons.Shared(), key, slices.Repeat([]addons.GuideAddress{{URL: first.url}}, 11)); !errors.Is(err, addons.ErrTooManyGuides) {
		t.Errorf("eleven guides: %v", err)
	}
	if _, err := e.service.SetCatalogGuides(t.Context(), addons.Shared(), key, []addons.GuideAddress{{URL: "ftp://" + strings.Repeat("x", 3)}}); !errors.Is(err, addons.ErrInvalidGuideURL) {
		t.Errorf("an ftp guide: %v", err)
	}
}
