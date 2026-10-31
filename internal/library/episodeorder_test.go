package library

import (
	"slices"
	"strconv"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// An addon numbering its episodes as TMDB does names TMDB's numbers in
// their identifiers: they match the group by them, whatever the addon's
// own season and number say. The group's season 0 holds its specials,
// then the addon's it leaves out.
func TestEpisodeGroupMatchesTMDBNumbers(t *testing.T) {
	t.Parallel()
	video := func(id string, season, episode int) stremio.Video {
		return stremio.Video{ID: id, Season: stremio.Number(season), Episode: stremio.Number(episode)}
	}
	meta := stremio.Meta{ID: "tmdb:77", Videos: []stremio.Video{
		// The addon calls TMDB's S01E02 its S01E05.
		video("tmdb:77:1:2", 1, 5), video("tmdb:77:1:1", 1, 1), video("tmdb:77:0:1", 0, 1), video("tmdb:77:0:2", 0, 2),
	}}
	group := &tmdb.EpisodeGroup{ID: "g", Groups: []tmdb.EpisodeSeason{
		{Name: "Extras", Order: 0, Episodes: []tmdb.GroupEpisode{{Season: 0, Episode: 2, Order: 0}}},
		{Name: "Part 1", Order: 1, Episodes: []tmdb.GroupEpisode{{Season: 1, Episode: 2, Order: 0}, {Season: 1, Episode: 1, Order: 1},
			{Show: 99, Season: 0, Episode: 1, Order: 2}}},
	}}
	l := newLayout(meta, group, 77, SpecialsLast, vocabularyOf("en"))
	var got []string
	for _, season := range l.seasons {
		for _, id := range season.videos {
			p := l.places[id]
			got = append(got, season.name+" "+strconv.Itoa(p.Season)+"x"+strconv.Itoa(p.Episode)+" "+id)
		}
	}
	// Another series' episode in the group is not this one's special.
	want := []string{"Part 1 1x1 tmdb:77:1:2", "Part 1 1x2 tmdb:77:1:1", "Extras 0x1 tmdb:77:0:2", "Extras 0x2 tmdb:77:0:1"}
	if !slices.Equal(got, want) {
		t.Errorf("layout:\n%q\nwant\n%q", got, want)
	}
}
