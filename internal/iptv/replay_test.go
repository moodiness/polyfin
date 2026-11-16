package iptv

import (
	"maps"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// A shown channel's archive is that of its first enabled stream, in the
// administrator's order then the quality's, whose entry has a kind of
// catch-up and days: earlier streams without one, disabled streams,
// hidden channels, channels in a hidden category or without an enabled
// stream, and channels without an archive have none.
func TestArchivesAreTheFirstStreamsWithOne(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	shift := func(days string) string { return `catchup="shift" catchup-days="` + days + `"` }
	list := newPlaylistServer(t, "#EXTM3U\n"+
		entry("News", "Zeb One 4K", "zeb-uhd.ts")+
		entry("News", "Zeb One FHD", "zeb-fhd.ts", shift("6"))+
		entry("News", "Zeb One HD", "zeb-hd.ts", shift("3"))+
		entry("News", "Zeb One SD", "zeb-sd.ts", shift("5"))+
		entry("News", "Pax HD", "pax-hd.ts", shift("6"))+
		entry("News", "Pax SD", "pax-sd.ts", shift("7"))+
		entry("News", "Kit HD", "kit-hd.ts", shift("2"))+
		entry("News", "Kit SD", "kit-sd.ts", shift("8"))+
		entry("News", "Orbe", "orbe.ts", shift("2"))+
		entry("News", "Lone", "lone.ts", shift("4"))+
		entry("News", "Quill", "quill.ts")+
		entry("Hidden", "Vex", "vex.ts", shift("1")))
	merged := ChannelsMerged
	addon := must(e.service.Add(ctx, addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Channels: &merged}}, false))
	for _, statement := range []string{
		// Zeb One's 4K entry has days without a kind, its FHD one a kind
		// without days: its HD stream is the first with an archive.
		`UPDATE iptv_entries SET catchup = '', catchup_days = 9 WHERE addon_id = $1 AND url LIKE '%/zeb-uhd.ts'`,
		`UPDATE iptv_entries SET catchup_days = 0 WHERE addon_id = $1 AND url LIKE '%/zeb-fhd.ts'`,
		// Pax's HD stream is disabled, Lone's only one too.
		`UPDATE iptv_streams s SET enabled = false FROM iptv_entries e WHERE s.addon_id = $1 AND e.addon_id = s.addon_id AND e.key = s.key
			AND (e.url LIKE '%/pax-hd.ts' OR e.url LIKE '%/lone.ts')`,
		// Kit's SD stream is put first.
		`UPDATE iptv_streams s SET sort = 1 FROM iptv_entries e WHERE s.addon_id = $1 AND e.addon_id = s.addon_id AND e.key = s.key
			AND e.url LIKE '%/kit-sd.ts'`,
		`UPDATE iptv_lineup SET enabled = false WHERE addon_id = $1 AND provider_name = 'Orbe'`,
		`UPDATE iptv_categories SET enabled = false WHERE addon_id = $1 AND provider_name = 'Hidden'`,
	} {
		if tag, err := e.service.db.Exec(ctx, statement, addon.ID); err != nil || tag.RowsAffected() == 0 {
			t.Fatalf("%s: %v %v", statement, tag, err)
		}
	}
	e.service.forget(addon.ID)
	ids := map[string]string{}
	for _, channel := range must(e.service.Channels(ctx, addon.ID)) {
		ids[channel.Name] = channel.ID
	}
	want := map[string]int{ids["Zeb One"]: 3, ids["Pax"]: 7, ids["Kit"]: 8}
	if len(ids) != 4 || len(want) != 3 {
		t.Fatalf("shown channels: %v", ids)
	}
	if got := must(e.service.Archives(ctx, addon.ID)); !maps.Equal(got, want) {
		t.Errorf("archives %v, want %v", got, want)
	}
}
