package iptv

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// A search lists the titles whose name starts with the term first, then
// those that hold it, with the option off or on; on, a name learned is
// listed, counted and found by either name, its own or the provider's,
// and the prefix of the name shown counts. Off, the provider's name only.
func TestSearchesListPrefixMatchesFirstWithLearnedNames(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	list := newPlaylistServer(t, "#EXTM3U\n"+
		"#EXTINF:-1 group-title=\"Movies\",The Zeb Tale\nhttps://tv.example/movie/user/pass/1.mkv\n"+
		"#EXTINF:-1 group-title=\"Movies\",Zeb Quest\nhttps://tv.example/movie/user/pass/2.mkv\n"+
		"#EXTINF:-1 group-title=\"Movies\",BRAND | Orb\nhttps://tv.example/movie/user/pass/3.mkv\n")
	on, off := true, false
	source := must(e.service.Add(ctx, shared, NewSource{Name: "List", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Movies: &on, LiveTv: &off}}, false)).ID
	search := func(term string) ([]string, int) {
		t.Helper()
		metas, total := must2(e.service.CatalogWindow(ctx, source, "movie", "movies", "", term, 0, 10))
		var names []string
		for _, meta := range metas {
			names = append(names, meta.Name)
		}
		return names, total
	}
	check := func(when, term string, want []string) {
		t.Helper()
		if names, total := search(term); !slices.Equal(names, want) || total != len(want) {
			t.Errorf("%s, %q: %q of %d, want %q", when, term, names, total, want)
		}
	}
	check("no name learned", "zeb", []string{"Zeb Quest", "The Zeb Tale"})

	orb := must(e.service.Catalog(ctx, source, "movie", "movies", 0, "", "orb"))
	if len(orb) != 1 {
		t.Fatalf("the movie to name: %+v", orb)
	}
	// Learned while the option is off: nothing kept.
	if err := e.service.LearnName(ctx, source, orb[0].ID, "Zebra Days"); err != nil {
		t.Fatal(err)
	}
	check("learned while off", "zebra", nil)

	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{MetadataNames: &on}}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.service.LearnName(ctx, source, orb[0].ID, "Zebra Days"); err != nil {
		t.Fatal(err)
	}
	check("a name learned", "zeb", []string{"Zeb Quest", "Zebra Days", "The Zeb Tale"})
	check("by the provider's name", "brand", []string{"Zebra Days"})
	if meta := must(e.service.Meta(ctx, source, orb[0].ID)); meta.Name != "Zebra Days" {
		t.Errorf("described as %q", meta.Name)
	}

	if err := e.service.Update(ctx, shared, source, Changes{Options: &OptionsPatch{MetadataNames: &off}}, false); err != nil {
		t.Fatal(err)
	}
	check("off", "zebra", nil)
	check("off, by the provider's name", "brand", []string{"BRAND | Orb"})
}
