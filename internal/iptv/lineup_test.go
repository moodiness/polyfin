package iptv

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// entry writes a playlist entry.
func entry(group, name, path string, attributes ...string) string {
	extra := ""
	for _, attribute := range attributes {
		extra += " " + attribute
	}
	return fmt.Sprintf("#EXTINF:-1%s group-title=\"%s\",%s\nhttps://live.example/%s\n", extra, group, name, path)
}

func (e env) lineup(source accounts.ID) []Channel {
	e.t.Helper()
	_, channels, err := e.service.ListChannels(e.t.Context(), addons.Shared(), source, ChannelFilter{Limit: 500})
	if err != nil {
		e.t.Fatal(err)
	}
	return channels
}

func (e env) categories(source accounts.ID) []Category {
	e.t.Helper()
	categories, err := e.service.Categories(e.t.Context(), addons.Shared(), source, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return categories
}

func byName(channels []Channel, name string) Channel {
	for _, c := range channels {
		if c.Name == name || c.ProviderName == name {
			return c
		}
	}
	return Channel{}
}

func categoryNamed(categories []Category, name string) Category {
	for _, c := range categories {
		if c.Name == name {
			return c
		}
	}
	return Category{}
}

func streamLabels(c Channel) []string {
	var labels []string
	for _, s := range c.Streams {
		state := ""
		if !s.Enabled {
			state = " (off)"
		}
		labels = append(labels, s.Label+state)
	}
	return labels
}

// describe tells a line-up's shape: categories in order with their
// channels in order, as "Category: Channel #number".
func (e env) describe(source accounts.ID) []string {
	e.t.Helper()
	var result []string
	for _, c := range e.lineup(source) {
		number := "-"
		if c.Number != nil {
			number = fmt.Sprint(*c.Number)
		}
		state := ""
		if !c.Enabled {
			state = " (off)"
		}
		result = append(result, fmt.Sprintf("%s: %s #%s%s", c.CategoryName, c.Name, number, state))
	}
	return result
}

// must stops a test on an error, as a panic.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// Every kind of edit survives refreshes and option changes, while the
// provider's values update underneath and rows the provider dropped go
// away.
func TestEditsSurviveRefreshesAndOptionChanges(t *testing.T) {
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	list := newPlaylistServer(t, "#EXTM3U\n"+
		entry("News", "FR: Zeb One HD", "1.ts", `tvg-chno="5"`, `tvg-logo="https://logos.example/zeb.png"`)+
		entry("News", "FR: Zeb One SD", "2.ts", `tvg-chno="6"`)+
		entry("News", "Orbe Info", "3.ts", `tvg-chno="7"`)+
		entry("Kids", "Quill Toons", "4.ts", `tvg-chno="8"`)+
		entry("Kids", "Lumo Kids", "5.ts", `tvg-chno="9"`)+
		entry("Docs", "Tac Docs", "6.ts"))
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false))
	source := addon.ID
	categories := e.categories(source)
	news, kids, docs := categoryNamed(categories, "News"), categoryNamed(categories, "Kids"), categoryNamed(categories, "Docs")

	// Categories: renamed, reordered, one turned off, a custom one.
	must(e.service.UpdateCategory(ctx, shared, source, news.ID, CategoryChanges{Name: &Nullable[string]{Value: new("Headlines")}}))
	must(e.service.UpdateCategory(ctx, shared, source, docs.ID, CategoryChanges{Enabled: new(false)}))
	favourites := must(e.service.CreateCategory(ctx, shared, source, "Favourites"))
	if err := e.service.OrderCategories(ctx, shared, source, []accounts.ID{favourites.ID, kids.ID, news.ID, docs.ID}); err != nil {
		t.Fatal(err)
	}
	// Channels: renamed, a new logo and description, moved, numbered,
	// turned off, reordered.
	channels := e.lineup(source)
	zebHD, orbe, quill, lumo := byName(channels, "FR: Zeb One HD"), byName(channels, "Orbe Info"), byName(channels, "Quill Toons"),
		byName(channels, "Lumo Kids")
	must(e.service.UpdateChannel(ctx, shared, source, zebHD.ID, ChannelChanges{Name: &Nullable[string]{Value: new("Zeb")},
		Logo: &Nullable[string]{Value: new("https://mine.example/zeb.png")}, Description: new("The news channel")}))
	must(e.service.UpdateChannel(ctx, shared, source, orbe.ID, ChannelChanges{Category: &Nullable[accounts.ID]{Value: &favourites.ID},
		Number: &Nullable[int]{Value: new(1)}}))
	must(e.service.UpdateChannel(ctx, shared, source, quill.ID, ChannelChanges{Enabled: new(false)}))
	zebSD := byName(channels, "FR: Zeb One SD")
	if err := e.service.MoveChannel(ctx, shared, source, zebSD.ID, &zebHD.ID); err != nil {
		t.Fatal(err)
	}
	// Streams: a custom one, first; the provider's turned off.
	zeb := must(e.service.AddStream(ctx, shared, source, zebHD.ID, "https://backup.example/zeb.ts", "Backup", false))
	must(e.service.SetStreams(ctx, shared, source, zebHD.ID, []StreamSetting{{ID: zeb.Streams[1].ID, Enabled: true}, {ID: zeb.Streams[0].ID, Enabled: false}}))
	// A guide mapping, as the mapping routes keep it.
	if _, err := e.service.db.Exec(ctx, `INSERT INTO live_guides (id, addon_id, catalog_type, catalog_id, position, url)
		VALUES ('00000000-0000-0000-0000-000000000001', $1, 'tv', 'channels', 1, 'https://guide.example/a.xml')`, source); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, `INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual)
		VALUES ($1, 'tv', 'channels', $2, '00000000-0000-0000-0000-000000000001', 'zeb.fr', true)`, source, zebHD.ID); err != nil {
		t.Fatal(err)
	}

	want := []string{"Favourites: Orbe Info #1", "Kids: Quill Toons #8 (off)", "Kids: Lumo Kids #9", "Headlines: FR: Zeb One SD #6", "Headlines: Zeb #5",
		"Docs: Tac Docs #-"}
	if got := e.describe(source); !slices.Equal(got, want) {
		t.Fatalf("edited line-up:\n%q\nwant\n%q", got, want)
	}
	check := func(when string, want []string) {
		t.Helper()
		if got := e.describe(source); !slices.Equal(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", when, got, want)
		}
		got := e.categories(source)
		var names []string
		for _, c := range got {
			names = append(names, fmt.Sprintf("%s %v", c.Name, c.Enabled))
		}
		if !slices.Equal(names[:3], []string{"Favourites true", "Kids true", "Headlines true"}) || !slices.Contains(names, "Docs false") {
			t.Errorf("%s: categories %q", when, names)
		}
		zeb, err := e.service.Channel(ctx, shared, source, zebHD.ID)
		if err != nil || zeb.Name != "Zeb" || zeb.Logo != "https://mine.example/zeb.png" || zeb.Description != "The news channel" ||
			!slices.Equal(streamLabels(zeb)[:2], []string{"Backup", "HD (off)"}) || zeb.Streams[0].Address != "https://backup.example/…" ||
			zeb.Mapping == nil || !zeb.Mapping.Manual || *zeb.Mapping.GuideChannel != "zeb.fr" {
			t.Errorf("%s: edited channel %+v %v", when, zeb, err)
		}
	}

	// A refresh: Zeb's provider name and number change, Lumo goes, a new
	// channel comes.
	list.set("#EXTM3U\n"+
		entry("News", "FR: Zeb One HD", "1.ts", `tvg-chno="15"`, `tvg-logo="https://logos.example/zeb2.png"`)+
		entry("News", "FR: Zeb One SD", "2.ts", `tvg-chno="6"`)+
		entry("News", "Orbe Info", "3.ts", `tvg-chno="7"`)+
		entry("Kids", "Quill Toons", "4.ts", `tvg-chno="8"`)+
		entry("Kids", "Pif Kids", "7.ts", `tvg-chno="10"`)+
		entry("Docs", "Tac Docs", "6.ts"), 0)
	if err := e.service.Refresh(ctx, shared, source, false); err != nil {
		t.Fatal(err)
	}
	check("after a refresh", []string{"Favourites: Orbe Info #1", "Kids: Quill Toons #8 (off)", "Kids: Pif Kids #10", "Headlines: FR: Zeb One SD #6",
		"Headlines: Zeb #15", "Docs: Tac Docs #-"})
	if zeb, _ := e.service.Channel(ctx, shared, source, zebHD.ID); zeb.ProviderLogo != "https://logos.example/zeb2.png" || zeb.ProviderNumber == nil || *zeb.ProviderNumber != 15 {
		t.Errorf("provider values underneath: %+v", zeb)
	}
	if lumo, err := e.service.Channel(ctx, shared, source, lumo.ID); !errors.Is(err, addons.ErrNotFound) {
		t.Errorf("a dropped channel: %+v %v", lumo, err)
	}

	// Merged, then original again: the HD entry's channel keeps its edits,
	// its streams merged with the SD entry's; the SD entry's channel, gone
	// while merged, comes back in the provider's order.
	merged := ChannelsMerged
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Channels: &merged}}, false); err != nil {
		t.Fatal(err)
	}
	check("merged", []string{"Favourites: Orbe Info #1", "Kids: Quill Toons #8 (off)", "Kids: Pif Kids #10", "Headlines: Zeb #15", "Docs: Tac Docs #-"})
	if zeb, _ := e.service.Channel(ctx, shared, source, zebHD.ID); !slices.Equal(streamLabels(zeb), []string{"Backup", "HD (off)", "SD"}) {
		t.Errorf("merged streams: %q", streamLabels(zeb))
	}
	original := ChannelsOriginal
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Channels: &original}}, false); err != nil {
		t.Fatal(err)
	}
	check("original again", []string{"Favourites: Orbe Info #1", "Kids: Quill Toons #8 (off)", "Kids: Pif Kids #10", "Headlines: Zeb #15",
		"Headlines: FR: Zeb One SD #6", "Docs: Tac Docs #-"})

	// Excluding a group drops its rows and their edits; the custom category
	// stays, its channels back home when their category goes.
	excluded := []string{"g:News"}
	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{Excluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	if got := e.describe(source); !slices.Equal(got, []string{"Kids: Quill Toons #8 (off)", "Kids: Pif Kids #10", "Docs: Tac Docs #-"}) {
		t.Errorf("News excluded: %q", got)
	}
	if got := e.categories(source); len(got) != 3 || got[0].Name != "Favourites" || got[0].Position != 1 || got[2].Position != 3 {
		t.Errorf("categories once News is excluded: %+v", got)
	}
	var maps int
	_ = e.service.db.QueryRow(ctx, "SELECT count(*) FROM live_guide_maps WHERE addon_id = $1", source).Scan(&maps)
	if maps != 0 {
		t.Errorf("mappings of dropped channels: %d", maps)
	}
}

// Merged entries of the same name in a category become one channel with
// a stream per entry, best quality first, whatever the list's order; the
// same name in another category stays apart.
func TestMergingOrdersStreamsByQuality(t *testing.T) {
	e := newEnv(t)
	list := newPlaylistServer(t, "#EXTM3U\n"+
		entry("News", "FR: Zeb One SD", "sd.ts")+
		entry("News", "FR: Zeb One FHD", "fhd.ts", `tvg-id="zeb.fr"`)+
		entry("News", "Zeb One", "plain.ts")+
		entry("News", "FR | Zeb One 4K", "uhd.ts")+
		entry("News", "FR: Zeb One HD", "hd.ts")+
		entry("Other", "Zeb One HD", "other.ts"))
	merged := ChannelsMerged
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Channels: &merged}}, false))
	channels := e.lineup(addon.ID)
	if len(channels) != 2 || channels[0].Name != "Zeb One" || channels[0].GuideID != "zeb.fr" {
		t.Fatalf("merged line-up: %+v", channels)
	}
	if got := streamLabels(channels[0]); !slices.Equal(got, []string{"4K", "FHD", "HD", "SD", "Source 5"}) {
		t.Errorf("streams %q", got)
	}
	metas := must(e.service.Channels(t.Context(), addon.ID))
	streams := must(e.service.Streams(t.Context(), addon.ID, metas[0].ID))
	var addresses []string
	for _, s := range streams {
		addresses = append(addresses, strings.TrimPrefix(s.URL, "https://live.example/"))
	}
	if !slices.Equal(addresses, []string{"uhd.ts", "fhd.ts", "hd.ts", "sd.ts", "plain.ts"}) {
		t.Errorf("stream order %q", addresses)
	}
}

// Countries are told by the category's name first, then the entry's: a
// flag or a prefix of a country code; the rest is OTHER, last.
func TestCountriesAreDetected(t *testing.T) {
	e := newEnv(t)
	list := newPlaylistServer(t, "#EXTM3U\n"+
		entry("FR| News", "Zeb", "1.ts")+
		entry("[DE] Sport", "Orbe", "2.ts")+
		entry("\U0001F1EE\U0001F1F9 Calcio", "Quill", "3.ts")+
		entry("UK: Movies", "Lumo", "4.ts")+
		entry("Mixed", "ES: Tac", "5.ts")+
		entry("Mixed", "(PT) Pif", "6.ts")+
		entry("Mixed", "Plain", "7.ts")+
		entry("Mixed", "XX: Not a country", "8.ts"))
	byCountry := CategoriesCountry
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Categories: &byCountry}}, false))
	var keys []string
	for _, c := range e.categories(addon.ID) {
		keys = append(keys, fmt.Sprintf("%s=%d", c.Key, c.Channels))
	}
	if want := []string{"c:DE=1", "c:ES=1", "c:FR=1", "c:GB=1", "c:IT=1", "c:PT=1", "c:OTHER=2"}; !slices.Equal(keys, want) {
		t.Errorf("categories %q, want %q", keys, want)
	}
	total, preview := must2(e.service.Preview(t.Context(), addons.Shared(), addon.ID, PreviewCountries, ""))
	if total != 8 || len(preview) != 7 || preview[6].Key != "c:OTHER" || preview[0].Key != "c:DE" {
		t.Errorf("preview by country: %d %+v", total, preview)
	}
	_, preview = must2(e.service.Preview(t.Context(), addons.Shared(), addon.ID, PreviewGroups, "mix"))
	if len(preview) != 1 || preview[0].Key != "g:Mixed" || preview[0].Channels != 4 {
		t.Errorf("preview by group, searched: %+v", preview)
	}
}

func must2[A, B any](a A, b B, err error) (A, B) {
	if err != nil {
		panic(err)
	}
	return a, b
}

// Excluded group and country keys keep their entries out, in both
// category modes; new channels arrive turned off when the option says so,
// and sequential numbering leaves numbers to apps.
func TestExclusionsAndNewChannels(t *testing.T) {
	e := newEnv(t)
	body := "#EXTM3U\n" + entry("FR| News", "Zeb", "1.ts", `tvg-chno="4"`) + entry("DE| News", "Orbe", "2.ts") + entry("Kids", "ES: Quill", "3.ts") +
		entry("Kids", "Lumo", "4.ts")
	list := newPlaylistServer(t, body)
	excluded := []string{"c:DE", "g:Kids"}
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Excluded: &excluded}}, false))
	if got := e.describe(addon.ID); !slices.Equal(got, []string{"FR| News: Zeb #4"}) {
		t.Errorf("excluded by group and country: %q", got)
	}
	byCountry, off, sequential := CategoriesCountry, false, NumberingSequence
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &OptionsPatch{Categories: &byCountry, NewChannels: &off,
		Numbering: &sequential}}, false); err != nil {
		t.Fatal(err)
	}
	if got := e.describe(addon.ID); !slices.Equal(got, []string{"FR: Zeb #-"}) {
		t.Errorf("excluded by country: %q", got)
	}
	list.set(body+entry("FR| Movies", "Tac", "5.ts"), 0)
	if err := e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := e.describe(addon.ID); !slices.Equal(got, []string{"FR: Zeb #-", "FR: Tac #- (off)"}) {
		t.Errorf("a new channel with new channels off: %q", got)
	}
	for _, bad := range []OptionsPatch{{Categories: new("planet")}, {Channels: new("some")}, {Numbering: new("roman")}, {Excluded: &[]string{strings.Repeat("x", 301)}}} {
		if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &bad}, false); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

// A keyword bulk change counts what it would change in a dry run, then
// changes only the channels whose names match, optionally in a category.
func TestKeywordBulkChangesWithDryRun(t *testing.T) {
	e := newEnv(t)
	list := newPlaylistServer(t, "#EXTM3U\n"+entry("News", "Zeb Sport", "1.ts")+entry("News", "Orbe", "2.ts")+entry("Kids", "Zeb Kids", "3.ts")+
		entry("Kids", "Zèb Junior", "4.ts"))
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false))
	shared, source := addons.Shared(), addon.ID
	matched, changed, err := e.service.BulkChannels(t.Context(), shared, source, Bulk{Q: "zeb", Enabled: false, DryRun: true})
	if err != nil || matched != 3 || changed != 0 {
		t.Errorf("dry run: %d %d %v", matched, changed, err)
	}
	if got := e.describe(source); slices.ContainsFunc(got, func(s string) bool { return strings.HasSuffix(s, "(off)") }) {
		t.Errorf("a dry run changed: %q", got)
	}
	kids := categoryNamed(e.categories(source), "Kids")
	if matched, changed, err := e.service.BulkChannels(t.Context(), shared, source, Bulk{Q: "ZEB", InCategory: &kids.ID, Enabled: false}); err != nil || matched != 2 || changed != 2 {
		t.Errorf("in a category: %d %d %v", matched, changed, err)
	}
	if matched, changed, _ := e.service.BulkChannels(t.Context(), shared, source, Bulk{Q: "zeb", Enabled: false}); matched != 3 || changed != 1 {
		t.Errorf("again everywhere: %d %d", matched, changed)
	}
	want := []string{"News: Zeb Sport #- (off)", "News: Orbe #-", "Kids: Zeb Kids #- (off)", "Kids: Zèb Junior #- (off)"}
	if got := e.describe(source); !slices.Equal(got, want) {
		t.Errorf("after the bulk change: %q", got)
	}
	for _, bad := range []Bulk{{}, {Q: "z"}, {Q: "zeb", Category: &kids.ID}, {InCategory: &kids.ID}, {IDs: make([]accounts.ID, 5001)}} {
		if _, _, err := e.service.BulkChannels(t.Context(), shared, source, bad); !errors.Is(err, ErrInvalidBulk) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}
