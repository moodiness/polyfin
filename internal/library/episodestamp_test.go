package library

import (
	"hash/maphash"
	"reflect"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A list's stamp holds every field of its videos a record keeps: each
// one changed changes it, a new field included, which this test then
// asks to be written in (see writeVideo). Streams, which records leave
// out, do not change it.
func TestStampsHoldEveryFieldOfAVideo(t *testing.T) {
	t.Parallel()
	sum := func(video stremio.Video) uint64 {
		var h maphash.Hash
		h.SetSeed(recordedSeed)
		writeVideo(&h, &video)
		return h.Sum64()
	}
	base := stremio.Video{ID: "tt1:1:1", Season: 1, Episode: 1}
	value := reflect.ValueOf(&base).Elem()
	for i := range value.NumField() {
		field := value.Type().Field(i)
		changed := base
		target := reflect.ValueOf(&changed).Elem().Field(i)
		switch {
		case field.Name == "Streams":
			changed.Streams = []stremio.Stream{{URL: "https://streams.example/1.mkv"}}
			if sum(changed) != sum(base) {
				t.Error("streams change the stamp")
			}
			continue
		case target.Kind() == reflect.String:
			target.SetString(target.String() + "changed")
		case target.Kind() == reflect.Int:
			target.SetInt(target.Int() + 7)
		case field.Type == reflect.TypeFor[*bool]():
			target.Set(reflect.ValueOf(new(false)))
		case field.Type == reflect.TypeFor[stremio.Names]():
			target.Set(reflect.ValueOf(stremio.Names{"Drama"}))
		default:
			t.Fatalf("Video.%s, of type %s: write it into the stamp (writeVideo), then tell this test how to change it", field.Name, field.Type)
		}
		if sum(changed) == sum(base) {
			t.Errorf("Video.%s does not change the stamp", field.Name)
		}
	}
	// Texts are told apart from their concatenations.
	if sum(stremio.Video{ID: "ab", Title: "c"}) == sum(stremio.Video{ID: "a", Title: "bc"}) {
		t.Error("two texts stamp as their concatenation")
	}
}

// The same list of a series stamps the same at each listing; one whose
// description renamed an episode does not.
func TestStampsTellAListChanged(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), listingAddon())
	e.listed(e.member, "Shows")
	show := itemID(titleKey(KindSeries, "tt100"))
	v, err := e.service.view(t.Context(), e.member)
	if err != nil {
		t.Fatal(err)
	}
	list := func() *outlinedEpisodes {
		t.Helper()
		l, err := e.service.listedEpisodes(t.Context(), v, show)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	first, again := list(), list()
	if first.stamp() != again.stamp() {
		t.Error("the same list stamped twice differently")
	}
	again.meta.Videos = slices.Clone(again.meta.Videos)
	again.meta.Videos[len(again.meta.Videos)-1].Title += " renamed"
	if first.stamp() == again.stamp() {
		t.Error("a list with an episode renamed stamped the same")
	}
}
