package library

import "testing"

// A catalog of a type Polyfin has no use of its own for, such as sport,
// is a library of videos as other's is; the types it has a use for keep
// it: movies, series, collections, live TV, Eclipse's music rows, and its
// own home videos and extras.
func TestAnyOtherTypeListsVideos(t *testing.T) {
	for _, tc := range []struct {
		stremioType, collection string
		kind                    Kind
		titles                  bool
	}{
		{"sport", "homevideos", KindVideo, true},
		{"events", "homevideos", KindVideo, true},
		{"other", "homevideos", KindVideo, true},
		{"channel", "homevideos", KindVideo, true},
		{"movie", "movies", KindMovie, true},
		{"series", "tvshows", KindSeries, true},
		{"anime", "", KindSeries, true},
		{"collection", "boxsets", "", false},
		{"tv", "", "", false},
		{"album", "music", "", false},
		{"track", "music", "", false},
		{VideoType, "homevideos", KindVideo, true},
		{ExtraType, "", "", false},
		{"", "", "", false},
	} {
		kind, titles := titleKind(tc.stremioType)
		if collection := collectionType(tc.stremioType); collection != tc.collection || kind != tc.kind || titles != tc.titles {
			t.Errorf("%q: library %q, titles %q %v; want %q, %q %v", tc.stremioType, collection, kind, titles, tc.collection, tc.kind, tc.titles)
		}
	}
	if AddonVideo(Item{Kind: KindVideo, StremioType: VideoType}) || !AddonVideo(Item{Kind: KindVideo, StremioType: "sport"}) {
		t.Error("home videos and sport videos told apart wrongly")
	}
}
