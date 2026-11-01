package library

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// A title shown as movies and shows numbers its videos as its description
// does; a video without a number comes after its season's numbered ones,
// in the description's order, in season 1 when it tells no season either.
// A video of season 0 with a number is a special. The description kept is
// left as it is.
func TestShownTitlesNumberTheirVideos(t *testing.T) {
	t.Parallel()
	meta := stremio.Meta{ID: "vault:1", Type: "library", Videos: []stremio.Video{
		{ID: "a", Season: 1, Episode: 2},
		{ID: "b"},
		{ID: "c", Season: 1, Episode: 1},
		{ID: "d", Season: 2},
		{ID: "e", Number: 4},
		{ID: "f", Season: 2, Episode: 3},
		{ID: "g"},
	}}
	kept := slices.Clone(meta.Videos)
	var got []string
	for _, video := range numberVideos(meta).Videos {
		got = append(got, fmt.Sprintf("%s %d.%d", video.ID, video.Season, video.EpisodeNumber()))
	}
	if want := []string{"a 1.2", "b 1.3", "c 1.1", "d 2.4", "e 0.4", "f 2.3", "g 1.4"}; !slices.Equal(got, want) {
		t.Errorf("numbered: %v, want %v", got, want)
	}
	if !slices.EqualFunc(meta.Videos, kept, func(a, b stremio.Video) bool { return a.ID == b.ID && a.Season == b.Season && a.Episode == b.Episode }) {
		t.Errorf("the description changed: %+v", meta.Videos)
	}

	// Seasons and episodes come from them, as for any series.
	shown := numberVideos(meta)
	if seasons := seasonNumbers(shown); !slices.Equal(seasons, []int{0, 1, 2}) {
		t.Errorf("seasons: %v", seasons)
	}
	series := Item{ID: itemID(titleKey(KindSeries, meta.ID)), Kind: KindSeries, Name: "Vault"}
	var places []string
	for _, episode := range episodes(series, shown, nil, nil, words{}, time.Now()) {
		places = append(places, fmt.Sprintf("%d.%d", episode.ParentIndexNumber, episode.IndexNumber))
	}
	if want := []string{"0.4", "1.1", "1.2", "1.3", "1.4", "2.3", "2.4"}; !slices.Equal(places, want) {
		t.Errorf("episodes: %v, want %v", places, want)
	}
}

// A title shown as movies and shows is a movie without videos, or with
// one its description numbers as no episode, a movie's file; with two
// videos or more, or one numbered as an episode, it is a show.
func TestShownTitlesAreMoviesOrShows(t *testing.T) {
	t.Parallel()
	r := record{Kind: KindSeries, Meta: &stremio.Meta{ID: "vault:1", Type: "library"}}
	for name, c := range map[string]struct {
		videos []stremio.Video
		want   Kind
	}{
		"no video":           {nil, KindMovie},
		"one file":           {[]stremio.Video{{ID: "a"}}, KindMovie},
		"one file of season": {[]stremio.Video{{ID: "a", Season: 2}}, KindMovie},
		"one episode":        {[]stremio.Video{{ID: "a", Season: 35, Episode: 1}}, KindSeries},
		"two files":          {[]stremio.Video{{ID: "a"}, {ID: "b"}}, KindSeries},
	} {
		meta := stremio.Meta{ID: "vault:1", Type: "library", Videos: c.videos}
		shown := shownMeta(r, meta)
		if got := shownKind(shown); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
